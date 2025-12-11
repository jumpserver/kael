"""
Utility functions for JumpServer MCP Server.

This module contains shared HTTP client and request handling functions.
"""

from __future__ import annotations

import os
import copy
from fastmcp.server.dependencies import get_access_token
from typing import Any, Dict, Optional

import httpx
import dotenv

dotenv.load_dotenv()


def _build_headers(token: str | None) -> Dict[str, str]:
    """Build HTTP headers with authorization if token is provided."""
    headers: Dict[str, str] = {"Accept": "application/json"}
    if token:
        headers["Authorization"] = f"Token {token}"
    return headers


def check_authentication() -> Dict[str, Any] | None:
    """Check if the current user is authenticated.
    
    Returns:
        None if authenticated, or a dict with error information if anonymous.
    """
    try:
        access_token = get_access_token()
        if not access_token:
            return {
                "error": "Authentication required",
                "message": "No access token found. Please authenticate first.",
            }
        
        claims = access_token.claims or {}
        is_anonymous = claims.get("is_anonymous", False)
        
        if is_anonymous:
            auth_error = claims.get("auth_error", "Authentication failed")
            return {
                "error": "Authentication failed",
                "message": f"Authentication failed: {auth_error}",
                "suggestion": "Use the login tool to authenticate with your username and password.",
            }
        
        return None  # Authenticated
    except Exception as e:
        return {
            "error": "Authentication check failed",
            "message": str(e),
        }


# Initialize HTTP client
host = os.getenv("JUMPSERVER_HOST", "http://localhost:8080").rstrip("/")
token = os.getenv("JUMPSERVER_TOKEN", "")

client = httpx.AsyncClient(
    base_url=host,
    timeout=httpx.Timeout(30.0, connect=10.0),
)


def _extract_response_data(response: httpx.Response) -> Any:
    """Extract data from HTTP response, trying JSON first, then text."""
    if not response.content:
        return None
    try:
        return response.json()
    except Exception:
        return response.text


def _extract_error_detail(exc: httpx.HTTPStatusError) -> Any:
    """Extract error details from HTTP error response."""
    if not exc.response.content:
        return None
    try:
        return exc.response.json()
    except Exception:
        return exc.response.text


async def safe_request(
    method: str,
    url: str,
    *,
    params: Optional[Dict[str, Any]] = None,
    json: Optional[Dict[str, Any]] = None,
    **kwargs: Any,
) -> Dict[str, Any]:
    """
    Make a safe HTTP request with comprehensive error handling.
    
    This function handles all errors internally and returns a consistent
    result dictionary, eliminating the need for try-catch blocks in callers.
    
    Args:
        method: HTTP method (GET, POST, PATCH, OPTIONS, etc.)
        url: URL path (relative to base_url)
        params: Query parameters
        json: JSON payload for POST/PATCH requests
        **kwargs: Additional arguments to pass to httpx.request
        
    Returns:
        Dict with:
            - "status": HTTP status code (if successful)
            - "result": Response data (JSON parsed or text)
            - "error": Error message (if failed)
            - "error_detail": Detailed error information (if available)
    """
    
    try:
        response = await client.request(method, url, params=params, json=json, **kwargs)
        response.raise_for_status()
        
        return {
            "status": response.status_code,
            "result": _extract_response_data(response),
        }
    except httpx.HTTPStatusError as exc:
        return {
            "status": exc.response.status_code,
            "error": f"HTTP {exc.response.status_code}",
            "error_detail": _extract_error_detail(exc),
        }
    except Exception as exc:
        return {
            "error": str(exc),
            "error_type": type(exc).__name__,
        }


async def request_resource(
    method: str,
    resource: str,
    *,
    resource_id: Optional[str] = None,
    params: Optional[Dict[str, Any]] = None,
    payload: Optional[Dict[str, Any]] = None,
) -> Dict[str, Any]:
    """
    Make a request to the JumpServer resource API.
    
    This function builds the correct URL and handles the request/response,
    returning a consistent result dictionary.
    
    Args:
        method: HTTP method
        resource: Resource name (e.g., "users", "user-groups")
        resource_id: Optional resource ID for detail operations
        params: Query parameters (supports search, limit, offset, etc.)
        payload: Request body for POST/PATCH
        
    Returns:
        Dict with resource, resource_id, status, result/error
    """
    # Build URL: /api/v1/resources/{resource}/ or /api/v1/resources/{resource}/{id}/
    url = f"/api/v1/resources/{resource}/"
    if resource_id:
        url = f"/api/v1/resources/{resource}/{resource_id}/"
    
    result = await safe_request(method, url, params=params, json=payload)
    
    # Add resource context to the result
    result["resource"] = resource
    if resource_id:
        result["resource_id"] = resource_id
    
    return result


def resolve_openapi_refs(spec: Dict[str, Any], base_path: str = "#") -> Dict[str, Any]:
    """
    Recursively resolve all $ref references in an OpenAPI spec.
    
    This function replaces all $ref references with their actual definitions,
    which helps avoid issues with strict schema validators that can't resolve
    references properly.
    
    Args:
        spec: The OpenAPI specification dictionary
        base_path: The base path for resolving references (used internally)
        
    Returns:
        A new OpenAPI spec with all $ref references resolved
    """
    if not isinstance(spec, dict):
        return spec
    
    # Create a deep copy to avoid modifying the original
    resolved = copy.deepcopy(spec)
    
    # Extract components for reference resolution
    components = resolved.get("components", {})
    schemas = components.get("schemas", {})
    
    def _resolve_ref(obj: Any, visited: set[str] = None) -> Any:
        """Internal function to recursively resolve references."""
        if visited is None:
            visited = set()
            
        if isinstance(obj, dict):
            # Handle allOf, anyOf, oneOf - resolve refs inside them
            if "allOf" in obj or "anyOf" in obj or "oneOf" in obj:
                result = {}
                # Copy non-composition keys first
                for key in obj:
                    if key not in ("allOf", "anyOf", "oneOf"):
                        result[key] = _resolve_ref(obj[key], visited)
                
                # Resolve composition schemas
                for comp_key in ("allOf", "anyOf", "oneOf"):
                    if comp_key in obj:
                        resolved_items = []
                        for item in obj[comp_key]:
                            resolved_item = _resolve_ref(item, visited)
                            resolved_items.append(resolved_item)
                        result[comp_key] = resolved_items
                
                return result
            
            # Check if this is a $ref
            if "$ref" in obj:
                ref_path = obj["$ref"]
                
                # Handle OpenAPI 3.0 format: #/components/schemas/SchemaName
                if ref_path.startswith("#/components/schemas/"):
                    schema_name = ref_path.split("/")[-1]
                    
                    # Prevent infinite recursion
                    if schema_name in visited:
                        # Return a placeholder to break the cycle
                        return {"type": "object", "description": f"Circular reference to {schema_name}"}
                    
                    if schema_name in schemas:
                        visited.add(schema_name)
                        resolved_schema = _resolve_ref(schemas[schema_name], visited)
                        visited.remove(schema_name)
                        
                        # If there are other keys besides $ref, merge them
                        other_keys = {k: v for k, v in obj.items() if k != "$ref"}
                        if other_keys:
                            if isinstance(resolved_schema, dict):
                                # Merge resolved schema with other properties
                                resolved_schema = {**resolved_schema, **other_keys}
                            else:
                                # Fallback: keep original structure
                                return obj
                        
                        return resolved_schema
                    else:
                        # Schema not found, return original ref
                        return obj
            
            # Recursively process all values in the dict
            return {k: _resolve_ref(v, visited) for k, v in obj.items()}
        
        elif isinstance(obj, list):
            # Recursively process all items in the list
            return [_resolve_ref(item, visited) for item in obj]
        
        else:
            # Primitive type, return as-is
            return obj
    
    # Resolve all references in the spec
    resolved = _resolve_ref(resolved)
    
    return resolved

