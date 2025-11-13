import logging
from typing import Optional
from datetime import datetime

from open_webui.env import SRC_LOG_LEVELS
from open_webui.jms.wisp.protobuf import service_pb2
from open_webui.jms.wisp.exceptions import WispError
from open_webui.jms.base import BaseWisp
from open_webui.jms.schemas import CommandRecord

logger = logging.getLogger(__name__)
logger.setLevel(SRC_LOG_LEVELS["WISP"])


class CommandHandler(BaseWisp):

    def __init__(self, session_id: str, session_info: dict):
        super().__init__()
        self.session_id = session_id
        self.session_info = session_info
        self.command_record: Optional[CommandRecord] = None

    async def record_command(self):
        req = service_pb2.CommandRequest(
            sid=self.session_id,
            org_id=self.session_info['org_id'],
            asset=self.session_info['asset'],
            account=self.session_info['account'],
            user=self.session_info['user'],
            timestamp=int(datetime.timestamp(datetime.now())),
            input=self.command_record.input,
            output=self.command_record.output,
            risk_level=self.command_record.risk_level,
            cmd_acl_id='',
            cmd_group_id='',
        )
        resp = self.stub.UploadCommand(req)
        if not resp.status.ok:
            error_message = f'Failed to upload command: {resp.status.err}'
            logger.error(error_message)
            raise WispError(error_message)
