<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import TitleBar from '@/components/TitleBar.vue'
import Toolbar from '@/components/Toolbar.vue'
import SessionTable from '@/components/SessionTable.vue'
import DetailPane from '@/components/DetailPane.vue'
import SplitPane from '@/components/SplitPane.vue'
import InterceptModal from '@/components/InterceptModal.vue'
import { useCaptureStore } from '@/stores/capture'
import type { CoreMessage, Lifecycle } from '@/types'

const store = useCaptureStore()
let unsubEvent: (() => void) | undefined
let unsubLife: (() => void) | undefined
let statusTimer: ReturnType<typeof setInterval> | undefined

onMounted(() => {
  unsubEvent = window.api.onCoreEvent((msg) => store.handleMessage(msg as CoreMessage))
  unsubLife = window.api.onCoreLifecycle((state) => store.setLifecycle(state as Lifecycle))
  // 主动拉取当前连接状态（补发在挂载前丢失的 lifecycle 事件）
  void window.api.getLifecycle().then((state) => store.setLifecycle(state as Lifecycle))
  window.api.sendCore({ cmd: 'status' })
  // 定时刷新状态（证书勾选/代理/拦截等），保证界面与 core 一致
  statusTimer = setInterval(() => {
    window.api.sendCore({ cmd: 'status' })
  }, 5000)
})

onUnmounted(() => {
  unsubEvent?.()
  unsubLife?.()
  if (statusTimer) clearInterval(statusTimer)
})

function resolveIntercept(action: 'continue' | 'drop', payload?: unknown): void {
  const entry = store.interceptQueue[0]
  if (entry) store.resolveIntercept(entry.id, action, payload as never)
}
</script>

<template>
  <div class="app">
    <TitleBar />
    <Toolbar />
    <SplitPane>
      <template #left><SessionTable /></template>
      <template v-if="store.selected" #right>
        <Transition name="slide"><DetailPane /></Transition>
      </template>
    </SplitPane>
    <InterceptModal v-if="store.interceptQueue.length > 0" :entry="store.interceptQueue[0]" @resolve="resolveIntercept" />
  </div>
</template>

<style scoped>
.app {
  display: flex;
  flex-direction: column;
  height: 100vh;
  overflow: hidden;
}
</style>
