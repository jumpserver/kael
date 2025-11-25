import json
import logging
from typing import Any, Dict, List, Optional

from open_webui.jms.wisp.exceptions import WispError
from open_webui.env import SRC_LOG_LEVELS

from .base import BaseWisp
from .wisp.protobuf.service_pb2 import HTTPRequest

logger = logging.getLogger(__name__)
logger.setLevel(SRC_LOG_LEVELS["WISP"])

CHAT_URL = "/api/v1/terminal/chats/"


class ChatHandler(BaseWisp):
    def list(self, query: Optional[Dict[str, Any]] = None) -> List[dict]:
        query = self._normalize_query(query)
        resp = self._request("GET", CHAT_URL, query=query, action="list chats")
        return self._loads(CHAT_URL, resp.body)

    def retrieve(self, chat_id: str) -> dict:
        self._ensure_id(chat_id)
        path = f"{CHAT_URL}{chat_id}/"
        resp = self._request("GET", path, action=f"retrieve chat {chat_id}")
        return self._loads(path, resp.body)

    def create(self, data: Dict[str, Any]) -> dict:
        body = self._dumps(data)
        resp = self._request("POST", CHAT_URL, body=body, action="create chat")
        return self._loads(CHAT_URL, resp.body)

    def update(self, chat_id: Optional[str] = None, data: Dict[str, Any] = None, query: Dict[str, Any] = None) -> dict:
        self._ensure_id(chat_id)
        path = f"{CHAT_URL}{chat_id}/"
        body = self._dumps(data)
        resp = self._request("PATCH", path, query=query, body=body, action=f"update chat {chat_id}")
        return self._loads(path, resp.body)

    def destroy(self, chat_id: Optional[str] = None, query: Dict[str, Any] = None) -> None:
        """
        If chat_id is provided, DELETE /chats/{id}/
        Else if query is provided, bulk DELETE /chats/ with query params
        """
        if chat_id:
            path, body, action = f"{CHAT_URL}{chat_id}/", None, f"delete chat {chat_id}"
        elif query is not None:
            path, query, action = CHAT_URL, query, "bulk delete chats"
        else:
            raise ValueError("Either chat_id or data must be provided")

        self._request("DELETE", path, query=query, action=action)

    def _request(
            self,
            method: str,
            path: str,
            *,
            query: Optional[Dict[str, str]] = None,
            body: Optional[bytes] = None,
            action: str = "call API",
    ):
        """
        Centralized request with optional retries and better error surfacing.
        """
        req = HTTPRequest(
            method=method,
            path=path,
            query=query,
            body=body
        )

        resp = self.stub.CallAPI(req)
        if resp.status.ok:
            return resp

        status_code = getattr(resp.status, "code", None)

        err_body = self._safe_decode(resp.body)
        err_msg = (
            f"Failed to {action}: "
            f"status={status_code}, err={resp.status.err}, body={err_body!r}"
        )

        logger.error(err_msg)
        raise WispError(err_msg)

    @staticmethod
    def _normalize_query(query: Optional[Dict[str, Any]]) -> Dict[str, str]:
        if not query:
            return {}
        return {str(k): ('' if v is None else str(v)) for k, v in query.items()}

    @staticmethod
    def _dumps(data: Dict[str, Any]) -> bytes:
        try:
            return json.dumps(data, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
        except (TypeError, ValueError) as e:
            raise WispError(f"Failed to serialize request body: {e}") from e

    @staticmethod
    def _loads(path: str, b: bytes) -> Any:
        try:
            return json.loads(b.decode("utf-8")) if b else None
        except (UnicodeDecodeError, json.JSONDecodeError) as e:
            raise WispError(f"Failed to parse response body: {e} path {path}") from e

    @staticmethod
    def _safe_decode(b: Optional[bytes]) -> str:
        if not b:
            return ""
        try:
            text = b.decode("utf-8", errors="replace")
            return text if len(text) <= 1000 else text[:1000] + "…<truncated>"
        except Exception:
            return "<non-textual body>"

    @staticmethod
    def _ensure_id(chat_id: Optional[str]) -> None:
        if not chat_id or not str(chat_id).strip():
            raise ValueError("chat_id must be a non-empty string")


chat_manager = ChatHandler()
