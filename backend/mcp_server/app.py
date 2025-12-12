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
from .utils import resolve_openapi_refs


BASE_DIR = Path(__file__).resolve().parent


CORE_HOST = os.environ.get("CORE_HOST", "http://localhost:8000")

# Load OpenAPI spec from local file
# Note: We use local file instead of URL because:
# 1. The OpenAPI URL requires authentication
# 2. At startup time, we don't have user requests yet, so we can't get auth tokens
# 3. FastMCP's official example uses URL, but that works for public APIs
with open(os.path.join(BASE_DIR, "swagger.json"), "r") as f:
    openapi_spec_raw = json.load(f)

# Resolve all $ref references to avoid FastMCP's internal reference resolution errors
# FastMCP internally converts OpenAPI to JSON Schema format (using $defs instead of components/schemas)
# and can fail with errors like "can't resolve reference #/$defs/PasswordRules from id #"
# when encountering complex reference structures (e.g., allOf with $ref).
# The official FastMCP example works because it uses simpler OpenAPI specs.
# Our JumpServer spec has more complex reference structures that need preprocessing.
RESOLVE_REFS = os.environ.get("RESOLVE_OPENAPI_REFS", "true").lower() == "true"
if RESOLVE_REFS:
    openapi_spec = resolve_openapi_refs(openapi_spec_raw)
else:
    openapi_spec = openapi_spec_raw


def disable_output_schema_validation(route, component):
    """
    Component function to disable strict output schema validation.
    
    This sets output_schema to None for all tools, which disables
    strict type validation and allows flexible response formats.
    """
    if hasattr(component, 'output_schema'):
        component.output_schema = None


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
    middleware=[ToolFilterMiddleware(),],
    mcp_component_fn=disable_output_schema_validation,
)