"""
Custom middleware for filtering tools based on authentication and permissions.
"""

from __future__ import annotations

import inspect
from typing import Sequence
from fastmcp.server.middleware.middleware import (
    CallNext,
    Middleware,
    MiddlewareContext,
)
from fastmcp.tools.tool import Tool
from fastmcp.server.dependencies import get_access_token, get_http_headers
from fastmcp.utilities.logging import get_logger
from .auth import JumpServerAuthProvider
import mcp.types

logger = get_logger(__name__)


def _extract_user_roles(claims: dict) -> list[str]:
    """Extract role names from user claims.
    
    Note: superuser automatically has admin role.
    """
    role_names = []
    is_superuser = claims.get("is_superuser", False)
    is_org_admin = claims.get("is_org_admin", False)
    
    if is_superuser:
        role_names.append("superuser")
        role_names.append("admin")  # superuser automatically has admin role
    elif is_org_admin:
        role_names.append("admin")
    
    return role_names


def _get_tool_permission_metadata(tool: Tool) -> tuple[bool | None, tuple[str, ...] | None]:
    """
    Extract permission metadata from tool function decorators.
    
    Returns:
        Tuple of (requires_auth, required_roles):
        - requires_auth: True if requires authentication, False if always visible, None if not specified
        - required_roles: Tuple of required roles, or None if no specific roles required
    
    Default behavior: If no decorator is found, defaults to require_admin (requires_auth=True, required_roles=("admin",))
    """
    # Special case: login tool is only visible to unauthenticated users
    if tool.name == "login":
        return None, None  # Special handling in _check_tool_permission
    
    # Get the function from the tool
    fn = getattr(tool, 'fn', None)
    if not fn:
        # No function found, default to requiring admin role
        logger.debug(f"Tool {tool.name} has no fn attribute, defaulting to require admin")
        return True, ("admin",)
    
    # Check for permission metadata on the function
    requires_auth = getattr(fn, '__requires_auth__', None)
    required_roles = getattr(fn, '__required_roles__', None)
    
    # If not found, try unwrapping to find metadata
    # Check all levels of wrapping to find decorator metadata
    if requires_auth is None and required_roles is None:
        unwrapped_fn = fn
        checked_functions = [fn]
        while hasattr(unwrapped_fn, '__wrapped__'):
            unwrapped_fn = unwrapped_fn.__wrapped__
            if unwrapped_fn in checked_functions:
                break  # Avoid infinite loop
            checked_functions.append(unwrapped_fn)
            
            # Check metadata on unwrapped function
            if requires_auth is None:
                requires_auth = getattr(unwrapped_fn, '__requires_auth__', None)
            if required_roles is None:
                required_roles = getattr(unwrapped_fn, '__required_roles__', None)
            
            # If we found both, we can stop
            if requires_auth is not None or required_roles is not None:
                break
    
    # Also check if there's a __function__ attribute (some wrappers store original function there)
    if requires_auth is None and required_roles is None:
        original_fn = getattr(fn, '__function__', None)
        if original_fn:
            requires_auth = getattr(original_fn, '__requires_auth__', None)
            required_roles = getattr(original_fn, '__required_roles__', None)
    
    # If no metadata found, default to requiring admin role
    # This means tools without decorators require admin role by default
    if requires_auth is None and required_roles is None:
        logger.debug(f"Tool {tool.name} has no permission decorator, defaulting to require admin")
        return True, ("admin",)
    
    logger.debug(
        f"Tool {tool.name} metadata from decorator: requires_auth={requires_auth}, required_roles={required_roles}"
    )
    return requires_auth, required_roles


def _check_tool_permission(tool: Tool, claims: dict) -> bool:
    """Check if user has permission to access a tool."""
    # Special case: login tool is only visible to unauthenticated users
    if tool.name == "login":
        is_anonymous = claims.get("is_anonymous", False)
        # Only show login tool to anonymous users
        return is_anonymous
    
    requires_auth, required_roles = _get_tool_permission_metadata(tool)
    
    # If tool is always visible (requires_auth=False), allow access
    if requires_auth is False:
        return True
    
    # Check if user is anonymous
    is_anonymous = claims.get("is_anonymous", False)
    
    # If tool requires authentication and user is anonymous, deny access
    if requires_auth and is_anonymous:
        return False
    
    # If tool requires specific roles, check user roles
    if required_roles is not None and len(required_roles) > 0:
        user_roles = _extract_user_roles(claims)
        # Check if user has any of the required roles
        has_role = any(role in required_roles for role in user_roles)
        if not has_role:
            logger.debug(
                f"User roles {user_roles} don't match required roles {required_roles} for tool {tool.name}"
            )
        return has_role
    
    # Tool requires authentication but no specific roles (required_roles is None)
    # This means any authenticated user can access it
    # For authenticated users (not anonymous), allow access
    if requires_auth is True:
        result = not is_anonymous
        if not result:
            logger.debug(f"Tool {tool.name} requires auth but user is anonymous")
        return result
    
    # If requires_auth is None (not specified), default to requiring authentication
    # This should not happen if decorators are properly applied, but handle it
    return not is_anonymous


class ToolFilterMiddleware(Middleware):
    """
    Middleware that filters tools based on authentication and permissions.
    
    Only shows tools that the current user has permission to use:
    - Login tool is only visible to unauthenticated users
    - Other tools require authentication (non-anonymous user)
    - Tools with role requirements are filtered based on user roles
    """
    
    async def on_list_tools(
        self,
        context: MiddlewareContext[mcp.types.ListToolsRequest],
        call_next: CallNext[mcp.types.ListToolsRequest, Sequence[Tool]],
    ) -> Sequence[Tool]:
        """Filter tools based on authentication and permissions."""
        # Get all tools first
        all_tools = await call_next(context)
        
        # Get current authentication status
        # Try to get from context first (set by AuthContextMiddleware)
        access_token = get_access_token()
        
        # If no token from context, try to verify directly from HTTP headers
        # This handles the case where MCP middleware runs before AuthContextMiddleware
        if not access_token:
            try:
                # Get HTTP headers to extract token
                headers = get_http_headers()
                auth_header = headers.get('authorization', '') or headers.get('Authorization', '')
                
                if auth_header and auth_header.lower().startswith('bearer '):
                    token = auth_header[7:]  # Remove "Bearer " prefix
                    # Try to verify token directly
                    from .app import mcp
                    auth_provider = mcp.auth
                    if isinstance(auth_provider, JumpServerAuthProvider):
                        access_token = await auth_provider.verify_token(token)
                        logger.debug(f"Verified token directly from headers: {access_token is not None}")
            except Exception as e:
                logger.debug(f"Failed to verify token from headers: {e}")
        
        # If still no token, only show tools that are always visible
        if not access_token:
            claims = {"is_anonymous": True}
        else:
            claims = access_token.claims or {}
            is_anonymous = claims.get("is_anonymous", False)
            claims["is_anonymous"] = is_anonymous
        
        # Filter tools based on permission metadata
        filtered_tools = []
        
        logger.info(
            f"Filtering tools: is_anonymous={claims.get('is_anonymous')}, "
            f"username={claims.get('username')}, "
            f"user_roles={_extract_user_roles(claims)}, "
            f"total_tools={len(all_tools)}"
        )

        username = claims.get('username')
        print("Claims: ", claims)
        
        for tool in all_tools:
            has_permission = _check_tool_permission(tool, claims)
            requires_auth, required_roles = _get_tool_permission_metadata(tool)
            
            logger.info(
                f"Tool {tool.name}: has_permission={has_permission}, "
                f"requires_auth={requires_auth}, required_roles={required_roles}, "
                f"is_anonymous={claims.get('is_anonymous')}, "
                f"user_roles={_extract_user_roles(claims)}"
            )
            
            if has_permission:
                print("Adding tool to filtered tools", tool.name)
                filtered_tools.append(tool)
            else:
                logger.info(f"Filtered out tool {tool.name} - user lacks required permissions")
        
        logger.info(f"Filtered {len(filtered_tools)} tools from {len(all_tools)} total tools")
        return filtered_tools

