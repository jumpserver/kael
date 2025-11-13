import logging
import time
import re

from urllib.parse import urlencode, parse_qs, urlparse

from fastapi import (
    Request,
    status,
)

from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse, RedirectResponse

from starlette_compress import CompressMiddleware

from starlette.middleware.base import BaseHTTPMiddleware
from starlette.middleware.sessions import SessionMiddleware

from starsessions import (
    SessionMiddleware as StarSessionsMiddleware,
    SessionAutoloadMiddleware,
)
from starsessions.stores.redis import RedisStore

from open_webui.utils import logger
from open_webui.utils.audit import AuditLevel, AuditLoggingMiddleware
from open_webui.internal.db import Session

from open_webui.config import CORS_ALLOW_ORIGIN
from open_webui.env import (
    AUDIT_EXCLUDED_PATHS,
    AUDIT_LOG_LEVEL,
    REDIS_URL,
    REDIS_KEY_PREFIX,
    MAX_BODY_LOG_SIZE,
    WEBUI_SECRET_KEY,
    WEBUI_SESSION_COOKIE_SAME_SITE,
    WEBUI_SESSION_COOKIE_SECURE,
    ENABLE_COMPRESSION_MIDDLEWARE,
    ENABLE_STAR_SESSIONS_MIDDLEWARE, SRC_LOG_LEVELS,
)

from open_webui.utils.auth import (
    get_http_authorization_cred,
)
from open_webui.utils.oauth import (
    decrypt_data,
    OAuthClientInformationFull,
)
from open_webui.utils.security_headers import SecurityHeadersMiddleware

log = logging.getLogger(__name__)
log.setLevel(SRC_LOG_LEVELS["MAIN"])

BASE_PATH = "/kael"


class RedirectMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request: Request, call_next):
        # Check if the request is a GET request
        if request.method == "GET":
            path = request.url.path
            query_params = dict(parse_qs(urlparse(str(request.url)).query))

            redirect_params = {}

            # Check for the specific watch path and the presence of 'v' parameter
            if path.endswith("/watch") and "v" in query_params:
                # Extract the first 'v' parameter
                youtube_video_id = query_params["v"][0]
                redirect_params["youtube"] = youtube_video_id

            if "shared" in query_params and len(query_params["shared"]) > 0:
                # PWA share_target support

                text = query_params["shared"][0]
                if text:
                    urls = re.match(r"https://\S+", text)
                    if urls:
                        from open_webui.retrieval.loaders.youtube import _parse_video_id

                        if youtube_video_id := _parse_video_id(urls[0]):
                            redirect_params["youtube"] = youtube_video_id
                        else:
                            redirect_params["load-url"] = urls[0]
                    else:
                        redirect_params["q"] = text

            if redirect_params:
                redirect_url = f"/?{urlencode(redirect_params)}"
                return RedirectResponse(url=redirect_url)

        # Proceed with the normal flow of other requests
        response = await call_next(request)
        return response


def init_middlewares(app):
    # Add the middleware to the app
    if ENABLE_COMPRESSION_MIDDLEWARE:
        app.add_middleware(CompressMiddleware)

    app.add_middleware(RedirectMiddleware)
    app.add_middleware(SecurityHeadersMiddleware)

    @app.middleware("http")
    async def commit_session_after_request(request: Request, call_next):
        response = await call_next(request)
        # log.debug("Commit session after request")
        Session.commit()
        return response

    @app.middleware("http")
    async def check_url(request: Request, call_next):
        start_time = int(time.time())
        request.state.token = get_http_authorization_cred(
            request.headers.get("Authorization")
        )

        request.state.enable_api_key = app.state.config.ENABLE_API_KEY
        response = await call_next(request)
        process_time = int(time.time()) - start_time
        response.headers["X-Process-Time"] = str(process_time)
        return response

    @app.middleware("http")
    async def inspect_websocket(request: Request, call_next):
        if (
                f"{BASE_PATH}/ws/socket.io" in request.url.path
                and request.query_params.get("transport") == "websocket"
        ):
            upgrade = (request.headers.get("Upgrade") or "").lower()
            connection = (request.headers.get("Connection") or "").lower().split(",")
            # Check that there's the correct headers for an upgrade, else reject the connection
            # This is to work around this upstream issue: https://github.com/miguelgrinberg/python-engineio/issues/367
            if upgrade != "websocket" or "upgrade" not in connection:
                return JSONResponse(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    content={"detail": "Invalid WebSocket upgrade request"},
                )
        return await call_next(request)

    app.add_middleware(
        CORSMiddleware,
        allow_origins=CORS_ALLOW_ORIGIN,
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )

    try:
        audit_level = AuditLevel(AUDIT_LOG_LEVEL)
    except ValueError as e:
        logger.error(f"Invalid audit level: {AUDIT_LOG_LEVEL}. Error: {e}")
        audit_level = AuditLevel.NONE

    if audit_level != AuditLevel.NONE:
        app.add_middleware(
            AuditLoggingMiddleware,
            audit_level=audit_level,
            excluded_paths=AUDIT_EXCLUDED_PATHS,
            max_body_size=MAX_BODY_LOG_SIZE,
        )

    ############################
    # OAuth Login & Callback
    ############################

    # Initialize OAuth client manager with any MCP tool servers using OAuth 2.1
    if len(app.state.config.TOOL_SERVER_CONNECTIONS) > 0:
        for tool_server_connection in app.state.config.TOOL_SERVER_CONNECTIONS:
            if tool_server_connection.get("type", "openapi") == "mcp":
                server_id = tool_server_connection.get("info", {}).get("id")
                auth_type = tool_server_connection.get("auth_type", "none")

                if server_id and auth_type == "oauth_2.1":
                    oauth_client_info = tool_server_connection.get("info", {}).get(
                        "oauth_client_info", ""
                    )

                    try:
                        oauth_client_info = decrypt_data(oauth_client_info)
                        app.state.oauth_client_manager.add_client(
                            f"mcp:{server_id}",
                            OAuthClientInformationFull(**oauth_client_info),
                        )
                    except Exception as e:
                        log.error(
                            f"Error adding OAuth client for MCP tool server {server_id}: {e}"
                        )
                        pass

    try:
        if ENABLE_STAR_SESSIONS_MIDDLEWARE:
            redis_session_store = RedisStore(
                url=REDIS_URL,
                prefix=(f"{REDIS_KEY_PREFIX}:session:" if REDIS_KEY_PREFIX else "session:"),
            )

            app.add_middleware(SessionAutoloadMiddleware)
            app.add_middleware(
                StarSessionsMiddleware,
                store=redis_session_store,
                cookie_name="owui-session",
                cookie_same_site=WEBUI_SESSION_COOKIE_SAME_SITE,
                cookie_https_only=WEBUI_SESSION_COOKIE_SECURE,
            )
            log.info("Using Redis for session")
        else:
            raise ValueError("No Redis URL provided")
    except Exception as e:
        app.add_middleware(
            SessionMiddleware,
            secret_key=WEBUI_SECRET_KEY,
            session_cookie="owui-session",
            same_site=WEBUI_SESSION_COOKIE_SAME_SITE,
            https_only=WEBUI_SESSION_COOKIE_SECURE,
        )
