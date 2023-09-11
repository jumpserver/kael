# -*- coding: utf-8 -*-
# Generated by the protocol buffer compiler.  DO NOT EDIT!
# source: common.proto
"""Generated protocol buffer code."""
from google.protobuf import descriptor as _descriptor
from google.protobuf import descriptor_pool as _descriptor_pool
from google.protobuf import symbol_database as _symbol_database
from google.protobuf.internal import builder as _builder
# @@protoc_insertion_point(imports)

_sym_db = _symbol_database.Default()




DESCRIPTOR = _descriptor_pool.Default().AddSerializedFile(b'\n\x0c\x63ommon.proto\x12\x07message\"e\n\x04User\x12\n\n\x02id\x18\x01 \x01(\t\x12\x0c\n\x04name\x18\x02 \x01(\t\x12\x10\n\x08username\x18\x03 \x01(\t\x12\x0c\n\x04role\x18\x04 \x01(\t\x12\x10\n\x08is_valid\x18\x05 \x01(\x08\x12\x11\n\tis_active\x18\x06 \x01(\x08\"n\n\x07\x41\x63\x63ount\x12\n\n\x02id\x18\x01 \x01(\t\x12\x0c\n\x04name\x18\x02 \x01(\t\x12\x10\n\x08username\x18\x04 \x01(\t\x12\x0e\n\x06secret\x18\x05 \x01(\t\x12\'\n\nsecretType\x18\x06 \x01(\x0b\x32\x13.message.LabelValue\"*\n\nLabelValue\x12\r\n\x05label\x18\x01 \x01(\t\x12\r\n\x05value\x18\x02 \x01(\t\"\xb0\x03\n\x05\x41sset\x12\n\n\x02id\x18\x01 \x01(\t\x12\x0c\n\x04name\x18\x02 \x01(\t\x12\x0f\n\x07\x61\x64\x64ress\x18\x03 \x01(\t\x12\x0e\n\x06org_id\x18\x04 \x01(\t\x12\x10\n\x08org_name\x18\x05 \x01(\t\x12$\n\tprotocols\x18\x06 \x03(\x0b\x32\x11.message.Protocol\x12)\n\x08specific\x18\x07 \x01(\x0b\x32\x17.message.Asset.Specific\x1a\x88\x02\n\x08Specific\x12\x0f\n\x07\x64\x62_name\x18\x01 \x01(\t\x12\x0f\n\x07use_ssl\x18\x02 \x01(\x08\x12\x0f\n\x07\x63\x61_cert\x18\x03 \x01(\t\x12\x13\n\x0b\x63lient_cert\x18\x04 \x01(\t\x12\x12\n\nclient_key\x18\x05 \x01(\t\x12\x1a\n\x12\x61llow_invalid_cert\x18\x06 \x01(\x08\x12\x11\n\tauto_fill\x18\x07 \x01(\t\x12\x19\n\x11username_selector\x18\x08 \x01(\t\x12\x19\n\x11password_selector\x18\t \x01(\t\x12\x17\n\x0fsubmit_selector\x18\n \x01(\t\x12\x0e\n\x06script\x18\x0b \x01(\t\x12\x12\n\nhttp_proxy\x18\x0c \x01(\t\"2\n\x08Protocol\x12\x0c\n\x04name\x18\x02 \x01(\t\x12\n\n\x02id\x18\x01 \x01(\x05\x12\x0c\n\x04port\x18\x03 \x01(\x05\"\x88\x01\n\x07Gateway\x12\n\n\x02id\x18\x01 \x01(\t\x12\x0c\n\x04name\x18\x02 \x01(\t\x12\n\n\x02ip\x18\x03 \x01(\t\x12\x0c\n\x04port\x18\x04 \x01(\x05\x12\x10\n\x08protocol\x18\x05 \x01(\t\x12\x10\n\x08username\x18\x06 \x01(\t\x12\x10\n\x08password\x18\x07 \x01(\t\x12\x13\n\x0bprivate_key\x18\x08 \x01(\t\"\x7f\n\nPermission\x12\x16\n\x0e\x65nable_connect\x18\x01 \x01(\x08\x12\x17\n\x0f\x65nable_download\x18\x02 \x01(\x08\x12\x15\n\renable_upload\x18\x03 \x01(\x08\x12\x13\n\x0b\x65nable_copy\x18\x04 \x01(\x08\x12\x14\n\x0c\x65nable_paste\x18\x05 \x01(\x08\"\xee\x01\n\nCommandACL\x12\n\n\x02id\x18\x01 \x01(\t\x12\x0c\n\x04name\x18\x02 \x01(\t\x12\x10\n\x08priority\x18\x03 \x01(\x05\x12*\n\x06\x61\x63tion\x18\x05 \x01(\x0e\x32\x1a.message.CommandACL.Action\x12\x11\n\tis_active\x18\x06 \x01(\x08\x12-\n\x0e\x63ommand_groups\x18\x07 \x03(\x0b\x32\x15.message.CommandGroup\"F\n\x06\x41\x63tion\x12\n\n\x06Reject\x10\x00\x12\n\n\x06\x41\x63\x63\x65pt\x10\x01\x12\n\n\x06Review\x10\x02\x12\x0b\n\x07Warning\x10\x03\x12\x0b\n\x07Unknown\x10\x04\"m\n\x0c\x43ommandGroup\x12\n\n\x02id\x18\x01 \x01(\t\x12\x0c\n\x04name\x18\x02 \x01(\t\x12\x0f\n\x07\x63ontent\x18\x03 \x01(\t\x12\x0c\n\x04Type\x18\x04 \x01(\t\x12\x0f\n\x07pattern\x18\x05 \x01(\t\x12\x13\n\x0bignore_case\x18\x06 \x01(\x08\"\x1f\n\nExpireInfo\x12\x11\n\texpire_at\x18\x01 \x01(\x03\"\xa2\x02\n\x07Session\x12\n\n\x02id\x18\x01 \x01(\t\x12\x0c\n\x04user\x18\x02 \x01(\t\x12\r\n\x05\x61sset\x18\x03 \x01(\t\x12\x0f\n\x07\x61\x63\x63ount\x18\x04 \x01(\t\x12.\n\nlogin_from\x18\x05 \x01(\x0e\x32\x1a.message.Session.LoginFrom\x12\x13\n\x0bremote_addr\x18\x06 \x01(\t\x12\x10\n\x08protocol\x18\x07 \x01(\t\x12\x12\n\ndate_start\x18\x08 \x01(\x03\x12\x0e\n\x06org_id\x18\t \x01(\t\x12\x0f\n\x07user_id\x18\n \x01(\t\x12\x10\n\x08\x61sset_id\x18\x0b \x01(\t\x12\x12\n\naccount_id\x18\x0c \x01(\t\"+\n\tLoginFrom\x12\x06\n\x02WT\x10\x00\x12\x06\n\x02ST\x10\x01\x12\x06\n\x02RT\x10\x02\x12\x06\n\x02\x44T\x10\x03\"j\n\x0cTerminalTask\x12\n\n\x02id\x18\x01 \x01(\t\x12#\n\x06\x61\x63tion\x18\x02 \x01(\x0e\x32\x13.message.TaskAction\x12\x12\n\nsession_id\x18\x03 \x01(\t\x12\x15\n\rterminated_by\x18\x04 \x01(\t\"\x85\x03\n\rTokenAuthInfo\x12\x0e\n\x06key_id\x18\x01 \x01(\t\x12\x12\n\nsecrete_id\x18\x02 \x01(\t\x12\x1d\n\x05\x61sset\x18\x03 \x01(\x0b\x32\x0e.message.Asset\x12\x1b\n\x04user\x18\x04 \x01(\x0b\x32\r.message.User\x12!\n\x07\x61\x63\x63ount\x18\x05 \x01(\x0b\x32\x10.message.Account\x12\'\n\npermission\x18\x06 \x01(\x0b\x32\x13.message.Permission\x12(\n\x0b\x65xpire_info\x18\x07 \x01(\x0b\x32\x13.message.ExpireInfo\x12)\n\x0c\x66ilter_rules\x18\x08 \x03(\x0b\x32\x13.message.CommandACL\x12\"\n\x08gateways\x18\t \x03(\x0b\x32\x10.message.Gateway\x12*\n\x07setting\x18\n \x01(\x0b\x32\x19.message.ComponentSetting\x12#\n\x08platform\x18\x0b \x01(\x0b\x32\x11.message.Platform\"\x83\x01\n\x08Platform\x12\n\n\x02id\x18\x01 \x01(\x05\x12\x0c\n\x04name\x18\x02 \x01(\t\x12\x10\n\x08\x63\x61tegory\x18\x03 \x01(\t\x12\x0f\n\x07\x63harset\x18\x04 \x01(\t\x12\x0c\n\x04type\x18\x05 \x01(\t\x12,\n\tprotocols\x18\x06 \x03(\x0b\x32\x19.message.PlatformProtocol\"\xa6\x01\n\x10PlatformProtocol\x12\n\n\x02id\x18\x01 \x01(\x05\x12\x0c\n\x04name\x18\x02 \x01(\t\x12\x0c\n\x04port\x18\x03 \x01(\x05\x12\x39\n\x08settings\x18\x04 \x03(\x0b\x32\'.message.PlatformProtocol.SettingsEntry\x1a/\n\rSettingsEntry\x12\x0b\n\x03key\x18\x01 \x01(\t\x12\r\n\x05value\x18\x02 \x01(\t:\x02\x38\x01\")\n\x10\x43omponentSetting\x12\x15\n\rmax_idle_time\x18\x01 \x01(\x05\"1\n\x07\x46orward\x12\n\n\x02id\x18\x01 \x01(\t\x12\x0c\n\x04Host\x18\x02 \x01(\t\x12\x0c\n\x04port\x18\x03 \x01(\x05\"=\n\rPublicSetting\x12\x15\n\rxpack_enabled\x18\x01 \x01(\x08\x12\x15\n\rvalid_license\x18\x02 \x01(\x08\"%\n\x06\x43ookie\x12\x0c\n\x04name\x18\x01 \x01(\t\x12\r\n\x05value\x18\x02 \x01(\t*\x1d\n\nTaskAction\x12\x0f\n\x0bKillSession\x10\x00*f\n\tRiskLevel\x12\n\n\x06Normal\x10\x00\x12\x0b\n\x07Warning\x10\x01\x12\n\n\x06Reject\x10\x02\x12\x10\n\x0cReviewReject\x10\x03\x12\x10\n\x0cReviewAccept\x10\x04\x12\x10\n\x0cReviewCancel\x10\x05\x42\x0bZ\t/protobufb\x06proto3')

_globals = globals()
_builder.BuildMessageAndEnumDescriptors(DESCRIPTOR, _globals)
_builder.BuildTopDescriptorsAndMessages(DESCRIPTOR, 'common_pb2', _globals)
if _descriptor._USE_C_DESCRIPTORS == False:

  DESCRIPTOR._options = None
  DESCRIPTOR._serialized_options = b'Z\t/protobuf'
  _PLATFORMPROTOCOL_SETTINGSENTRY._options = None
  _PLATFORMPROTOCOL_SETTINGSENTRY._serialized_options = b'8\001'
  _globals['_TASKACTION']._serialized_start=2716
  _globals['_TASKACTION']._serialized_end=2745
  _globals['_RISKLEVEL']._serialized_start=2747
  _globals['_RISKLEVEL']._serialized_end=2849
  _globals['_USER']._serialized_start=25
  _globals['_USER']._serialized_end=126
  _globals['_ACCOUNT']._serialized_start=128
  _globals['_ACCOUNT']._serialized_end=238
  _globals['_LABELVALUE']._serialized_start=240
  _globals['_LABELVALUE']._serialized_end=282
  _globals['_ASSET']._serialized_start=285
  _globals['_ASSET']._serialized_end=717
  _globals['_ASSET_SPECIFIC']._serialized_start=453
  _globals['_ASSET_SPECIFIC']._serialized_end=717
  _globals['_PROTOCOL']._serialized_start=719
  _globals['_PROTOCOL']._serialized_end=769
  _globals['_GATEWAY']._serialized_start=772
  _globals['_GATEWAY']._serialized_end=908
  _globals['_PERMISSION']._serialized_start=910
  _globals['_PERMISSION']._serialized_end=1037
  _globals['_COMMANDACL']._serialized_start=1040
  _globals['_COMMANDACL']._serialized_end=1278
  _globals['_COMMANDACL_ACTION']._serialized_start=1208
  _globals['_COMMANDACL_ACTION']._serialized_end=1278
  _globals['_COMMANDGROUP']._serialized_start=1280
  _globals['_COMMANDGROUP']._serialized_end=1389
  _globals['_EXPIREINFO']._serialized_start=1391
  _globals['_EXPIREINFO']._serialized_end=1422
  _globals['_SESSION']._serialized_start=1425
  _globals['_SESSION']._serialized_end=1715
  _globals['_SESSION_LOGINFROM']._serialized_start=1672
  _globals['_SESSION_LOGINFROM']._serialized_end=1715
  _globals['_TERMINALTASK']._serialized_start=1717
  _globals['_TERMINALTASK']._serialized_end=1823
  _globals['_TOKENAUTHINFO']._serialized_start=1826
  _globals['_TOKENAUTHINFO']._serialized_end=2215
  _globals['_PLATFORM']._serialized_start=2218
  _globals['_PLATFORM']._serialized_end=2349
  _globals['_PLATFORMPROTOCOL']._serialized_start=2352
  _globals['_PLATFORMPROTOCOL']._serialized_end=2518
  _globals['_PLATFORMPROTOCOL_SETTINGSENTRY']._serialized_start=2471
  _globals['_PLATFORMPROTOCOL_SETTINGSENTRY']._serialized_end=2518
  _globals['_COMPONENTSETTING']._serialized_start=2520
  _globals['_COMPONENTSETTING']._serialized_end=2561
  _globals['_FORWARD']._serialized_start=2563
  _globals['_FORWARD']._serialized_end=2612
  _globals['_PUBLICSETTING']._serialized_start=2614
  _globals['_PUBLICSETTING']._serialized_end=2675
  _globals['_COOKIE']._serialized_start=2677
  _globals['_COOKIE']._serialized_end=2714
# @@protoc_insertion_point(module_scope)