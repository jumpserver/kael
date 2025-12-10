"""
Login tool for JumpServer MCP server.

Allows users to authenticate using username and password.
"""

from __future__ import annotations

import os
from typing import Any, Dict
import httpx
from fastmcp.utilities.logging import get_logger

from .app import mcp
from .permissions import always_visible_to_anonymous

logger = get_logger(__name__)


@mcp.tool("login")
@always_visible_to_anonymous
async def login(username: str, password: str) -> Dict[str, Any]:
    """
    Login to JumpServer using username and password.
    
    This tool allows unauthenticated users to obtain a bearer token by providing
    their credentials. Once logged in, this tool will no longer be visible.
    
    Args:
        username: JumpServer username
        password: JumpServer password
    
    Returns:
        Dict containing:
        - token: Bearer token to use for authentication
        - message: Success message
        - instructions: Instructions on how to use the token
    """
    # Get JumpServer host from environment
    jumpserver_host = os.getenv("CORE_HOST", "http://core:8080").rstrip("/")
    
    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            response = await client.post(
                f"{jumpserver_host}/api/v1/authentication/tokens/",
                json={
                    "username": username,
                    "password": password,
                },
            )
            
            if response.status_code == 201:
                # Success - token created
                data = response.json()
                token = data.get("token", data.get("key", ""))
                
                logger.info(f"User {username} logged in successfully")
                
                return {
                    "success": True,
                    "token": token,
                    "message": "Login successful! You can now use this token for authentication.",
                    "instructions": {
                        "summary": "Save this token and use it when connecting to this MCP server.",
                        "usage": {
                            "header_name": "Authorization",
                            "header_value": f"Bearer {token}",
                            "full_header": f"Authorization: Bearer {token}",
                        },
                        "for_mcp_clients": (
                            "If you're using an MCP client:\n"
                            "1. Save this token securely\n"
                            "2. Reconnect to the MCP server with this token in the Authorization header\n"
                            "3. The token will be used for all subsequent requests"
                        ),
                        "token_info": {
                            "length": len(token),
                            "preview": f"{token[:20]}..." if len(token) > 20 else token,
                            "note": "Keep this token secure and do not share it",
                        },
                    },
                }
            elif response.status_code == 400:
                # Bad request - likely invalid credentials
                error_data = response.json() if response.content else {}
                error_msg = error_data.get("detail", error_data.get("error", "Invalid credentials"))
                
                logger.warning(f"Login failed for user {username}: {error_msg}")
                
                return {
                    "success": False,
                    "error": "Invalid credentials",
                    "message": error_msg,
                }
            else:
                # Other error
                error_msg = f"HTTP {response.status_code}"
                try:
                    error_data = response.json()
                    error_msg = error_data.get("detail", error_data.get("error", error_msg))
                except Exception:
                    pass
                
                logger.error(f"Login failed for user {username}: {error_msg}")
                
                return {
                    "success": False,
                    "error": "Login failed",
                    "message": error_msg,
                    "status_code": response.status_code,
                }
    
    except httpx.ConnectError as e:
        logger.error(f"Connection error during login: {e}")
        return {
            "success": False,
            "error": "Connection error",
            "message": f"Cannot connect to JumpServer at {jumpserver_host}",
        }
    except httpx.TimeoutException as e:
        logger.error(f"Timeout during login: {e}")
        return {
            "success": False,
            "error": "Timeout",
            "message": "Connection to JumpServer timed out",
        }
    except Exception as e:
        logger.exception(f"Unexpected error during login: {e}")
        return {
            "success": False,
            "error": "Unexpected error",
            "message": str(e),
        }

