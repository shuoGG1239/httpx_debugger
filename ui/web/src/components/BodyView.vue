<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { bytesToText, hexDump, isBinary } from '@/utils/bytes'
import { copyText } from '@/utils/clipboard'

const props = defineProps<{
  bytes: Uint8Array
  contentType?: string
  truncated?: boolean
}>()

const pretty = ref(true)
const text = computed(() => bytesToText(props.bytes))
const binary = computed(() => isBinary(props.bytes))
const looksJson = computed(() => {
  if (binary.value) return false
  const t = text.value.trim()
  if (!t) return false
  if (props.contentType?.includes('json')) return true
  return t.startsWith('{') || t.startsWith('[')
})

watch(
  () => props.bytes,
  () => {
    pretty.value = true
  },
)

const displayText = computed(() => {
  if (!looksJson.value || !pretty.value) return text.value
  try {
    return JSON.stringify(JSON.parse(text.value), null, 2)
  } catch {
    return text.value
  }
})
</script>

<template>
  <div class="body">
    <div class="bar">
      <span class="kind" v-if="binary">二进制</span>
      <span class="kind" v-else-if="looksJson">JSON</span>
      <span class="kind" v-else-if="bytes.length === 0">空</span>
      <span class="kind" v-else>文本</span>
      <span class="dim">{{ bytes.length }} B{{ truncated ? '（超出上限已截断）' : '' }}</span>
      <span class="spacer" />
      <button v-if="looksJson" class="mini" @click="pretty = !pretty">{{ pretty ? '压缩' : '美化' }}</button>
      <button class="mini" @click="copyText(binary ? hexDump(bytes) : displayText)">复制</button>
    </div>
    <pre v-if="bytes.length > 0" class="content">{{ binary ? hexDump(bytes) : displayText }}</pre>
    <div v-else class="empty">（空）</div>
  </div>
</template>

<style scoped>
.body {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  padding: 0 14px 12px;
}
.bar {
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  padding-bottom: 4px;
  font-size: 11.5px;
  color: var(--fg-dim);
}
.kind {
  color: var(--accent);
  font-weight: 600;
}
.spacer {
  flex: 1;
}
.mini {
  font: inherit;
  font-size: 11.5px;
  color: var(--fg-dim);
  background: var(--bg-input);
  border: 1px solid var(--border);
  border-radius: 5px;
  padding: 2px 8px;
  cursor: pointer;
}
.mini:hover {
  background: var(--bg-hover);
}
.content {
  flex: 1;
  margin: 0;
  padding: 10px;
  overflow: auto;
  background: var(--bg-code);
  border: 1px solid var(--border);
  border-radius: 6px;
  font-family: var(--mono);
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-all;
  color: var(--fg);
}
.empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--fg-dim);
  font-size: 12.5px;
  background: var(--bg-code);
  border: 1px solid var(--border);
  border-radius: 6px;
}
</style>
