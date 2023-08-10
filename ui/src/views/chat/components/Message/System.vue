<script setup>
import { ref, toRefs, inject, watch, computed, onMounted } from 'vue'
import { useChatStore } from '@/store'

const chatStore = useChatStore()
const $axios = inject("$axios")
const props = defineProps({
  item: Object,
  index: Number,
  to: {
    type: String,
    default: '#content'
  }
})
const { item, to, index } = toRefs(props)
const isShow = ref(true)

const currentChat = computed(() => chatStore.filterChat.chats[index.value] || {})

const onClick = (value) => {
  console.log('value: ', value)
  $axios.post(
    '/kael/jms_state/',
    {
      id: item.value.conversation_id,
      activate_review: value
    }
  ).finally(() => {
    isShow.value = false
    chatStore.updateChatConversationDisabledById(index.value, true)
  })
}

watch(isShow, (value) => {
  if (value) {
    chatStore.setFilterChatDisabled(true)
  } else {
    chatStore.setFilterChatDisabled(false)
  }
}, { immediate: true })

onMounted(() => {
  const currentChatValue = { ...currentChat.value }
  if (currentChatValue.hasOwnProperty('disabled')) {
    isShow.value = !currentChatValue.disabled
  } else {
    isShow.value = true
  }
})
</script>

<template>
  <Teleport :to="to">
    <div v-if="isShow" class="modal-mask">
      <n-card
        title="命令复核"
        size="huge"
        role="dialog"
        :bordered="false"
        aria-modal="true"
        :segmented="{
          content: true
        }"
      >
      <span>
        {{ item.system_message }}
      </span>
      <template #footer>
        <div class="footer text-center">
          <n-button secondary @click="onClick(1)">
            是
          </n-button>
          <n-button secondary class="ml-10px" @click="onClick(0)">
            否
          </n-button>
        </div>
      </template>
    </n-card>
  </div>
  </Teleport>
</template>

<style lang="scss" scoped>
.modal-mask {
  position: fixed;
  width: 100%;
  height: 100%;
  background-color: rgba(0, 0, 0, .4);
  display: flex;
  justify-content: center;
  align-items: center;
}
::v-deep(.n-card) {
  width: 450px;
  background-color: #393a3c;
  .n-card-header {
    padding: 10px;
    text-align: center;
  }
  .n-card__content {
    padding: 12px;
  }
  .n-card__footer {
    padding: 12px;
    border-top: none;
    &:not(:first-child) {
      border-top: none;
    }
  }
}
</style>
