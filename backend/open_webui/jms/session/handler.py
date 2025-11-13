import asyncio
import logging
from datetime import datetime

from open_webui.jms.wisp.protobuf import service_pb2
from open_webui.jms.wisp.exceptions import WispError
from open_webui.jms.wisp.protobuf.common_pb2 import Session, User
from open_webui.jms import CommandHandler, ReplayHandler
from open_webui.env import SRC_LOG_LEVELS
from ..account import AccountChatHandler
from ..base import BaseWisp

logger = logging.getLogger(__name__)
logger.setLevel(SRC_LOG_LEVELS["WISP"])


class JMSSession(BaseWisp):
    def __init__(self, chat: dict):
        super().__init__()
        self.chat_id = chat['id']

        self.command_handler = CommandHandler(self.chat_id, chat['session_info'])
        self.replay_handler = ReplayHandler(self.chat_id)

    async def close_session(self) -> None:
        req = service_pb2.SessionFinishRequest(
            id=self.chat_id,
            date_end=int(datetime.now().timestamp())
        )
        resp = self.stub.FinishSession(req)

        if not resp.status.ok:
            error_message = f'Failed to close session: {resp.status.err}'
            logger.error(error_message)
            raise WispError(error_message)

    async def close(self) -> None:
        await asyncio.sleep(1)
        await self.replay_handler.upload()
        await self.close_session()


class SessionHandler(BaseWisp):

    def __init__(self, sid: str, ip: str, user: User):
        super().__init__()
        self.sid = sid
        self.remote_address = ip
        self.user = user

    def create_session(self, chat_model: str) -> Session:
        account_handler = AccountChatHandler()
        account_data = account_handler.get_account()

        req_session = Session(
            user_id=self.user.id,
            user=f'{self.user.name}({self.user.username})',
            account_id=account_data['id'],
            account=f'{account_data["name"]}({account_data["username"]})',
            org_id=account_data['org_id'],
            asset_id=account_data['asset']['id'],
            asset=account_data['asset']['name'],
            login_from=Session.LoginFrom.WT,
            protocol=chat_model,
            date_start=int(datetime.now().timestamp()),
            remote_addr=self.remote_address,
        )
        req = service_pb2.SessionCreateRequest(data=req_session)
        resp = self.stub.CreateSession(req)
        if not resp.status.ok:
            error_message = f'Failed to create session: {resp.status.err}'
            logger.error(error_message)
            raise WispError(error_message)
        return resp.data
