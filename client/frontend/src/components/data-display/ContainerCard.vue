<template>
  <div class="container-card" :class="[statusClass]" @click="$emit('select')">
    <!-- 名称 -->
    <div class="card-header">
      <h3 class="container-name" :title="container.Names?.[0] || container.Id">{{ displayName }}</h3>
    </div>

    <!-- 状态 -->
    <div class="status-row">
      <span class="status-badge" :class="containerState">{{ statusLabel }}</span>
      <span v-if="hasUpdate" class="update-badge">
        <DynamicIcon name="cloud-download" :size="12" />
        可更新
      </span>
    </div>

    <!-- 信息列表 -->
    <div class="container-info">
      <div class="info-row">
        <span class="info-label">镜像</span>
        <span class="info-value" :title="container.Image">{{ shortImage }}</span>
      </div>
      <div class="info-row">
        <span class="info-label">端口</span>
        <span class="info-value">{{ portsText }}</span>
      </div>
      <div class="info-row">
        <span class="info-label">运行时间</span>
        <span class="info-value">{{ uptimeText }}</span>
      </div>
    </div>

    <!-- 操作按钮 -->
    <div v-if="showActions" class="card-actions">
      <button 
        v-if="canLifecycle && isRunning"
        class="action-btn stop"
        title="停止"
      @click.stop="$emit('stop')"
      >
        <DynamicIcon name="stop" :size="14" />
      </button>
      <button 
        v-if="canLifecycle && !isRunning"
        class="action-btn success"
        title="启动"
      @click.stop="$emit('start')"
      >
        <DynamicIcon name="play" :size="14" />
      </button>
      <button v-if="canLifecycle" class="action-btn warning" title="重启" @click.stop="$emit('restart')">
        <DynamicIcon name="restart" :size="14" />
      </button>
      <button v-if="canLogs" class="action-btn info" title="日志" @click.stop="$emit('logs')">
        <DynamicIcon name="file-text" :size="14" />
      </button>
      <button v-if="!isRemoteEnvironment" class="action-btn terminal" title="终端" @click.stop="$emit('terminal')">
        <DynamicIcon name="terminal" :size="14" />
      </button>
      <button v-if="!isRemoteEnvironment" class="action-btn danger" title="删除" @click.stop="$emit('remove')">
        <DynamicIcon name="delete" :size="14" />
      </button>
      <button
        v-if="!isRemoteEnvironment"
        class="action-btn more"
        title="更多"
        :class="{ 'is-active': moreMenuOpen }"
        @click.stop="toggleMoreMenu"
      >
        <DynamicIcon name="more-vertical" :size="14" />
      </button>
    </div>

    <AnchoredMenu
      v-if="!isRemoteEnvironment"
      :open="moreMenuOpen"
      :anchor="moreMenuAnchor"
      @update:open="handleMoreMenuOpenChange"
    >
      <button type="button" role="menuitem" @click="handleForceStopFromMenu">
        <DynamicIcon name="zap" :size="14" />
        强制停止
      </button>
    </AnchoredMenu>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import AnchoredMenu from '@/components/ui/AnchoredMenu.vue'
import { formatDockerImageReference } from '@/utils/format.js'
import { containerDisplayStatus } from '@/utils/resourceStatus.js'
import { remoteCapabilityAllowed, useEnvironmentContext } from '@/composables/useEnvironmentContext.js'

const props = defineProps({
  container: {
    type: Object,
    required: true
  }
})

const emit = defineEmits(['select', 'start', 'stop', 'restart', 'logs', 'terminal', 'remove', 'force-stop'])

const environmentContext = useEnvironmentContext()
const isRemoteEnvironment = computed(() => environmentContext.isRemoteEnvironment?.value === true)
const canLifecycle = computed(() => !isRemoteEnvironment.value || remoteCapabilityAllowed(environmentContext.environment?.value, 'container.manage'))
const canLogs = computed(() => !isRemoteEnvironment.value || remoteCapabilityAllowed(environmentContext.environment?.value, 'container.logs'))
const showActions = computed(() => canLifecycle.value || canLogs.value || !isRemoteEnvironment.value)

const moreMenuOpen = ref(false)
const moreMenuAnchor = ref(null)

function toggleMoreMenu(event) {
  if (moreMenuOpen.value) {
    closeMoreMenu()
    return
  }
  moreMenuAnchor.value = event?.currentTarget || null
  moreMenuOpen.value = true
}

function closeMoreMenu() {
  moreMenuOpen.value = false
  moreMenuAnchor.value = null
}

function handleMoreMenuOpenChange(open) {
  if (!open) closeMoreMenu()
}

function handleForceStopFromMenu() {
  closeMoreMenu()
  emit('force-stop')
}

// 显示名称
const displayName = computed(() => {
  const name = props.container.Names?.[0] || ''
  return name.replace(/^\//, '') || props.container.Id?.substring(0, 12) || '未知'
})

// 镜像名
const shortImage = computed(() => {
  const image = formatDockerImageReference(props.container.Image)
  if (image.length > 30) {
    return image.substring(0, 27) + '...'
  }
  return image || '<none>'
})

const hasUpdate = computed(() => Boolean(
  props.container?.UpdateAvailable ?? props.container?.updateAvailable
))

// 状态判断
const isRunning = computed(() => {
  return props.container.State === 'running'
})

const containerState = computed(() => {
  return containerDisplayStatus(props.container)
})

const statusClass = computed(() => {
  return `container-${containerState.value}`
})

// 状态标签
const statusLabel = computed(() => {
  const labels = {
    'running': '运行中',
    'unhealthy': '不健康',
    'health-starting': '检测中',
    'stopped': '已停止',
    'paused': '已暂停',
    'restarting': '重启中',
    'removing': '移除中',
    'created': '已创建',
    'error': '故障',
    'unknown': '未知'
  }
  return labels[containerState.value] || containerState.value
})

// 端口信息
const portsText = computed(() => {
  const ports = props.container.Ports || []
  if (ports.length === 0) return '无映射'
  const publicPorts = dedupePublicPorts(ports)
  if (publicPorts.length === 0) return `${ports.length} 个端口`
  return `${publicPorts.length} 个公开端口`
})

function dedupePublicPorts(ports) {
  const seen = new Set()
  const result = []
  for (const port of ports) {
    if (!port.PublicPort) continue
    const key = `${port.PublicPort}:${port.PrivatePort}:${port.Type || 'tcp'}`
    if (seen.has(key)) continue
    seen.add(key)
    result.push(port)
  }
  return result
}

// 运行时间
const uptimeText = computed(() => {
  const status = props.container.Status || ''
  if (status.startsWith('Up ')) {
    return status.substring(3).replace(' ago', '')
  }
  if (status.startsWith('Exited')) {
    const match = status.match(/Exited \(\d+\) (.+)/)
    return match ? match[1] : '已停止'
  }
  return status || '-'
})

</script>

<style scoped>
.container-card {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  cursor: pointer;
  position: relative;
  overflow: hidden;
  min-height: 236px;
  aspect-ratio: auto;
}

.container-card:hover {
  border-color: var(--border-medium);
  transform: none;
  box-shadow: inset 0 0 0 1px var(--border-subtle);
}

.container-card:active {
  transform: translateY(0) scale(0.99);
  transition-duration: 0.1s;
}

.container-stopped {
  opacity: 0.9;
}

.container-stopped .container-name {
  color: var(--text-tertiary);
}

/* 运行中的容器 - 主题色渐变 */
.container-running {
  background: linear-gradient(135deg, var(--bg-elevated) 0%, var(--color-primary-500-5) 100%);
}

.container-running:hover {
  box-shadow: inset 0 0 0 1px var(--border-subtle);
}

/* 头部 */
.card-header {
  margin-bottom: 6px;
}

.container-name {
  font-size: 0.9375rem;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 状态行 */
.status-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  flex-wrap: wrap;
}

.status-badge {
  display: inline-block;
  padding: 3px 8px;
  border-radius: 6px;
  font-size: 0.6875rem;
  font-weight: 500;
}

.update-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border-radius: 6px;
  background: var(--color-warning-100);
  color: var(--color-warning-700);
  font-size: 0.6875rem;
  font-weight: 600;
}

.status-badge.running {
  background: var(--color-success-100);
  color: var(--color-success-700);
}

.status-badge.unhealthy {
  background: var(--color-danger-100);
  color: var(--color-danger-700);
}

.status-badge.health-starting {
  background: var(--color-warning-100);
  color: var(--color-warning-700);
}

.status-badge.stopped {
  background: var(--bg-tertiary);
  color: var(--text-tertiary);
}

.status-badge.created {
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

.status-badge.paused {
  background: var(--color-warning-100);
  color: var(--color-warning-700);
}

.status-badge.restarting,
.status-badge.removing {
  background: var(--color-warning-100);
  color: var(--color-warning-700);
}

.status-badge.error {
  background: var(--color-danger-100);
  color: var(--color-danger-700);
}

/* 信息列表 */
.container-info {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 8px;
  flex: 1;
}

.info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.info-label {
  font-size: 0.6875rem;
  color: var(--text-tertiary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.info-value {
  font-size: 0.8125rem;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 140px;
}

.info-value.mono {
  font-family: 'JetBrains Mono', monospace;
}

/* 操作按钮 */
.card-actions {
  display: flex;
  gap: 6px;
  padding-top: 8px;
  padding-bottom: 2px;
  margin-top: auto;
  border-top: 1px solid var(--border-subtle);
  flex-shrink: 0;
}

.action-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  background: var(--bg-tertiary);
  border: none;
  border-radius: 8px;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

/* 启动 - 绿色 */
.action-btn.success {
  background: var(--color-success-100);
  color: var(--color-success-600);
}

.action-btn.success svg {
  stroke: var(--color-success-600);
}

.action-btn.success:hover {
  background: var(--color-success-200);
  color: var(--color-success-700);
}

.action-btn.success:hover svg {
  stroke: var(--color-success-700);
}

/* 停止 - 粉红色 */
.action-btn.stop {
  background: var(--color-danger-100);
  color: var(--color-danger-600);
}

.action-btn.stop svg {
  stroke: var(--color-danger-600);
}

.action-btn.stop:hover {
  background: var(--color-danger-200);
  color: var(--color-danger-700);
}

.action-btn.stop:hover svg {
  stroke: var(--color-danger-700);
}

/* 重启 - 黄色 */
.action-btn.warning {
  background: var(--color-warning-100);
  color: var(--color-warning-600);
}

.action-btn.warning:hover {
  background: var(--color-warning-200);
  color: var(--color-warning-700);
}

/* 查看/日志 - 浅蓝色 */
.action-btn.info {
  background: var(--color-primary-100);
  color: var(--color-primary-600);
}

.action-btn.info svg {
  stroke: #2563eb;
}

.action-btn.info:hover {
  background: var(--color-primary-200);
  color: var(--color-primary-800);
}

.action-btn.info:hover svg {
  stroke: #1d4ed8;
}

/* 终端 - 浅黑色 */
.action-btn.terminal {
  background: var(--bg-tertiary);
  color: var(--text-secondary);
}

.action-btn.terminal:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

/* 更多菜单 - 浅黑色 */
.action-btn.more {
  background: var(--bg-tertiary);
  color: var(--text-secondary);
}

.action-btn.more:hover,
.action-btn.more.is-active {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

/* 删除 - 粉红色 */
.action-btn.danger {
  background: var(--color-danger-100);
  color: var(--color-danger-600);
}

.action-btn.danger svg {
  stroke: var(--color-danger-600);
}

.action-btn.danger:hover {
  background: var(--color-danger-200);
  color: var(--color-danger-700);
}

.action-btn.danger:hover svg {
  stroke: var(--color-danger-700);
}

.action-btn svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}
</style>
