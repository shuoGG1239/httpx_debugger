<script setup lang="ts">
import { ref } from 'vue'

// 拖拽比例持久化到 localStorage（组件在无右栏/有右栏切换时重建也能恢复）
const saved = Number(localStorage.getItem('httpx-split-left') ?? 46)
const left = ref(Number.isFinite(saved) && saved >= 18 && saved <= 80 ? saved : 46)
const dragging = ref(false)

let onMove: ((e: MouseEvent) => void) | null = null
let onUp: (() => void) | null = null

function startDrag(e: MouseEvent): void {
  const container = (e.currentTarget as HTMLElement).parentElement
  if (!container) return
  dragging.value = true
  const rect = container.getBoundingClientRect()
  onMove = (ev: MouseEvent) => {
    const pct = ((ev.clientX - rect.left) / rect.width) * 100
    left.value = Math.min(80, Math.max(18, pct))
    localStorage.setItem('httpx-split-left', String(left.value))
  }
  onUp = () => {
    dragging.value = false
    if (onMove) window.removeEventListener('mousemove', onMove)
    if (onUp) window.removeEventListener('mouseup', onUp)
    onMove = null
    onUp = null
  }
  window.addEventListener('mousemove', onMove)
  window.addEventListener('mouseup', onUp)
}
</script>

<template>
  <div class="split" :class="{ dragging }">
    <section class="pane left" :style="{ width: $slots.right ? left + '%' : '100%' }">
      <slot name="left" />
    </section>
    <div v-if="$slots.right" class="divider" @mousedown="startDrag" />
    <section v-if="$slots.right" class="pane right">
      <slot name="right" />
    </section>
  </div>
</template>

<style scoped>
.split {
  flex: 1;
  display: flex;
  min-height: 0;
}
.pane {
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}
.left {
  flex: none;
}
.right {
  flex: 1;
}
.divider {
  flex: none;
  width: 5px;
  cursor: col-resize;
  background: transparent;
  position: relative;
}
.divider:hover,
.split.dragging .divider {
  background: var(--border);
}
</style>
