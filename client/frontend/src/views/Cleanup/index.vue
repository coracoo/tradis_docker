<template>
  <ResourceWorkbench class="cleanup-page cleanup-workbench" role="main" aria-label="Docker 空间清理">
    <template #rail>
      <section class="resource-rail cleanup-summary-rail" aria-label="空间清理概览">
        <article class="rail-card tone-primary">
          <span class="rail-icon"><DynamicIcon name="disk" :size="18" /></span>
          <div class="rail-copy">
            <strong>{{ formatKnownBytes(evaluation.summary.knownReclaimableBytes) }}</strong>
            <span>预计可释放</span>
          </div>
          <small>{{ evaluation.summary.unknownSizeCount ? `另有 ${evaluation.summary.unknownSizeCount} 项容量未知` : '基于当前 Docker 使用情况' }}</small>
        </article>
        <article class="rail-card tone-info">
          <span class="rail-icon"><DynamicIcon name="search" :size="18" /></span>
          <div class="rail-copy">
            <strong>{{ evaluation.summary.candidateCount }}</strong>
            <span>清理候选</span>
          </div>
          <small>仅列出当前未使用资源</small>
        </article>
        <article class="rail-card tone-success">
          <span class="rail-icon"><DynamicIcon name="check" :size="18" /></span>
          <div class="rail-copy">
            <strong>{{ selectedItems.length }}</strong>
            <span>已选择</span>
          </div>
          <small>{{ selectedSizeText }}</small>
        </article>
        <article class="rail-card tone-muted">
          <span class="rail-icon"><DynamicIcon name="clock" :size="18" /></span>
          <div class="rail-copy">
            <strong>{{ evaluationTime.time }}</strong>
            <span>最近评估</span>
          </div>
          <small>{{ evaluationTime.date }}</small>
        </article>
      </section>
    </template>

    <template #toolbar>
      <div class="toolbar workbench-toolbar">
        <div class="toolbar-left workbench-toolbar-left">
          <SearchInput
            v-model="query"
            :loading="evaluating"
            placeholder="搜索清理候选..."
          />
          <SegmentedTabs
            v-model="riskFilter"
            :options="riskFilters"
            aria-label="风险筛选"
            compact
          />
        </div>
        <div class="toolbar-right">
          <button
            v-ripple
            type="button"
            class="secondary-btn"
            :disabled="evaluating || taskRunning"
            @click="refreshEvaluation"
          >
            <DynamicIcon name="refresh" :size="16" :class="{ 'is-spinning': evaluating }" />
            重新评估
          </button>
          <button
            v-ripple
            type="button"
            class="primary-btn cleanup-submit"
            data-action="cleanup"
            :disabled="selectedItems.length === 0 || taskRunning || loading"
            @click="requestCleanup"
          >
            <DynamicIcon name="brush" :size="16" />
            {{ taskRunning ? '正在清理' : `清理 ${selectedItems.length || ''}`.trim() }}
          </button>
        </div>
      </div>
    </template>

    <ResourcePanel class="cleanup-list-panel">
      <div class="cleanup-panel-content">
        <section v-if="activeTask" class="cleanup-task-progress" data-task-progress>
          <div class="task-progress-header">
            <div class="task-progress-title">
              <DynamicIcon :name="taskRunning ? 'refresh' : taskStatusIcon" :size="17" :class="{ 'is-spinning': taskRunning }" />
              <div>
                <strong>{{ taskStatusTitle }}</strong>
                <span>{{ activeTask.id || activeTask.taskId }}</span>
              </div>
            </div>
            <StatusBadge :status="taskStatusBadge" :label="taskStatusLabel" variant="minimal" />
          </div>
          <div v-if="taskLogs.length" class="task-log-lines" aria-label="清理任务进度">
            <div v-for="(log, index) in taskLogs.slice(-6)" :key="`${log.time || ''}-${index}`" class="task-log-line" :class="`is-${log.type || 'info'}`">
              <span>{{ formatLogTime(log.time) }}</span>
              <p>{{ log.message }}</p>
            </div>
          </div>
          <p v-else class="task-waiting-copy">任务已在后台接管，离开此页面不会中断。</p>
        </section>

        <div v-if="error" class="cleanup-error" role="alert">
          <DynamicIcon name="error" :size="17" />
          <span>{{ error }}</span>
        </div>

        <div v-if="evaluating && !evaluation.categories.length" class="cleanup-empty-state">
          <DynamicIcon name="refresh" :size="24" class="is-spinning" />
          <span>正在评估 Docker 空间...</span>
        </div>
        <div v-else-if="!filteredCategories.length" class="cleanup-empty-state">
          <DynamicIcon name="check" :size="24" />
          <span>{{ evaluation.summary.candidateCount ? '没有符合筛选条件的候选' : '当前没有可清理项目' }}</span>
        </div>

        <section
          v-for="category in filteredCategories"
          v-else
          :key="category.key"
          class="cleanup-category"
          :data-category="category.key"
        >
          <header class="cleanup-category-header">
            <button
              type="button"
              class="category-toggle"
              :data-category-toggle="category.key"
              :aria-expanded="isCategoryExpanded(category.key)"
              :title="isCategoryExpanded(category.key) ? '收起' : '展开'"
              @click="toggleExpanded(category.key)"
            >
              <DynamicIcon :name="isCategoryExpanded(category.key) ? 'chevron-down' : 'arrow-right'" :size="17" />
            </button>
            <button
              type="button"
              class="category-check"
              role="checkbox"
              :aria-checked="categorySelectionState(category)"
              :aria-label="`选择${category.label}`"
              @click="toggleCategory(category)"
            >
              <DynamicIcon :name="categorySelectionIcon(category)" :size="14" />
            </button>
            <span class="category-kind" :class="`risk-${category.risk}`">
              <DynamicIcon :name="categoryIcon(category.key)" :size="17" />
            </span>
            <div class="category-heading">
              <strong>{{ category.label }}</strong>
              <span>{{ category.count ?? category.items.length }} 项</span>
            </div>
            <StatusBadge
              :status="riskStatus(category.risk)"
              :label="riskLabel(category.risk)"
              variant="minimal"
            />
            <span class="category-size">{{ formatCategorySize(category) }}</span>
          </header>

          <div v-if="isCategoryExpanded(category.key)" class="cleanup-items">
            <button
              v-for="item in category.items"
              :key="item.key"
              type="button"
              class="cleanup-item"
              :class="{ selected: selectedKeys.has(item.key) }"
              role="checkbox"
              :aria-checked="selectedKeys.has(item.key)"
              :data-item="item.key"
              @click="toggleItem(item)"
            >
              <span class="item-check">
                <DynamicIcon v-if="selectedKeys.has(item.key)" name="check" :size="13" />
              </span>
              <span class="item-copy">
                <strong>{{ item.name }}</strong>
                <small>{{ item.detail || item.id }}</small>
              </span>
              <span class="cleanup-item-size">{{ formatItemSize(item) }}</span>
            </button>
          </div>
        </section>
      </div>
    </ResourcePanel>

    <ConfirmDialog
      v-if="showHighRiskConfirm"
      :visible="showHighRiskConfirm"
      type="danger"
      title="确认删除未使用数据卷"
      message="数据卷可能包含无法恢复的持久化数据。系统会在执行前再次检查使用关系，但删除后不能撤销。"
      confirm-text="确认清理"
      @confirm="confirmHighRiskCleanup"
      @cancel="showHighRiskConfirm = false"
    />
  </ResourceWorkbench>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import SegmentedTabs from '@/components/ui/SegmentedTabs.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import ResourcePanel from '@/components/resource-workbench/ResourcePanel.vue'
import ResourceWorkbench from '@/components/resource-workbench/ResourceWorkbench.vue'
import { formatBytes, formatTimeTwoLines } from '@/utils/format.js'
import { useCleanupPage } from './useCleanupPage.js'

const riskFilters = [
  { value: 'all', label: '全部风险' },
  { value: 'low', label: '低风险' },
  { value: 'medium', label: '中风险' },
  { value: 'high', label: '高风险' }
]

const {
  evaluation,
  query,
  riskFilter,
  filteredCategories,
  selectedKeys,
  selectedItems,
  selectedKnownBytes,
  selectedUnknownSizeCount,
  requiresHighRiskConfirmation,
  activeTask,
  taskLogs,
  taskRunning,
  loading,
  evaluating,
  error,
  initialize,
  loadEvaluation,
  toggleItem,
  toggleCategory,
  submitCleanup
} = useCleanupPage()

const collapsedCategories = ref(new Set())
const showHighRiskConfirm = ref(false)

const selectedSizeText = computed(() => {
  const known = formatKnownBytes(selectedKnownBytes.value)
  return selectedUnknownSizeCount.value ? `${known}，另有未知容量` : known
})

const evaluationTime = computed(() => {
  const formatted = formatTimeTwoLines(evaluation.value.evaluatedAt)
  return { date: formatted.date, time: formatted.time || '—' }
})

const taskStatus = computed(() => String(activeTask.value?.status || '').toLowerCase())
const taskStatusTitle = computed(() => {
  if (taskStatus.value === 'pending') return '清理任务等待执行'
  if (taskRunning.value) return '清理任务运行中'
  if (['success', 'completed'].includes(taskStatus.value)) return '空间清理完成'
  if (['error', 'failed'].includes(taskStatus.value)) return '空间清理失败'
  return '空间清理任务'
})
const taskStatusLabel = computed(() => {
  if (taskStatus.value === 'pending') return '等待中'
  if (taskRunning.value) return '运行中'
  if (['success', 'completed'].includes(taskStatus.value)) return '已完成'
  if (['error', 'failed'].includes(taskStatus.value)) return '失败'
  return '已结束'
})
const taskStatusBadge = computed(() => {
  if (taskRunning.value || taskStatus.value === 'pending') return 'running'
  if (['success', 'completed'].includes(taskStatus.value)) return 'success'
  if (['error', 'failed'].includes(taskStatus.value)) return 'error'
  return 'default'
})
const taskStatusIcon = computed(() => (
  ['success', 'completed'].includes(taskStatus.value) ? 'success' : 'error'
))

onMounted(() => initialize())

function isCategoryExpanded(key) {
  return !collapsedCategories.value.has(key)
}

function toggleExpanded(key) {
  const next = new Set(collapsedCategories.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  collapsedCategories.value = next
}

function categorySelectionState(category) {
  if (!category.items.length) return 'false'
  const count = category.items.filter(item => selectedKeys.value.has(item.key)).length
  if (count === 0) return 'false'
  if (count === category.items.length) return 'true'
  return 'mixed'
}

function categorySelectionIcon(category) {
  const state = categorySelectionState(category)
  if (state === 'true') return 'check'
  if (state === 'mixed') return 'minus'
  return 'plus'
}

function categoryIcon(key) {
  return {
    build_cache: 'layers',
    dangling_image: 'image',
    unused_image: 'image',
    stopped_container: 'container',
    unused_network: 'network',
    unused_volume: 'database'
  }[key] || 'brush'
}

function riskLabel(risk) {
  return { low: '低风险', medium: '中风险', high: '高风险' }[risk] || '未知风险'
}

function riskStatus(risk) {
  return { low: 'success', medium: 'warning', high: 'error' }[risk] || 'unknown'
}

function formatKnownBytes(bytes) {
  const value = Number(bytes || 0)
  return value > 0 ? formatBytes(value) : '0 B'
}

function formatItemSize(item) {
  if (item.sizeState === 'unknown') return '未知'
  if (item.sizeState === 'not_applicable') return '不适用'
  return formatKnownBytes(item.sizeBytes)
}

function formatCategorySize(category) {
  if (Number(category.knownReclaimableBytes || 0) > 0) {
    const suffix = category.unknownSizeCount ? ' + 未知' : ''
    return formatBytes(category.knownReclaimableBytes) + suffix
  }
  if (category.unknownSizeCount) return '容量未知'
  if (category.items.every(item => item.sizeState === 'not_applicable')) return '不适用'
  return '0 B'
}

function formatLogTime(value) {
  if (!value) return '--:--:--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--:--:--'
  return date.toLocaleTimeString('zh-CN', { hour12: false })
}

async function refreshEvaluation() {
  try {
    await loadEvaluation({ force: true })
  } catch {
    // The composable exposes the user-facing error state.
  }
}

function requestCleanup() {
  if (requiresHighRiskConfirmation.value) {
    showHighRiskConfirm.value = true
    return
  }
  runCleanup(false)
}

async function confirmHighRiskCleanup() {
  showHighRiskConfirm.value = false
  await runCleanup(true)
}

async function runCleanup(confirmHighRisk) {
  try {
    await submitCleanup({ confirmHighRisk })
  } catch {
    // The composable exposes the user-facing error state.
  }
}
</script>

<style scoped>
.cleanup-workbench {
  --cleanup-row-height: 56px;
}

.cleanup-summary-rail .tone-primary .rail-icon {
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

.cleanup-summary-rail .tone-info .rail-icon {
  background: var(--color-info-100);
  color: var(--color-info-700);
}

.cleanup-summary-rail .tone-success .rail-icon {
  background: var(--color-success-100);
  color: var(--color-success-700);
}

.cleanup-summary-rail .tone-muted .rail-icon {
  background: var(--bg-tertiary);
  color: var(--text-secondary);
}

.cleanup-list-panel {
  min-height: 0;
}

.cleanup-panel-content {
  box-sizing: border-box;
  min-height: 100%;
  padding: 14px;
}

.cleanup-task-progress,
.cleanup-error {
  margin-bottom: 12px;
  border: 1px solid var(--resource-line);
  border-radius: 10px;
  background: var(--resource-surface);
}

.cleanup-task-progress {
  padding: 12px 14px;
}

.task-progress-header,
.task-progress-title {
  display: flex;
  align-items: center;
}

.task-progress-header {
  justify-content: space-between;
  gap: 16px;
}

.task-progress-title {
  min-width: 0;
  gap: 10px;
  color: var(--color-primary-600);
}

.task-progress-title > div {
  display: flex;
  flex-direction: column;
  min-width: 0;
  gap: 2px;
}

.task-progress-title strong {
  color: var(--resource-ink);
  font-size: 0.875rem;
  font-weight: 650;
}

.task-progress-title span,
.task-waiting-copy {
  color: var(--resource-muted);
  font-size: 0.72rem;
}

.task-log-lines {
  display: grid;
  gap: 4px;
  margin-top: 10px;
  padding-top: 9px;
  border-top: 1px solid var(--resource-line);
}

.task-log-line {
  display: grid;
  grid-template-columns: 64px minmax(0, 1fr);
  gap: 8px;
  align-items: baseline;
  min-width: 0;
}

.task-log-line > span {
  color: var(--text-tertiary);
  font-family: var(--font-mono, ui-monospace, monospace);
  font-size: 0.68rem;
}

.task-log-line p,
.task-waiting-copy {
  margin: 0;
}

.task-log-line p {
  overflow-wrap: anywhere;
  color: var(--resource-muted);
  font-size: 0.75rem;
  line-height: 1.45;
}

.task-log-line.is-error p { color: var(--color-danger-700); }
.task-log-line.is-warning p { color: var(--color-warning-700); }
.task-log-line.is-success p { color: var(--color-success-700); }

.task-waiting-copy {
  margin-top: 9px;
  padding-top: 9px;
  border-top: 1px solid var(--resource-line);
}

.cleanup-error {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 10px 12px;
  border-color: var(--color-danger-200);
  background: var(--color-danger-100);
  color: var(--color-danger-700);
  font-size: 0.8125rem;
}

.cleanup-category {
  overflow: hidden;
  border: 1px solid var(--resource-line);
  border-radius: 10px;
  background: var(--resource-panel);
}

.cleanup-category + .cleanup-category {
  margin-top: 9px;
}

.cleanup-category-header {
  display: grid;
  grid-template-columns: 28px 28px 34px minmax(180px, 1fr) auto minmax(90px, auto);
  align-items: center;
  gap: 8px;
  min-height: 54px;
  padding: 0 14px 0 10px;
  background: var(--resource-surface);
}

.category-toggle,
.category-check {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--resource-muted);
  cursor: pointer;
}

.category-toggle:hover,
.category-check:hover {
  background: var(--resource-row-hover);
  color: var(--resource-accent-strong);
}

.category-check {
  border: 1px solid var(--resource-line-strong);
  background: var(--bg-elevated);
}

.category-check[aria-checked="true"],
.category-check[aria-checked="mixed"] {
  border-color: var(--resource-accent);
  background: var(--resource-accent);
  color: var(--text-inverse);
}

.category-kind {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 8px;
}

.category-kind.risk-low {
  background: var(--color-success-100);
  color: var(--color-success-700);
}

.category-kind.risk-medium {
  background: var(--color-warning-100);
  color: var(--color-warning-700);
}

.category-kind.risk-high {
  background: var(--color-danger-100);
  color: var(--color-danger-700);
}

.category-heading {
  display: flex;
  align-items: baseline;
  min-width: 0;
  gap: 8px;
}

.category-heading strong {
  overflow: hidden;
  color: var(--resource-ink);
  font-size: 0.875rem;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.category-heading span,
.category-size {
  color: var(--resource-muted);
  font-size: 0.75rem;
  white-space: nowrap;
}

.category-size {
  min-width: 80px;
  text-align: right;
  font-family: var(--font-mono, ui-monospace, monospace);
}

.cleanup-items {
  border-top: 1px solid var(--resource-line);
}

.cleanup-item {
  box-sizing: border-box;
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr) minmax(90px, auto);
  align-items: center;
  gap: 10px;
  width: 100%;
  min-height: var(--cleanup-row-height);
  padding: 8px 14px 8px 46px;
  border: 0;
  background: transparent;
  color: var(--resource-ink);
  text-align: left;
  cursor: pointer;
  transition: background-color var(--motion-duration-quick) var(--motion-ease-out);
}

.cleanup-item + .cleanup-item {
  border-top: 1px solid var(--resource-line);
}

.cleanup-item:hover {
  background: var(--resource-row-hover);
}

.cleanup-item.selected {
  background: var(--resource-row-focus);
  box-shadow: inset 3px 0 0 var(--resource-accent);
}

.item-check {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border: 1px solid var(--resource-line-strong);
  border-radius: 5px;
  background: var(--bg-elevated);
  color: var(--text-inverse);
}

.cleanup-item.selected .item-check {
  border-color: var(--resource-accent);
  background: var(--resource-accent);
}

.item-copy {
  display: flex;
  flex-direction: column;
  min-width: 0;
  gap: 2px;
}

.item-copy strong,
.item-copy small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.item-copy strong {
  color: var(--resource-ink);
  font-size: 0.8125rem;
  font-weight: 600;
}

.item-copy small {
  color: var(--resource-muted);
  font-family: var(--font-mono, ui-monospace, monospace);
  font-size: 0.7rem;
}

.cleanup-item-size {
  color: var(--resource-muted);
  font-family: var(--font-mono, ui-monospace, monospace);
  font-size: 0.75rem;
  text-align: right;
  white-space: nowrap;
}

.cleanup-empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  min-height: 260px;
  color: var(--resource-muted);
  font-size: 0.8125rem;
}

.is-spinning {
  animation: resource-refresh-spin 0.9s linear infinite;
}

@media (max-width: 900px) {
  .cleanup-category-header {
    grid-template-columns: 28px 28px 32px minmax(0, 1fr) auto;
  }

  .category-size {
    display: none;
  }

  .cleanup-item {
    padding-left: 14px;
  }
}

@media (max-width: 640px) {
  .cleanup-category-header {
    grid-template-columns: 28px 28px minmax(0, 1fr) auto;
  }

  .category-kind {
    display: none;
  }

  .cleanup-item {
    grid-template-columns: 24px minmax(0, 1fr);
  }

  .cleanup-item-size {
    grid-column: 2;
    text-align: left;
  }
}
</style>
