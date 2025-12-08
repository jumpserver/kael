"""Permission control for MCP tools based on user roles."""

from __future__ import annotations

from typing import Callable, Any
from functools import wraps

from fastmcp.server.dependencies import get_access_token
from fastmcp.utilities.logging import get_logger

logger = get_logger(__name__)


class PermissionDenied(Exception):
    """Raised when a user doesn't have permission to access a resource."""
    pass


def _extract_user_roles(claims: dict[str, Any]) -> list[str]:
    """Extract role names from user claims.
    
    Note: superuser automatically has admin role.
    
    Args:
        claims: User claims dictionary
        
    Returns:
        List of role names
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


def _check_user_permission(claims: dict[str, Any], allowed_roles: tuple[str, ...]) -> tuple[bool, list[str]]:
    """Check if user has permission based on roles.
    
    Note: superuser automatically has admin role, so if "admin" is in allowed_roles,
    superuser will also have permission.
    
    Args:
        claims: User claims dictionary
        allowed_roles: Tuple of allowed role names
        
    Returns:
        Tuple of (has_permission, role_names)
    """
    username = claims.get("username", "unknown")
    
    # Extract user roles (superuser automatically includes admin)
    role_names = _extract_user_roles(claims)
    
    # Check if user has any of the allowed roles
    has_permission = any(role in allowed_roles for role in role_names)
    
    return has_permission, role_names


def require_role(*allowed_roles: str):
    """Decorator to require specific roles for a tool.
    
    Args:
        *allowed_roles: One or more role names that are allowed to access the tool.
                       Special roles:
                       - "superuser": Users with is_superuser=True
                       - "admin": Users with admin role (you can customize this)
                       - Any custom role from JumpServer
    
    Example:
        @mcp.tool("admin-tool")
        @require_role("superuser", "admin")
        async def admin_only_tool():
            ...
    """
    def decorator(func: Callable) -> Callable:
        # Store permission metadata on the function for middleware to read
        func.__required_roles__ = allowed_roles  # type: ignore[attr-defined]
        func.__requires_auth__ = True  # type: ignore[attr-defined]
        
        # Also register in the global registry (tool name will be set by @mcp.tool)
        # We'll register it when the tool is actually registered
        
        @wraps(func)
        async def wrapper(*args: Any, **kwargs: Any) -> Any:
            # Get access token
            access_token = get_access_token()
            if not access_token:
                raise PermissionDenied("Authentication required")
            
            claims = access_token.claims or {}
            
            # Check if user is anonymous (authentication failed)
            is_anonymous = claims.get("is_anonymous", False)
            if is_anonymous:
                auth_error = claims.get("auth_error", "Authentication failed")
                logger.warning(
                    f"Anonymous user denied access to {func.__name__}. "
                    f"Authentication failed: {auth_error}"
                )
                raise PermissionDenied(
                    f"Authentication required. Authentication failed: {auth_error}. "
                    f"Use the login tool to authenticate."
                )
            
            username = claims.get("username", "unknown")
            
            # Check permission
            has_permission, role_names = _check_user_permission(claims, allowed_roles)
            
            if not has_permission:
                logger.warning(
                    f"User {username} denied access to {func.__name__}. "
                    f"Required roles: {allowed_roles}, User roles: {role_names}"
                )
                raise PermissionDenied(
                    f"Access denied. Required roles: {', '.join(allowed_roles)}. "
                    f"Your roles: {', '.join(role_names) if role_names else 'none'}"
                )
            
            logger.debug(f"User {username} granted access with roles: {role_names}")
            return await func(*args, **kwargs)
        
        # Copy metadata to wrapper
        wrapper.__required_roles__ = allowed_roles  # type: ignore[attr-defined]
        wrapper.__requires_auth__ = True  # type: ignore[attr-defined]
        
        # Store the original function for later registration
        wrapper.__original_func__ = func  # type: ignore[attr-defined]
        wrapper.__permission_roles__ = allowed_roles  # type: ignore[attr-defined]
        
        return wrapper
    return decorator


def require_authentication(func: Callable) -> Callable:
    """Decorator to require authentication (non-anonymous user) for a tool.
    
    This decorator ensures that only authenticated (non-anonymous) users
    can access the tool. Anonymous users (authentication failed) will be denied.
    
    Example:
        @mcp.tool("my-tool")
        @require_authentication
        async def my_tool():
            ...
    """
    # Store permission metadata on the function for middleware to read
    func.__requires_auth__ = True  # type: ignore[attr-defined]
    func.__required_roles__ = None  # type: ignore[attr-defined]
    
    @wraps(func)
    async def wrapper(*args: Any, **kwargs: Any) -> Any:
        # Get access token
        access_token = get_access_token()
        if not access_token:
            raise PermissionDenied("Authentication required")
        
        claims = access_token.claims or {}
        
        # Check if user is anonymous (authentication failed)
        is_anonymous = claims.get("is_anonymous", False)
        if is_anonymous:
            auth_error = claims.get("auth_error", "Authentication failed")
            logger.warning(
                f"Anonymous user denied access to {func.__name__}. "
                f"Authentication failed: {auth_error}"
            )
            raise PermissionDenied(
                f"Authentication required. Authentication failed: {auth_error}. "
                f"Use the login tool to authenticate."
            )
        
        return await func(*args, **kwargs)
    
    # Copy metadata to wrapper
    wrapper.__requires_auth__ = True  # type: ignore[attr-defined]
    wrapper.__required_roles__ = None  # type: ignore[attr-defined]
    
    # Store the original function for later registration
    wrapper.__original_func__ = func  # type: ignore[attr-defined]
    wrapper.__permission_roles__ = None  # type: ignore[attr-defined]
    
    return wrapper


def require_superuser(func: Callable) -> Callable:
    """Convenience decorator to require superuser role."""
    return require_role("superuser")(func)


def require_admin(func: Callable) -> Callable:
    """Convenience decorator to require admin role."""
    return require_role("admin")(func)


def always_visible_to_anonymous(func: Callable) -> Callable:
    """Decorator to mark a tool as always visible, even to anonymous users.
    
    This is used for tools like login and get-user-info that should be
    accessible without authentication.
    
    Example:
        @mcp.tool("login")
        @always_visible_to_anonymous
        async def login():
            ...
    """
    # Store permission metadata on the function for middleware to read
    func.__requires_auth__ = False  # type: ignore[attr-defined]
    func.__required_roles__ = None  # type: ignore[attr-defined]
    
    @wraps(func)
    async def wrapper(*args: Any, **kwargs: Any) -> Any:
        # No permission check needed - always visible
        return await func(*args, **kwargs)
    
    # Copy metadata to wrapper
    wrapper.__requires_auth__ = False  # type: ignore[attr-defined]
    wrapper.__required_roles__ = None  # type: ignore[attr-defined]
    
    return wrapper


def get_current_user() -> dict[str, Any] | None:
    """Get current user information from access token.
    
    Returns:
        Dict with user information, or None if not authenticated.
    """
    access_token = get_access_token()
    if not access_token:
        return None
    
    claims = access_token.claims or {}
    return {
        "username": claims.get("username"),
        "id": claims.get("sub"),
        "name": claims.get("name"),
        "email": claims.get("email"),
        "is_superuser": claims.get("is_superuser", False),
        "is_active": claims.get("is_active", True),
        "jumpserver_user_data": claims.get("jumpserver_user_data", {}),
    }

