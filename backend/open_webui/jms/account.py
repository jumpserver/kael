import logging
from typing import Any

from open_webui.jms.wisp.exceptions import WispError
from open_webui.env import SRC_LOG_LEVELS
from open_webui.jms.wisp.protobuf import service_pb2

from .base import BaseWisp

logger = logging.getLogger(__name__)
logger.setLevel(SRC_LOG_LEVELS["WISP"])


class AccountChatHandler(BaseWisp):

    def get_account(self) -> dict[Any, Any] | dict[str, Any] | dict[str, str] | dict[bytes, bytes]:
        resp = self.stub.GetAccountChat(service_pb2.Empty())
        if not resp.status.ok:
            error_message = f'Failed to get account info: {resp.status.err}'
            logger.error(error_message)
            raise WispError(error_message)
        return dict(resp.payload)
