import asyncio
import json
import logging
import os
import datetime as dt
from pydantic import BaseModel
from sqlalchemy import text
import aiohttp
import requests

from fastapi import (
    Depends,
    HTTPException,
    Request,
    status,
)

from fastapi.responses import FileResponse

from starlette.responses import Response

from open_webui.socket.main import (
    get_event_emitter,
    get_models_in_use,
    get_active_user_ids,
)
from open_webui.internal.db import Session
from open_webui.models.models import Models
from open_webui.models.users import UserModel, Users
from open_webui.models.chats import Chats

from open_webui.config import (
    # Retrieval (Web Search)
    GOOGLE_DRIVE_CLIENT_ID,
    GOOGLE_DRIVE_API_KEY,
    ONEDRIVE_CLIENT_ID_PERSONAL,
    ONEDRIVE_CLIENT_ID_BUSINESS,
    ONEDRIVE_SHAREPOINT_URL,
    ONEDRIVE_SHAREPOINT_TENANT_ID,
    ENABLE_ONEDRIVE_PERSONAL,
    ENABLE_ONEDRIVE_BUSINESS,
    # WebUI
    WEBUI_AUTH,
    # Misc
    CACHE_DIR,
    DEFAULT_LOCALE,
    OAUTH_PROVIDERS,
    # Admin
    ENABLE_ADMIN_CHAT_ACCESS,
    BYPASS_ADMIN_ACCESS_CONTROL,
    ENABLE_ADMIN_EXPORT,
)
from open_webui.env import (
    CHANGELOG,
    VERSION,
    ENABLE_SIGNUP_PASSWORD_CONFIRMATION,
    # SCIM
    ENABLE_WEBSOCKET_SUPPORT,
    BYPASS_MODEL_ACCESS_CONTROL,
    ENABLE_VERSION_UPDATE_CHECK,
    AIOHTTP_CLIENT_SESSION_SSL, SRC_LOG_LEVELS,
)

from open_webui.utils.models import (
    get_all_models,
    get_all_base_models,
    check_model_access,
    get_filtered_models,
)
from open_webui.utils.chat import (
    generate_chat_completion as chat_completion_handler,
    chat_completed as chat_completed_handler,
    chat_action as chat_action_handler,
)
from open_webui.utils.embeddings import generate_embeddings
from open_webui.utils.middleware import process_chat_payload, process_chat_response

from open_webui.utils.auth import (
    get_http_authorization_cred,
    decode_token,
    get_admin_user,
    get_verified_user,
)
from open_webui.utils.oauth import (
    get_oauth_client_info_with_dynamic_client_registration,
    encrypt_data, OAuthClientManager, OAuthManager,
)

from open_webui.tasks import (
    list_task_ids_by_item_id,
    create_task,
    stop_task,
    list_tasks,
)  # Import from tasks.py

from open_webui.constants import ERROR_MESSAGES

log = logging.getLogger(__name__)
log.setLevel(SRC_LOG_LEVELS["MAIN"])

BASE_PATH = "/kael"


def setup_lazy_routes(app):
    oauth_client_manager = OAuthClientManager(app)
    oauth_manager = OAuthManager(app)

    ##################################
    #
    # Chat Endpoints
    #
    ##################################

    @app.get(f"{BASE_PATH}/api/models")
    @app.get(f"{BASE_PATH}/api/v1/models")  # Experimental: Compatibility with OpenAI API
    async def get_models(
            request: Request, refresh: bool = False, user=Depends(get_verified_user)
    ):
        all_models = await get_all_models(request, refresh=refresh, user=user)

        models = []
        for model in all_models:
            # Filter out filter pipelines
            if "pipeline" in model and model["pipeline"].get("type", None) == "filter":
                continue

            try:
                model_tags = [
                    tag.get("name")
                    for tag in model.get("info", {}).get("meta", {}).get("tags", [])
                ]
                tags = [tag.get("name") for tag in model.get("tags", [])]

                tags = list(set(model_tags + tags))
                model["tags"] = [{"name": tag} for tag in tags]
            except Exception as e:
                log.debug(f"Error processing model tags: {e}")
                model["tags"] = []
                pass

            models.append(model)

        model_order_list = request.app.state.config.MODEL_ORDER_LIST
        if model_order_list:
            model_order_dict = {model_id: i for i, model_id in enumerate(model_order_list)}
            # Sort models by order list priority, with fallback for those not in the list
            models.sort(
                key=lambda model: (
                    model_order_dict.get(model.get("id", ""), float("inf")),
                    (model.get("name", "") or ""),
                )
            )

        models = get_filtered_models(models, user)

        log.debug(
            f"/api/models returned filtered models accessible to the user: {json.dumps([model.get('id') for model in models])}"
        )
        return {"data": models}

    @app.get(f"{BASE_PATH}/api/models/base")
    async def get_base_models(request: Request, user=Depends(get_admin_user)):
        models = await get_all_base_models(request, user=user)
        return {"data": models}

    ##################################
    # Embeddings
    ##################################

    @app.post(f"{BASE_PATH}/api/embeddings")
    @app.post(f"{BASE_PATH}/api/v1/embeddings")  # Experimental: Compatibility with OpenAI API
    async def embeddings(
            request: Request, form_data: dict, user=Depends(get_verified_user)
    ):
        """
        OpenAI-compatible embeddings endpoint.

        This handler:
          - Performs user/model checks and dispatches to the correct backend.
          - Supports OpenAI, Ollama, arena models, pipelines, and any compatible provider.

        Args:
            request (Request): Request context.
            form_data (dict): OpenAI-like payload (e.g., {"model": "...", "input": [...]})
            user (UserModel): Authenticated user.

        Returns:
            dict: OpenAI-compatible embeddings response.
        """
        # Make sure models are loaded in app state
        if not request.app.state.MODELS:
            await get_all_models(request, user=user)
        # Use generic dispatcher in utils.embeddings
        return await generate_embeddings(request, form_data, user)

    @app.post(f"{BASE_PATH}/api/chat/completions")
    @app.post(f"{BASE_PATH}/api/v1/chat/completions")  # Experimental: Compatibility with OpenAI API
    async def chat_completion(
            request: Request,
            form_data: dict,
            user=Depends(get_verified_user),
    ):
        if not request.app.state.MODELS:
            await get_all_models(request, user=user)

        model_id = form_data.get("model", None)
        model_item = form_data.pop("model_item", {})
        tasks = form_data.pop("background_tasks", None)

        metadata = {}
        try:
            if not model_item.get("direct", False):
                if model_id not in request.app.state.MODELS:
                    raise Exception("Model not found")

                model = request.app.state.MODELS[model_id]
                model_info = Models.get_model_by_id(model_id)

                # Check if user has access to the model
                if not BYPASS_MODEL_ACCESS_CONTROL and (
                        user.role != "admin" or not BYPASS_ADMIN_ACCESS_CONTROL
                ):
                    try:
                        check_model_access(user, model)
                    except Exception as e:
                        raise e
            else:
                model = model_item
                model_info = None

                request.state.direct = True
                request.state.model = model

            model_info_params = (
                model_info.params.model_dump() if model_info and model_info.params else {}
            )

            # Chat Params
            stream_delta_chunk_size = form_data.get("params", {}).get(
                "stream_delta_chunk_size"
            )
            reasoning_tags = form_data.get("params", {}).get("reasoning_tags")

            # Model Params
            if model_info_params.get("stream_delta_chunk_size"):
                stream_delta_chunk_size = model_info_params.get("stream_delta_chunk_size")

            if model_info_params.get("reasoning_tags") is not None:
                reasoning_tags = model_info_params.get("reasoning_tags")

            metadata = {
                "user_id": user.id,
                "chat_id": form_data.pop("chat_id", None),
                "message_id": form_data.pop("id", None),
                "session_id": form_data.pop("session_id", None),
                "filter_ids": form_data.pop("filter_ids", []),
                "tool_ids": form_data.get("tool_ids", None),
                "tool_servers": form_data.pop("tool_servers", None),
                "files": form_data.get("files", None),
                "features": form_data.get("features", {}),
                "variables": form_data.get("variables", {}),
                "model": model,
                "direct": model_item.get("direct", False),
                "params": {
                    "stream_delta_chunk_size": stream_delta_chunk_size,
                    "reasoning_tags": reasoning_tags,
                    "function_calling": (
                        "native"
                        if (
                                form_data.get("params", {}).get("function_calling") == "native"
                                or model_info_params.get("function_calling") == "native"
                        )
                        else "default"
                    ),
                },
            }

            if metadata.get("chat_id") and (user and user.role != "admin"):
                if not metadata["chat_id"].startswith("local:"):
                    chat = Chats.get_chat_by_id_and_user_id(metadata["chat_id"], user.id)
                    if chat is None:
                        raise HTTPException(
                            status_code=status.HTTP_404_NOT_FOUND,
                            detail=ERROR_MESSAGES.DEFAULT(),
                        )

            request.state.metadata = metadata
            form_data["metadata"] = metadata

        except Exception as e:
            log.debug(f"Error processing chat metadata: {e}")
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=str(e),
            )

        async def process_chat(request, form_data, user, metadata, model):
            try:
                form_data, metadata, events = await process_chat_payload(
                    request, form_data, user, metadata, model
                )

                response = await chat_completion_handler(request, form_data, user)
                if metadata.get("chat_id") and metadata.get("message_id"):
                    try:
                        if not metadata["chat_id"].startswith("local:"):
                            Chats.upsert_message_to_chat_by_id_and_message_id(
                                metadata["chat_id"],
                                metadata["message_id"],
                                {
                                    "model": model_id,
                                },
                            )
                    except:
                        pass

                return await process_chat_response(
                    request, response, form_data, user, metadata, model, events, tasks
                )
            except asyncio.CancelledError:
                log.info("Chat processing was cancelled")
                try:
                    event_emitter = get_event_emitter(metadata)
                    await asyncio.shield(
                        event_emitter(
                            {"type": "chat:tasks:cancel"},
                        )
                    )
                except Exception as e:
                    pass
                finally:
                    raise  # re-raise to ensure proper task cancellation handling
            except Exception as e:
                log.debug(f"Error processing chat payload: {e}")
                if metadata.get("chat_id") and metadata.get("message_id"):
                    # Update the chat message with the error
                    try:
                        if not metadata["chat_id"].startswith("local:"):
                            Chats.upsert_message_to_chat_by_id_and_message_id(
                                metadata["chat_id"],
                                metadata["message_id"],
                                {
                                    "error": {"content": str(e)},
                                },
                            )

                        event_emitter = get_event_emitter(metadata)
                        await event_emitter(
                            {
                                "type": "chat:message:error",
                                "data": {"error": {"content": str(e)}},
                            }
                        )
                        await event_emitter(
                            {"type": "chat:tasks:cancel"},
                        )

                    except:
                        pass
            finally:
                try:
                    if mcp_clients := metadata.get("mcp_clients"):
                        for client in reversed(mcp_clients.values()):
                            await client.disconnect()
                except Exception as e:
                    log.debug(f"Error cleaning up: {e}")
                    pass

        if (
                metadata.get("session_id")
                and metadata.get("chat_id")
                and metadata.get("message_id")
        ):
            # Asynchronous Chat Processing
            task_id, _ = await create_task(
                request.app.state.redis,
                process_chat(request, form_data, user, metadata, model),
                id=metadata["chat_id"],
            )
            return {"status": True, "task_id": task_id}
        else:
            return await process_chat(request, form_data, user, metadata, model)

    # Alias for chat_completion (Legacy)
    generate_chat_completions = chat_completion
    generate_chat_completion = chat_completion

    @app.post(f"{BASE_PATH}/api/chat/completed")
    async def chat_completed(
            request: Request, form_data: dict, user=Depends(get_verified_user)
    ):
        try:
            model_item = form_data.pop("model_item", {})

            if model_item.get("direct", False):
                request.state.direct = True
                request.state.model = model_item

            return await chat_completed_handler(request, form_data, user)
        except Exception as e:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=str(e),
            )

    @app.post(BASE_PATH + "/api/chat/actions/{action_id}")
    async def chat_action(
            request: Request, action_id: str, form_data: dict, user=Depends(get_verified_user)
    ):
        try:
            model_item = form_data.pop("model_item", {})

            if model_item.get("direct", False):
                request.state.direct = True
                request.state.model = model_item

            return await chat_action_handler(request, action_id, form_data, user)
        except Exception as e:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=str(e),
            )

    @app.post(BASE_PATH + "/api/tasks/stop/{task_id}")
    async def stop_task_endpoint(
            request: Request, task_id: str, user=Depends(get_verified_user)
    ):
        try:
            result = await stop_task(request.app.state.redis, task_id)
            return result
        except ValueError as e:
            raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(e))

    @app.get(f"{BASE_PATH}/api/tasks")
    async def list_tasks_endpoint(request: Request, user=Depends(get_verified_user)):
        return {"tasks": await list_tasks(request.app.state.redis)}

    @app.get(BASE_PATH + "/api/tasks/chat/{chat_id}")
    async def list_tasks_by_chat_id_endpoint(
            request: Request, chat_id: str, user=Depends(get_verified_user)
    ):
        chat = Chats.get_chat_by_id(chat_id)
        if chat is None or chat.user_id != user.id:
            return {"task_ids": []}

        task_ids = await list_task_ids_by_item_id(request.app.state.redis, chat_id)

        log.debug(f"Task IDs for chat {chat_id}: {task_ids}")
        return {"task_ids": task_ids}

    ##################################
    #
    # Config Endpoints
    #
    ##################################

    @app.get(f"{BASE_PATH}/api/config")
    async def get_app_config(request: Request, user=Depends(get_verified_user)):
        user_count = 1
        onboarding = False

        if user is None:
            onboarding = user_count == 0

        return {
            **({"onboarding": True} if onboarding else {}),
            "status": True,
            "name": app.state.WEBUI_NAME,
            "version": VERSION,
            "default_locale": str(DEFAULT_LOCALE),
            "oauth": {
                "providers": {
                    name: config.get("name", name)
                    for name, config in OAUTH_PROVIDERS.items()
                }
            },
            "features": {
                "auth": WEBUI_AUTH,
                "auth_trusted_header": bool(app.state.AUTH_TRUSTED_EMAIL_HEADER),
                "enable_signup_password_confirmation": ENABLE_SIGNUP_PASSWORD_CONFIRMATION,
                "enable_ldap": app.state.config.ENABLE_LDAP,
                "enable_api_key": app.state.config.ENABLE_API_KEY,
                "enable_signup": app.state.config.ENABLE_SIGNUP,
                "enable_login_form": app.state.config.ENABLE_LOGIN_FORM,
                "enable_websocket": ENABLE_WEBSOCKET_SUPPORT,
                "enable_version_update_check": ENABLE_VERSION_UPDATE_CHECK,
                **(
                    {
                        "enable_direct_connections": app.state.config.ENABLE_DIRECT_CONNECTIONS,
                        "enable_channels": app.state.config.ENABLE_CHANNELS,
                        "enable_notes": app.state.config.ENABLE_NOTES,
                        "enable_web_search": app.state.config.ENABLE_WEB_SEARCH,
                        "enable_code_execution": app.state.config.ENABLE_CODE_EXECUTION,
                        "enable_code_interpreter": app.state.config.ENABLE_CODE_INTERPRETER,
                        "enable_image_generation": app.state.config.ENABLE_IMAGE_GENERATION,
                        "enable_autocomplete_generation": app.state.config.ENABLE_AUTOCOMPLETE_GENERATION,
                        "enable_community_sharing": app.state.config.ENABLE_COMMUNITY_SHARING,
                        "enable_message_rating": app.state.config.ENABLE_MESSAGE_RATING,
                        "enable_user_webhooks": app.state.config.ENABLE_USER_WEBHOOKS,
                        "enable_admin_export": ENABLE_ADMIN_EXPORT,
                        "enable_admin_chat_access": ENABLE_ADMIN_CHAT_ACCESS,
                        "enable_google_drive_integration": app.state.config.ENABLE_GOOGLE_DRIVE_INTEGRATION,
                        "enable_onedrive_integration": app.state.config.ENABLE_ONEDRIVE_INTEGRATION,
                        **(
                            {
                                "enable_onedrive_personal": ENABLE_ONEDRIVE_PERSONAL,
                                "enable_onedrive_business": ENABLE_ONEDRIVE_BUSINESS,
                            }
                            if app.state.config.ENABLE_ONEDRIVE_INTEGRATION
                            else {}
                        ),
                    }
                    if user is not None
                    else {}
                ),
            },
            **(
                {
                    "default_models": app.state.config.DEFAULT_MODELS,
                    "default_prompt_suggestions": app.state.config.DEFAULT_PROMPT_SUGGESTIONS,
                    "user_count": user_count,
                    "code": {
                        "engine": app.state.config.CODE_EXECUTION_ENGINE,
                    },
                    "audio": {
                        "tts": {
                            "engine": app.state.config.TTS_ENGINE,
                            "voice": app.state.config.TTS_VOICE,
                            "split_on": app.state.config.TTS_SPLIT_ON,
                        },
                        "stt": {
                            "engine": app.state.config.STT_ENGINE,
                        },
                    },
                    "file": {
                        "max_size": app.state.config.FILE_MAX_SIZE,
                        "max_count": app.state.config.FILE_MAX_COUNT,
                        "image_compression": {
                            "width": app.state.config.FILE_IMAGE_COMPRESSION_WIDTH,
                            "height": app.state.config.FILE_IMAGE_COMPRESSION_HEIGHT,
                        },
                    },
                    "permissions": {**app.state.config.USER_PERMISSIONS},
                    "google_drive": {
                        "client_id": GOOGLE_DRIVE_CLIENT_ID.value,
                        "api_key": GOOGLE_DRIVE_API_KEY.value,
                    },
                    "onedrive": {
                        "client_id_personal": ONEDRIVE_CLIENT_ID_PERSONAL,
                        "client_id_business": ONEDRIVE_CLIENT_ID_BUSINESS,
                        "sharepoint_url": ONEDRIVE_SHAREPOINT_URL.value,
                        "sharepoint_tenant_id": ONEDRIVE_SHAREPOINT_TENANT_ID.value,
                    },
                    "ui": {
                        "pending_user_overlay_title": app.state.config.PENDING_USER_OVERLAY_TITLE,
                        "pending_user_overlay_content": app.state.config.PENDING_USER_OVERLAY_CONTENT,
                        "response_watermark": app.state.config.RESPONSE_WATERMARK,
                    },
                    "license_metadata": app.state.LICENSE_METADATA,
                    "active_entries": app.state.USER_COUNT,
                }
                if user is not None
                else {
                    **(
                        {
                            "ui": {
                                "pending_user_overlay_title": app.state.config.PENDING_USER_OVERLAY_TITLE,
                                "pending_user_overlay_content": app.state.config.PENDING_USER_OVERLAY_CONTENT,
                            }
                        }
                        if user and user.role == "pending"
                        else {}
                    ),
                    **(
                        {
                            "metadata": {
                                "login_footer": app.state.LICENSE_METADATA.get(
                                    "login_footer", ""
                                ),
                                "auth_logo_position": app.state.LICENSE_METADATA.get(
                                    "auth_logo_position", ""
                                ),
                            }
                        }
                        if app.state.LICENSE_METADATA
                        else {}
                    ),
                }
            ),
        }

    class UrlForm(BaseModel):
        url: str

    @app.get(f"{BASE_PATH}/api/webhook")
    async def get_webhook_url(user=Depends(get_admin_user)):
        return {
            "url": app.state.config.WEBHOOK_URL,
        }

    @app.post(f"{BASE_PATH}/api/webhook")
    async def update_webhook_url(form_data: UrlForm, user=Depends(get_admin_user)):
        app.state.config.WEBHOOK_URL = form_data.url
        app.state.WEBHOOK_URL = app.state.config.WEBHOOK_URL
        return {"url": app.state.config.WEBHOOK_URL}

    @app.get(f"{BASE_PATH}/api/version")
    async def get_app_version():
        return {
            "version": VERSION,
        }

    @app.get(f"{BASE_PATH}/api/version/updates")
    async def get_app_latest_release_version(user=Depends(get_verified_user)):
        if not ENABLE_VERSION_UPDATE_CHECK:
            log.debug(
                f"Version update check is disabled, returning current version as latest version"
            )
            return {"current": VERSION, "latest": VERSION}
        try:
            timeout = aiohttp.ClientTimeout(total=1)
            async with aiohttp.ClientSession(timeout=timeout, trust_env=True) as session:
                async with session.get(
                        "https://api.github.com/repos/open-webui/open-webui/releases/latest",
                        ssl=AIOHTTP_CLIENT_SESSION_SSL,
                ) as response:
                    response.raise_for_status()
                    data = await response.json()
                    latest_version = data["tag_name"]

                    return {"current": VERSION, "latest": latest_version[1:]}
        except Exception as e:
            log.debug(e)
            return {"current": VERSION, "latest": VERSION}

    @app.get(f"{BASE_PATH}/api/changelog")
    async def get_app_changelog():
        return {key: CHANGELOG[key] for idx, key in enumerate(CHANGELOG) if idx < 5}

    @app.get(f"{BASE_PATH}/api/usage")
    async def get_current_usage(user=Depends(get_verified_user)):
        """
        Get current usage statistics for Open WebUI.
        This is an experimental endpoint and subject to change.
        """
        try:
            return {"model_ids": get_models_in_use(), "user_ids": get_active_user_ids()}
        except Exception as e:
            log.error(f"Error getting usage statistics: {e}")
            raise HTTPException(status_code=500, detail="Internal Server Error")

    async def register_client(self, request, client_id: str) -> bool:
        server_type, server_id = client_id.split(":", 1)

        connection = None
        connection_idx = None

        for idx, conn in enumerate(request.app.state.config.TOOL_SERVER_CONNECTIONS or []):
            if conn.get("type", "openapi") == server_type:
                info = conn.get("info", {})
                if info.get("id") == server_id:
                    connection = conn
                    connection_idx = idx
                    break

        if connection is None or connection_idx is None:
            log.warning(
                f"Unable to locate MCP tool server configuration for client {client_id} during re-registration"
            )
            return False

        server_url = connection.get("url")
        oauth_server_key = (connection.get("config") or {}).get("oauth_server_key")

        try:
            oauth_client_info = (
                await get_oauth_client_info_with_dynamic_client_registration(
                    request,
                    client_id,
                    server_url,
                    oauth_server_key,
                )
            )
        except Exception as e:
            log.error(f"Dynamic client re-registration failed for {client_id}: {e}")
            return False

        try:
            request.app.state.config.TOOL_SERVER_CONNECTIONS[connection_idx] = {
                **connection,
                "info": {
                    **connection.get("info", {}),
                    "oauth_client_info": encrypt_data(
                        oauth_client_info.model_dump(mode="json")
                    ),
                },
            }
        except Exception as e:
            log.error(
                f"Failed to persist updated OAuth client info for tool server {client_id}: {e}"
            )
            return False

        oauth_client_manager.remove_client(client_id)
        oauth_client_manager.add_client(client_id, oauth_client_info)
        log.info(f"Re-registered OAuth client {client_id} for tool server")
        return True

    @app.get(BASE_PATH + "/oauth/clients/{client_id}/authorize")
    async def oauth_client_authorize(
            client_id: str,
            request: Request,
            response: Response,
            user=Depends(get_verified_user),
    ):
        # ensure_valid_client_registration
        client = oauth_client_manager.get_client(client_id)
        client_info = oauth_client_manager.get_client_info(client_id)
        if client is None or client_info is None:
            raise HTTPException(status.HTTP_404_NOT_FOUND)

        if not await oauth_client_manager._preflight_authorization_url(client, client_info):
            log.info(
                "Detected invalid OAuth client %s; attempting re-registration",
                client_id,
            )

            registered = await register_client(request, client_id)
            if not registered:
                raise HTTPException(
                    status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                    detail="Failed to re-register OAuth client",
                )

            client = oauth_client_manager.get_client(client_id)
            client_info = oauth_client_manager.get_client_info(client_id)
            if client is None or client_info is None:
                raise HTTPException(
                    status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                    detail="OAuth client unavailable after re-registration",
                )

            if not await oauth_client_manager._preflight_authorization_url(
                    client, client_info
            ):
                raise HTTPException(
                    status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                    detail="OAuth client registration is still invalid after re-registration",
                )

        return await oauth_client_manager.handle_authorize(request, client_id=client_id)

    @app.get(BASE_PATH + "/oauth/clients/{client_id}/callback")
    async def oauth_client_callback(
            client_id: str,
            request: Request,
            response: Response,
            user=Depends(get_verified_user),
    ):
        return await oauth_client_manager.handle_callback(
            request,
            client_id=client_id,
            user_id=user.id if user else None,
            response=response,
        )

    @app.get(BASE_PATH + "/oauth/{provider}/login")
    async def oauth_login(provider: str, request: Request):
        return await oauth_manager.handle_login(request, provider)

    # OAuth login logic is as follows:
    # 1. Attempt to find a user with matching subject ID, tied to the provider
    # 2. If OAUTH_MERGE_ACCOUNTS_BY_EMAIL is true, find a user with the email address provided via OAuth
    #    - This is considered insecure in general, as OAuth providers do not always verify email addresses
    # 3. If there is no user, and ENABLE_OAUTH_SIGNUP is true, create a user
    #    - Email addresses are considered unique, so we fail registration if the email address is already taken
    @app.get(BASE_PATH + "/oauth/{provider}/login/callback")
    @app.get(BASE_PATH + "/oauth/{provider}/callback")  # Legacy endpoint
    async def oauth_login_callback(provider: str, request: Request, response: Response):
        return await oauth_manager.handle_callback(request, provider, response)

    @app.get(f"{BASE_PATH}/manifest.json")
    async def get_manifest_json():
        if app.state.EXTERNAL_PWA_MANIFEST_URL:
            return requests.get(app.state.EXTERNAL_PWA_MANIFEST_URL).json()
        else:
            return {
                "name": app.state.WEBUI_NAME,
                "short_name": app.state.WEBUI_NAME,
                "description": f"{app.state.WEBUI_NAME} is an open, extensible, user-friendly interface for AI that adapts to your workflow.",
                "start_url": "/",
                "display": "standalone",
                "background_color": "#343541",
                "icons": [
                    {
                        "src": "/static/logo.png",
                        "type": "image/png",
                        "sizes": "500x500",
                        "purpose": "any",
                    },
                    {
                        "src": "/static/logo.png",
                        "type": "image/png",
                        "sizes": "500x500",
                        "purpose": "maskable",
                    },
                ],
                "share_target": {
                    "action": "/",
                    "method": "GET",
                    "params": {"text": "shared"},
                },
            }

    @app.get(f"{BASE_PATH}/opensearch.xml")
    async def get_opensearch_xml():
        xml_content = rf"""
        <OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/" xmlns:moz="http://www.mozilla.org/2006/browser/search/">
        <ShortName>{app.state.WEBUI_NAME}</ShortName>
        <Description>Search {app.state.WEBUI_NAME}</Description>
        <InputEncoding>UTF-8</InputEncoding>
        <Image width="16" height="16" type="image/x-icon">{app.state.config.WEBUI_URL}/static/favicon.png</Image>
        <Url type="text/html" method="get" template="{app.state.config.WEBUI_URL}/?q={"{searchTerms}"}"/>
        <moz:SearchForm>{app.state.config.WEBUI_URL}</moz:SearchForm>
        </OpenSearchDescription>
        """
        return Response(content=xml_content, media_type="application/xml")

    @app.get(f"{BASE_PATH}/health")
    async def healthcheck():
        UTC = getattr(dt, "UTC", dt.timezone.utc)
        upTime = dt.datetime.now().astimezone().astimezone(UTC)
        now_utc = dt.datetime.now(UTC)
        return {
            "timestamp": now_utc.isoformat().replace("+00:00", "Z"),  # ISO8601 with Z
            "uptime": str(now_utc - upTime),
        }

    @app.get(f"{BASE_PATH}/health/db")
    async def healthcheck_with_db():
        Session.execute(text("SELECT 1;")).all()
        return {"status": True}

    @app.get(BASE_PATH + "/cache/{path:path}")
    async def serve_cache_file(
            path: str,
            user=Depends(get_verified_user),
    ):
        file_path = os.path.abspath(os.path.join(CACHE_DIR, path))
        # prevent path traversal
        if not file_path.startswith(os.path.abspath(CACHE_DIR)):
            raise HTTPException(status_code=404, detail="File not found")
        if not os.path.isfile(file_path):
            raise HTTPException(status_code=404, detail="File not found")
        return FileResponse(file_path)
