<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Header, InterceptEntry, ModifiedRequest, ModifiedResponse } from '@/types'
import { b64ToBytes, bytesToText, hexDump, isBinary, textToB64 } from '@/utils/bytes'

const props = defineProps<{ entry: InterceptEntry | null }>()
const emit = defineEmits<{
  (e: 'resolve', action: 'continue' | 'drop', payload?: ModifiedRequest | ModifiedResponse): void
}>()

const method = ref('GET')
const url = ref('')
const status = ref(200)
const statusText = ref('')
const headers = ref<Header[]>([])
const bodyText = ref('')
const bodyBinary = ref(false)

watch(
  () => props.entry,
  (e) => {
    if (!e) return
    headers.value = e.headers.map((h) => ({ ...h }))
    const bytes = b64ToBytes(e.body)
    bodyBinary.value = isBinary(bytes)
    bodyText.value = bodyBinary.value ? '' : bytesToText(bytes)
    if (e.kind === 'request') {
      method.value = e.method
      url.value = e.url
    } else {
      status.value = e.status
      statusText.value = e.statusText
    }
  },
  { immediate: true },
)

const title = computed(() => (props.entry ? `拦截${props.entry.kind === 'request' ? '请求' : '响应'} · ${props.entry.id}` : ''))
const subtitle = computed(() => {
  const e = props.entry
  if (!e) return ''
  return e.kind === 'request'
    ? `${e.method} ${e.url}`
    : `${e.status} ${e.statusText} · ${e.headers.find((h) => /^content-type$/i.test(h.name))?.value ?? ''}`
})

function addHeader(): void {
  headers.value.push({ name: '', value: '' })
}
function removeHeader(i: number): void {
  headers.value.splice(i, 1)
}

function buildPayload(): ModifiedRequest | ModifiedResponse | undefined {
  const e = props.entry
  if (!e) return undefined
  const body = bodyBinary.value ? e.body : textToB64(bodyText.value)
  if (e.kind === 'request') {
    return { method: method.value || e.method, url: url.value || e.url, headers: headers.value, body }
  }
  return {
    status: status.value || e.status,
    statusText: statusText.value || e.statusText,
    headers: headers.value,
    body,
  }
}
</script>

<template>
  <div class="overlay">
    <div class="modal" @click.stop>
      <header class="head">
        <span class="title">{{ title }}</span>
        <span class="hint">超时（60s）后自动放行原包</span>
      </header>
      <div class="sub">{{ subtitle }}</div>

      <template v-if="entry?.kind === 'request'">
        <div class="row">
          <label>Method</label>
          <input v-model="method" class="input method" spellcheck="false" />
          <label class="url-label">URL</label>
          <input v-model="url" class="input grow" spellcheck="false" />
        </div>
      </template>
      <template v-else>
        <div class="row">
          <label>Status</label>
          <input v-model.number="status" type="number" class="input method" />
          <input v-model="statusText" class="input grow" spellcheck="false" />
        </div>
      </template>

      <div class="section-title">
        Headers
        <button class="mini" @click="addHeader">+ 添加</button>
      </div>
      <div class="headers">
        <div v-for="(h, i) in headers" :key="i" class="hrow">
          <input v-model="h.name" class="input hname" placeholder="name" spellcheck="false" />
          <input v-model="h.value" class="input grow" placeholder="value" spellcheck="false" />
          <button class="mini danger" @click="removeHeader(i)">✕</button>
        </div>
        <div v-if="headers.length === 0" class="empty-h">（无头）</div>
      </div>

      <div class="section-title">
        Body
        <span v-if="bodyBinary" class="hint">二进制，不可编辑（原样透传）</span>
      </div>
      <textarea
        v-if="!bodyBinary"
        v-model="bodyText"
        class="body"
        spellcheck="false"
        placeholder="（空）"
      ></textarea>
      <pre v-else class="body hex">{{ hexDump(b64ToBytes(entry!.body)) }}</pre>

      <footer class="foot">
        <button class="btn drop" @click="emit('resolve', 'drop')">丢弃</button>
        <span class="spacer" />
        <button class="btn" @click="emit('resolve', 'continue')">原样继续</button>
        <button class="btn primary" @click="emit('resolve', 'continue', buildPayload())">继续并应用修改</button>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  background: rgba(8, 10, 13, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 50;
}
.modal {
  width: min(760px, 92vw);
  max-height: 86vh;
  display: flex;
  flex-direction: column;
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: 10px;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.5);
  padding: 14px 16px;
  gap: 8px;
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.title {
  font-size: 13.5px;
  font-weight: 700;
  color: var(--fg);
}
.sub {
  font-size: 12px;
  color: var(--fg-dim);
  word-break: break-all;
}
.hint {
  font-size: 11.5px;
  color: var(--fg-dim);
}
.row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.row label {
  font-size: 12px;
  color: var(--fg-dim);
  flex: none;
}
.url-label {
  margin-left: 8px;
}
.input {
  font: inherit;
  font-size: 12.5px;
  color: var(--fg);
  background: var(--bg-input);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 5px 8px;
  outline: none;
}
.input:focus {
  border-color: var(--accent);
}
.method {
  width: 84px;
  flex: none;
}
.grow {
  flex: 1;
}
.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  font-weight: 600;
  color: var(--fg);
  margin-top: 4px;
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
.mini.danger:hover {
  color: var(--danger);
  border-color: var(--danger);
}
.headers {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 180px;
  overflow-y: auto;
  padding: 2px;
}
.hrow {
  display: flex;
  gap: 6px;
  align-items: center;
}
.hname {
  width: 220px;
  flex: none;
}
.empty-h {
  font-size: 12px;
  color: var(--fg-dim);
  padding: 4px 0;
}
.body {
  flex: 1;
  min-height: 180px;
  max-height: 40vh;
  resize: vertical;
  font-family: var(--mono);
  font-size: 12px;
  line-height: 1.5;
  color: var(--fg);
  background: var(--bg-code);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 8px 10px;
  outline: none;
  white-space: pre-wrap;
  word-break: break-all;
}
.body.hex {
  margin: 0;
  overflow: auto;
}
.foot {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 4px;
}
.spacer {
  flex: 1;
}
.btn {
  font: inherit;
  font-size: 12.5px;
  color: var(--fg);
  background: var(--bg-input);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 6px 14px;
  cursor: pointer;
}
.btn:hover {
  background: var(--bg-hover);
}
.btn.drop {
  color: var(--danger);
  border-color: var(--danger);
}
.btn.primary {
  background: var(--accent);
  border-color: var(--accent);
  color: #0b0e11;
  font-weight: 600;
}
</style>
