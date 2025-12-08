"""JumpServer authentication provider for FastMCP.

Simple bearer token authentication using JumpServer session IDs.
"""

from __future__ import annotations

import os
import time

from fastmcp.server.dependencies import get_http_headers
import httpx
import dotenv
from pydantic import AnyHttpUrl

from fastmcp.server.auth import TokenVerifier
from fastmcp.server.auth.auth import AccessToken
from fastmcp.utilities.logging import get_logger
from starlette.responses import JSONResponse
from starlette.routing import Route
from starlette.requests import Request, HTTPConnection
from starlette.authentication import AuthCredentials, AuthenticationBackend
from mcp.server.auth.middleware.bearer_auth import AuthenticatedUser

logger = get_logger(__name__)
dotenv.load_dotenv()


class JumpServerAuthProvider(TokenVerifier):
    """Simple JumpServer session token authentication.
    
    Validates bearer tokens in format `jms-<sessionid>` by calling
    JumpServer's profile API with the session cookie.
    """

    def __init__(
        self,
        *,
        jumpserver_host: str | None = None,
        timeout_seconds: int = 10,
        base_url: AnyHttpUrl | str | None = None,
    ):
        """Initialize JumpServer authentication provider.

        Args:
            jumpserver_host: JumpServer host URL (defaults to JUMPSERVER_HOST env var)
            timeout_seconds: HTTP request timeout (default: 10)
            base_url: Base URL of this server (optional)
        """
        super().__init__(base_url=base_url)
        
        jumpserver_host_final = jumpserver_host or os.getenv("CORE_HOST") or "http://core:8080"
        logger.info(f"CORE_HOST: {jumpserver_host_final}")
        if jumpserver_host_final:
            jumpserver_host_final = jumpserver_host_final.rstrip("/")
        
        if not jumpserver_host_final:
            raise ValueError(
                "jumpserver_host is required - set via parameter or JUMPSERVER_HOST env var"
            )
        
        self.jumpserver_host = jumpserver_host_final
        self.timeout_seconds = timeout_seconds
        
        logger.info(f"Initialized JumpServer auth provider for {jumpserver_host_final}")

    async def verify_token(self, token: str | None = None) -> AccessToken | None:
        """Verify JumpServer session token or bearer token.
        
        If no token is provided, attempts to authenticate using session cookies.
        If a bearer token is provided (from login tool), uses it for authentication.
        Returns an anonymous token if authentication fails, allowing access
        to login tool for authentication.
        
        Args:
            token: Optional bearer token. If None, will try to authenticate using cookies.
        """
        
        headers = get_http_headers()
        headers['Accept'] = 'application/json'

        # If token is provided and starts with 'jms', remove authorization header
        # to use cookie-based authentication instead
        if token and token.startswith('jms'):
            headers.pop('authorization', None)
            token = None # Use cookie-based instead of bearer token

        try:
            # Request user profile with session cookie or bearer token
            async with httpx.AsyncClient(timeout=self.timeout_seconds) as client:
                response = await client.get(
                    f"{self.jumpserver_host}/api/v1/users/profile/",
                    headers=headers,
                )
                if response.status_code != 200:
                    logger.debug(f"Profile API failed: {response.status_code}")
                    # Return anonymous token to allow access to login tool
                    return self._create_anonymous_token(token, f"HTTP {response.status_code}")
                
                user_data = response.json()
                logger.info(f"Authenticated user: {user_data.get('username', 'unknown')}")
                
                return self.create_access_token(token, user_data)
        
        except Exception as e:
            logger.debug(f"Token verification error: {e}")
            # Return anonymous token to allow access to login tool
            return self._create_anonymous_token(token, str(e))

    def create_access_token(self, token: str, user_data: dict) -> AccessToken:
        if not token:
            token = "jumpserver-cookie-auth" # If not provided, access key validate error
        return AccessToken(
            token=token,
            client_id="jumpserver-mcp",
            scopes=[],
            expires_at=None,
            claims={
                "sub": str(user_data.get("id", "unknown")),
                "username": user_data.get("username"),
                "name": user_data.get("name"),
                "email": user_data.get("email"),
                "is_active": user_data.get("is_active"),
                "is_org_admin": user_data.get("is_org_admin", False),
                "is_superuser": user_data.get("is_superuser", False),
                "jumpserver_user_data": user_data,  # Contains full user data including roles
                "is_anonymous": False,
            },
        )
    
    def _create_anonymous_token(self, token: str, error: str) -> AccessToken:
        """Create an anonymous token for failed authentication.
        
        This allows access to login tool even when authentication fails.
        """
        return AccessToken(
            token=token or "anonymous",
            client_id="jumpserver",
            scopes=["login"],  # Limited scope for login tool only
            expires_at=None,
            claims={
                "sub": "anonymous",
                "username": None,
                "is_anonymous": True,
                "auth_error": error,
            },
        )

    def get_middleware(self) -> list:
        """Get HTTP application-level middleware for this auth provider.
        
        Override to use custom AuthenticationBackend that calls verify_token
        even when no authorization header is present.
        """
        from starlette.middleware import Middleware
        from starlette.middleware.authentication import AuthenticationMiddleware
        from mcp.server.auth.middleware.auth_context import AuthContextMiddleware
        
        return [
            Middleware(
                AuthenticationMiddleware,
                backend=_JumpServerAuthBackend(self), # Always call verify_token, even if no token is provided
            ),
            Middleware(AuthContextMiddleware),
        ]

    def get_routes(self, mcp_path: str | None = None, **kwargs) -> list[Route]:
        """Handle /register requests (MCP clients may try to register)."""
        async def handle_register(request: Request):
            return JSONResponse(
                status_code=400,
                content={
                    "error": "client_registration_not_supported",
                    "error_description": (
                        "This server uses simple bearer token authentication. "
                        "No client registration needed. Use: Authorization: Bearer jms-<sessionid>"
                    ),
                },
            )
        
        return [Route("/mcp/register", handle_register, methods=["POST"])]


class _JumpServerAuthBackend(AuthenticationBackend):
    """
    Custom authentication backend that always calls verify_token,
    even when no authorization header is present.
    
    This allows login tool to be accessible even when authentication fails.
    """
    
    def __init__(self, token_verifier: JumpServerAuthProvider):
        self.token_verifier = token_verifier

    async def authenticate(self, conn: HTTPConnection):
        # Try to get token from authorization header
        auth_header = next(
            (conn.headers.get(key) for key in conn.headers if key.lower() == "authorization"),
            None,
        )
        
        token = None
        if auth_header and auth_header.lower().startswith("bearer "):
            token = auth_header[7:]  # Remove "Bearer " prefix
        
        # Always call verify_token, even if no token is provided
        # This allows cookie-based authentication and anonymous tokens for login tool
        auth_info = await self.token_verifier.verify_token(token)

        if not auth_info:
            return None

        if auth_info.expires_at and auth_info.expires_at < int(time.time()):
            return None

        from starlette.authentication import AuthCredentials
        return AuthCredentials(auth_info.scopes), AuthenticatedUser(auth_info)
