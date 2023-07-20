<script setup>
import { computed } from 'vue'
import { useChatStore } from '@/store'

const chatStore = useChatStore()
const activeTab = computed(() => chatStore.activeTab)
const sessions = computed(() => chatStore.sessionsStore)
const loading = computed(() => chatStore.loading)

const switchTab = (id) => {
  if (loading.value) return

  chatStore.setActiveNum(id)
}

const onNewChat = () => {
  chatStore.setTabNum()
  chatStore.setActiveNum(chatStore.tabNum)
}

const onDelete = (id) => {
  chatStore.removeSessionsStore(id)
}

</script>
<template>
  <n-layout-sider
    collapse-mode="width"
    :collapsed-width="0"
    :width="240"
    show-trigger="arrow-circle"
    content-style="padding: 16px;"
    bordered
    class="bg-[#202123]"
  >
    <div class="box-border">
      <n-button
        secondary
        class="mb-16px w-full border border-solid border-[#545557] h-44px rounded-6px"
        :disabled="loading"
        @click="onNewChat"
      >
        <SvgIcon name="add" class="mr-44px" />
        New chat
      </n-button>
      <div 
        v-for="(item, index) in sessions"
        :key="index"
        class="card border hover:bg-neutral-100 dark:hover:bg-[#2A2B32] border-[#e5e7eb] dark:border-neutral-800"
        :class="{'active-tab': activeTab === item.id, 'not-allowed': loading}"
        @click="switchTab(item.id)"
      >
        <span class="title">
          <SvgIcon name="chat" class="mr-8px" />
          <span style="user-select: none;">{{ item.name }}</span>
        </span>
        <span v-if="activeTab === item.id" class="action">
          <!-- <SvgIcon name="delete" @click.stop="onDelete(item.id)" /> -->
        </span>
      </div>
    </div>
  </n-layout-sider>
</template>

<style lang="scss" scoped>
.card {
  display: flex;
  width: 100%;
  height: 46px;
  padding: 12px;
  margin-bottom: 8px;
  border-radius: 6px;
  cursor: pointer;
  .title {
    flex: 1;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
}

.not-allowed {
  cursor: not-allowed;
}
.active-tab {
  background-color: #343540;
}
::v-deep(.n-button) {
  justify-content: left;
}
</style>