# -*- coding: utf-8 -*-
# Generated by the protocol buffer compiler.  DO NOT EDIT!
# source: service.proto
"""Generated protocol buffer code."""
from google.protobuf import descriptor as _descriptor
from google.protobuf import descriptor_pool as _descriptor_pool
from google.protobuf import symbol_database as _symbol_database
from google.protobuf.internal import builder as _builder
# @@protoc_insertion_point(imports)

_sym_db = _symbol_database.Default()


import common_pb2 as common__pb2


DESCRIPTOR = _descriptor_pool.Default().AddSerializedFile(b'\n\rservice.proto\x12\x07message\x1a\x0c\x63ommon.proto\"V\n\x17\x41ssetLoginTicketRequest\x12\x0f\n\x07user_id\x18\x01 \x01(\t\x12\x10\n\x08\x61sset_id\x18\x02 \x01(\t\x12\x18\n\x10\x61\x63\x63ount_username\x18\x04 \x01(\t\"\x8e\x01\n\x18\x41ssetLoginTicketResponse\x12\x1f\n\x06status\x18\x01 \x01(\x0b\x32\x0f.message.Status\x12(\n\x0bticket_info\x18\x02 \x01(\x0b\x32\x13.message.TicketInfo\x12\x14\n\x0cneed_confirm\x18\x03 \x01(\x08\x12\x11\n\tticket_id\x18\x04 \x01(\t\"!\n\x06Status\x12\n\n\x02ok\x18\x01 \x01(\x08\x12\x0b\n\x03\x65rr\x18\x02 \x01(\t\"\x1d\n\x0cTokenRequest\x12\r\n\x05token\x18\x01 \x01(\t\"V\n\rTokenResponse\x12\x1f\n\x06status\x18\x01 \x01(\x0b\x32\x0f.message.Status\x12$\n\x04\x64\x61ta\x18\x02 \x01(\x0b\x32\x16.message.TokenAuthInfo\"6\n\x14SessionCreateRequest\x12\x1e\n\x04\x64\x61ta\x18\x01 \x01(\x0b\x32\x10.message.Session\"X\n\x15SessionCreateResponse\x12\x1f\n\x06status\x18\x01 \x01(\x0b\x32\x0f.message.Status\x12\x1e\n\x04\x64\x61ta\x18\x02 \x01(\x0b\x32\x10.message.Session\"R\n\x14SessionFinishRequest\x12\n\n\x02id\x18\x01 \x01(\t\x12\x0f\n\x07success\x18\x02 \x01(\x08\x12\x10\n\x08\x64\x61te_end\x18\x03 \x01(\x03\x12\x0b\n\x03\x65rr\x18\x04 \x01(\t\"4\n\x11SessionFinishResp\x12\x1f\n\x06status\x18\x01 \x01(\x0b\x32\x0f.message.Status\"=\n\rReplayRequest\x12\x12\n\nsession_id\x18\x01 \x01(\t\x12\x18\n\x10replay_file_path\x18\x02 \x01(\t\"1\n\x0eReplayResponse\x12\x1f\n\x06status\x18\x01 \x01(\x0b\x32\x0f.message.Status\"\xb5\x01\n\x0e\x43ommandRequest\x12\x0b\n\x03sid\x18\x01 \x01(\t\x12\x0e\n\x06org_id\x18\x02 \x01(\t\x12\r\n\x05input\x18\x03 \x01(\t\x12\x0e\n\x06output\x18\x04 \x01(\t\x12\x0c\n\x04user\x18\x05 \x01(\t\x12\r\n\x05\x61sset\x18\x06 \x01(\t\x12\x0f\n\x07\x61\x63\x63ount\x18\x07 \x01(\t\x12\x11\n\ttimestamp\x18\x08 \x01(\x03\x12&\n\nrisk_level\x18\t \x01(\x0e\x32\x12.message.RiskLevel\"2\n\x0f\x43ommandResponse\x12\x1f\n\x06status\x18\x01 \x01(\x0b\x32\x0f.message.Status\"&\n\x13\x46inishedTaskRequest\x12\x0f\n\x07task_id\x18\x01 \x01(\t\"3\n\x0cTaskResponse\x12#\n\x04task\x18\x01 \x01(\x0b\x32\x15.message.TerminalTask\")\n\x13RemainReplayRequest\x12\x12\n\nreplay_dir\x18\x01 \x01(\t\"{\n\x14RemainReplayResponse\x12\x1f\n\x06status\x18\x01 \x01(\x0b\x32\x0f.message.Status\x12\x15\n\rsuccess_files\x18\x02 \x03(\t\x12\x15\n\rfailure_files\x18\x03 \x03(\t\x12\x14\n\x0c\x66\x61ilure_errs\x18\x04 \x03(\t\"1\n\x0eStatusResponse\x12\x1f\n\x06status\x18\x01 \x01(\x0b\x32\x0f.message.Status\"L\n\x15\x43ommandConfirmRequest\x12\x12\n\nsession_id\x18\x01 \x01(\t\x12\x12\n\ncmd_acl_id\x18\x02 \x01(\t\x12\x0b\n\x03\x63md\x18\x03 \x01(\t\"&\n\x07ReqInfo\x12\x0e\n\x06method\x18\x01 \x01(\t\x12\x0b\n\x03url\x18\x02 \x01(\t\"\\\n\x16\x43ommandConfirmResponse\x12\x1f\n\x06status\x18\x01 \x01(\x0b\x32\x0f.message.Status\x12!\n\x04info\x18\x02 \x01(\x0b\x32\x13.message.TicketInfo\"\x85\x01\n\nTicketInfo\x12#\n\tcheck_req\x18\x01 \x01(\x0b\x32\x10.message.ReqInfo\x12$\n\ncancel_req\x18\x02 \x01(\x0b\x32\x10.message.ReqInfo\x12\x19\n\x11ticket_detail_url\x18\x03 \x01(\t\x12\x11\n\treviewers\x18\x04 \x03(\t\".\n\rTicketRequest\x12\x1d\n\x03req\x18\x01 \x01(\x0b\x32\x10.message.ReqInfo\"Z\n\x13TicketStateResponse\x12\"\n\x04\x44\x61ta\x18\x01 \x01(\x0b\x32\x14.message.TicketState\x12\x1f\n\x06status\x18\x02 \x01(\x0b\x32\x0f.message.Status\"\x86\x01\n\x0bTicketState\x12)\n\x05state\x18\x01 \x01(\x0e\x32\x1a.message.TicketState.State\x12\x11\n\tprocessor\x18\x02 \x01(\t\"9\n\x05State\x12\x08\n\x04Open\x10\x00\x12\x0c\n\x08\x41pproved\x10\x01\x12\x0c\n\x08Rejected\x10\x02\x12\n\n\x06\x43losed\x10\x03\"P\n\x0e\x46orwardRequest\x12\x0c\n\x04host\x18\x01 \x01(\t\x12\x0c\n\x04port\x18\x02 \x01(\x05\x12\"\n\x08gateways\x18\x03 \x03(\x0b\x32\x10.message.Gateway\"\"\n\x14\x46orwardDeleteRequest\x12\n\n\x02id\x18\x01 \x01(\t\"Z\n\x0f\x46orwardResponse\x12\x1f\n\x06status\x18\x01 \x01(\x0b\x32\x0f.message.Status\x12\n\n\x02id\x18\x02 \x01(\t\x12\x0c\n\x04host\x18\x03 \x01(\t\x12\x0c\n\x04port\x18\x04 \x01(\x05\"^\n\x15PublicSettingResponse\x12\x1f\n\x06status\x18\x01 \x01(\x0b\x32\x0f.message.Status\x12$\n\x04\x64\x61ta\x18\x02 \x01(\x0b\x32\x16.message.PublicSetting\"\x07\n\x05\x45mpty\"D\n\x12ListenPortResponse\x12\x1f\n\x06status\x18\x01 \x01(\x0b\x32\x0f.message.Status\x12\r\n\x05ports\x18\x02 \x03(\x05\"\x1f\n\x0fPortInfoRequest\x12\x0c\n\x04port\x18\x01 \x01(\x05\"T\n\x10PortInfoResponse\x12\x1f\n\x06status\x18\x01 \x01(\x0b\x32\x0f.message.Status\x12\x1f\n\x04\x64\x61ta\x18\x02 \x01(\x0b\x32\x11.message.PortInfo\"M\n\x08PortInfo\x12\x1d\n\x05\x61sset\x18\x01 \x01(\x0b\x32\x0e.message.Asset\x12\"\n\x08gateways\x18\x02 \x03(\x0b\x32\x10.message.Gateway\"+\n\x0bPortFailure\x12\x0c\n\x04port\x18\x01 \x01(\x05\x12\x0e\n\x06reason\x18\x02 \x01(\t\"8\n\x12PortFailureRequest\x12\"\n\x04\x64\x61ta\x18\x01 \x03(\x0b\x32\x14.message.PortFailure2\xd6\n\n\x07Service\x12\x43\n\x10GetTokenAuthInfo\x12\x15.message.TokenRequest\x1a\x16.message.TokenResponse\"\x00\x12>\n\nRenewToken\x12\x15.message.TokenRequest\x1a\x17.message.StatusResponse\"\x00\x12P\n\rCreateSession\x12\x1d.message.SessionCreateRequest\x1a\x1e.message.SessionCreateResponse\"\x00\x12L\n\rFinishSession\x12\x1d.message.SessionFinishRequest\x1a\x1a.message.SessionFinishResp\"\x00\x12\x45\n\x10UploadReplayFile\x12\x16.message.ReplayRequest\x1a\x17.message.ReplayResponse\"\x00\x12\x44\n\rUploadCommand\x12\x17.message.CommandRequest\x1a\x18.message.CommandResponse\"\x00\x12I\n\x0c\x44ispatchTask\x12\x1c.message.FinishedTaskRequest\x1a\x15.message.TaskResponse\"\x00(\x01\x30\x01\x12R\n\x11ScanRemainReplays\x12\x1c.message.RemainReplayRequest\x1a\x1d.message.RemainReplayResponse\"\x00\x12X\n\x13\x43reateCommandTicket\x12\x1e.message.CommandConfirmRequest\x1a\x1f.message.CommandConfirmResponse\"\x00\x12\x66\n\x1d\x43heckOrCreateAssetLoginTicket\x12 .message.AssetLoginTicketRequest\x1a!.message.AssetLoginTicketResponse\"\x00\x12J\n\x10\x43heckTicketState\x12\x16.message.TicketRequest\x1a\x1c.message.TicketStateResponse\"\x00\x12\x41\n\x0c\x43\x61ncelTicket\x12\x16.message.TicketRequest\x1a\x17.message.StatusResponse\"\x00\x12\x44\n\rCreateForward\x12\x17.message.ForwardRequest\x1a\x18.message.ForwardResponse\"\x00\x12I\n\rDeleteForward\x12\x1d.message.ForwardDeleteRequest\x1a\x17.message.StatusResponse\"\x00\x12\x44\n\x10GetPublicSetting\x12\x0e.message.Empty\x1a\x1e.message.PublicSettingResponse\"\x00\x12?\n\x0eGetListenPorts\x12\x0e.message.Empty\x1a\x1b.message.ListenPortResponse\"\x00\x12\x44\n\x0bGetPortInfo\x12\x18.message.PortInfoRequest\x1a\x19.message.PortInfoResponse\"\x00\x12K\n\x11HandlePortFailure\x12\x1b.message.PortFailureRequest\x1a\x17.message.StatusResponse\"\x00\x42\x0bZ\t/protobufb\x06proto3')

_globals = globals()
_builder.BuildMessageAndEnumDescriptors(DESCRIPTOR, _globals)
_builder.BuildTopDescriptorsAndMessages(DESCRIPTOR, 'service_pb2', _globals)
if _descriptor._USE_C_DESCRIPTORS == False:

  DESCRIPTOR._options = None
  DESCRIPTOR._serialized_options = b'Z\t/protobuf'
  _globals['_ASSETLOGINTICKETREQUEST']._serialized_start=40
  _globals['_ASSETLOGINTICKETREQUEST']._serialized_end=126
  _globals['_ASSETLOGINTICKETRESPONSE']._serialized_start=129
  _globals['_ASSETLOGINTICKETRESPONSE']._serialized_end=271
  _globals['_STATUS']._serialized_start=273
  _globals['_STATUS']._serialized_end=306
  _globals['_TOKENREQUEST']._serialized_start=308
  _globals['_TOKENREQUEST']._serialized_end=337
  _globals['_TOKENRESPONSE']._serialized_start=339
  _globals['_TOKENRESPONSE']._serialized_end=425
  _globals['_SESSIONCREATEREQUEST']._serialized_start=427
  _globals['_SESSIONCREATEREQUEST']._serialized_end=481
  _globals['_SESSIONCREATERESPONSE']._serialized_start=483
  _globals['_SESSIONCREATERESPONSE']._serialized_end=571
  _globals['_SESSIONFINISHREQUEST']._serialized_start=573
  _globals['_SESSIONFINISHREQUEST']._serialized_end=655
  _globals['_SESSIONFINISHRESP']._serialized_start=657
  _globals['_SESSIONFINISHRESP']._serialized_end=709
  _globals['_REPLAYREQUEST']._serialized_start=711
  _globals['_REPLAYREQUEST']._serialized_end=772
  _globals['_REPLAYRESPONSE']._serialized_start=774
  _globals['_REPLAYRESPONSE']._serialized_end=823
  _globals['_COMMANDREQUEST']._serialized_start=826
  _globals['_COMMANDREQUEST']._serialized_end=1007
  _globals['_COMMANDRESPONSE']._serialized_start=1009
  _globals['_COMMANDRESPONSE']._serialized_end=1059
  _globals['_FINISHEDTASKREQUEST']._serialized_start=1061
  _globals['_FINISHEDTASKREQUEST']._serialized_end=1099
  _globals['_TASKRESPONSE']._serialized_start=1101
  _globals['_TASKRESPONSE']._serialized_end=1152
  _globals['_REMAINREPLAYREQUEST']._serialized_start=1154
  _globals['_REMAINREPLAYREQUEST']._serialized_end=1195
  _globals['_REMAINREPLAYRESPONSE']._serialized_start=1197
  _globals['_REMAINREPLAYRESPONSE']._serialized_end=1320
  _globals['_STATUSRESPONSE']._serialized_start=1322
  _globals['_STATUSRESPONSE']._serialized_end=1371
  _globals['_COMMANDCONFIRMREQUEST']._serialized_start=1373
  _globals['_COMMANDCONFIRMREQUEST']._serialized_end=1449
  _globals['_REQINFO']._serialized_start=1451
  _globals['_REQINFO']._serialized_end=1489
  _globals['_COMMANDCONFIRMRESPONSE']._serialized_start=1491
  _globals['_COMMANDCONFIRMRESPONSE']._serialized_end=1583
  _globals['_TICKETINFO']._serialized_start=1586
  _globals['_TICKETINFO']._serialized_end=1719
  _globals['_TICKETREQUEST']._serialized_start=1721
  _globals['_TICKETREQUEST']._serialized_end=1767
  _globals['_TICKETSTATERESPONSE']._serialized_start=1769
  _globals['_TICKETSTATERESPONSE']._serialized_end=1859
  _globals['_TICKETSTATE']._serialized_start=1862
  _globals['_TICKETSTATE']._serialized_end=1996
  _globals['_TICKETSTATE_STATE']._serialized_start=1939
  _globals['_TICKETSTATE_STATE']._serialized_end=1996
  _globals['_FORWARDREQUEST']._serialized_start=1998
  _globals['_FORWARDREQUEST']._serialized_end=2078
  _globals['_FORWARDDELETEREQUEST']._serialized_start=2080
  _globals['_FORWARDDELETEREQUEST']._serialized_end=2114
  _globals['_FORWARDRESPONSE']._serialized_start=2116
  _globals['_FORWARDRESPONSE']._serialized_end=2206
  _globals['_PUBLICSETTINGRESPONSE']._serialized_start=2208
  _globals['_PUBLICSETTINGRESPONSE']._serialized_end=2302
  _globals['_EMPTY']._serialized_start=2304
  _globals['_EMPTY']._serialized_end=2311
  _globals['_LISTENPORTRESPONSE']._serialized_start=2313
  _globals['_LISTENPORTRESPONSE']._serialized_end=2381
  _globals['_PORTINFOREQUEST']._serialized_start=2383
  _globals['_PORTINFOREQUEST']._serialized_end=2414
  _globals['_PORTINFORESPONSE']._serialized_start=2416
  _globals['_PORTINFORESPONSE']._serialized_end=2500
  _globals['_PORTINFO']._serialized_start=2502
  _globals['_PORTINFO']._serialized_end=2579
  _globals['_PORTFAILURE']._serialized_start=2581
  _globals['_PORTFAILURE']._serialized_end=2624
  _globals['_PORTFAILUREREQUEST']._serialized_start=2626
  _globals['_PORTFAILUREREQUEST']._serialized_end=2682
  _globals['_SERVICE']._serialized_start=2685
  _globals['_SERVICE']._serialized_end=4051
# @@protoc_insertion_point(module_scope)