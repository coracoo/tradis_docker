<template>
  <div
    v-if="links.length"
    class="ports-cell ports-cell-linkable"
    :tabindex="links.length > 1 ? 0 : undefined"
    :aria-label="links.length > 1 ? `${links.length} 个公开端口，按 Tab 查看` : undefined"
  >
    <a
      v-if="links.length === 1"
      :href="links[0].url"
      target="_blank"
      rel="noopener noreferrer"
      class="port-jump-link"
      :title="`打开 ${links[0].label}`"
    >{{ links[0].label }}</a>
    <template v-else>
      <span class="port-summary">{{ links[0].label }} +{{ links.length - 1 }}</span>
      <div class="ports-popover">
        <div class="ports-popover-title">点击端口跳转内网地址</div>
        <a
          v-for="link in links"
          :key="link.url"
          :href="link.url"
          target="_blank"
          rel="noopener noreferrer"
          class="port-jump-link"
        >{{ link.label }} → {{ link.url }}</a>
      </div>
    </template>
  </div>
  <span v-else class="ports-cell" :title="fallbackTitle">{{ fallbackText }}</span>
</template>

<script setup>
import { computed } from 'vue'
import { buildPortLinks } from '../utils/portLinks.js'

// 容器端口单元格：单个公开端口直接渲染内网链接，多个端口悬浮浮层逐个跳转，
// 无公开端口时退回纯文本（formatPorts/formatPortsTitle 的结果）。
const props = defineProps({
  ports: { type: Array, default: () => [] },
  fallbackText: { type: String, default: '-' },
  fallbackTitle: { type: String, default: '' }
})

const links = computed(() => buildPortLinks(props.ports))
</script>

<style scoped>
.ports-cell-linkable {
  position: relative;
  cursor: pointer;
}

.port-jump-link {
  color: var(--primary-color, #3b82f6);
  text-decoration: none;
}

.port-jump-link:hover {
  text-decoration: underline;
}

.port-summary {
  color: var(--primary-color, #3b82f6);
}

.ports-popover {
  position: absolute;
  top: 100%;
  left: 0;
  z-index: 40;
  min-width: 220px;
  padding: 8px 10px;
  border-radius: 8px;
  background: var(--bg-elevated, #1e293b);
  border: 1px solid var(--border-subtle, rgba(148, 163, 184, 0.3));
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
  opacity: 0;
  pointer-events: none;
  transform: scale(var(--motion-tooltip-scale));
  transform-origin: 0 0;
  transition:
    opacity var(--motion-tooltip-out) var(--motion-ease-out),
    transform var(--motion-tooltip-out) var(--motion-ease-out),
    visibility 0s linear var(--motion-tooltip-out);
  visibility: hidden;
  white-space: nowrap;
}

.ports-cell-linkable:hover .ports-popover,
.ports-cell-linkable:focus-within .ports-popover {
  opacity: 1;
  pointer-events: auto;
  transform: scale(1);
  transition:
    opacity var(--motion-tooltip-in) var(--motion-ease-out) var(--motion-tooltip-delay),
    transform var(--motion-tooltip-in) var(--motion-ease-out) var(--motion-tooltip-delay),
    visibility 0s linear var(--motion-tooltip-delay);
  visibility: visible;
}

.ports-popover-title {
  font-size: 11px;
  opacity: 0.6;
  margin-bottom: 4px;
}

.ports-popover .port-jump-link {
  display: block;
  padding: 2px 0;
  font-size: 12px;
}

.ports-cell {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
