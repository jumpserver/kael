from open_webui.jms.wisp import grpc_channel
from open_webui.jms.wisp.protobuf import service_pb2_grpc


class BaseWisp:

    def __init__(self):
        self.stub = service_pb2_grpc.ServiceStub(grpc_channel)
