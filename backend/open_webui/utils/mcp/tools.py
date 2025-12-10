from fastapi import Request

import logging

from open_webui.env import SRC_LOG_LEVELS
from open_webui.models.users import UserModel
from open_webui.utils.mcp.client import MCPClient

log = logging.getLogger(__name__)
log.setLevel(SRC_LOG_LEVELS["MAIN"])


async def get_mcp_tools_by_server_conn(mcp_server_connection: dict, request: Request, user: UserModel, extra_params: dict) -> list[str]:
    """
    Fetch MCP tool names for a server connection.

    We only need the list of tool ids for the tools listing endpoint, so we
    connect, fetch specs, and immediately disconnect to avoid leaving the
    streamable-http client running in the background (which can trigger
    anyio cancel-scope errors later).
    """
    server_id = mcp_server_connection.get("info", {}).get("id")
    auth_type = mcp_server_connection.get("auth_type", "")

    headers = {}
    cookie = '; '.join([f'{key}={value}' for key, value in request.cookies.items()])
    headers.update({'cookie': cookie})

    if auth_type == "bearer":
        headers["Authorization"] = (
            f"Bearer {mcp_server_connection.get('key', '')}"
        )
    elif auth_type == "none":
        # No authentication
        pass
    elif auth_type == "session":
        headers["Authorization"] = (
           f"Bearer jms-{request.cookies.get('jms_sessionid', '')}"
        )
    elif auth_type == "system_oauth":
        oauth_token = extra_params.get("__oauth_token__", None)
        if oauth_token:
            headers["Authorization"] = (
                f"Bearer {oauth_token.get('access_token', '')}"
            )
    elif auth_type == "oauth_2.1":
        try:
            splits = server_id.split(":")
            server_id = splits[-1] if len(splits) > 1 else server_id

            oauth_token = await request.app.state.oauth_client_manager.get_oauth_token(
                user.id, f"mcp:{server_id}"
            )

            if oauth_token:
                headers["Authorization"] = (
                    f"Bearer {oauth_token.get('access_token', '')}"
                )
        except Exception as e:
            log.error(f"Error getting OAuth token: {e}")
            oauth_token = None

    mcp_client = MCPClient()
    try:
        await mcp_client.connect(
            url=mcp_server_connection.get("url", ""),
            headers=headers if headers else None,
            timeout=2,
        )
        tool_specs = await mcp_client.list_tool_specs()

        return [
            f"{server_id}_{tool_spec['name']}"
            for tool_spec in tool_specs
        ]
    finally:
        # Ensure the streamable-http transport is torn down promptly.
        await mcp_client.disconnect()



# def try_again(mcp_server_connection: str, request: Request, user: UserModel, extra_params: dict) -> list[dict]:
#     mcp_client = MCPClient()
#     await mcp_client.connect(
#         url=mcp_server_connection.get("url", ""),
#         headers=headers if headers else None,
#     )
#     print(f"mcp_client: {mcp_client}")

#     tool_specs = await mcp_client.list_tool_specs()

#     for tool_spec in tool_specs:
#         def make_tool_function(client, function_name):
#             async def tool_function(**kwargs):
#                 return await client.call_tool(
#                     function_name,
#                     function_args=kwargs,
#                 )

#             return tool_function

#         tool_function = make_tool_function(
#             mcp_client, tool_spec["name"]
#         )


#         mcp_tools_dict[f"{server_id}_{tool_spec['name']}"] = {
#             "spec": {
#                 **tool_spec,
#                 "name": f"{server_id}_{tool_spec['name']}",
#             },
#             "callable": tool_function,
#             "type": "mcp",
#             "client": mcp_client,
#             "direct": False,
#         }
