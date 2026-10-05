<template>
  <span
    class="status-badge"
    :class="[`status-${type}`, { 'status-pulse': pulse, 'status-minimal': variant === 'minimal' }]"
    role="status"
    aria-live="polite"
    :aria-label="`${label}状态`"
  >
    <span v-if="showDot || variant === 'minimal'" class="status-dot" :class="[`dot-${status}`]"></span>
    <span v-if="icon" class="status-icon">
      <svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" v-html="iconSvg"></svg>
    </span>
    <span class="status-label">{{ label }}</span>
  </span>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  status: {
    type: String,
    default: 'default',
    validator: (value) => [
      'running', 'stopped', 'paused', 'restarting', 'removing', 'created', 'partial',
      'healthy', 'unhealthy', 'health-starting',
      'unused', 'used', 'system', 'custom',
      'update', 'error', 'warning', 'success', 'default',
      'info', 'secondary', 'accent', 'unknown'
    ].includes(value)
  },
  label: {
    type: String,
    default: ''
  },
  type: {
    type: String,
    default: ''
  },
  showDot: {
    type: Boolean,
    default: true
  },
  variant: {
    type: String,
    default: 'filled',
    validator: (value) => ['filled', 'minimal'].includes(value)
  },
  icon: {
    type: String,
    default: ''
  },
  pulse: {
    type: Boolean,
    default: false
  }
})

// 状态配置映射。
// 设计约定：stopped/unused 统一用 default（灰色）而非 danger（红色），
// 红色只留给 unhealthy/error/dead/failed 等真正异常的状态。
const statusConfig = {
  running: { label: '运行中', type: 'success', icon: 'play' },
  healthy: { label: '健康', type: 'success', icon: 'check' },
  unhealthy: { label: '不健康', type: 'danger', icon: 'alert' },
  'health-starting': { label: '检测中', type: 'warning', icon: 'refresh' },
  stopped: { label: '已停止', type: 'default', icon: 'square' },
  paused: { label: '已暂停', type: 'warning', icon: 'pause' },
  restarting: { label: '重启中', type: 'warning', icon: 'refresh' },
  removing: { label: '移除中', type: 'warning', icon: 'refresh' },
  created: { label: '已创建', type: 'primary', icon: 'circle' },
  partial: { label: '部分运行', type: 'warning', icon: 'circle' },
  unused: { label: '未使用', type: 'default', icon: 'circle' },
  used: { label: '使用中', type: 'success', icon: 'check' },
  system: { label: '系统', type: 'warning', icon: 'home' },
  custom: { label: '自定义', type: 'primary', icon: 'settings' },
  update: { label: '有可更新', type: 'primary', icon: 'arrow-up' },
  error: { label: '错误', type: 'danger', icon: 'x' },
  warning: { label: '警告', type: 'warning', icon: 'alert' },
  success: { label: '成功', type: 'success', icon: 'check' },
  default: { label: '默认', type: 'default', icon: 'circle' },
  unknown: { label: '未知', type: 'default', icon: 'circle' }
}

// 图标 SVG
const iconSvgs = {
  play: '<polygon points="5 3 19 12 5 21 5 3"/>',
  square: '<rect x="4" y="4" width="16" height="16" rx="2"/>',
  pause: '<rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/>',
  refresh: '<polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>',
  circle: '<circle cx="12" cy="12" r="10"/>',
  check: '<polyline points="20 6 9 17 4 12"/>',
  home: '<path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><polyline points="9 22 9 12 15 12 15 22"/>',
  settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>',
  'arrow-up': '<line x1="12" y1="19" x2="12" y2="5"/><polyline points="5 12 12 5 19 12"/>',
  x: '<line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>',
  alert: '<path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/>'
}

const config = computed(() => statusConfig[props.status] || statusConfig.default)
const type = computed(() => props.type || config.value.type)
const label = computed(() => props.label || config.value.label)
const iconSvg = computed(() => iconSvgs[props.icon || config.value.icon] || iconSvgs.circle)
</script>

<style scoped>
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  border-radius: 9999px;
  font-size: 0.75rem;
  font-weight: 600;
  line-height: 1;
  white-space: nowrap;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background-color: currentColor;
  box-shadow: 0 0 0 3px color-mix(in srgb, currentColor 18%, transparent);
  flex-shrink: 0;
}

.status-icon {
  display: flex;
  align-items: center;
  justify-content: center;
}

.status-icon svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2.5;
  stroke-linecap: round;
  stroke-linejoin: round;
}

/* 状态类型样式 */
.status-success {
  background-color: var(--color-success-100);
  color: var(--color-success-700);
}

.status-danger {
  background-color: var(--color-danger-100);
  color: var(--color-danger-700);
}

.status-warning {
  background-color: var(--color-warning-100);
  color: var(--color-warning-700);
}

.status-primary {
  background-color: var(--color-primary-100);
  color: var(--color-primary-700);
}

.status-secondary {
  background-color: var(--color-secondary-100);
  color: var(--color-secondary-700);
}

.status-accent {
  background-color: var(--color-accent-100);
  color: var(--color-accent-700);
}

.status-info {
  background-color: var(--color-secondary-100);
  color: var(--color-secondary-700);
}

.status-default {
  background-color: var(--color-gray-100);
  color: var(--color-gray-600);
}

/* === minimal 变体：表格/列表场景的极简风（圆点 + 文字，无底色） === */
/* 与 filled 变体并存：filled 用于卡片/详情头部（强调），minimal 用于表格行（克制）。 */
.status-minimal {
  background: transparent;
  padding: 3px;
  border-radius: 0;
  font-weight: 400;
  color: var(--text-secondary);
}

.status-minimal .status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background-color: var(--text-tertiary);
  box-shadow: 0 0 0 3px var(--border-subtle);
  /* 覆盖 filled 模式里 dot 用 currentColor 的设定 */
}

/* minimal 圆点颜色映射（按 status，不按 type）。
   约定：running/used 绿、stopped/unused 灰、unhealthy/error 红、paused/restarting/partial 黄、update 主色 */
.status-minimal .dot-running,
.status-minimal .dot-used,
.status-minimal .dot-healthy,
.status-minimal .dot-success {
  background-color: var(--color-success-500);
  box-shadow: 0 0 0 3px var(--ring-success);
}

.status-minimal .dot-stopped,
.status-minimal .dot-unused,
.status-minimal .dot-default,
.status-minimal .dot-unknown {
  background-color: var(--text-tertiary);
  box-shadow: 0 0 0 3px var(--border-subtle);
}

.status-minimal .dot-created {
  background-color: var(--color-primary-500, var(--color-primary));
  box-shadow: 0 0 0 3px var(--ring-primary, color-mix(in srgb, var(--color-primary, #3b82f6) 18%, transparent));
}

.status-minimal .dot-unhealthy,
.status-minimal .dot-error {
  background-color: var(--color-danger-500);
  box-shadow: 0 0 0 3px var(--ring-danger);
}

.status-minimal .dot-paused,
.status-minimal .dot-restarting,
.status-minimal .dot-removing,
.status-minimal .dot-partial,
.status-minimal .dot-warning,
.status-minimal .dot-system,
.status-minimal .dot-health-starting {
  background-color: var(--color-warning-500);
  box-shadow: 0 0 0 3px var(--ring-warning);
}

.status-minimal .dot-update {
  background-color: var(--color-primary-500, var(--color-primary));
  box-shadow: 0 0 0 3px var(--ring-primary, color-mix(in srgb, var(--color-primary, #3b82f6) 18%, transparent));
}

/* 脉冲动画 */
.status-pulse .status-dot {
  animation: pulse-dot 2s ease-in-out infinite;
}

@keyframes pulse-dot {
  0%, 100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.6;
    transform: scale(1.2);
  }
}

/* Dark mode - 使用 color-mix 变量 */
html.dark .status-success {
  background-color: var(--color-success-500-20);
}

html.dark .status-danger {
  background-color: var(--color-danger-500-20);
}

html.dark .status-warning {
  background-color: var(--color-warning-500-15);
}

html.dark .status-primary {
  background-color: var(--color-primary-500-20);
}

html.dark .status-secondary,
html.dark .status-info {
  background-color: var(--color-secondary-500-20);
}

html.dark .status-accent {
  background-color: var(--color-accent-500-20);
}

html.dark .status-default {
  background-color: var(--color-gray-500-20);
}
</style>
