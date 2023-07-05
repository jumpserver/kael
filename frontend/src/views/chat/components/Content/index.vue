<script setup>
import { ref, reactive, onMounted, computed } from 'vue'
import Message from '../Message/index.vue'

const value = ref('')
const loading = ref(false)
const dataSources = reactive([
  {
    id: 1,
    message: {
      content: '### 你好',
      create_time: "2023-07-04T16:40:11.741097",
      role: 'assistant'
    },
    create_time: '2021-08-01 12:00:00',
    parent: 'ew',
  },
  {
    id: 2,
    message: {
      content: '### 你好===============================',
      create_time: "2023-07-04T16:40:11.741097",
      role: 'assistant-1'
    },
    create_time: '2021-08-01 12:00:00',
    parent: 'ew',
  }
])

let webSocket = reactive(null)

const onWebsocketOpen = (msg) => {
  console.log('msg:===================onWebsocketOpen ', msg)
}
const onWebSocketMessage = (msg) => {
  const data = JSON.parse(msg.data)
  console.log('data: ', data)
}
const onWebSocketError = (msg) => {
  console.log('msg:===================onWebSocketError ', msg)
}
const onWebSocketClose = (msg) => {
  console.log('msg:===================onWebSocketClose ', msg)
}

const onSend = () => {
  const message = {
    content: value.value,
    sender: "user",
    new_conversation: true,
    model: 'gpt_3_5',
  }
  webSocket.send(JSON.stringify(message))
  value.value = ''
}

const initWebSocket = () => {
  const path = 'ws://127.0.0.1:8800/chat'
  webSocket = new WebSocket(path)
  webSocket.onopen = onWebsocketOpen
  webSocket.onmessage = onWebSocketMessage
  webSocket.onerror = onWebSocketError
  webSocket.onclose = onWebSocketClose
}

onMounted(() => {
  initWebSocket()
})
</script>

<template>
  <div class="content">
    <main class="flex-1 overflow-hidden">
      <div id="scrollRef" class="h-1/1 overflow-hidden overflow-y-auto p-4">
        <div class="h-1/1">
          <div v-if="!dataSources.length">
            没有数据
          </div>
          <template v-else>
            <div class="overflow-y-auto">
              <Message
                v-for="(item, index) of dataSources"
                :key="index"
                :error="item.error_detail"
                :loading="loading"
                :message="item.message"
                @regenerate="onRegenerate(index)"
                @delete="handleDelete(index)"
              />
            </div>
          </template>
        </div>
      </div>
    </main>
    <footer class="footer p-4">
      <div class="flex">
        <n-input v-model:value="value" type="text" placeholder="来说点什么吧..." />
        <n-button
          type="primary"
          class="ml-10px"
          :disabled="!loading"
          @click="onSend"
        >Send</n-button>
      </div>
    </footer>
  </div>
</template>

<style lang="scss" scoped>
.content {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
}
#scrollRef::-webkit-scrollbar {
  background-color: rgba(0, 0, 0, 0.25);
  width: 5px;
}
</style>
