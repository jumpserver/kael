<script setup>
import { ref, onMounted, computed, inject, onUnmounted, nextTick } from 'vue'
import Message from '../Message/index.vue'
import Empty from '../Empty/index.vue'
import Footer from '../Footer/index.vue'
import { useChat } from '../../hooks/useChat.js'
import { createWebSocket, onSend, closeWs } from '@/utils/socket'
import { useChatStore } from '@/store'
import { pageScroll } from '@/utils/common'

const { hasChat, setLoading, getInputFocus, addChatConversationById, addTemporaryLoadingChat, updateChatConversationContentById } = useChat()
const chatStore = useChatStore()
const $axios = inject("$axios")
const currentConversationId = ref('')
const env = import.meta.env

const currentSessionStore = computed(() => chatStore.filterChat)

const onWebSocketMessage = (data) => {
  currentConversationId.value = data?.conversation_id
  const types = ['waiting', 'reject', 'error', 'finish']
  if (types.includes(data.type)) {
    onSystemMessage(data)
    return
  }
  if (data.type === 'message') {
    currentConversationId.value = data.conversation_id
    if (hasChat(data.message.id)) {
      chatStore.removeLastChat()
      addChatConversationById(data)
    } else {
      updateChatConversationContentById(data.message.id, data.message.content)
    }
    if (data.message?.type === 'finish') {
      setLoading(false)
      nextTick(() => getInputFocus())
    }
  }
}

const onSystemMessage = (data) => {
  data = {
    ...data,
    error: 'error',
    message: {
      content: data.system_message,
      role: 'assistant',
      create_time: new Date()
    }
  }
  chatStore.removeLastChat()
  addChatConversationById(data)
  setLoading(false)
  nextTick(() => getInputFocus())

  if (data.type === 'waiting') {
    const sessionState = data?.meta?.session_state || ''
    if (sessionState === 'lock') {
      chatStore.setFilterChatDisabled(true)
      return
    }
    if (sessionState === 'unlock') {
      chatStore.setFilterChatDisabled(false)
      return
    }
  }
  if (data.type === 'finish') {
    chatStore.setFilterChatDisabled(true)
  }
}

const onSendHandle = (value) => {
  const chat = {
    message: {
      content: value,
      role: 'user',
      create_time: new Date()
    }
  }
  addChatConversationById(chat)
  addTemporaryLoadingChat()
  const message = {
    content: value,
    conversation_id: currentConversationId.value || null
  }
  onSend(message)
}

const initWebSocket = () => {
  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
  const path = `${protocol}://${window.location.host}/kael/chat/`
  const localPath = env.VITE_APP_BASE_SOCKET + '/kael/chat/'
  const url = env.MODE === 'development' ? localPath : path
  createWebSocket(url, onWebSocketMessage)
}

const onStopHandle = () => {
  $axios.post(
    '/kael/interrupt_current_ask/',
    { id: currentConversationId.value || '' }
  ).finally(() => {
    setLoading(false)
  })
}

onMounted(() => {
  initWebSocket()
  pageScroll('scrollRef')
  window.addEventListener('beforeunload', (event) => {
    event.preventDefault()
    closeWs()
  })
})

onUnmounted(() => {
  window?.removeEventListener('beforeunload')
})
</script>

<template>
  <template v-if="!currentSessionStore.chats?.length">
    <Empty />
  </template>
  <div v-else class="content" id="content">
    <main class="flex-1 overflow-y-auto dark:bg-[#343540]">
      <div id="scrollRef" class="overflow-hidden pt-4 pb-4">
        <div>
          <div class="overflow-y-auto">
            <Message
            v-for="(item, index) of currentSessionStore.chats"
            :key="index"
            :index="index"
            :item="item"
            :message="item.message"
            @delete="handleDelete(index)"
            />   
          </div>
        </div>
      </div>
    </main>
    <Footer @send="onSendHandle" @stop="onStopHandle" />
    </div>
  </template>

<style lang="scss" scoped>
.content {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
}
.footer {
  .n-input {
    height: 58px;
    line-height: 58px;
    border-radius: 12px;
  }
}
</style>