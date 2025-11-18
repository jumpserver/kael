import logging
import time
import uuid
from typing import Optional
from fastapi import Request

from open_webui.jms import chat_manager
from open_webui.jms import SessionHandler
from open_webui.jms.wisp.protobuf.common_pb2 import User
from open_webui.internal.db import Base, get_db
from open_webui.models.tags import TagModel, Tags
from open_webui.models.folders import Folders
from open_webui.env import SRC_LOG_LEVELS

from pydantic import BaseModel, ConfigDict
from sqlalchemy import BigInteger, Boolean, Column, String, Text, JSON, Index
from sqlalchemy import or_, and_, text
from sqlalchemy.sql.expression import bindparam

####################
# Chat DB Schema
####################

log = logging.getLogger(__name__)
log.setLevel(SRC_LOG_LEVELS["MODELS"])


class Chat(Base):
    __tablename__ = "chat"

    id = Column(String, primary_key=True)
    user_id = Column(String)
    title = Column(Text)
    chat = Column(JSON)

    created_at = Column(BigInteger)
    updated_at = Column(BigInteger)

    share_id = Column(Text, unique=True, nullable=True)
    archived = Column(Boolean, default=False)
    pinned = Column(Boolean, default=False, nullable=True)

    meta = Column(JSON, server_default="{}")
    folder_id = Column(Text, nullable=True)

    __table_args__ = (
        # Performance indexes for common queries
        # WHERE folder_id = ...
        Index("folder_id_idx", "folder_id"),
        # WHERE user_id = ... AND pinned = ...
        Index("user_id_pinned_idx", "user_id", "pinned"),
        # WHERE user_id = ... AND archived = ...
        Index("user_id_archived_idx", "user_id", "archived"),
        # WHERE user_id = ... ORDER BY updated_at DESC
        Index("updated_at_user_id_idx", "updated_at", "user_id"),
        # WHERE folder_id = ... AND user_id = ...
        Index("folder_id_user_id_idx", "folder_id", "user_id"),
    )


class ChatModel(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: str
    user_id: str
    title: str
    chat: dict

    created_at: int  # timestamp in epoch
    updated_at: int  # timestamp in epoch

    share_id: Optional[str] = None
    archived: bool = False
    pinned: Optional[bool] = False

    meta: dict = {}
    folder_id: Optional[str] = None


####################
# Forms
####################


class ChatForm(BaseModel):
    chat: dict
    folder_id: Optional[str] = None


class ChatImportForm(ChatForm):
    meta: Optional[dict] = {}
    pinned: Optional[bool] = False
    created_at: Optional[int] = None
    updated_at: Optional[int] = None


class ChatTitleMessagesForm(BaseModel):
    title: str
    messages: list[dict]


class ChatTitleForm(BaseModel):
    title: str


class ChatResponse(BaseModel):
    id: str
    user_id: str
    title: str
    chat: dict
    updated_at: int  # timestamp in epoch
    created_at: int  # timestamp in epoch
    share_id: Optional[str] = None  # id of the chat to be shared
    archived: bool
    pinned: Optional[bool] = False
    meta: dict = {}
    session_info: dict = {}
    folder_id: Optional[str] = None


class ChatTitleIdResponse(BaseModel):
    id: str
    title: str
    updated_at: int
    created_at: int


class ChatTable:
    @staticmethod
    def get_ip(request: Request):
        client_host, client_port = request.client or (None, None)
        ip = client_host

        xff = request.headers.get("x-forwarded-for")
        xri = request.headers.get("x-real-ip")
        if xff:
            ip = xff.split(",")[0].strip()
        elif xri:
            ip = xri.strip()
        return ip

    def insert_new_chat(self, form_data: ChatForm, sid: str, request: Request, user: User):
        ip = self.get_ip(request)
        session_handler = SessionHandler(sid=sid, ip=ip, user=user)
        chat_model = form_data.chat.get("models", [None])[0] or ''
        session = session_handler.create_session(chat_model)

        data = {
            'id': session.id,
            'user_id': session.user_id,
            'title': form_data.chat['title'] if 'title' in form_data.chat else 'New Chat',
            'chat': form_data.chat,
            'socket_id': sid,
            'folder_id': form_data.folder_id or '',
            'session_info': {
                'org_id': session.org_id,
                'asset': session.asset,
                'account': session.account,
                'user': session.user
            }
        }

        chat = chat_manager.create(data)
        return chat

    # TODO
    def import_chat(
            self, user_id: str, form_data: ChatImportForm
    ) -> Optional[ChatModel]:
        with get_db() as db:
            id = str(uuid.uuid4())
            chat = ChatModel(
                **{
                    "id": id,
                    "user_id": user_id,
                    "title": (
                        form_data.chat["title"]
                        if "title" in form_data.chat
                        else "New Chat"
                    ),
                    "chat": form_data.chat,
                    "meta": form_data.meta,
                    "pinned": form_data.pinned,
                    "folder_id": form_data.folder_id,
                    "created_at": (
                        form_data.created_at
                        if form_data.created_at
                        else int(time.time())
                    ),
                    "updated_at": (
                        form_data.updated_at
                        if form_data.updated_at
                        else int(time.time())
                    ),
                }
            )

            result = Chat(**chat)
            db.add(result)
            db.commit()
            db.refresh(result)
            return ChatModel.model_validate(result) if result else None

    @staticmethod
    def update_chat_by_id(_id: str, chat: dict) -> Optional[ChatModel]:
        chat_dict = chat_manager.update(
            _id,
            {
                'chat': chat,
                'title': chat["title"] if "title" in chat else "New Chat",
            }
        )
        return chat_dict

    def update_chat_title_by_id(self, _id: str, title: str) -> Optional[ChatModel]:
        chat_dict = self.get_chat_by_id(_id)
        if chat_dict is None:
            return None

        chat = chat_dict['chat']
        chat["title"] = title
        return self.update_chat_by_id(_id, chat)

    # TODO
    def update_chat_tags_by_id(
            self, id: str, tags: list[str], user
    ) -> Optional[ChatModel]:
        chat = self.get_chat_by_id(id)
        if chat is None:
            return None

        self.delete_all_tags_by_id_and_user_id(id, user.id)

        for tag in chat['meta'].get("tags", []):
            if self.count_chats_by_tag_name_and_user_id(tag, user.id) == 0:
                Tags.delete_tag_by_name_and_user_id(tag, user.id)

        for tag_name in tags:
            if tag_name.lower() == "none":
                continue

            self.add_chat_tag_by_id_and_user_id_and_tag_name(id, user.id, tag_name)
        return self.get_chat_by_id(id)

    def get_chat_title_by_id(self, _id: str) -> Optional[str]:
        chat_dict = self.get_chat_by_id(_id)
        if chat_dict is None:
            return None

        return chat_dict['chat'].get("title", "New Chat")

    def get_messages_map_by_chat_id(self, _id: str) -> Optional[dict]:
        chat_dict = self.get_chat_by_id(_id)
        if chat_dict is None:
            return None

        return chat_dict['chat'].get("history", {}).get("messages", {}) or {}

    def get_message_by_id_and_message_id(
            self, _id: str, message_id: str
    ) -> Optional[dict]:
        chat_dict = self.get_chat_by_id(_id)
        if chat_dict is None:
            return None

        return chat_dict['chat'].get("history", {}).get("messages", {}).get(message_id, {})

    def upsert_message_to_chat_by_id_and_message_id(
            self, _id: str, message_id: str, message: dict
    ):
        chat_dict = self.get_chat_by_id(_id)
        if chat_dict is None:
            return None

        # Sanitize message content for null characters before upserting
        if isinstance(message.get("content"), str):
            message["content"] = message["content"].replace("\x00", "")

        chat = chat_dict['chat']
        history = chat.get("history", {})

        if message_id in history.get("messages", {}):
            history["messages"][message_id] = {
                **history["messages"][message_id],
                **message,
            }
        else:
            history["messages"][message_id] = message

        history["currentId"] = message_id

        chat["history"] = history
        return self.update_chat_by_id(_id, chat)

    def add_message_status_to_chat_by_id_and_message_id(
            self, _id: str, message_id: str, status: dict
    ):
        chat_dict = self.get_chat_by_id(_id)
        if chat_dict is None:
            return None

        chat = chat_dict['chat']
        history = chat.get("history", {})

        if message_id in history.get("messages", {}):
            status_history = history["messages"][message_id].get("statusHistory", [])
            status_history.append(status)
            history["messages"][message_id]["statusHistory"] = status_history

        chat["history"] = history
        return self.update_chat_by_id(_id, chat)

    # TODO
    def insert_shared_chat_by_chat_id(self, chat_id: str) -> Optional[ChatModel]:
        with get_db() as db:
            # Get the existing chat to share
            chat = db.get(Chat, chat_id)
            # Check if the chat is already shared
            if chat['share_id']:
                return self.get_chat_by_id_and_user_id(chat['share_id'], "shared")
            # Create a new chat with the same data, but with a new ID
            shared_chat = ChatModel(
                **{
                    "id": str(uuid.uuid4()),
                    "user_id": f"shared-{chat_id}",
                    "title": chat['title'],
                    "chat": chat['chat'],
                    "meta": chat['meta'],
                    "pinned": chat['pinned'],
                    "folder_id": chat['folder_id'],
                    "created_at": chat['created_at'],
                    "updated_at": int(time.time()),
                }
            )
            shared_result = Chat(**shared_chat.model_dump())
            db.add(shared_result)
            db.commit()
            db.refresh(shared_result)

            # Update the original chat with the share_id
            result = (
                db.query(Chat)
                .filter_by(id=chat_id)
                .update({"share_id": shared_chat.id})
            )
            db.commit()
            return shared_chat if (shared_result and result) else None

    # TODO
    def update_shared_chat_by_chat_id(self, chat_id: str) -> Optional[ChatModel]:
        try:
            with get_db() as db:
                chat = db.get(Chat, chat_id)
                shared_chat = (
                    db.query(Chat).filter_by(user_id=f"shared-{chat_id}").first()
                )

                if shared_chat is None:
                    return self.insert_shared_chat_by_chat_id(chat_id)

                shared_chat.title = chat['title']
                shared_chat.chat = chat['chat']
                shared_chat.meta = chat['meta']
                shared_chat.pinned = chat['pinned']
                shared_chat.folder_id = chat['folder_id']
                shared_chat.updated_at = int(time.time())
                db.commit()
                db.refresh(shared_chat)

                return ChatModel.model_validate(shared_chat)
        except Exception:
            return None

    # TODO
    def delete_shared_chat_by_chat_id(self, chat_id: str) -> bool:
        try:
            with get_db() as db:
                db.query(Chat).filter_by(user_id=f"shared-{chat_id}").delete()
                db.commit()

                return True
        except Exception:
            return False

    @staticmethod
    def unarchive_all_chats_by_user_id(user_id: str) -> bool:
        try:
            chat_manager.update(data={"archived": False}, query={"user_id": user_id})
            return True
        except Exception:
            return False

    @staticmethod
    def update_chat_share_id_by_id(
            _id: str, share_id: Optional[str]
    ):
        data = {'share_id': share_id}
        chat_dict = chat_manager.update(_id, data)
        return chat_dict

    def toggle_chat_pinned_by_id(self, _id: str):
        chat_dict = self.get_chat_by_id(_id)
        pinned = not chat_dict.get('pinned', False)
        data = {'pinned': pinned}
        return chat_manager.update(_id, data)

    def toggle_chat_archive_by_id(self, _id: str):
        chat_dict = self.get_chat_by_id(_id)
        archived = not chat_dict.get('archived', False)
        data = {'archived': archived}
        chat_dict = chat_manager.update(_id, data)
        return chat_dict

    @staticmethod
    def archive_all_chats_by_user_id(user_id: str) -> bool:
        try:
            chat_manager.update(data={"archived": True}, query={"user_id": user_id})
            return True
        except Exception:
            return False

    @staticmethod
    def get_archived_chat_list_by_user_id(
            user_id: str,
            filter: Optional[dict] = None,
            skip: int = 0,
            limit: int = 50,
    ) -> list:
        query = {
            'user_id': user_id,
            'archived': True,
            'offset': skip,
            'limit': limit,
        }

        if filter:
            query_key = filter.get("query")
            if query_key:
                query['search'] = query_key

            order_by = filter.get("order_by")
            direction = filter.get("direction")

            if order_by and direction:
                if direction.lower() == "asc":
                    query['ordering'] = 'date_updated'

        resp = chat_manager.list(query=query)
        return resp['results']

    @staticmethod
    def get_chat_list_by_user_id(
            user_id: str,
            include_archived: bool = False,
            filter: Optional[dict] = None,
            skip: int = 0,
            limit: int = 50,
    ) -> list:
        query = {
            'user_id': user_id,
            'offset': skip,
            'limit': limit,
        }

        if not include_archived:
            query['archived'] = False

        if filter:
            query_key = filter.get("query")
            if query_key:
                query['search'] = query_key

            order_by = filter.get("order_by")
            direction = filter.get("direction")

            if order_by and direction:
                if direction.lower() == "asc":
                    query['ordering'] = 'date_updated'

        resp = chat_manager.list(query=query)
        return resp['results']

    @staticmethod
    def get_chat_title_id_list_by_user_id(
            user_id: str,
            include_archived: bool = False,
            include_folders: bool = False,
            include_pinned: bool = False,
            skip: Optional[int] = 0,
            limit: Optional[int] = 10,
    ) -> list[ChatTitleIdResponse]:
        query = {
            'user_id': user_id,
            'offset': skip,
            'limit': limit,
            'fields_size': 'mini'
        }

        if not include_folders:
            query['folder_id'] = None

        if not include_pinned:
            query['pinned'] = False

        if not include_archived:
            query['archived'] = False

        resp = chat_manager.list(query=query)
        filtered = resp['results']
        return [
            ChatTitleIdResponse.model_validate(
                {
                    "id": chat['id'],
                    "title": chat['title'],
                    "updated_at": chat['updated_at'],
                    "created_at": chat['created_at'],
                }
            )
            for chat in filtered
        ]

    @staticmethod
    def get_chat_list_by_chat_ids(
            chat_ids: list[str], skip: int = 0, limit: int = 50
    ) -> list:
        query = {
            'archived': False,
            'ids': ','.join(chat_ids),
            'offset': skip,
            'limit': limit,
        }

        resp = chat_manager.list(query=query)
        filtered = resp['results']
        return filtered

    @staticmethod
    def get_chat_by_id(_id: str):
        return chat_manager.retrieve(_id)

    # TODO
    def get_chat_by_share_id(self, id: str) -> Optional[ChatModel]:
        try:
            with get_db() as db:
                # it is possible that the shared link was deleted. hence,
                # we check if the chat is still shared by checking if a chat with the share_id exists
                chat = db.query(Chat).filter_by(share_id=id).first()

                if chat:
                    return self.get_chat_by_id(id)
                else:
                    return None
        except Exception:
            return None

    def get_chat_by_id_and_user_id(self, _id: str, user_id: str):
        return self.get_chat_by_id(_id)
        # try:
        #     with get_db() as db:
        #         chat = db.query(Chat).filter_by(id=id, user_id=user_id).first()
        #         return ChatModel.model_validate(chat)
        # except Exception:
        #     return None

    @staticmethod
    def get_chats(skip: int = 0, limit: int = 50):
        return chat_manager.list()

    @staticmethod
    def get_chats_by_user_id(user_id: str):
        query = {
            'user_id': user_id,
        }
        return chat_manager.list(query=query)

    @staticmethod
    def get_pinned_chats_by_user_id(user_id: str):
        query = {
            'user_id': user_id,
            'pinned': True,
            'archived': False,
        }
        return chat_manager.list(query=query)

    @staticmethod
    def get_archived_chats_by_user_id(user_id: str):
        query = {
            'user_id': user_id,
            'archived': True,
        }
        return chat_manager.list(query=query)

    def get_chats_by_user_id_and_search_text(
            self,
            user_id: str,
            search_text: str,
            include_archived: bool = False,
            skip: int = 0,
            limit: int = 60,
    ) -> list[ChatModel]:
        """
        Filters chats based on a search query using Python, allowing pagination using skip and limit.
        """
        search_text = search_text.replace("\u0000", "").lower().strip()

        if not search_text:
            return self.get_chat_list_by_user_id(
                user_id, include_archived, filter={}, skip=skip, limit=limit
            )

        search_text_words = search_text.split(" ")

        is_pinned = None
        if "pinned:true" in search_text_words:
            is_pinned = True
        elif "pinned:false" in search_text_words:
            is_pinned = False

        is_archived = None
        if "archived:true" in search_text_words:
            is_archived = True
        elif "archived:false" in search_text_words:
            is_archived = False

        search_text_words = [
            word
            for word in search_text_words
            if (
                    not word.startswith("tag:")
                    and not word.startswith("folder:")
                    and not word.startswith("pinned:")
                    and not word.startswith("archived:")
                    and not word.startswith("shared:")
            )
        ]

        search_text = " ".join(search_text_words)
        query = {
            'user_id': user_id,
            'skip': skip,
            'limit': limit,
        }

        if is_archived is not None:
            query['archived'] = is_archived
        elif not include_archived:
            query['archived'] = False

        if is_pinned is not None:
            query['pinned'] = is_pinned

        query['search'] = search_text
        resp = chat_manager.list(query=query)

        # Validate and return chats
        return resp['results']

    def get_chats_by_folder_id_and_user_id(
            self, folder_id: str, user_id: str, skip: int = 0, limit: int = 60
    ) -> list[ChatModel]:
        with get_db() as db:
            query = db.query(Chat).filter_by(folder_id=folder_id, user_id=user_id)
            query = query.filter(or_(Chat.pinned == False, Chat.pinned == None))
            query = query.filter_by(archived=False)

            query = query.order_by(Chat.updated_at.desc())

            if skip:
                query = query.offset(skip)
            if limit:
                query = query.limit(limit)

            all_chats = query.all()
            return [ChatModel.model_validate(chat) for chat in all_chats]

    # TODO
    def get_chats_by_folder_ids_and_user_id(
            self, folder_ids: list[str], user_id: str
    ) -> list[ChatModel]:
        with get_db() as db:
            query = db.query(Chat).filter(
                Chat.folder_id.in_(folder_ids), Chat.user_id == user_id
            )
            query = query.filter(or_(Chat.pinned == False, Chat.pinned == None))
            query = query.filter_by(archived=False)

            query = query.order_by(Chat.updated_at.desc())

            all_chats = query.all()
            return [ChatModel.model_validate(chat) for chat in all_chats]

    @staticmethod
    def update_chat_folder_id_by_id_and_user_id(
            _id: str, user_id: str, folder_id: str
    ):
        data = {
            'folder_id': folder_id,
            'pinned': False
        }
        return chat_manager.update(_id, data)

    # TODO
    def get_chat_tags_by_id_and_user_id(self, id: str, user_id: str) -> list[TagModel]:
        with get_db() as db:
            chat = db.get(Chat, id)
            tags = chat['meta'].get("tags", [])
            return [Tags.get_tag_by_name_and_user_id(tag, user_id) for tag in tags]

    # TODO
    def get_chat_list_by_user_id_and_tag_name(
            self, user_id: str, tag_name: str, skip: int = 0, limit: int = 50
    ) -> list[ChatModel]:
        with get_db() as db:
            query = db.query(Chat).filter_by(user_id=user_id)
            tag_id = tag_name.replace(" ", "_").lower()

            log.info(f"DB dialect name: {db.bind.dialect.name}")
            if db.bind.dialect.name == "sqlite":
                # SQLite JSON1 querying for tags within the meta JSON field
                query = query.filter(
                    text(
                        f"EXISTS (SELECT 1 FROM json_each(Chat.meta, '$.tags') WHERE json_each.value = :tag_id)"
                    )
                ).params(tag_id=tag_id)
            elif db.bind.dialect.name == "postgresql":
                # PostgreSQL JSON query for tags within the meta JSON field (for `json` type)
                query = query.filter(
                    text(
                        "EXISTS (SELECT 1 FROM json_array_elements_text(Chat.meta->'tags') elem WHERE elem = :tag_id)"
                    )
                ).params(tag_id=tag_id)
            else:
                raise NotImplementedError(
                    f"Unsupported dialect: {db.bind.dialect.name}"
                )

            all_chats = query.all()
            log.debug(f"all_chats: {all_chats}")
            return [ChatModel.model_validate(chat) for chat in all_chats]

    # TODO
    def add_chat_tag_by_id_and_user_id_and_tag_name(
            self, _id: str, user_id: str, tag_name: str
    ) -> Optional[ChatModel]:
        tag = Tags.get_tag_by_name_and_user_id(tag_name, user_id)
        if tag is None:
            tag = Tags.insert_new_tag(tag_name, user_id)
        try:
            with get_db() as db:
                chat = db.get(Chat, _id)

                tag_id = tag.id
                if tag_id not in chat['meta'].get("tags", []):
                    chat.meta = {
                        **chat['meta'],
                        "tags": list(set(chat['meta'].get("tags", []) + [tag_id])),
                    }

                db.commit()
                db.refresh(chat)
                return ChatModel.model_validate(chat)
        except Exception:
            return None

    @staticmethod
    def count_chats_by_tag_name_and_user_id(tag_name: str, user_id: str) -> int:
        return 0
        # with get_db() as db:  # Assuming `get_db()` returns a session object
        #     query = db.query(Chat).filter_by(user_id=user_id, archived=False)
        #
        #     # Normalize the tag_name for consistency
        #     tag_id = tag_name.replace(" ", "_").lower()
        #
        #     if db.bind.dialect.name == "sqlite":
        #         # SQLite JSON1 support for querying the tags inside the `meta` JSON field
        #         query = query.filter(
        #             text(
        #                 f"EXISTS (SELECT 1 FROM json_each(Chat.meta, '$.tags') WHERE json_each.value = :tag_id)"
        #             )
        #         ).params(tag_id=tag_id)
        #
        #     elif db.bind.dialect.name == "postgresql":
        #         # PostgreSQL JSONB support for querying the tags inside the `meta` JSON field
        #         query = query.filter(
        #             text(
        #                 "EXISTS (SELECT 1 FROM json_array_elements_text(Chat.meta->'tags') elem WHERE elem = :tag_id)"
        #             )
        #         ).params(tag_id=tag_id)
        #
        #     else:
        #         raise NotImplementedError(
        #             f"Unsupported dialect: {db.bind.dialect.name}"
        #         )
        #
        #     # Get the count of matching records
        #     count = query.count()
        #
        #     # Debugging output for inspection
        #     log.info(f"Count of chats for tag '{tag_name}': {count}")
        #
        #     return count

    def count_chats_by_folder_id_and_user_id(self, folder_id: str, user_id: str) -> int:
        with get_db() as db:
            query = db.query(Chat).filter_by(user_id=user_id)

            query = query.filter_by(folder_id=folder_id)
            count = query.count()

            log.info(f"Count of chats for folder '{folder_id}': {count}")
            return count

    def delete_tag_by_id_and_user_id_and_tag_name(
            self, _id: str, user_id: str, tag_name: str
    ) -> bool:
        try:
            chat_dict = self.get_chat_by_id(_id)
            tags = chat_dict['meta'].get("tags", [])
            tag_id = tag_name.replace(" ", "_").lower()
            tags = [tag for tag in tags if tag != tag_id]
            meta = {
                **chat_dict['meta'],
                "tags": list(set(tags)),
            }
            chat_manager.update(_id, {'meta': meta})
            return True
        except Exception:
            return False

    def delete_all_tags_by_id_and_user_id(self, _id: str, user_id: str) -> bool:
        try:
            chat_dict = self.get_chat_by_id(_id)
            data = {
                'meta': {
                    **chat_dict['meta'],
                    "tags": [],
                }
            }
            chat_manager.update(_id, data)
            return True
        except Exception:
            return False

    @staticmethod
    def delete_chat_by_id(self, _id: str) -> bool:
        try:
            chat_manager.destroy(_id)
            return True
            # return True and self.delete_shared_chat_by_chat_id(id)
        except Exception:
            return False

    @staticmethod
    def delete_chat_by_id_and_user_id(_id: str, user_id: str) -> bool:
        try:
            chat_manager.destroy(_id, {'user_id': user_id})
            return True
            # return True and self.delete_shared_chat_by_chat_id(id)
        except Exception:
            return False

    @staticmethod
    def delete_chats_by_user_id(user_id: str) -> bool:
        try:
            # self.delete_shared_chats_by_user_id(user_id)
            chat_manager.destroy('', {'user_id': user_id})
            return True
        except Exception:
            return False

    @staticmethod
    def delete_chats_by_user_id_and_folder_id(
            user_id: str, folder_id: str
    ) -> bool:
        try:
            chat_manager.destroy('', {'user_id': user_id, 'folder_id': folder_id})
            return True
        except Exception:
            return False

    # TODO
    def delete_shared_chats_by_user_id(self, user_id: str) -> bool:
        try:
            with get_db() as db:
                chats_by_user = db.query(Chat).filter_by(user_id=user_id).all()
                shared_chat_ids = [f"shared-{chat['id']}" for chat in chats_by_user]

                db.query(Chat).filter(Chat.user_id.in_(shared_chat_ids)).delete()
                db.commit()

                return True
        except Exception:
            return False


Chats = ChatTable()
