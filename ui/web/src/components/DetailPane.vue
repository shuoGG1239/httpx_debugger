<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useCaptureStore } from '@/stores/capture'
import { bodyBytes, formatBytes, formatDuration } from '@/utils/bytes'
import { copyText } from '@/utils/clipboard'
import { sessionToCurl } from '@/utils/curl'
import BodyView from './BodyView.vue'

const store = useCaptureStore()
const tab = ref<'overview' | 'request' | 'response'>('overview')

const selected = computed(() => store.selected)
const requestBytes = computed(() => (selected.value ? bodyBytes(selected.value.requestBody) : new Uint8Array(0)))
const responseBytes = computed(() => (selected.value ? bodyBytes(selected.value.responseBody) : new Uint8Array(0)))

watch(selected, (s) => {
  if (s) tab.value = 'overview'
})

function headersToText(hs: { name: string; value: string }[]): string {
  return hs.map((h) => `${h.name}: ${h.value}`).join('\n')
}
</script>

<template>
  <div class="detail">
    <template v-if="selected">
      <div v-if="selected.error" class="error-banner">⚠ {{ selected.error }}</div>
      <div class="tabs">
        <button :class="{ on: tab === 'overview' }" @click="tab = 'overview'">概览</button>
        <button :class="{ on: tab === 'request' }" @click="tab = 'request'">请求</button>
        <button :class="{ on: tab === 'response' }" @click="tab = 'response'">响应</button>
      </div>

      <!-- 概览 -->
      <div v-if="tab === 'overview'" class="overview">
        <div class="kv-actions">
          <button class="act" @click="store.replay(selected)">▶ 重放</button>
          <button class="act" @click="copyText(sessionToCurl(selected))">复制 cURL</button>
          <button class="act" @click="copyText(selected.url)">复制 URL</button>
        </div>
        <div class="kv">
          <div class="kv-row"><span class="k">URL</span><span class="v">{{ selected.url }}</span></div>
          <div class="kv-row"><span class="k">方法</span><span class="v">{{ selected.method }}</span></div>
          <div class="kv-row"><span class="k">协议</span><span class="v">{{ selected.scheme }}</span></div>
          <div class="kv-row"><span class="k">Host</span><span class="v">{{ selected.host }}</span></div>
          <div class="kv-row"><span class="k">状态</span><span class="v">{{ selected.status }} {{ selected.statusText }}</span></div>
          <div class="kv-row"><span class="k">客户端</span><span class="v">{{ selected.clientAddr }}</span></div>
          <div class="kv-row"><span class="k">时间</span><span class="v">{{ new Date(selected.startedAt).toLocaleString() }}</span></div>
          <div class="kv-row"><span class="k">耗时</span><span class="v">{{ formatDuration(selected.elapsedMs) }}</span></div>
          <div class="kv-row"><span class="k">请求体</span><span class="v">{{ formatBytes(requestBytes.length) }}{{ selected.requestTruncated ? '（已截断）' : '' }}</span></div>
          <div class="kv-row"><span class="k">响应体</span><span class="v">{{ formatBytes(responseBytes.length) }}{{ selected.responseTruncated ? '（已截断）' : '' }}</span></div>
          <div class="kv-row"><span class="k">响应类型</span><span class="v">{{ selected.responseContentType || '—' }}</span></div>
        </div>
      </div>

      <!-- 请求 -->
      <div v-else-if="tab === 'request'" class="msg">
        <div class="msg-head">
          <span class="msg-title">请求头</span>
          <button class="copy" @click="copyText(headersToText(selected.requestHeaders))">复制</button>
        </div>
        <pre class="headers">{{ headersToText(selected.requestHeaders) }}</pre>
        <div class="msg-head">
          <span class="msg-title">请求体 <span class="dim">{{ formatBytes(requestBytes.length) }}{{ selected.requestTruncated ? '（已截断）' : '' }}</span></span>
        </div>
        <BodyView :bytes="requestBytes" :content-type="selected.requestContentType" :truncated="selected.requestTruncated" />
      </div>

      <!-- 响应 -->
      <div v-else class="msg">
        <div class="msg-head">
          <span class="msg-title">响应头</span>
          <button class="copy" @click="copyText(headersToText(selected.responseHeaders))">复制</button>
        </div>
        <pre class="headers">{{ headersToText(selected.responseHeaders) }}</pre>
        <div class="msg-head">
          <span class="msg-title">响应体 <span class="dim">{{ formatBytes(responseBytes.length) }}{{ selected.responseTruncated ? '（已截断）' : '' }}</span></span>
        </div>
        <BodyView :bytes="responseBytes" :content-type="selected.responseContentType" :truncated="selected.responseTruncated" />
      </div>
    </template>

    <div v-else class="placeholder">
      选择一个会话查看详情
    </div>
  </div>
</template>

<style scoped>
.detail {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-width: 0;
  background: var(--bg-panel);
}
.error-banner {
  flex: none;
  padding: 6px 12px;
  font-size: 12px;
  color: #fff;
  background: var(--danger);
}
.tabs {
  flex: none;
  display: flex;
  gap: 2px;
  padding: 6px 10px 0;
  border-bottom: 1px solid var(--border);
}
.tabs button {
  font: inherit;
  font-size: 12.5px;
  color: var(--fg-dim);
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  padding: 6px 12px;
  cursor: pointer;
}
.tabs button.on {
  color: var(--fg);
  border-bottom-color: var(--accent);
}
.kv {
  flex: 1;
  overflow-y: auto;
  padding: 10px 14px;
  font-size: 12.5px;
}
.overview {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.kv-actions {
  flex: none;
  display: flex;
  gap: 6px;
  padding: 8px 14px 0;
}
.act {
  font: inherit;
  font-size: 12px;
  color: var(--fg);
  background: var(--bg-input);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 4px 10px;
  cursor: pointer;
}
.act:hover {
  background: var(--bg-hover);
  border-color: var(--accent);
}
.kv-row {
  display: grid;
  grid-template-columns: 88px 1fr;
  gap: 8px;
  padding: 4px 0;
  border-bottom: 1px solid var(--border-faint);
  word-break: break-all;
}
.k {
  color: var(--fg-dim);
}
.msg {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.msg-head {
  flex: none;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 14px 4px;
}
.msg-title {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--fg);
}
.dim {
  color: var(--fg-dim);
  font-weight: 400;
  font-size: 12px;
}
.copy {
  font: inherit;
  font-size: 11.5px;
  color: var(--fg-dim);
  background: var(--bg-input);
  border: 1px solid var(--border);
  border-radius: 5px;
  padding: 2px 8px;
  cursor: pointer;
}
.copy:hover {
  background: var(--bg-hover);
}
.headers {
  flex: none;
  margin: 0 14px;
  padding: 8px 10px;
  max-height: 40%;
  overflow: auto;
  background: var(--bg-code);
  border: 1px solid var(--border);
  border-radius: 6px;
  font-family: var(--mono);
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
  color: var(--fg);
}
.placeholder {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--fg-dim);
  font-size: 13px;
}
</style>
