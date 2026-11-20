<script setup lang="ts">
</script>

<template>
  <header class="titlebar">
    <span class="title">HTTPX Debugger</span>
    <span class="subtitle">HTTP/HTTPS 抓包工具</span>
    <span class="spacer" />
  </header>
</template>

<style scoped>
.titlebar {
  height: 38px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 12px;
  /* 右侧预留 Windows 原生窗口按钮区（最小化/最大化/关闭 ≈ 138px） */
  padding-right: 140px;
  background: var(--bg-chrome);
  border-bottom: 1px solid var(--border);
  -webkit-app-region: drag;
  user-select: none;
}
.title {
  font-size: 13px;
  font-weight: 600;
  color: var(--fg);
}
.subtitle {
  font-size: 12px;
  color: var(--fg-dim);
}
.spacer {
  flex: 1;
}
</style>
