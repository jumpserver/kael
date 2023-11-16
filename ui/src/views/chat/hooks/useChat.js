import { useChatStore } from '@/store'
import { pageScroll } from '@/utils/common'

export function useChat() {
  const chatStore = useChatStore()

  const setLoading = (loading) => {
    chatStore.setLoading(loading)
  }

  const getInputFocus = () => {
    const dom = document.getElementsByClassName('n-input__textarea-el')[0]
    dom?.focus()
  }

  const onNewChat = (name) => {
    chatStore.setTabNum()
    const data = {
      name: name || `new chat ${chatStore.tabNum}`,
      id: chatStore.tabNum,
      conversation_id: '',
      chats: []
    }
    chatStore.addChatToStore(data)
  } 

  const addChatConversationById = (chat) => {
    chatStore.addConversationToActiveChat(chat)
    if (chat?.conversation_id) {
      chatStore.setActiveChatConversationId(chat.conversation_id)
    }
    pageScroll('scrollRef')
  }

  // 添加系统消息到当前聊天
  const addSystemMessageToCurrentChat = (data) => {
    const activeChat = chatStore.activeChat
    if (activeChat.conversation_id === data.conversation_id) {
      chatStore.addConversationToActiveChat(data)
    } else {
      const chatsStore = chatStore.chatsStore.filter(item => item.conversation_id === data.conversation_id)
      if (chatsStore.length > 0) {
        chatsStore[0].chats?.push(data)
      } else {
        chatStore.chatsStore[0].chats?.push(data)
      }
    }
    pageScroll('scrollRef')
  }

  // 设置对应的聊天是否禁用
  const setCorrespondChatDisabled = (data, disabled) => {
    const activeChat = chatStore.activeChat
    if (activeChat.conversation_id === data.conversation_id) {
      chatStore.setActiveChatDisabled(disabled)
    } else {
      const chatsStore = chatStore.chatsStore.filter(item => item.conversation_id === data.conversation_id)
      if (chatsStore.length > 0) {
        chatsStore[0].disabled = disabled
      }
    }
  }

  const addTemporaryLoadingChat = () => {
    const temporaryChat = {
      message: {
        content: 'loading',
        role: 'assistant',
        create_time: new Date()
      }
    }
    addChatConversationById(temporaryChat)
  }

  const onNewChatOrAddChatConversationById = (chat) => {
    onNewChat(chat.message.content)
    addChatConversationById(chat)
  }

  const updateChatConversationContentById = (id, content) => {
    chatStore.updateChatConversationContentById(id, content)
    pageScroll('scrollRef')
  }

  const hasChat = (id) => {
    const chats = chatStore.activeChat.chats
    const filterChat = chats.filter((chat) => chat.message.id === id)
    if (filterChat.length > 0) {
      return false
    }
    return true
  }

  return {
    chatStore,
    onNewChat,
    onNewChatOrAddChatConversationById,
    hasChat,
    getInputFocus,
    setLoading,
    addChatConversationById,
    addTemporaryLoadingChat,
    setCorrespondChatDisabled,
    addSystemMessageToCurrentChat,
    updateChatConversationContentById
  }
}
