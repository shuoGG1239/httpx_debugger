<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useCaptureStore } from '@/stores/capture'
import { formatBytes, formatDuration, formatTime } from '@/utils/bytes'
import { copyText } from '@/utils/clipboard'
import { sessionToCurl } from '@/utils/curl'
import ContextMenu, { type MenuItem } from './ContextMenu.vue'
import type { Session } from '@/types'

const store = useCaptureStore()
const rows = computed(() => store.filtered)

const tbodyEl = ref<HTMLElement | null>(null)
const atBottom = ref(true)

const menu = ref<{ x: number; y: number; items: MenuItem[] } | null>(null)

function openMenu(e: MouseEvent, s: Session): void {
  menu.value = {
    x: e.clientX,
    y: e.clientY,
    items: [
      { label: '复制 URL', onClick: () => copyText(s.url) },
      { label: '复制为 cURL', onClick: () => copyText(sessionToCurl(s)) },
      { label: '重放（重新发送）', onClick: () => store.replay(s) },
    ],
  }
}

function onScroll(): void {
  const el = tbodyEl.value
  if (!el) return
  atBottom.value = el.scrollTop + el.clientHeight >= el.scrollHeight - 40
}

watch(
  () => store.sessionCount,
  async () => {
    if (!atBottom.value) return
    await nextTick()
    const el = tbodyEl.value
    if (el) el.scrollTop = el.scrollHeight
  },
)

onMounted(() => {
  const el = tbodyEl.value
  if (el) el.addEventListener('scroll', onScroll, { passive: true })
})
onUnmounted(() => {
  const el = tbodyEl.value
  if (el) el.removeEventListener('scroll', onScroll)
})

function methodClass(m: string): string {
  return 'm-' + m.toLowerCase()
}
function statusClass(s: number): string {
  if (s >= 500) return 'st-5xx'
  if (s >= 400) return 'st-4xx'
  if (s >= 300) return 'st-3xx'
  if (s >= 200) return 'st-2xx'
  return ''
}
</script>

<template>
  <div class="session-table">
    <div class="thead">
      <span class="c-time">时间</span>
      <span class="c-method">方法</span>
      <span class="c-status">状态</span>
      <span class="c-scheme">类型</span>
      <span class="c-host">Host</span>
      <span class="c-path">路径</span>
      <span class="c-size">大小</span>
      <span class="c-ms">耗时</span>
    </div>
    <div ref="tbodyEl" class="tbody">
      <div
        v-for="s in rows"
        :key="s.id"
        class="row"
        :class="{ selected: s.id === store.selectedId, err: !!s.error }"
        @click="store.select(s.id)"
        @contextmenu.prevent="openMenu($event, s)"
      >
        <span class="c-time" :title="new Date(s.startedAt).toLocaleString()">{{ formatTime(s.startedAt) }}</span>
        <span class="c-method"><i :class="['tag', methodClass(s.method)]">{{ s.method }}</i></span>
        <span class="c-status" :class="statusClass(s.status)">{{ s.status || '—' }}</span>
        <span class="c-scheme">{{ s.scheme }}</span>
        <span class="c-host">{{ s.host }}</span>
        <span class="c-path">{{ s.url }}</span>
        <span class="c-size">{{ formatBytes((s.responseBody?.length ?? 0) + (s.requestBody?.length ?? 0)) }}</span>
        <span class="c-ms">{{ formatDuration(s.elapsedMs) }}</span>
      </div>
      <div v-if="rows.length === 0" class="empty">
        {{ store.sessionCount === 0 ? '暂无会话：点「开始捕获」，让目标程序走系统代理（HTTP/HTTPS）' : '没有匹配的会话' }}
      </div>
    </div>
    <ContextMenu v-if="menu" :x="menu.x" :y="menu.y" :items="menu.items" @close="menu = null" />
  </div>
</template>

<style scoped>
.session-table {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-width: 0;
}
.thead,
.row {
  display: grid;
  grid-template-columns: 84px 64px 52px 52px 1fr 1.8fr 70px 64px;
  gap: 4px;
  align-items: center;
  padding: 0 8px;
  font-size: 12px;
  white-space: nowrap;
}
.thead {
  flex: none;
  height: 26px;
  color: var(--fg-dim);
  background: var(--bg-panel);
  border-bottom: 1px solid var(--border);
  font-weight: 500;
}
.tbody {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
}
.row {
  height: 26px;
  border-bottom: 1px solid var(--border-faint);
  cursor: pointer;
  color: var(--fg);
}
.row:hover {
  background: var(--bg-hover);
}
.row.selected {
  background: var(--bg-selected);
}
.row.err {
  color: var(--danger);
}
.c-path {
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--fg-dim);
}
.c-host {
  overflow: hidden;
  text-overflow: ellipsis;
}
.c-ms {
  color: var(--fg-dim);
  text-align: right;
}
.tag {
  font-style: normal;
  font-weight: 600;
  padding: 1px 5px;
  border-radius: 4px;
  font-size: 11px;
}
.m-get { color: #4fc3f7; background: rgba(79, 195, 247, 0.12); }
.m-post { color: #81c784; background: rgba(129, 199, 132, 0.12); }
.m-put { color: #ffb74d; background: rgba(255, 183, 77, 0.12); }
.m-patch { color: #ba68c8; background: rgba(186, 104, 200, 0.12); }
.m-delete { color: #e57373; background: rgba(229, 115, 115, 0.12); }
.m-head, .m-options, .m-connect { color: #90a4ae; background: rgba(144, 164, 174, 0.12); }
.st-2xx { color: var(--ok); }
.st-3xx { color: var(--warn); }
.st-4xx { color: #ffa726; }
.st-5xx { color: var(--danger); font-weight: 600; }
.empty {
  padding: 24px 16px;
  color: var(--fg-dim);
  font-size: 12.5px;
  text-align: center;
}
</style>
