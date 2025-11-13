import os
import textwrap
import logging
from pathlib import Path
from datetime import datetime

from open_webui.jms.wisp import PROJECT_DIR
from open_webui.jms.wisp.protobuf import service_pb2
from open_webui.jms.wisp.exceptions import WispError
from open_webui.env import SRC_LOG_LEVELS
from .asciinema import AsciinemaWriter
from ..base import BaseWisp

logger = logging.getLogger(__name__)
logger.setLevel(SRC_LOG_LEVELS["WISP"])


class ReplayHandler(BaseWisp):
    DEFAULT_ENCODING = "utf-8"
    REPLAY_DIR = os.path.join(PROJECT_DIR, 'data/replay')

    def __init__(self, session_id: str):
        super().__init__()
        self.session_id = session_id
        self.replay_writer: AsciinemaWriter | None = None
        self.file_writer = None
        self.file: Path | None = None

    async def _prepare(self):
        self.ensure_replay_dir()
        path = self._replay_path()

        if self.file is None:
            self.file = path

        if not path.exists():
            try:
                path.touch()
            except Exception as e:
                logger.error(f"Failed to create replay file: {path.name} -> {e}")
                raise

            try:
                self.file_writer = path.open(mode="w", encoding=self.DEFAULT_ENCODING, buffering=1)
                self.replay_writer = AsciinemaWriter(self.file_writer)
                self.replay_writer.write_header()
            except Exception as e:
                logger.error(f"Failed to init writer for new file {path.name}: {e}")
                self.file_writer = None
                self.replay_writer = None
                raise
            return

        if self.file_writer and not self.file_writer.closed and self.replay_writer is not None:
            return

        try:
            self.file_writer = path.open(mode="a", encoding=self.DEFAULT_ENCODING, buffering=1)
            self.replay_writer = AsciinemaWriter(self.file_writer)
        except Exception as e:
            logger.error(f"Failed to reopen writer for {path.name}: {e}")
            self.file_writer = None
            self.replay_writer = None

    def ensure_replay_dir(self):
        os.makedirs(self.REPLAY_DIR, exist_ok=True)

    def _replay_path(self) -> Path:
        return Path(os.path.join(self.REPLAY_DIR, f"{self.session_id}.cast"))

    def _write_row(self, row):
        row = row.replace("\n", "\r\n")
        row = row.replace("\r\r\n", "\r\n")
        row = f"{row} \r\n"

        try:
            self.replay_writer.write_row(row.encode(self.DEFAULT_ENCODING))
        except Exception as e:
            logger.error(f"Failed to write replay row: {e}")

    async def write_input(self, input_str):
        await self._prepare()
        current_time = datetime.now()
        formatted_time = current_time.strftime("%Y-%m-%d %H:%M:%S")
        input_str = f"[{formatted_time}]#: {input_str}"
        self._write_row(input_str)

    async def write_output(self, output_str):
        await self._prepare()
        wrapper = textwrap.TextWrapper(width=self.replay_writer.WIDTH)
        output_str = wrapper.fill(output_str)
        output_str = f"\r\n {output_str} \r\n"
        self._write_row(output_str)

    async def upload(self):
        await self._prepare()
        try:
            if self.file_writer and not self.file_writer.closed:
                try:
                    self.file_writer.flush()
                except Exception:
                    pass
                self.file_writer.close()
        except Exception as e:
            logger.warning(f"Failed to flush/close before upload: {e}")

        try:
            replay_request = service_pb2.ReplayRequest(
                session_id=self.session_id,
                replay_file_path=self.file.absolute().as_posix()
            )
            resp = self.stub.UploadReplayFile(replay_request)

            if not resp.status.ok:
                error_message = f'Failed to upload replay file: {self.file.name} {resp.status.err}'
                logger.error(error_message)
                raise WispError(error_message)
        except Exception as e:
            logger.error(f'Failed to upload replay file upload {e}')
        finally:
            self.replay_writer = None
            self.file_writer = None
