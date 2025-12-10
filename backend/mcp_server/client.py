from __future__ import annotations
import httpx
from fastmcp.server.dependencies import get_http_headers
import os

CORE_HOST = os.environ.get("CORE_HOST", "http://localhost:8000")


async def _attach_request_headers(request: httpx.Request) -> None:
    """Propagate incoming request headers to outbound core requests."""
    headers = get_http_headers()

    token = headers.get('authorization', '').replace('Bearer', '').strip()
    if token.startswith('jms'):
        headers.pop('authorization')

    cookies = {}
    for cookie in headers.get('cookie', '').split(';'):
        if '=' not in cookie:
            continue
        key, value = cookie.strip().split('=', 1)
        cookies[key] = value

    cookie_prefix = cookies.get('SESSION_COOKIE_NAME_PREFIX') or 'jms_'
    csrf_token = cookies.get(f'{cookie_prefix}csrftoken')
    if csrf_token:
        headers['X-CSRFToken'] = csrf_token
    
    request.headers.update(headers)


client = httpx.AsyncClient(
    base_url=CORE_HOST,
    event_hooks={"request": [_attach_request_headers]},
)
