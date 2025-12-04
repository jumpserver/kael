from fastmcp import FastMCP
from fastapi import FastAPI
import uvicorn

# Create MCP server
mcp = FastMCP("Analytics Tools")

@mcp.tool
def analyze_pricing(category: str) -> dict:
    return {
        "category": category,
    }
    

# Create ASGI app from MCP server
mcp_app = mcp.http_app(transport="streamable-http", path="/")

# Key: Pass lifespan to FastAPI
app = FastAPI(title="E-commerce API", lifespan=mcp_app.lifespan)

# Mount the MCP server
app.mount("/mcp", mcp_app)

# Now: API at /products/*, MCP at /analytics/mcp/

if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8000)