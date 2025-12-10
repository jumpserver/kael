from fastmcp.server.dependencies import get_access_token

from ..app import mcp
from ..permissions import require_authentication


@mcp.tool("get-profile-info")
@require_authentication
async def get_profile_info() -> dict:
    """Returns information about the authenticated user."""

    token = get_access_token()
    return token.claims