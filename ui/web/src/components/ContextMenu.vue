<script setup lang="ts">
import { watch, nextTick, ref } from 'vue'

export interface MenuItem {
  label: string
  onClick: () => void
  disabled?: boolean
}

const props = defineProps<{
  x: number
  y: number
  items: MenuItem[]
}>()
const emit = defineEmits<{ (e: 'close'): void }>()

const el = ref<HTMLElement | null>(null)

// 面板出现后校正位置，防止超出视口
watch(
  () => [props.x, props.y],
  async () => {
    await nextTick()
    const node = el.value
    if (!node) return
    const r = node.getBoundingClientRect()
    if (r.right > window.innerWidth) node.style.left = `${props.x - r.width}px`
    if (r.bottom > window.innerHeight) node.style.top = `${props.y - r.height}px`
  },
)
</script>

<template>
  <div class="ctx-layer" @mousedown.self="emit('close')" @contextmenu.prevent="emit('close')">
    <div ref="el" class="ctx-menu" :style="{ left: x + 'px', top: y + 'px' }">
      <button
        v-for="(it, i) in items"
        :key="i"
        class="item"
        :disabled="it.disabled"
        @click="
          () => {
            emit('close')
            it.onClick()
          }
        "
      >
        {{ it.label }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.ctx-layer {
  position: fixed;
  inset: 0;
  z-index: 60;
}
.ctx-menu {
  position: fixed;
  min-width: 160px;
  display: flex;
  flex-direction: column;
  padding: 4px;
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.45);
}
.item {
  font: inherit;
  font-size: 12.5px;
  text-align: left;
  color: var(--fg);
  background: none;
  border: none;
  border-radius: 5px;
  padding: 6px 10px;
  cursor: pointer;
}
.item:hover:not(:disabled) {
  background: var(--bg-hover);
}
.item:disabled {
  opacity: 0.4;
  cursor: default;
}
</style>
