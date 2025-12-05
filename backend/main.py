import uvicorn
from contextlib import asynccontextmanager
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from dotenv import find_dotenv, load_dotenv
from pathlib import Path
from fastmcp import FastMCP
from open_webui.main import app as open_webui_app
from mcp_server.server import mcp

BASE_DIR = Path(__file__).parent.parent
print(f"BASE_DIR: {BASE_DIR}")
load_dotenv(find_dotenv(str(BASE_DIR / ".env")))

# mcp = FastMCP("JumpServer MCP Server")

@mcp.tool("list-users")
async def list_users():
    return {"users": ["user1", "user2", "user3"]}

mcp_app = mcp.http_app(transport="streamable-http", path="/")


@asynccontextmanager
async def merged_lifespan(app: FastAPI):
    # Run open_webui_app's lifespan first to initialize its state
    # Use the lifespan_context method to properly run it
    if hasattr(open_webui_app, 'router') and hasattr(open_webui_app.router, 'lifespan_context'):
        async with open_webui_app.router.lifespan_context(open_webui_app):
            # Now run mcp_app's lifespan
            # FastMCP requires lifespan to be passed to parent app
            # Check if mcp_app has a lifespan attribute
            if hasattr(mcp_app, 'lifespan'):
                # mcp_app.lifespan is a context manager that takes the app as argument
                async with mcp_app.lifespan(app):
                    yield
            else:
                yield
    else:
        # Fallback: try to get lifespan directly from open_webui_app
        # This shouldn't happen with FastAPI, but just in case
        if hasattr(mcp_app, 'lifespan'):
            async with mcp_app.lifespan(app):
                yield
        else:
            yield


# Create main app with merged lifespan
# FastMCP requires lifespan to be passed to parent app
app = FastAPI(lifespan=merged_lifespan)

# Mount sub-applications
# This preserves each app's state, so request.app.state.config works correctly
# When a request comes to open_webui_app, request.app will point to open_webui_app
app.mount("/kael/mcp", mcp_app)
app.mount("/", open_webui_app)

# Add CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
    allow_credentials=True,
)


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8083)