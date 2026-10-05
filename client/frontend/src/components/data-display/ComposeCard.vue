<template>
  <div 
    class="compose-card"
    :class="[statusClass, { 'is-selected': selected, 'is-self': project.isSelf, 'is-standalone': isStandalone }]"
    @click="$emit('click', project)"
  >
    <!-- 名称 -->
    <div class="card-header">
      <div class="name-wrapper">
        <!-- Compose 项目图标 -->
        <DynamicIcon v-if="!isStandalone" class="type-icon" name="folder" :size="16" />
        <!-- 独立容器图标 -->
        <DynamicIcon v-else class="type-icon" name="container" :size="16" />
        <span class="project-title-copy">
          <h3 class="project-name" :title="project.name">{{ project.name }}</h3>
          <small v-if="project.remark && !isStandalone" class="project-remark" :title="project.remark">{{ project.remark }}</small>
        </span>
        <button
          v-if="!isStandalone && !isRemoteEnvironment"
          type="button"
          data-remote-write
          class="remark-edit-btn"
          title="编辑备注"
          @click.stop="$emit('remark', project)"
        >
          <DynamicIcon name="edit" :size="13" />
        </button>
      </div>
      <span v-if="project.isSelf" class="self-badge">达呗</span>
    </div>

    <!-- 状态行 -->
    <div class="status-row">
      <span class="status-badge" :class="status">{{ statusLabel }}</span>
      <span v-if="project.updateAvailable && !project.isSelf && !isStandalone" class="update-badge">
        可更新{{ project.updateCount > 0 ? ` ${project.updateCount}` : '' }}
      </span>
      <span
        v-if="project.gitSource && !isStandalone"
        class="git-badge"
        :title="`${project.gitSource.repoUrl}${project.gitSource.branch ? ` # ${project.gitSource.branch}` : ''}${project.gitSource.commitHash ? ` @ ${project.gitSource.commitHash.slice(0, 8)}` : ''}`"
      >
        Git
      </span>
      <span class="type-tag" :class="isStandalone ? 'standalone' : 'compose'">{{ typeLabel }}</span>
      <span class="stats-text">{{ runningCount }}/{{ totalCount }} 运行</span>
    </div>

    <!-- 信息列表 -->
    <div class="compose-info">
      <div class="info-row">
        <span class="info-label">{{ isStandalone ? '镜像' : '路径' }}</span>
        <span class="info-value" :title="isStandalone ? containerImage : project.path">{{ isStandalone ? shortImage : shortPath }}</span>
      </div>
      <div class="info-row">
        <span class="info-label">资源</span>
        <span class="info-value">{{ resourceText }}</span>
      </div>
    </div>

    <!-- 操作按钮 -->
    <div v-if="!project.isSelf && showActions" class="card-actions" :class="{ 'is-processing': processing }">
      <button 
        v-if="canLifecycle && status === 'running'"
        v-ripple
        class="action-btn stop"
        title="停止"
        :disabled="processing"
        @click.stop="$emit('stop', project)"
      >
        <span v-if="isActionProcessing('stop')" class="btn-spinner"></span>
        <DynamicIcon v-else name="stop" :size="14" />
      </button>
      <button 
        v-if="canLifecycle && status !== 'running'"
        v-ripple
        class="action-btn success"
        title="启动"
        :disabled="processing"
        @click.stop="$emit('start', project)"
      >
        <span v-if="isActionProcessing('start')" class="btn-spinner"></span>
        <DynamicIcon v-else name="play" :size="14" />
      </button>
      <button v-if="canLifecycle" v-ripple class="action-btn warning" title="重启" :disabled="processing" @click.stop="$emit('restart', project)">
        <span v-if="isActionProcessing('restart')" class="btn-spinner"></span>
        <DynamicIcon v-else name="restart" :size="14" />
      </button>
      <!-- 更新按钮 - 只在有可更新时显示 -->
      <button 
        v-if="!isRemoteEnvironment && !isStandalone && project.updateAvailable"
        v-ripple
        class="action-btn update" 
        title="更新镜像"
        :disabled="processing"
        @click.stop="$emit('update', project)"
      >
        <span v-if="isActionProcessing('update')" class="btn-spinner"></span>
        <DynamicIcon v-else name="cloud-download" :size="14" />
      </button>
      <button
        v-if="!isRemoteEnvironment && !isStandalone && project.gitSource"
        v-ripple
        class="action-btn git-sync"
        title="同步 Git 仓库（不自动部署）"
        :disabled="processing"
        @click.stop="$emit('sync-git', project)"
      >
        <span v-if="isActionProcessing('git-sync')" class="btn-spinner"></span>
        <DynamicIcon v-else name="git-branch" :size="14" />
      </button>
      <button v-if="canLogs" v-ripple class="action-btn info" :title="isStandalone ? '容器日志' : '项目日志'" :disabled="processing" @click.stop="$emit('logs', project)">
        <DynamicIcon name="file-text" :size="14" />
      </button>
      <!-- 编辑按钮只对 Compose 项目显示 -->
      <button v-if="!isRemoteEnvironment && !isStandalone" v-ripple class="action-btn info" title="编辑" :disabled="processing" @click.stop="$emit('edit', project)">
        <span v-if="isActionProcessing('edit')" class="btn-spinner"></span>
        <DynamicIcon v-else name="edit" :size="14" />
      </button>
      <!-- 终端按钮只对独立容器显示 -->
      <button v-if="!isRemoteEnvironment && isStandalone" v-ripple class="action-btn terminal" title="终端" :disabled="processing" @click.stop="$emit('terminal', project)">
        <span v-if="isActionProcessing('terminal')" class="btn-spinner"></span>
        <DynamicIcon v-else name="terminal" :size="14" />
      </button>
      <!-- 清除按钮只对 Compose 项目显示（保留文件，只清除容器） -->
      <button v-if="!isRemoteEnvironment && !isStandalone" v-ripple class="action-btn clear" title="清除容器（保留文件）" :disabled="processing" @click.stop="$emit('down', project)">
        <span v-if="isActionProcessing('down')" class="btn-spinner"></span>
        <DynamicIcon v-else name="eraser" :size="14" />
      </button>
      <!-- 销毁按钮只对 Compose 项目显示，独立容器显示删除 -->
      <button v-if="!isRemoteEnvironment && !isStandalone" v-ripple class="action-btn danger" title="销毁" :disabled="processing" @click.stop="$emit('destroy', project)">
        <span v-if="isActionProcessing('destroy')" class="btn-spinner"></span>
        <DynamicIcon v-else name="delete" :size="14" />
      </button>
      <button v-if="!isRemoteEnvironment && isStandalone" v-ripple class="action-btn danger" title="删除" :disabled="processing" @click.stop="$emit('remove', project)">
        <span v-if="isActionProcessing('remove')" class="btn-spinner"></span>
        <DynamicIcon v-else name="delete" :size="14" />
      </button>
      <button
        v-if="!isRemoteEnvironment"
        v-ripple
        class="action-btn more"
        title="更多"
        :class="{ 'is-active': moreMenuOpen }"
        :disabled="processing"
        @click.stop="toggleMoreMenu"
      >
        <DynamicIcon name="more-vertical" :size="14" />
      </button>
    </div>

    <!-- 自身项目提示 -->
    <div v-else class="self-message">
      当前运行项目，不可操作
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
import { formatBytesCompact } from '@/utils/format.js'
import { composeProjectDisplayStatus, composeProjectStateLabel } from '@/utils/resourceStatus.js'
import { remoteCapabilityAllowed, useEnvironmentContext } from '@/composables/useEnvironmentContext.js'

const props = defineProps({
  project: {
    type: Object,
    required: true
  },
  selected: {
    type: Boolean,
    default: false
  },
  processingAction: {
    type: String,
    default: ''
  }
})

const emit = defineEmits(['click', 'start', 'stop', 'restart', 'edit', 'remark', 'view', 'logs', 'terminal', 'destroy', 'remove', 'down', 'update', 'sync-git', 'force-stop'])

const environmentContext = useEnvironmentContext()
const isRemoteEnvironment = computed(() => environmentContext.isRemoteEnvironment?.value === true)
const lifecycleCapability = computed(() => isStandalone.value ? 'container.manage' : 'compose.manage')
const logCapability = computed(() => isStandalone.value ? 'container.logs' : 'compose.logs')
const canLifecycle = computed(() => !isRemoteEnvironment.value || remoteCapabilityAllowed(environmentContext.environment?.value, lifecycleCapability.value))
const canLogs = computed(() => !isRemoteEnvironment.value || remoteCapabilityAllowed(environmentContext.environment?.value, logCapability.value))
const showActions = computed(() => canLifecycle.value || canLogs.value || !isRemoteEnvironment.value)

const processing = computed(() => Boolean(props.processingAction))

function isActionProcessing(action) {
  return props.processingAction === action
}

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
  emit('force-stop', props.project)
}

const status = computed(() => composeProjectDisplayStatus(props.project))

// 状态样式类
const statusClass = computed(() => {
  return `status-${status.value}`
})

// 状态标签
const statusLabel = computed(() => composeProjectStateLabel(props.project))

// 运行中的容器数
const runningCount = computed(() => {
  return props.project.containers?.filter(c => {
    const state = String(c.State || c.state || '').toLowerCase()
    return state === 'running'
  }).length || 0
})

// 总容器数
const totalCount = computed(() => {
  return props.project.containers?.length || 0
})

// 路径缩写
const shortPath = computed(() => {
  const path = props.project.path
  if (!path || path === '-') return '-'
  if (path.length > 30) {
    return '...' + path.slice(-27)
  }
  return path
})

// 是否为独立容器（非 Compose 项目）
const isStandalone = computed(() => {
  return props.project.type === 'container'
})

// 类型标签
const typeLabel = computed(() => {
  return isStandalone.value ? '独立容器' : 'Compose'
})

// 获取容器镜像名
const containerImage = computed(() => {
  const container = props.project.containers?.[0]
  return container?.Image || '-'
})

// 镜像名缩写
const shortImage = computed(() => {
  const image = containerImage.value
  if (!image || image === '-') return '-'
  // 去除 tag，只保留镜像名
  const nameWithoutTag = image.split(':')[0]
  if (nameWithoutTag.length > 25) {
    return '...' + nameWithoutTag.slice(-22)
  }
  return nameWithoutTag
})

const resourceText = computed(() => {
  const stats = props.project.ResourceStats
  if (!stats || status.value !== 'running') return '-'
  const cpu = Number(stats.cpu_percent || 0)
  const memory = Number(stats.memory_usage || 0)
  return `CPU ${cpu.toFixed(cpu >= 10 ? 0 : 1)}% · ${memory > 0 ? formatBytesCompact(memory) : 'MEM -'}`
})
</script>

<style scoped>
.compose-card {
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

.compose-card:hover {
  border-color: var(--border-medium);
  transform: none;
  box-shadow: inset 0 0 0 1px var(--border-subtle);
}

.compose-card:active {
  transform: translateY(0) scale(0.99);
  transition-duration: 0.1s;
}

.compose-card.is-selected {
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.compose-card.is-self {
  background: linear-gradient(135deg, var(--bg-elevated) 0%, var(--color-warning-500-10) 100%);
  border-color: var(--color-warning-200);
}

.compose-card.is-self:hover {
  border-color: var(--color-warning-300);
  box-shadow: 0 4px 12px var(--ring-warning);
}

/* 运行中的项目 - 主题色渐变 */
.compose-card.status-running {
  background: linear-gradient(135deg, var(--bg-elevated) 0%, var(--color-primary-500-5) 100%);
}

.compose-card.status-running:hover {
  box-shadow: 0 4px 12px var(--ring-primary);
}

.compose-card.status-unhealthy,
.compose-card.status-error {
  background: linear-gradient(135deg, var(--bg-elevated) 0%, var(--color-danger-100) 100%);
}

.compose-card.status-unhealthy:hover,
.compose-card.status-error:hover {
  box-shadow: 0 4px 12px var(--ring-danger);
}

.compose-card.status-health-starting,
.compose-card.status-restarting,
.compose-card.status-paused,
.compose-card.status-removing,
.compose-card.status-partial {
  background: linear-gradient(135deg, var(--bg-elevated) 0%, var(--color-warning-100) 100%);
}

.compose-card.status-health-starting:hover,
.compose-card.status-restarting:hover,
.compose-card.status-paused:hover,
.compose-card.status-removing:hover,
.compose-card.status-partial:hover {
  box-shadow: 0 4px 12px var(--ring-warning);
}

.compose-card.status-stopped {
  background: linear-gradient(135deg, var(--bg-elevated) 0%, var(--bg-tertiary) 100%);
}

.compose-card.status-stopped:hover {
  box-shadow: 0 4px 12px var(--border-subtle);
}

.compose-card.status-created {
  background: linear-gradient(135deg, var(--bg-elevated) 0%, var(--color-primary-100) 100%);
}

.compose-card.status-created:hover {
  box-shadow: 0 4px 12px var(--ring-primary);
}

/* 头部 */
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
}

.project-name {
  font-size: 0.9375rem;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
}

.project-title-copy {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
}

.project-remark {
  min-width: 0;
  overflow: hidden;
  color: var(--text-tertiary);
  font-size: 0.75rem;
  font-weight: 400;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.remark-edit-btn {
  width: 24px;
  height: 24px;
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 5px;
  color: var(--text-tertiary);
  background: transparent;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.16s ease, color 0.16s ease, background-color 0.16s ease;
}

.compose-card:hover .remark-edit-btn,
.remark-edit-btn:focus-visible {
  opacity: 1;
}

.remark-edit-btn:hover {
  color: var(--color-primary-600);
  background: var(--color-primary-100);
}

.self-badge {
  font-size: 0.625rem;
  padding: 2px 6px;
  background: var(--color-warning-100);
  color: var(--color-warning-700);
  border-radius: 4px;
  font-weight: 500;
  flex-shrink: 0;
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

.status-badge.paused {
  background: var(--color-warning-100);
  color: var(--color-warning-700);
}

.status-badge.restarting {
  background: var(--color-warning-100);
  color: var(--color-warning-700);
}

.status-badge.created {
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

.status-badge.removing,
.status-badge.partial {
  background: var(--color-warning-100);
  color: var(--color-warning-700);
}

.status-badge.error {
  background: var(--color-danger-100);
  color: var(--color-danger-700);
}

.update-badge {
  display: inline-block;
  padding: 3px 8px;
  border-radius: 6px;
  font-size: 0.6875rem;
  font-weight: 500;
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

.git-badge {
  display: inline-block;
  padding: 3px 7px;
  color: var(--color-accent-700);
  background: var(--color-accent-100);
  border-radius: 6px;
  font-size: 0.6875rem;
  font-weight: 500;
}

.stats-text {
  font-size: 0.75rem;
  color: var(--text-tertiary);
  margin-left: auto;
}

/* 类型标签 */
.type-tag {
  display: inline-block;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 0.625rem;
  font-weight: 500;
}

.type-tag.compose {
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

.type-tag.standalone {
  background: var(--color-secondary-100);
  color: var(--color-secondary-700);
}

/* 名称包装器 */
.name-wrapper {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 0;
}

.type-icon {
  flex-shrink: 0;
  color: var(--text-secondary);
}

/* 信息列表 */
.compose-info {
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
}

.info-value.mono {
  font-family: 'JetBrains Mono', monospace;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 140px;
}

/* 操作按钮 */
.card-actions {
  display: flex;
  gap: 6px;
  padding-top: 8px;
  padding-bottom: 2px;
  margin-top: auto;
  border-top: 1px solid var(--border-subtle);
  flex-wrap: wrap;
  position: relative;
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

.action-btn.warning svg {
  stroke: var(--color-warning-600);
}

.action-btn.warning:hover {
  background: var(--color-warning-200);
  color: var(--color-warning-700);
}

.action-btn.warning:hover svg {
  stroke: var(--color-warning-700);
}

/* 编辑/查看 - 浅蓝色 */
.action-btn.info {
  background: var(--color-primary-100);
  color: var(--color-primary-600);
}

.action-btn.info svg {
  stroke: var(--color-primary-600);
}

.action-btn.info:hover {
  background: var(--color-primary-200);
  color: var(--color-primary-800);
}

.action-btn.info:hover svg {
  stroke: var(--color-primary-700);
}

/* 更新 - 紫色 */
.action-btn.update {
  background: var(--color-accent-100);
  color: var(--color-accent-600);
}

.action-btn.update svg {
  stroke: var(--color-accent-600);
}

.action-btn.update:hover {
  background: var(--color-accent-200);
  color: var(--color-accent-700);
}

.action-btn.update:hover svg {
  stroke: var(--color-accent-700);
}

.action-btn.git-sync {
  color: var(--color-accent-600);
  background: var(--color-accent-100);
}

.action-btn.git-sync:hover {
  color: var(--color-accent-700);
  background: var(--color-accent-200);
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

/* 销毁/删除 - 粉红色 */
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

/* 清除按钮 - 橙色警告 */
.action-btn.clear {
  background: var(--color-warning-100);
  color: var(--color-warning-600);
}

.action-btn.clear svg {
  stroke: var(--color-warning-600);
}

.action-btn.clear:hover {
  background: var(--color-warning-200);
  color: var(--color-warning-700);
}

.action-btn.clear:hover svg {
  stroke: var(--color-warning-700);
}

.action-btn svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.self-message {
  padding-top: 10px;
  padding-bottom: 2px;
  margin-top: auto;
  border-top: 1px solid var(--border-subtle);
  text-align: center;
  font-size: 0.75rem;
  color: var(--color-warning-600);
}

.btn-spinner {
  display: inline-block;
  width: 14px;
  height: 14px;
  border: 2px solid currentColor;
  border-right-color: transparent;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.card-actions.is-processing .action-btn {
  opacity: 0.85;
}
</style>
