<template>
  <ResourceTable
    :columns="columns"
    :rows="projects"
    table-key="compose-v2"
    :row-key="composeProjectIdentity"
    :selected-key="selectedName"
    :loading="loading"
    aria-label="Compose 项目列表"
    @select="handleSelect"
    @sort="handleSort"
  >
    <template #header-check>
      <input
        class="compose-batch-check"
        type="checkbox"
        :checked="allCheckableChecked"
        :indeterminate.prop="partiallyChecked"
        :disabled="checkableIds.length === 0"
        title="全选/取消当前列表全部项目"
        @click.stop
        @change="emit('toggle-check-all')"
      />
    </template>

    <template #cell-check="{ row: project }">
      <input
        class="compose-batch-check"
        type="checkbox"
        :checked="checkedIds.includes(composeProjectIdentity(project))"
        :disabled="Boolean(project.isSelf || project.identityError)"
        :title="project.identityError || (project.isSelf ? '项目本身不允许批量操作' : '勾选后可批量启动/停止')"
        @click.stop
        @change="emit('toggle-check', composeProjectIdentity(project))"
      />
    </template>

    <template v-for="column in sortableColumns" :key="column.key" #[`header-${column.key}`]>
      {{ column.label }}
      <span class="sort-icon" :class="getSortIcon(column.key)">
        <DynamicIcon v-if="getSortIcon(column.key) === 'asc'" name="chevron-up" :size="14" />
        <DynamicIcon v-else-if="getSortIcon(column.key) === 'desc'" name="chevron-down" :size="14" />
        <DynamicIcon v-else name="arrow-up-down" :size="14" />
      </span>
    </template>

    <template #cell-name="{ row: project }">
      <div class="name-wrapper">
        <span class="resource-kind compose">
          <DynamicIcon name="compose" :size="16" class="project-icon-small" />
        </span>
        <span class="resource-name-copy">
          <strong>{{ project.name }}</strong>
          <!-- 同名多路径的 Compose 项目（外部部署共用项目名）以路径短名区分，避免列表出现不可辨别的同名条目。 -->
          <small v-if="duplicateName(project) && project.path" :title="project.path">
            {{ shortPathLabel(project.path) }}
          </small>
          <small>{{ project.type === 'container' ? '独立容器' : `${project.containers?.length || 0} 个服务` }}</small>
        </span>
        <span v-if="project.gitSource" class="git-tag" :title="project.gitSource.repoUrl">Git</span>
        <span v-if="project.isSelf" class="self-tag">达咩</span>
        <span v-if="project.invalidName" class="invalid-name-tag" :title="project.invalidReason">名称不规范</span>
		<span v-if="project.identityError" class="invalid-name-tag" :title="project.identityError">身份冲突</span>
      </div>
    </template>

    <template #cell-updateCount="{ row: project }">
            <button
              v-if="!isRemoteEnvironment && project.updateAvailable && !project.isSelf && project.type !== 'container'"
              v-ripple
              class="update-btn"
              :disabled="isProcessing(project)"
              @click.stop="handleAction('update', project)"
            >
              <span v-if="isActionProcessing(project, 'update')" class="btn-spinner"></span>
              <DynamicIcon v-else name="cloud-download" :size="14" />
              <span v-if="!isProcessing(project)">更新 {{ project.updateCount > 0 ? `(${project.updateCount})` : '' }}</span>
            </button>
            <span v-else class="text-muted">-</span>
    </template>

    <template #cell-remark="{ row: project }">
      <button
        v-if="!isRemoteEnvironment"
        type="button"
        class="remark-button"
        :data-testid="`compose-remark-${project.name}`"
        :title="project.remark || '添加备注'"
        @click.stop="handleAction('remark', project)"
      >
        <span>{{ project.remark || '-' }}</span>
        <DynamicIcon name="edit" :size="13" />
      </button>
    </template>

    <template #cell-containerCount="{ row: project }">
      {{ project.containers?.length || 0 }}
    </template>

    <template #cell-resources="{ row: project }">
            <span v-if="!hasRunningContainer(project)" class="resource-empty">-</span>
            <div v-else class="resource-usage-cell">
              <span class="resource-metric">
                <b class="resource-metric-label">CPU</b>
                <span class="resource-metric-value">{{ formatStatPercent(project.ResourceStats?.cpu_percent) }}</span>
              </span>
              <span class="resource-metric">
                <b class="resource-metric-label">MEM</b>
                <span class="resource-metric-value">{{ formatStatPercent(project.ResourceStats?.memory_percent) }}</span>
              </span>
              <span class="resource-metric">
                <b class="resource-metric-label">NET</b>
                <span class="resource-metric-value">{{ formatBytesShort(sumStats(project.ResourceStats?.network_rx, project.ResourceStats?.network_tx)) }}</span>
              </span>
              <span class="resource-metric">
                <b class="resource-metric-label">I/O</b>
                <span class="resource-metric-value">{{ formatBytesShort(sumStats(project.ResourceStats?.block_read, project.ResourceStats?.block_write)) }}</span>
              </span>
            </div>
    </template>

    <template #cell-path="{ row: project }">
      <span class="cell-mono" :title="project.path || '-'">{{ project.path || '-' }}</span>
    </template>

    <template #cell-status="{ row: project }">
      <StatusBadge :status="getProjectBadgeStatus(project)" :label="getStatusText(project)" variant="minimal" />
    </template>

    <template #cell-actions="{ row: project }">
      <div v-if="rowHasActions(project)" class="cell-actions">
            <div class="action-buttons compact-actions">
              <template v-if="!project.isSelf">
              <button 
                v-if="canLifecycle(project) && getStatusText(project) === '运行中'"
                v-ripple
                class="table-btn stop" 
                title="停止" 
                :disabled="isProcessing(project)"
                @click.stop="handleAction('stop', project)"
              >
                <span v-if="isActionProcessing(project, 'stop')" class="btn-spinner"></span>
                <DynamicIcon v-else name="stop" :size="14" />
              </button>
              <button 
                v-if="canLifecycle(project) && getStatusText(project) !== '运行中'"
                v-ripple
                class="table-btn success" 
                title="启动" 
                :disabled="isProcessing(project)"
                @click.stop="handleAction('start', project)"
              >
                <span v-if="isActionProcessing(project, 'start')" class="btn-spinner"></span>
                <DynamicIcon v-else name="play" :size="14" />
              </button>
              <button v-if="canLifecycle(project)" v-ripple class="table-btn warning" title="重启" :disabled="isProcessing(project)" @click.stop="handleAction('restart', project)">
                <span v-if="isActionProcessing(project, 'restart')" class="btn-spinner"></span>
                <DynamicIcon v-else name="refresh" :size="14" />
              </button>
              <button
                v-if="!isRemoteEnvironment && project.type !== 'container'"
                v-ripple
                class="table-btn info"
                title="编辑"
                :disabled="isProcessing(project)"
                @click.stop="handleAction('edit', project)"
              >
                <span v-if="isActionProcessing(project, 'edit')" class="btn-spinner"></span>
                <DynamicIcon v-else name="edit" :size="14" />
              </button>
              <button
                v-if="isRemoteEnvironment && canLogs(project)"
                v-ripple
                class="table-btn info"
                :title="project.type === 'container' ? '容器日志' : '项目日志'"
                :disabled="isProcessing(project)"
                @click.stop="handleAction('logs', project)"
              >
                <DynamicIcon name="file-text" :size="14" />
              </button>
              <button
                v-if="!isRemoteEnvironment"
                v-ripple
                class="table-btn"
                title="更多"
                :class="{ 'is-active': openActionMenuName === composeProjectIdentity(project) }"
                :disabled="isProcessing(project)"
                @click.stop="toggleActionMenu(project, $event)"
              >
                <DynamicIcon name="more-vertical" :size="14" />
              </button>
              </template>
              <span v-else class="self-tag">项目本身不允许操作</span>
            </div>
      </div>
    </template>

    <template #empty>
      <EmptyState v-bind="emptyProps" />
    </template>
  </ResourceTable>

  <AnchoredMenu
    v-if="!isRemoteEnvironment"
    :open="Boolean(openActionMenuName)"
    :anchor="actionMenuAnchor"
    @update:open="handleActionMenuOpenChange"
  >
    <template v-if="activeActionProject">
      <button v-if="activeActionProject.type !== 'container'" type="button" role="menuitem" @click="handleMenuAction('logs', activeActionProject)">
        <DynamicIcon name="file-text" :size="14" />
        项目日志
      </button>
      <button v-if="composeProtectionEnabled && activeActionProject.type !== 'container'" type="button" role="menuitem" @click="handleMenuAction('protect', activeActionProject)">
        <DynamicIcon name="shield" :size="14" />
        应用保护
      </button>
      <button v-if="activeActionProject.type !== 'container' && activeActionProject.gitSource" type="button" role="menuitem" @click="handleMenuAction('sync-git', activeActionProject)">
        <DynamicIcon name="git-branch" :size="14" />
        Git 同步
      </button>
      <button v-if="activeActionProject.type === 'container'" type="button" role="menuitem" @click="handleMenuAction('terminal', activeActionProject)">
        <DynamicIcon name="terminal" :size="14" />
        终端
      </button>
      <button type="button" role="menuitem" @click="handleMenuAction('force-stop', activeActionProject)">
        <DynamicIcon name="zap" :size="14" />
        强制停止
      </button>
      <button v-if="activeActionProject.type !== 'container'" type="button" role="menuitem" @click="handleMenuAction('down', activeActionProject)">
        <DynamicIcon name="eraser" :size="14" />
        清除容器
      </button>
      <button v-if="activeActionProject.type !== 'container'" type="button" role="menuitem" class="danger" @click="handleMenuAction('destroy', activeActionProject)">
        <DynamicIcon name="delete" :size="14" />
        销毁
      </button>
      <button v-else type="button" role="menuitem" class="danger" @click="handleMenuAction('remove', activeActionProject)">
        <DynamicIcon name="delete" :size="14" />
        删除
      </button>
    </template>
  </AnchoredMenu>
</template>

<script setup>
import { computed, ref } from 'vue'
import { composeProjectIdentity } from '../utils/projectIdentity.js'
import EmptyState from '@/components/ui/EmptyState.vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import AnchoredMenu from '@/components/ui/AnchoredMenu.vue'
import { formatBytesCompact } from '@/utils/format.js'
import { composeProjectDisplayStatus, composeProjectStateLabel } from '@/utils/resourceStatus.js'
import ResourceTable from '@/components/resource-workbench/ResourceTable.vue'
import { remoteCapabilityAllowed, useEnvironmentContext } from '@/composables/useEnvironmentContext.js'
import { composeProtectionEnabled } from '@edition/compose-features'


// 同名项目判定：列表内存在多个同名且路径不同的 Compose 项目。
function duplicateName(project) {
  if (project?.type === 'container' || !project?.name) return false
  return (props.projects || []).some(
    other => other !== project && other?.type !== 'container' && other?.name === project.name
  )
}

// 路径短标签：取末两段目录，便于同名区分。
function shortPathLabel(path) {
  const parts = String(path || '').split('/').filter(Boolean)
  return '…/' + parts.slice(-2).join('/')
}

const props = defineProps({
  projects: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  selectedName: { type: String, default: '' },
  sortState: { type: Object, default: () => ({ prop: '', order: '' }) },
  processingActions: {
    type: [Map, Object],
    default: () => new Map()
  },
  checkedIds: { type: Array, default: () => [] }
})

const environmentContext = useEnvironmentContext()
const isRemoteEnvironment = computed(() => environmentContext.isRemoteEnvironment?.value === true)

function projectCapability(project, kind) {
  const prefix = project?.type === 'container' ? 'container' : 'compose'
  return `${prefix}.${kind}`
}

function canLifecycle(project) {
  return !isRemoteEnvironment.value || remoteCapabilityAllowed(environmentContext.environment?.value, projectCapability(project, 'manage'))
}

function canLogs(project) {
  return !isRemoteEnvironment.value || remoteCapabilityAllowed(environmentContext.environment?.value, projectCapability(project, 'logs'))
}

function rowHasActions(project) {
  return !project?.isSelf && (canLifecycle(project) || canLogs(project) || !isRemoteEnvironment.value)
}

// 批量选择：以项目身份（名称+路径）为勾选键；项目本身（isSelf）不参与批量操作。
const checkableIds = computed(() =>
  props.projects.filter(project => !project?.isSelf && !project?.identityError).map(project => composeProjectIdentity(project))
)
const allCheckableChecked = computed(() =>
  checkableIds.value.length > 0 && checkableIds.value.every(id => props.checkedIds.includes(id))
)
const partiallyChecked = computed(() =>
  !allCheckableChecked.value && checkableIds.value.some(id => props.checkedIds.includes(id))
)

function isProcessing(project) {
  const identity = composeProjectIdentity(project)
  if (props.processingActions instanceof Map) {
    return props.processingActions.has(identity)
  }
  return props.processingActions?.has?.(identity)
}

function isActionProcessing(project, action) {
  const identity = composeProjectIdentity(project)
  if (props.processingActions instanceof Map) {
    return props.processingActions.get(identity) === action
  }
  return props.processingActions?.get?.(identity) === action
}

const emit = defineEmits(['select', 'action', 'sort', 'toggle-check', 'toggle-check-all'])
const openActionMenuName = ref('')
const activeActionProject = ref(null)
const actionMenuAnchor = ref(null)
const columns = [
  { key: 'check', label: '', cellClass: 'cell-check', width: 40, minWidth: 36 },
  { key: 'name', label: '名称', sortable: true, cellClass: 'cell-name', width: 180, minWidth: 160, flex: true },
  { key: 'remark', label: '备注', sortable: true, width: 126, minWidth: 96 },
  { key: 'updateCount', label: '更新', sortable: true, width: 112, minWidth: 104 },
  { key: 'containerCount', label: '容器数量', sortable: true, width: 86, minWidth: 78 },
  { key: 'resources', label: '资源', width: 150, minWidth: 130 },
  { key: 'path', label: '项目路径', sortable: true, width: 140, minWidth: 110 },
  { key: 'status', label: '运行状态', sortable: true, width: 96, minWidth: 88 },
  { key: 'actions', label: '操作', cellClass: 'cell-actions', width: 170, minWidth: 148 }
]
const sortableColumns = columns.filter(column => column.sortable)

const emptyProps = {
  title: '暂无项目',
  description: '点击"新建项目"按钮创建第一个 Compose 项目',
  icon: 'box'
}

function isRunning(state) {
  const s = String(state || '').toLowerCase()
  return s === 'running' || s === '运行中'
}

function hasRunningContainer(project) {
  return (project.containers || []).some(c => isRunning(c.State))
}

function getStatusText(project) {
  // 列表/卡片按 Docker 实际生命周期展示（运行中/已创建/检测中/已停止/部分运行…），
  // 而不是把所有非运行、非停止的一律叫"不健康"。
  return composeProjectStateLabel(project)
}

function getProjectBadgeStatus(project) {
  return composeProjectDisplayStatus(project)
}

function formatStatPercent(value) {
  if (value === undefined || value === null) return '-'
  const num = Number(value)
  if (Number.isNaN(num)) return '-'
  return `${num.toFixed(num >= 10 ? 0 : 1)}%`
}

function sumStats(...values) {
  return values.reduce((sum, value) => sum + Number(value || 0), 0)
}

function formatBytesShort(value) {
  const num = Number(value || 0)
  if (num <= 0) return '-'
  return formatBytesCompact(num)
}

function getSortIcon(prop) {
  if (props.sortState.prop !== prop) return ''
  return props.sortState.order === 'ascending' ? 'asc' : 'desc'
}

function handleSort(prop) {
  emit('sort', prop)
}

function handleSelect(project) {
  closeActionMenu()
  emit('select', project)
}

function handleAction(action, project) {
  closeActionMenu()
  emit('action', { action, project })
}

function closeActionMenu() {
  openActionMenuName.value = ''
  activeActionProject.value = null
  actionMenuAnchor.value = null
}

function handleActionMenuOpenChange(open) {
  if (!open) closeActionMenu()
}

function toggleActionMenu(project, event) {
  const identity = composeProjectIdentity(project)
  if (openActionMenuName.value === identity) {
    closeActionMenu()
    return
  }
  openActionMenuName.value = identity
  activeActionProject.value = project || null
  actionMenuAnchor.value = event?.currentTarget || null
}

function handleMenuAction(action, project) {
  handleAction(action, project)
}
</script>

<style scoped>
.compose-batch-check {
  width: 15px;
  height: 15px;
  cursor: pointer;
  accent-color: var(--color-primary-600, #2563eb);
}

.compose-batch-check:disabled {
  cursor: not-allowed;
}

.list-view {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  overflow-x: hidden;
  overflow-y: hidden;
}

.list-loading {
  padding: 16px;
}

.list-skeleton {
  display: flex;
  gap: 16px;
  padding: 12px 0;
  border-bottom: 1px solid var(--border-subtle);
}

.skeleton-cell {
  height: 16px;
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
  border-radius: 4px;
}

.list-empty {
  padding: 40px;
}

.remark-button {
  width: auto;
  max-width: 100%;
  min-width: 0;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 4px 2px;
  border: 0;
  color: var(--text-secondary);
  background: transparent;
  cursor: pointer;
  font: inherit;
  text-align: left;
}

.remark-button span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.remark-button :deep(svg) {
  flex: 0 0 auto;
  opacity: 0;
  transition: opacity var(--motion-duration-quick) var(--motion-ease-out);
}

.remark-button:hover {
  color: var(--color-primary-600);
}

.remark-button:hover :deep(svg),
.remark-button:focus-visible :deep(svg) {
  opacity: 1;
}

.resource-table {
  width: 100%;
  min-width: 0;
  display: grid;
  padding: 0 12px 10px;
  font-size: 0.875rem;
  box-sizing: border-box;
}

.table-head,
.resource-row {
  display: grid;
  grid-template-columns: var(--resource-grid-template, minmax(140px, 1.25fr) 70px 72px minmax(110px, 0.9fr) minmax(100px, 0.9fr) 88px 154px);
  align-items: center;
  gap: 8px;
}

.table-head {
  position: sticky;
  top: 0;
  z-index: 2;
  min-height: 42px;
  padding: 0 8px;
  border-bottom: 1px solid var(--border-subtle);
  background: var(--bg-elevated);
}

.table-head > span,
.table-head > button {
  position: relative;
  text-align: left;
  min-width: 0;
  min-height: 100%;
  display: inline-flex;
  align-items: center;
  padding: 0;
  border: 0;
  font-weight: 600;
  color: var(--text-secondary);
  background: transparent;
  white-space: nowrap;
  overflow: hidden;
}

.table-head > span:not(:last-child)::after,
.table-head > button:not(:last-child)::after {
  content: "";
  position: absolute;
  top: 9px;
  right: -5px;
  bottom: 9px;
  width: 10px;
  cursor: col-resize;
}

.table-head > span:not(:last-child):hover::after,
.table-head > button:not(:last-child):hover::after {
  right: -1px;
  width: 1px;
  background: var(--border-default);
}

.table-head .sortable {
  cursor: pointer;
  user-select: none;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.table-head .sortable:hover {
  color: var(--text-primary);
  background: transparent;
}

.sort-icon {
  display: inline-flex;
  align-items: center;
  margin-left: 4px;
  vertical-align: middle;
  color: var(--text-tertiary);
}

.sort-icon.asc,
.sort-icon.desc {
  color: var(--color-primary-500);
}

.resource-row > [role="cell"] {
  min-width: 0;
  min-height: 68px;
  display: flex;
  align-items: center;
  overflow: hidden;
  color: var(--text-primary);
}

.resource-row .status-badge {
  flex: 0 0 auto;
  min-width: 64px;
}

.resource-row {
  width: 100%;
  min-height: 68px;
  padding: 0 8px;
  border-bottom: 1px solid var(--border-subtle);
  background: transparent;
  cursor: pointer;
  outline: none;
  transform-origin: center;
  transition:
    transform var(--motion-duration-quick) var(--motion-ease-out),
    background var(--motion-duration-quick) var(--motion-ease-out),
    box-shadow var(--motion-duration-quick) var(--motion-ease-out);
  box-sizing: border-box;
}

.resource-row:active {
  transform: scale(0.992);
}

.resource-row:hover,
.resource-row.is-selected {
  background: var(--color-primary-50);
}

.resource-row.is-selected {
  box-shadow: inset 4px 0 0 var(--color-primary-500);
  animation: project-row-select var(--motion-duration-fast) var(--motion-ease-out);
}

@keyframes project-row-select {
  0% { transform: scale(1); }
  44% { transform: scale(0.988); }
  100% { transform: scale(1); }
}

.cell-name {
  font-weight: 400;
}

.name-wrapper {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  overflow: hidden;
}

.name-wrapper > span:not(.git-tag):not(.self-tag) {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.project-icon-small {
  color: var(--color-primary-500);
  flex-shrink: 0;
}

.self-tag {
  font-size: 0.6875rem;
  padding: 2px 6px;
  background: var(--color-warning-100);
  color: var(--color-warning-700);
  border-radius: 4px;
  font-weight: 500;
}

.git-tag {
  padding: 2px 6px;
  color: var(--color-accent-700);
  background: var(--color-accent-100);
  border-radius: 4px;
  font-size: 0.6875rem;
  font-weight: 500;
}

.cell-mono {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.8125rem;
}

.resource-usage-cell {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  justify-content: start;
  gap: 4px 6px;
  width: 100%;
  max-width: 100%;
  min-width: 0;
  padding-right: 8px;
  box-sizing: border-box;
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.6875rem;
  color: var(--text-secondary);
}

.resource-empty {
  color: var(--text-tertiary);
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.8125rem;
}

.resource-usage-cell .resource-metric {
  display: grid;
  grid-template-columns: 25px minmax(0, 1fr);
  align-items: baseline;
  justify-content: start;
  gap: 4px;
  min-height: 20px;
  padding: 2px 5px;
  box-sizing: border-box;
  background: color-mix(in srgb, var(--bg-tertiary) 72%, transparent);
  border-left: 2px solid var(--color-info-500);
  border-radius: 3px;
  white-space: nowrap;
  min-width: 0;
  overflow: hidden;
}

.resource-usage-cell .resource-metric:nth-child(2) { border-left-color: var(--color-primary-500); }
.resource-usage-cell .resource-metric:nth-child(3) { border-left-color: var(--color-success-500); }
.resource-usage-cell .resource-metric:nth-child(4) { border-left-color: var(--color-warning-500); }

.resource-metric-label {
  color: var(--text-tertiary);
  font-size: 0.625rem;
  font-weight: 600;
  letter-spacing: 0.02em;
}

.resource-metric-value {
  min-width: 0;
  overflow: hidden;
  color: var(--text-secondary);
  text-overflow: ellipsis;
}

.update-btn {
  display: flex;
  align-items: center;
  flex-wrap: nowrap;
  gap: 4px;
  padding: 4px 10px;
  background: var(--color-accent-100);
  color: var(--color-accent-600);
  border: none;
  border-radius: 6px;
  font-size: 0.75rem;
  font-weight: 500;
  white-space: nowrap;
  cursor: pointer;
}

.update-btn:hover {
  background: var(--color-accent-200);
  color: var(--color-accent-700);
}

.update-btn svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.text-muted {
  color: var(--text-tertiary);
}

.cell-actions {
  width: 100%;
  min-width: 0;
  white-space: nowrap;
}

.action-buttons {
  position: relative;
  display: flex;
  align-items: center;
  gap: 4px;
  width: 100%;
  min-height: 32px;
  flex-wrap: nowrap;
}

.compact-actions {
  width: 100%;
  max-width: 100%;
}

.table-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 32px;
  border-radius: 8px;
  background: var(--bg-tertiary);
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.table-btn.success {
  background: var(--color-success-100);
  color: var(--color-success-600);
}

.table-btn.success svg {
  stroke: var(--color-success-600);
}

.table-btn.success:hover {
  background: var(--color-success-200);
  color: var(--color-success-700);
}

.table-btn.success:hover svg {
  stroke: var(--color-success-700);
}

.table-btn.stop {
  background: var(--color-danger-100);
  color: var(--color-danger-600);
}

.table-btn.stop svg {
  stroke: var(--color-danger-600);
}

.table-btn.stop:hover {
  background: var(--color-danger-200);
  color: var(--color-danger-700);
}

.table-btn.stop:hover svg {
  stroke: var(--color-danger-700);
}

.table-btn.warning {
  background: var(--color-warning-100);
  color: var(--color-warning-600);
}

.table-btn.warning svg {
  stroke: var(--color-warning-600);
}

.table-btn.warning:hover {
  background: var(--color-warning-200);
  color: var(--color-warning-700);
}

.table-btn.warning:hover svg {
  stroke: var(--color-warning-700);
}

.table-btn.info {
  background: var(--color-primary-100);
  color: var(--color-primary-600);
}

.table-btn.info svg {
  stroke: var(--color-primary-600);
}

.table-btn.info:hover {
  background: var(--color-primary-200);
  color: var(--color-primary-700);
}

.table-btn.info:hover svg {
  stroke: var(--color-primary-700);
}

.table-btn.git-sync {
  color: var(--color-accent-600);
  background: var(--color-accent-100);
}

.table-btn.git-sync:hover {
  color: var(--color-accent-700);
  background: var(--color-accent-200);
}

.table-btn.terminal {
  background: var(--bg-tertiary);
  color: var(--text-secondary);
}

.table-btn.terminal:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

.table-btn.danger {
  background: var(--color-danger-100);
  color: var(--color-danger-600);
}

.table-btn.danger svg {
  stroke: var(--color-danger-600);
}

.table-btn.danger:hover {
  background: var(--color-danger-200);
  color: var(--color-danger-700);
}

.table-btn.danger:hover svg {
  stroke: var(--color-danger-700);
}

.table-btn.clear {
  background: var(--color-warning-100);
  color: var(--color-warning-600);
}

.table-btn.clear svg {
  stroke: var(--color-warning-600);
}

.table-btn.clear:hover {
  background: var(--color-warning-200);
  color: var(--color-warning-700);
}

.table-btn.clear:hover svg {
  stroke: var(--color-warning-700);
}

.table-btn svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.table-btn.is-active {
  color: var(--text-primary);
  background: var(--bg-secondary);
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

.update-btn:disabled,
.table-btn:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}
</style>
