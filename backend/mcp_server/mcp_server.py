import uvicorn
from contextlib import asynccontextmanager
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse, RedirectResponse
from fastmcp import FastMCP


fastapi_app = FastAPI(
    title="api",
    description="api + mcp",
)

@fastapi_app.get("/")
async def root():
    return RedirectResponse(url="/docs")

mcp = FastMCP("JumpServer MCP Server")

@mcp.tool("list-users")
async def list_users():
    return {"users": ["user1", "user2", "user3"]}

# Create mcp_app first
mcp_app = mcp.http_app(transport="streamable-http", path="/a")


@asynccontextmanager
async def merged_lifespan(app: FastAPI):
    # Run fastapi_app's lifespan if it has one
    if hasattr(fastapi_app, 'router') and hasattr(fastapi_app.router, 'lifespan_context'):
        async with fastapi_app.router.lifespan_context(fastapi_app):
            # Run mcp_app's lifespan
            # FastMCP requires lifespan to be passed to parent app
            if hasattr(mcp_app, 'lifespan'):
                async with mcp_app.lifespan(app):
                    yield
            else:
                yield
    else:
        # If fastapi_app doesn't have lifespan, just run mcp_app's lifespan
        if hasattr(mcp_app, 'lifespan'):
            async with mcp_app.lifespan(app):
                yield
        else:
            yield


# Create main app with merged lifespan
# FastMCP requires lifespan to be passed to parent app
app = FastAPI(lifespan=merged_lifespan)

# Mount sub-applications
app.mount("/mcp", mcp_app)
app.mount("/", fastapi_app)

# Add CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
    allow_credentials=True,
)



if __name__ == "__main__":
    uvicorn.run(
        app,
        host="0.0.0.0",
        port="8000",
    )