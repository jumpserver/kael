"""
JumpServer MCP Server Application

This module initializes the FastMCP server instance.
"""

from __future__ import annotations
import os
import json
from pathlib import Path
from fastmcp import FastMCP
from fastmcp.server.dependencies import get_http_headers
import httpx
from .auth import JumpServerAuthProvider
from .middleware import ToolFilterMiddleware
from .client import client


BASE_DIR = Path(__file__).resolve().parent


CORE_HOST = os.environ.get("CORE_HOST", "http://localhost:8000")
with open(os.path.join(BASE_DIR, "swagger.json"), "r") as f:
    openapi_spec = json.load(f)


mcp = FastMCP.from_openapi(
    name="JumpServer MCP Server",
    openapi_spec=openapi_spec,
    client=client,
    instructions="""
        Interact with JumpServer's generic resource API.
        Use `data://resources` to discover resource names, inspect schemas
        via `data://resources/{resource}/schema/{action}`, then run the tools
        to list, create, update, or get resource entries.
    """,
    auth=JumpServerAuthProvider(),
    middleware=[ToolFilterMiddleware()],
)