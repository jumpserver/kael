from pydantic import BaseModel
from typing import Optional

from open_webui.jms.wisp.protobuf.common_pb2 import RiskLevel


class CommandRecord(BaseModel):
    input: Optional[str] = None
    output: Optional[str] = None
    risk_level: str = RiskLevel.Normal
