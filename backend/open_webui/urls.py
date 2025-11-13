from fastapi import APIRouter
from fastapi.staticfiles import StaticFiles

from open_webui.routers import (
    audio,
    images,
    ollama,
    openai,
    retrieval,
    pipelines,
    tasks,
    auths,
    channels,
    chats,
    notes,
    folders,
    configs,
    groups,
    files,
    functions,
    memories,
    models,
    knowledge,
    prompts,
    evaluations,
    tools,
    users,
    utils,
    scim,
)

from open_webui.config import (
    # Misc
    STATIC_DIR,
)
from open_webui.env import (
    # SCIM
    SCIM_ENABLED,
)

from open_webui.routers.common import setup_lazy_routes

# Base path configuration - easily changeable
BASE_PATH = "/kael"


def setup_routes(app, socket_app):
    # Create main API router
    main_router = APIRouter()

    main_router.include_router(ollama.router, prefix="/ollama", tags=["ollama"])
    main_router.include_router(openai.router, prefix="/openai", tags=["openai"])

    main_router.include_router(pipelines.router, prefix="/api/v1/pipelines", tags=["pipelines"])
    main_router.include_router(tasks.router, prefix="/api/v1/tasks", tags=["tasks"])
    main_router.include_router(images.router, prefix="/api/v1/images", tags=["images"])

    main_router.include_router(audio.router, prefix="/api/v1/audio", tags=["audio"])
    main_router.include_router(retrieval.router, prefix="/api/v1/retrieval", tags=["retrieval"])

    main_router.include_router(configs.router, prefix="/api/v1/configs", tags=["configs"])

    main_router.include_router(auths.router, prefix="/api/v1/auths", tags=["auths"])
    main_router.include_router(users.router, prefix="/api/v1/users", tags=["users"])

    main_router.include_router(channels.router, prefix="/api/v1/channels", tags=["channels"])
    main_router.include_router(chats.router, prefix="/api/v1/chats", tags=["chats"])
    main_router.include_router(notes.router, prefix="/api/v1/notes", tags=["notes"])

    main_router.include_router(models.router, prefix="/api/v1/models", tags=["models"])
    main_router.include_router(knowledge.router, prefix="/api/v1/knowledge", tags=["knowledge"])
    main_router.include_router(prompts.router, prefix="/api/v1/prompts", tags=["prompts"])
    main_router.include_router(tools.router, prefix="/api/v1/tools", tags=["tools"])

    main_router.include_router(memories.router, prefix="/api/v1/memories", tags=["memories"])
    main_router.include_router(folders.router, prefix="/api/v1/folders", tags=["folders"])
    main_router.include_router(groups.router, prefix="/api/v1/groups", tags=["groups"])
    main_router.include_router(files.router, prefix="/api/v1/files", tags=["files"])
    main_router.include_router(functions.router, prefix="/api/v1/functions", tags=["functions"])
    main_router.include_router(
        evaluations.router, prefix="/api/v1/evaluations", tags=["evaluations"]
    )
    main_router.include_router(utils.router, prefix="/api/v1/utils", tags=["utils"])

    # SCIM 2.0 API for identity management
    if SCIM_ENABLED:
        main_router.include_router(scim.router, prefix="/api/v1/scim/v2", tags=["scim"])

    setup_lazy_routes(app)

    app.include_router(main_router, prefix=BASE_PATH)
    app.mount(f"{BASE_PATH}/ws", socket_app)
    app.mount(f"{BASE_PATH}/kael", StaticFiles(directory=STATIC_DIR), name="kael")
    app.mount(f"{BASE_PATH}/static", StaticFiles(directory=STATIC_DIR), name="static")
