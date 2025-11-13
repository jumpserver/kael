import asyncio
import logging
import mimetypes
import os
import sys

from contextlib import asynccontextmanager
import anyio.to_thread

from fastapi import (
    FastAPI,
    HTTPException,
    Request,
    applications
)
from fastapi.staticfiles import StaticFiles
from fastapi.openapi.docs import get_swagger_ui_html

from starlette.exceptions import HTTPException as StarletteHTTPException
from starlette.datastructures import Headers

from open_webui.jms import setup_poll_jms_event
from open_webui.utils.logger import start_logger
from open_webui.socket.main import (
    app as socket_app,
    periodic_usage_pool_cleanup,
)

from open_webui.models.functions import Functions

from open_webui.config import (
    THREAD_POOL_SIZE,
    ENV,
    reset_config,
)
from open_webui.env import (
    LICENSE_KEY,
    REDIS_URL,
    REDIS_CLUSTER,
    REDIS_SENTINEL_HOSTS,
    REDIS_SENTINEL_PORT,
    GLOBAL_LOG_LEVEL,
    SAFE_MODE,
    SRC_LOG_LEVELS,
    VERSION,
    INSTANCE_ID,
    WEBUI_BUILD_HASH,
    RESET_CONFIG_ON_START, FRONTEND_BUILD_DIR,
)

from open_webui.utils.models import (
    get_all_models,
)

from open_webui.utils.auth import (
    get_license_data,
)
from open_webui.utils.plugin import install_tool_and_function_dependencies
from open_webui.utils.redis import get_redis_connection

from open_webui.tasks import (
    redis_task_command_listener,
)  # Import from tasks.py

from open_webui.utils.redis import get_sentinels_from_env
from open_webui.app import init_app_config
from open_webui.middlewares import init_middlewares
from open_webui.urls import setup_routes

if SAFE_MODE:
    print("SAFE MODE ENABLED")
    Functions.deactivate_all_functions()

logging.basicConfig(stream=sys.stdout, level=GLOBAL_LOG_LEVEL)
log = logging.getLogger(__name__)
log.setLevel(SRC_LOG_LEVELS["MAIN"])


class SPAStaticFiles(StaticFiles):
    async def get_response(self, path: str, scope):
        try:
            return await super().get_response(path, scope)
        except (HTTPException, StarletteHTTPException) as ex:
            if ex.status_code == 404:
                if path.endswith(".js"):
                    # Return 404 for javascript files
                    raise ex
                else:
                    return await super().get_response("index.html", scope)
            else:
                raise ex


print(
    rf"""
 ██████╗ ██████╗ ███████╗███╗   ██╗    ██╗    ██╗███████╗██████╗ ██╗   ██╗██╗
██╔═══██╗██╔══██╗██╔════╝████╗  ██║    ██║    ██║██╔════╝██╔══██╗██║   ██║██║
██║   ██║██████╔╝█████╗  ██╔██╗ ██║    ██║ █╗ ██║█████╗  ██████╔╝██║   ██║██║
██║   ██║██╔═══╝ ██╔══╝  ██║╚██╗██║    ██║███╗██║██╔══╝  ██╔══██╗██║   ██║██║
╚██████╔╝██║     ███████╗██║ ╚████║    ╚███╔███╔╝███████╗██████╔╝╚██████╔╝██║
 ╚═════╝ ╚═╝     ╚══════╝╚═╝  ╚═══╝     ╚══╝╚══╝ ╚══════╝╚═════╝  ╚═════╝ ╚═╝


v{VERSION} - building the best AI user interface.
{f"Commit: {WEBUI_BUILD_HASH}" if WEBUI_BUILD_HASH != "dev-build" else ""}
https://github.com/open-webui/open-webui
"""
)


@asynccontextmanager
async def lifespan(app: FastAPI):
    app.state.instance_id = INSTANCE_ID
    start_logger()

    if RESET_CONFIG_ON_START:
        reset_config()

    if LICENSE_KEY:
        get_license_data(app, LICENSE_KEY)

    # This should be blocking (sync) so functions are not deactivated on first /get_models calls
    # when the first user lands on the / route.
    log.info("Installing external dependencies of functions and tools...")
    install_tool_and_function_dependencies()

    app.state.redis = get_redis_connection(
        redis_url=REDIS_URL,
        redis_sentinels=get_sentinels_from_env(
            REDIS_SENTINEL_HOSTS, REDIS_SENTINEL_PORT
        ),
        redis_cluster=REDIS_CLUSTER,
        async_mode=True,
    )

    if app.state.redis is not None:
        app.state.redis_task_command_listener = asyncio.create_task(
            redis_task_command_listener(app)
        )

    if THREAD_POOL_SIZE and THREAD_POOL_SIZE > 0:
        limiter = anyio.to_thread.current_default_thread_limiter()
        limiter.total_tokens = THREAD_POOL_SIZE

    asyncio.create_task(periodic_usage_pool_cleanup())

    if app.state.config.ENABLE_BASE_MODELS_CACHE:
        await get_all_models(
            Request(
                # Creating a mock request object to pass to get_all_models
                {
                    "type": "http",
                    "asgi.version": "3.0",
                    "asgi.spec_version": "2.0",
                    "method": "GET",
                    "path": "/internal",
                    "query_string": b"",
                    "headers": Headers({}).raw,
                    "client": ("127.0.0.1", 12345),
                    "server": ("127.0.0.1", 80),
                    "scheme": "http",
                    "app": app,
                }
            ),
            None,
        )

    setup_poll_jms_event()
    yield

    if hasattr(app.state, "redis_task_command_listener"):
        app.state.redis_task_command_listener.cancel()


app = FastAPI(
    title="JumpServer Chat && Open WebUI",
    docs_url="/docs" if ENV == "dev" else None,
    openapi_url="/openapi.json" if ENV == "dev" else None,
    redoc_url=None,
    lifespan=lifespan,
)

init_app_config(app)
init_middlewares(app)
# Setup all routes
setup_routes(app, socket_app)


def swagger_ui_html(*args, **kwargs):
    return get_swagger_ui_html(
        *args,
        **kwargs,
        swagger_js_url="/static/swagger-ui/swagger-ui-bundle.js",
        swagger_css_url="/static/swagger-ui/swagger-ui.css",
        swagger_favicon_url="/static/swagger-ui/favicon.png",
    )


applications.get_swagger_ui_html = swagger_ui_html
BASE_PATH = "/kael"
if os.path.exists(FRONTEND_BUILD_DIR):
    mimetypes.add_type("text/javascript", ".js")
    app.mount(
        BASE_PATH,
        SPAStaticFiles(directory=FRONTEND_BUILD_DIR, html=True),
        name="spa-static-files",
    )
else:
    log.warning(
        f"Frontend build directory not found at '{FRONTEND_BUILD_DIR}'. Serving API only."
    )
