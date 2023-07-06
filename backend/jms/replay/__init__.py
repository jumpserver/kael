import os
from pathlib import Path

from jms.base import BaseWisp
from wisp.protobuf import service_pb2
from wisp.protobuf.common_pb2 import Session

from .asciinema import AsciinemaWriter


class ReplayHandler(BaseWisp):
    REPLAY_DIR = "data/replay"
    DEFAULT_ENCODING = "utf-8"

    def __init__(self, session: Session):
        super().__init__()
        self.session = session
        self.replay_writer = None
        self.file_writer = None
        self.file = None
        self.build_file()

    def build_file(self):
        self.ensure_replay_dir()

        replay_file_path = os.path.join(self.REPLAY_DIR, f"{self.session.id}.cast")
        file = Path(replay_file_path)

        try:
            if file.exists():
                file.unlink()

            file.touch()
            self.file = file
            self.file_writer = file.open(mode="w", encoding=self.DEFAULT_ENCODING)
            self.replay_writer = AsciinemaWriter(self.file_writer)
            self.replay_writer.write_header()
        except Exception as e:
            print(file.name, f"create replay file error: {str(e)}")

    def ensure_replay_dir(self):
        dir_path = os.path.join(os.getcwd(), self.REPLAY_DIR)
        os.makedirs(dir_path, exist_ok=True)

    def write_row(self, row):
        row = row.replace("\n", "\r\n")
        row = row.replace("\r\r\n", "\r\n")
        row = f"{row} \r\n"

        try:
            self.replay_writer.write_row(row.encode(self.DEFAULT_ENCODING))
        except Exception as e:
            print(f"write replay row failed: {str(e)}")

    def write_input(self, input_str):
        input_str = f"[Input]: \r\n {input_str}"
        self.write_row(input_str)

    def write_output(self, output_str):
        output_str = f"[Output]: \r\n {output_str} \r\n"
        self.write_row(output_str)

    def upload(self):
        try:
            self.file_writer.close()

            replay_request = service_pb2.ReplayRequest(
                session_id=self.session.id,
                replay_file_path=self.file.absolute().as_posix()
            )
            resp = self.stub.UploadReplayFile(replay_request)

            if not resp.status.ok:
                print('录像文件上传失败', self.file.name, resp.status.err)
        except Exception as e:
            print('录像文件上传失败-未知错误', self.file.name, f"close replay file error: {str(e)}")