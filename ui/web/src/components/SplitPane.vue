<script setup lang="ts">
import { ref } from 'vue'

const left = ref(46) // 左栏宽度百分比
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
    <section class="pane left" :style="{ width: left + '%' }">
      <slot name="left" />
    </section>
    <div class="divider" @mousedown="startDrag" />
    <section class="pane right">
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
