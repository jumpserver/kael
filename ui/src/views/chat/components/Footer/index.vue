<script setup>
import { ref, computed, nextTick } from 'vue'
import { useChatStore } from '@/store'
import { useChat } from '../../hooks/useChat.js'
import { LunaEvent, MESSAGES } from '@/utils/luna'

const lunaEvent = new LunaEvent()
const chatStore = useChatStore()
const { setLoading, getInputFocus } = useChat()

const value = ref('')
const isIM = ref(false)
const emit = defineEmits()
const loading = computed(() => chatStore.loading)
const disabled = computed(() => chatStore.activeChat.disabled || false)
const isGlobalDisabled = computed(() => chatStore.globalDisabled || false)

const onSendHandle = () => {
  if (!value.value) return

  setLoading(true)
  emit('send', value.value)
  value.value = ''
}

const onStopHandle = () => {
  emit('stop')
  nextTick(() => {
    getInputFocus()
  })
}

const onKeyEnter = (event) => {
  lunaEvent.sendEventToLuna(MESSAGES.KEYBOARDEVENT)
  if (!isIM.value) {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      onSendHandle()
    }
  } else {
    if (event.key === 'Enter' && event.ctrlKey) {
      event.preventDefault()
      onSendHandle()
    }
  }
}
</script>

<template>
  <footer class="footer dark:bg-[#343540] w-full">
    <div v-if="loading" class="sticky bottom-0 left-0 flex justify-center">
      <n-button type="warning" @click="onStopHandle()">
        <i class="fa fa-stop-circle-o mr-4px"></i> Stop
      </n-button>
    </div>
    <div class="flex w-full max-w-800px mx-auto p-4">
      <n-input
        v-model:value="value"
        type="textarea"
        autofocus
        :autosize="{ minRows: 1, maxRows: 7 }"
        placeholder="来说点什么吧..."
        class="dark:bg-[#40414f] hover:border-transparent"
        style="--n-border-hover: 1px solid transparent; --n-color-focus: transparent; --n-border-focus: 1px solid transparent; --n-box-shadow-focus: 0 0 8px 0 rgba(193, 194, 198, 0.3);"
        :disabled="loading || disabled || isGlobalDisabled"
        @compositionstart="isIM = true"
        @compositionend="isIM = false"
        @keypress="onKeyEnter"
      >
        <template #suffix>
          <n-button
            quaternary
            class="ml-10px"
            :disabled="loading || disabled || isGlobalDisabled"
            @click="onSendHandle"
          >
            <i class="fa fa-send"></i>
          </n-button>
        </template>
      </n-input>
    </div>
  </footer>
</template>

<style>
.n-input.n-input--textarea .n-input-wrapper {
  resize: none !important;
  padding-top: 11px !important;
  padding-bottom: 10px !important;
}
</style>

<style lang="scss" scoped>
.footer {
  .n-input {
    min-height: 58px;
    border-radius: 12px;
  }
}
</style>