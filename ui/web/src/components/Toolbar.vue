<script setup lang="ts">
import { computed, ref } from 'vue'
import { useCaptureStore } from '@/stores/capture'
import { buildHar } from '@/utils/har'

const store = useCaptureStore()

const capturing = computed(() => !!store.status?.capturing)
const caInstalled = computed(() => !!store.status?.caInstalled)
const caPending = computed(() => !store.connected)
const hasSessions = computed(() => store.sessionCount > 0)

const pattern = ref(store.interceptPattern)
const direction = ref(store.interceptDirection)

function commitIntercept(): void {
  store.applyIntercept({ pattern: pattern.value, direction: direction.value })
}

function exportHar(): void {
  const json = buildHar(store.filtered.length > 0 ? store.filtered : store.sessions)
  const name = `httpx-${new Date().toISOString().replace(/[:.]/g, '-')}.har`
  void window.api.saveHar(json, name)
}
</script>

<template>
  <div class="toolbar">
    <button class="btn primary" :class="{ active: capturing }" @click="store.toggleCapture()">
      {{ capturing ? '⏹ 停止捕获' : '▶ 开始捕获' }}
    </button>
    <button
      class="btn"
      :class="{ active: store.interceptEnabled, flash: store.pendingIntercepts > 0 }"
      :title="'拦截模式：命中规则时暂停请求/响应供编辑。当前排队 ' + store.pendingIntercepts"
      @click="store.applyIntercept({ enabled: !store.interceptEnabled })"
    >
      拦截{{ store.pendingIntercepts > 0 ? ` ·${store.pendingIntercepts}` : '' }}
    </button>
    <template v-if="store.interceptEnabled">
      <input
        v-model="pattern"
        class="filter small"
        type="text"
        placeholder="host 关键字，空=全部"
        spellcheck="false"
        @change="commitIntercept"
        @keydown.enter="commitIntercept"
      />
      <select v-model="direction" class="filter small" @change="commitIntercept">
        <option value="both">请求+响应</option>
        <option value="request">仅请求</option>
        <option value="response">仅响应</option>
      </select>
    </template>
    <button
      class="btn"
      :class="{ active: store.redirectEnabled }"
      :title="'WinDivert 全流量抓取：抓所有程序的 TCP（需管理员 + 驱动，见 scripts/setup-windivert.ps1）'"
      @click="store.toggleRedirect()"
    >
      全流量{{ store.redirectEnabled ? ' 开' : '' }}
    </button>
    <button
      class="btn"
      :disabled="caPending"
      :class="{ 'ca-ok': caInstalled, 'ca-bad': !caPending && !caInstalled }"
      :title="caPending ? '等待核心连接…（启动后约 2-3 秒）' : caInstalled ? '根证书已信任，无需重复安装（点击可重装）' : '未安装：HTTPS 明文需要信任根证书'"
      @click="store.installCA()"
    >
      {{ caPending ? '证书…' : `安装证书${caInstalled ? ' ✓' : ''}` }}
    </button>
    <span class="sep" />
    <input
      v-model="store.filter"
      class="filter"
      type="text"
      placeholder="过滤：(GET OR POST) AND 200 / host 关键词"
      spellcheck="false"
    />
    <span class="sep" />
    <span class="hint" v-if="store.lifecycle?.error">⚠ {{ store.lifecycle.error }}</span>
    <span class="hint" v-if="store.status?.redirectError">⚠ {{ store.status.redirectError }}</span>
    <span class="notice" :class="store.notice?.kind" v-if="store.notice">{{ store.notice.text }}</span>
    <span class="spacer" />
    <button class="btn" :disabled="!hasSessions" @click="exportHar()">导出 HAR</button>
    <button class="btn" :disabled="!hasSessions" @click="store.clearSessions()">清空</button>
  </div>
</template>

<style scoped>
.toolbar {
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: var(--bg-panel);
  border-bottom: 1px solid var(--border);
}
.btn {
  font: inherit;
  font-size: 12.5px;
  color: var(--fg);
  background: var(--bg-input);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 5px 12px;
  cursor: pointer;
  transition: background 0.12s;
}
.btn:hover:not(:disabled) {
  background: var(--bg-hover);
}
.btn:disabled {
  opacity: 0.4;
  cursor: default;
}
.btn.primary {
  background: var(--accent);
  border-color: var(--accent);
  color: #fff;
  font-weight: 600;
}
.btn.primary:hover:not(:disabled) {
  background: var(--accent-strong);
  border-color: var(--accent-strong);
}
.btn.primary.active {
  background: var(--danger);
  border-color: var(--danger);
  color: #fff;
}
.btn.active {
  background: var(--bg-hover);
  border-color: var(--accent);
}
.filter {
  flex: 1;
  max-width: 420px;
  font: inherit;
  font-size: 12.5px;
  color: var(--fg);
  background: var(--bg-input);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 5px 10px;
  outline: none;
}
.filter.small {
  flex: none;
  max-width: 180px;
  padding: 4px 8px;
  font-size: 12px;
}
.filter:focus {
  border-color: var(--accent);
}
.btn.flash {
  border-color: var(--warn);
  color: var(--warn);
}
.btn.ca-ok {
  border-color: var(--ok);
  color: var(--ok);
}
.btn.ca-bad {
  border-color: var(--danger);
  color: var(--danger);
}
.sep {
  width: 1px;
  height: 20px;
  background: var(--border);
}
.hint {
  font-size: 12px;
  color: var(--danger);
  max-width: 420px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.notice {
  font-size: 12px;
  max-width: 380px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.notice.ok {
  color: var(--ok);
}
.notice.err {
  color: var(--danger);
}
.spacer {
  flex: 1;
}
</style>
