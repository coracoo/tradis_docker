<template>
  <ResourceWorkbench class="scheduled-tasks-workbench">
    <template #rail>
      <div class="resource-rail scheduled-tasks-rail">
        <article v-for="item in taskRail" :key="item.label" class="rail-card">
          <span class="rail-icon" :class="item.tone">
            <DynamicIcon :name="item.icon" :size="17" />
          </span>
          <div class="rail-copy">
            <strong>{{ item.value }}</strong>
            <span>{{ item.label }}</span>
          </div>
          <small>{{ item.note }}</small>
        </article>
      </div>
    </template>

    <template #toolbar>
      <div class="toolbar workbench-toolbar scheduled-toolbar">
        <div class="toolbar-left workbench-toolbar-left">
          <SearchInput
            v-model="searchQuery"
            placeholder="搜索任务名称、类型、调度或目标..."
          />
        </div>
        <div class="toolbar-right">
          <button class="secondary-btn" :disabled="loading" @click="loadJobs">
            <DynamicIcon name="refresh" :size="16" />
            刷新
          </button>
          <button class="primary-btn" @click="openCreateDialog">
            <DynamicIcon name="plus" :size="16" />
            新建任务
          </button>
        </div>
      </div>
    </template>

    <ResourcePanel>
      <ResourceTable
        :columns="taskColumns"
        :rows="filteredJobs"
        table-key="scheduled-tasks-v2"
        row-key="ID"
        :loading="loading && jobs.length === 0"
        :interactive="false"
        aria-label="定时任务列表"
      >
        <template #empty>
          <EmptyState
            v-if="jobs.length === 0"
            title="暂无定时任务"
            description="创建任务后，系统会按调度计划在后台执行"
          >
            <template #action>
              <button class="primary-btn empty-primary" @click="openCreateDialog">
                <DynamicIcon name="plus" :size="16" />
                新建任务
              </button>
            </template>
          </EmptyState>
          <EmptyState v-else title="没有匹配的任务" description="调整搜索关键词后再试" />
        </template>

        <template #cell-name="{ row: job }">
          <div class="resource-name">
            <span class="resource-kind task">
              <DynamicIcon name="clock" :size="16" />
            </span>
            <span class="resource-name-copy">
              <strong>{{ job.Name }}</strong>
              <small>任务 #{{ job.ID }}</small>
            </span>
          </div>
        </template>
        <template #cell-type="{ row: job }">
          <span class="type-badge" :class="`type-${job.TaskType}`">{{ taskTypeLabel(job.TaskType) }}</span>
        </template>
        <template #cell-cron="{ row: job }">
          <code class="cron-code">{{ job.CronExpr }}</code>
        </template>
        <template #cell-target="{ row: job }">
          <span class="cell-ellipsis" :title="job.Target">
            {{ job.TaskType === 'protection_backup' ? protectionTargetLabel(job) : (needsTarget(job.TaskType) ? formatTarget(job) : '-') }}
          </span>
        </template>
        <template #cell-enabled="{ row: job }">
          <button class="enable-toggle" :class="{ enabled: job.Enabled }" @click.stop="toggleEnabled(job)">
            <span class="toggle-dot"></span>
            <span>{{ job.Enabled ? '启用' : '禁用' }}</span>
          </button>
        </template>
        <template #cell-last="{ row: job }">
          <div class="time-cell">
            <span>{{ formatTime(job.LastRunAt) }}</span>
            <span v-if="job.LastStatus" class="status-badge" :class="`status-${job.LastStatus}`">
              {{ statusLabel(job.LastStatus) }}
            </span>
          </div>
        </template>
        <template #cell-next="{ row: job }">
          <span class="mono-cell">{{ formatTime(job.NextRunAt) }}</span>
        </template>
        <template #cell-actions="{ row: job }">
          <div class="cell-actions">
            <button class="table-btn primary" title="立即执行" @click.stop="runNow(job)">
              <DynamicIcon name="play" :size="14" />
            </button>
            <button class="table-btn" title="执行历史" @click.stop="openRunsDialog(job)">
              <DynamicIcon name="history" :size="14" />
            </button>
            <button class="table-btn" title="编辑" @click.stop="openEditDialog(job)">
              <DynamicIcon name="edit" :size="14" />
            </button>
            <button class="table-btn danger" title="删除" @click.stop="removeJob(job)">
              <DynamicIcon name="trash" :size="14" />
            </button>
          </div>
        </template>
      </ResourceTable>
    </ResourcePanel>

    <!-- 创建/编辑对话框 -->
    <Modal v-model:visible="showFormDialog" :title="editingJob ? '编辑定时任务' : '新建定时任务'" width="560px">
      <div class="task-form">
        <div class="form-group">
          <label class="form-label">任务名称</label>
          <input v-model="form.Name" class="form-input" placeholder="例如：每日清理构建缓存" />
        </div>

        <div class="form-group" v-if="!editingJob">
          <label class="form-label">任务类型</label>
          <select v-model="form.TaskType" class="form-select" @change="onTaskTypeChange">
            <option value="container_start">容器启动</option>
            <option value="container_stop">容器停止</option>
            <option value="container_restart">容器重启</option>
            <option value="image_update_check">镜像更新检测</option>
            <option value="build_cache_prune">清理构建缓存</option>
          </select>
        </div>

        <!-- 容器多选 -->
        <div class="form-group" v-if="needsTarget(form.TaskType)">
          <label class="form-label">目标容器（可多选）</label>
          <div class="container-select">
            <div v-if="containersLoading" class="container-loading">加载容器中...</div>
            <div v-else-if="filteredContainers.length === 0" class="container-empty">没有可用容器</div>
            <div v-else class="container-list">
              <label
                v-for="c in filteredContainers"
                :key="c.Id"
                class="container-item"
                :class="{ selected: selectedContainerIds.has(c.Id) }"
              >
                <input
                  type="checkbox"
                  :checked="selectedContainerIds.has(c.Id)"
                  @change="toggleContainer(c.Id)"
                />
                <span class="container-name">{{ containerDisplayName(c) }}</span>
              </label>
            </div>
          </div>
        </div>

        <!-- 构建缓存清理参数 -->
        <div class="form-group" v-if="form.TaskType === 'build_cache_prune'">
          <label class="form-checkbox-label">
            <input v-model="pruneAll" type="checkbox" />
            <span>清理所有缓存（含正在使用的，谨慎勾选）</span>
          </label>
        </div>

        <!-- Cron 表达式 -->
        <div class="form-group">
          <label class="form-label">调度时间</label>
          <div class="cron-presets">
            <button
              v-for="p in cronPresets"
              :key="p.expr"
              class="preset-btn"
              :class="{ active: form.CronExpr === p.expr }"
              @click="form.CronExpr = p.expr"
            >{{ p.label }}</button>
          </div>
          <input v-model="form.CronExpr" class="form-input" placeholder="例如：0 3 * * * 表示每天 3 点" />
          <p class="form-hint">
            支持 5 字段 cron（分 时 日 月 周）或描述符 @daily / @hourly / @weekly。
            <span v-if="cronPreview" class="cron-next">下次运行：{{ cronPreview }}</span>
          </p>
        </div>

        <div class="form-group">
          <label class="form-checkbox-label">
            <input v-model="form.Enabled" type="checkbox" />
            <span>立即启用</span>
          </label>
        </div>
      </div>
      <template #footer>
        <button class="btn btn-default" @click="showFormDialog = false">取消</button>
        <button class="btn btn-primary" :disabled="!isFormValid || submitting" @click="submitForm">
          {{ submitting ? '保存中...' : (editingJob ? '保存' : '创建') }}
        </button>
      </template>
    </Modal>

    <!-- 执行历史对话框 -->
    <Modal v-model:visible="showRunsDialog" :title="`执行历史 - ${runsJob?.Name || ''}`" width="640px">
      <div class="runs-list">
        <div v-if="runsLoading" class="loading-state small">
          <DynamicIcon name="loader" :size="24" />
          <span>加载中...</span>
        </div>
        <div v-else-if="runs.length === 0" class="empty-cell">暂无执行记录</div>
        <div v-else class="run-items">
          <div v-for="run in runs" :key="run.ID" class="run-item">
            <span class="run-status-dot" :class="`dot-${run.Status}`"></span>
            <span class="run-time">{{ formatTime(run.CreatedAt) }}</span>
            <span class="status-badge" :class="`status-${run.Status}`">{{ statusLabel(run.Status) }}</span>
            <span v-if="run.Error" class="run-error" :title="run.Error">{{ run.Error }}</span>
          </div>
        </div>
      </div>
      <template #footer>
        <button class="btn btn-default" @click="showRunsDialog = false">关闭</button>
      </template>
    </Modal>
  </ResourceWorkbench>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import Modal from '@/components/feedback/Modal.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import ResourceWorkbench from '@/components/resource-workbench/ResourceWorkbench.vue'
import ResourcePanel from '@/components/resource-workbench/ResourcePanel.vue'
import ResourceTable from '@/components/resource-workbench/ResourceTable.vue'
import { usePageAction } from '@/composables/usePageAction.js'
import { useUiStore } from '@/stores/ui.js'
import taskApi from '@/api/scheduledTasks.js'
import containerApi from '@/api/containers.js'
import { listProtectionPolicies } from '@edition/scheduled-task-features'
import { formatTime } from '@/utils/format.js'

const uiStore = useUiStore()

const jobs = ref([])
const loading = ref(false)
const submitting = ref(false)
const searchQuery = ref('')
const protectionPolicies = ref([])

// 容器列表（创建表单用）
const containersList = ref([])
const containersLoading = ref(false)
const selectedContainerIds = ref(new Set())

// 表单状态
const showFormDialog = ref(false)
const editingJob = ref(null)
const pruneAll = ref(false)
const form = ref({
  Name: '',
  TaskType: 'build_cache_prune',
  CronExpr: '@daily',
  Enabled: true
})

// cron 预览
const cronPreview = ref('')

// 执行历史
const showRunsDialog = ref(false)
const runsJob = ref(null)
const runs = ref([])
const runsLoading = ref(false)

const cronPresets = [
  { label: '每小时', expr: '@hourly' },
  { label: '每天 0 点', expr: '@daily' },
  { label: '每天 3 点', expr: '0 3 * * *' },
  { label: '每周', expr: '@weekly' },
  { label: '每月', expr: '0 0 1 * *' }
]

const CONTAINER_TASKS = ['container_start', 'container_stop', 'container_restart']

const taskColumns = [
  { key: 'name', label: '名称', width: 164, minWidth: 142, flex: true },
  { key: 'type', label: '类型', width: 112, minWidth: 96 },
  { key: 'cron', label: '调度', width: 102, minWidth: 90 },
  { key: 'target', label: '目标', width: 120, minWidth: 104 },
  { key: 'enabled', label: '状态', width: 88, minWidth: 82 },
  { key: 'last', label: '上次运行', width: 148, minWidth: 128 },
  { key: 'next', label: '下次运行', width: 130, minWidth: 114 },
  { key: 'actions', label: '操作', headerClass: 'actions-heading', cellClass: 'actions-cell', width: 140, minWidth: 136 }
]

const taskRail = computed(() => {
  const enabled = jobs.value.filter((job) => job.Enabled).length
  const containerTasks = jobs.value.filter((job) => CONTAINER_TASKS.includes(job.TaskType)).length
  const failed = jobs.value.filter((job) => ['failed', 'error'].includes(job.LastStatus)).length
  return [
    { label: '全部任务', value: jobs.value.length, note: '已登记的后台调度', icon: 'clock', tone: 'info' },
    { label: '已启用', value: enabled, note: '当前参与调度', icon: 'check-circle', tone: 'success' },
    { label: '容器任务', value: containerTasks, note: '启动、停止与重启', icon: 'container', tone: 'primary' },
    { label: '最近失败', value: failed, note: failed ? '建议检查执行历史' : '最近运行正常', icon: 'alert-circle', tone: failed ? 'warning' : 'muted' }
  ]
})

function needsTarget(type) {
  return CONTAINER_TASKS.includes(type)
}

function taskTypeLabel(type) {
  const map = {
    container_start: '容器启动',
    container_stop: '容器停止',
    container_restart: '容器重启',
    image_update_check: '镜像更新检测',
    build_cache_prune: '清理构建缓存',
    protection_backup: '应用保护备份'
  }
  return map[type] || type
}

function statusLabel(status) {
  const map = { running: '运行中', success: '成功', failed: '失败', completed: '完成', pending: '等待', error: '失败' }
  return map[status] || status
}

function formatTarget(job) {
  if (!job.Target) return '-'
  const ids = String(job.Target).split(',').filter(Boolean)
  if (ids.length <= 1) return ids[0]?.slice(0, 12) || '-'
  return `${ids[0].slice(0, 12)} 等 ${ids.length} 个`
}

const filteredContainers = computed(() => containersList.value)

const filteredJobs = computed(() => {
  const keyword = searchQuery.value.trim().toLowerCase()
  if (!keyword) return jobs.value
  return jobs.value.filter((job) => {
    const parts = [
      job.Name,
      job.TaskType,
      taskTypeLabel(job.TaskType),
      job.CronExpr,
      job.Target,
      formatTarget(job),
      protectionTargetLabel(job),
      job.Enabled ? '启用' : '禁用',
      statusLabel(job.LastStatus || '')
    ]
    return parts.some((part) => String(part || '').toLowerCase().includes(keyword))
  })
})

function containerDisplayName(c) {
  const name = (c.Names?.[0] || c.Name || '').replace(/^\//, '')
  return `${name || c.Id.slice(0, 12)}`
}

function protectionTargetLabel(job) {
  if (job?.TaskType !== 'protection_backup') return ''
  const policy = protectionPolicies.value.find(item => item.id === job.Target)
  return policy ? `${policy.projectName} · ${policy.name}` : '应用保护策略已删除'
}

const isFormValid = computed(() => {
  if (!form.value.Name?.trim()) return false
  if (!form.value.CronExpr?.trim()) return false
  if (needsTarget(form.value.TaskType) && selectedContainerIds.value.size === 0) return false
  return true
})

// --- 数据加载 ---
async function loadJobs() {
  loading.value = true
  try {
    const [jobsResult, policiesResult] = await Promise.allSettled([
      taskApi.list(),
      listProtectionPolicies()
    ])
    if (jobsResult.status === 'rejected') {
      throw jobsResult.reason
    }
    jobs.value = jobsResult.value?.jobs || []
    if (policiesResult.status === 'fulfilled') {
      protectionPolicies.value = Array.isArray(policiesResult.value) ? policiesResult.value : []
    } else {
      protectionPolicies.value = []
      uiStore.toastWarning('应用保护策略暂时无法读取，其他定时任务仍可正常管理')
    }
  } catch (e) {
    uiStore.toastError('加载任务失败: ' + (e.message || ''))
  } finally {
    loading.value = false
  }
}

async function loadContainers() {
  containersLoading.value = true
  try {
    const data = await containerApi.list({ all: true })
    containersList.value = Array.isArray(data) ? data : (data?.containers || [])
  } catch (e) {
    uiStore.toastError('加载容器列表失败: ' + (e.message || ''))
  } finally {
    containersLoading.value = false
  }
}

function toggleContainer(id) {
  const next = new Set(selectedContainerIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selectedContainerIds.value = next
}

function onTaskTypeChange() {
  // 切换到容器任务时自动加载容器
  if (needsTarget(form.value.TaskType) && containersList.value.length === 0) {
    loadContainers()
  }
  if (!needsTarget(form.value.TaskType)) {
    selectedContainerIds.value = new Set()
  }
}

// --- cron 预览（防抖）---
let cronTimer = null
watch(() => form.value.CronExpr, (expr) => {
  cronPreview.value = ''
  if (cronTimer) clearTimeout(cronTimer)
  if (!expr?.trim()) return
  cronTimer = setTimeout(async () => {
    try {
      const res = await taskApi.previewCron(expr.trim())
      if (res?.valid && res.next_run) {
        cronPreview.value = formatTime(res.next_run)
      }
    } catch {
      // 静默：输入未完成时频繁失败
    }
  }, 400)
})

// --- 表单 ---
function resetForm() {
  form.value = { Name: '', TaskType: 'build_cache_prune', CronExpr: '@daily', Enabled: true }
  pruneAll.value = false
  selectedContainerIds.value = new Set()
  editingJob.value = null
  cronPreview.value = ''
}

async function openCreateDialog() {
  resetForm()
  showFormDialog.value = true
}

async function openEditDialog(job) {
  resetForm()
  editingJob.value = job
  form.value = {
    Name: job.Name,
    TaskType: job.TaskType,
    CronExpr: job.CronExpr,
    Enabled: job.Enabled
  }
  if (job.TaskType === 'build_cache_prune' && job.PayloadJSON) {
    try { pruneAll.value = JSON.parse(job.PayloadJSON).all === true } catch { pruneAll.value = false }
  }
  if (needsTarget(job.TaskType) && job.Target) {
    String(job.Target).split(',').filter(Boolean).forEach(id => selectedContainerIds.value.add(id.trim()))
    if (containersList.value.length === 0) await loadContainers()
  }
  showFormDialog.value = true
}

async function submitForm() {
  if (!isFormValid.value) return
  submitting.value = true
  try {
    const payload = {
      name: form.value.Name.trim(),
      task_type: form.value.TaskType,
      cron_expr: form.value.CronExpr.trim(),
      enabled: form.value.Enabled
    }
    if (needsTarget(form.value.TaskType)) {
      payload.target = Array.from(selectedContainerIds.value).join(',')
    }
    if (form.value.TaskType === 'build_cache_prune') {
      payload.payload_json = JSON.stringify({ all: pruneAll.value })
    }

    if (editingJob.value) {
      await taskApi.update(editingJob.value.ID, payload)
      uiStore.toastSuccess('任务已更新')
    } else {
      await taskApi.create(payload)
      uiStore.toastSuccess('任务已创建')
    }
    showFormDialog.value = false
    await loadJobs()
  } catch (e) {
    uiStore.toastError('保存失败: ' + (e.message || ''))
  } finally {
    submitting.value = false
  }
}

async function toggleEnabled(job) {
  try {
    await taskApi.update(job.ID, { name: job.Name, task_type: job.TaskType, cron_expr: job.CronExpr, enabled: !job.Enabled })
    uiStore.toastSuccess(job.Enabled ? '已禁用' : '已启用')
    await loadJobs()
  } catch (e) {
    uiStore.toastError('操作失败: ' + (e.message || ''))
  }
}

async function runNow(job) {
  const confirmed = await uiStore.confirm({
    title: '立即执行',
    message: `确定立即执行任务「${job.Name}」吗？`,
    confirmText: '执行'
  })
  if (!confirmed) return
  try {
    await taskApi.runNow(job.ID)
    uiStore.toastSuccess('已触发，请稍后查看执行历史')
    setTimeout(() => loadJobs(), 2000)
  } catch (e) {
    uiStore.toastError('触发失败: ' + (e.message || ''))
  }
}

async function removeJob(job) {
  const confirmed = await uiStore.confirm({
    type: 'danger',
    title: '删除任务',
    message: `确定删除定时任务「${job.Name}」吗？此操作不可撤销。`,
    confirmText: '删除'
  })
  if (!confirmed) return
  try {
    await taskApi.remove(job.ID)
    uiStore.toastSuccess('已删除')
    await loadJobs()
  } catch (e) {
    uiStore.toastError('删除失败: ' + (e.message || ''))
  }
}

async function openRunsDialog(job) {
  runsJob.value = job
  runs.value = []
  runsLoading.value = true
  showRunsDialog.value = true
  try {
    const data = await taskApi.runs(job.ID)
    runs.value = data?.runs || []
  } catch (e) {
    uiStore.toastError('加载执行历史失败: ' + (e.message || ''))
  } finally {
    runsLoading.value = false
  }
}

usePageAction({ create: openCreateDialog })

onMounted(() => {
  loadJobs()
})
</script>

<style scoped>
.scheduled-tasks-workbench {
  --resource-accent: var(--color-primary-500);
  --resource-accent-strong: var(--color-primary-700);
  --resource-row-hover: color-mix(in srgb, var(--color-primary-500) 7%, var(--bg-elevated));
}

.rail-icon.info,
.rail-icon.primary {
  color: var(--color-primary-700);
  background: var(--color-primary-100);
}

.rail-icon.success {
  color: var(--color-success-700);
  background: var(--color-success-100);
}

.rail-icon.warning {
  color: var(--color-warning-700);
  background: var(--color-warning-100);
}

.rail-icon.muted {
  color: var(--text-secondary);
  background: var(--bg-tertiary);
}

.toolbar-right,
.secondary-btn,
.resource-name,
.resource-name-copy,
.time-cell {
  display: flex;
  align-items: center;
}

.toolbar-right {
  gap: 8px;
  flex: 0 0 auto;
}

.secondary-btn,
.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 38px;
  padding: 0 14px;
  gap: 7px;
  border: 1px solid var(--border-default);
  border-radius: 9px;
  background: var(--bg-elevated);
  color: var(--text-primary);
  font-family: var(--font-sans, Inter, ui-sans-serif, system-ui, sans-serif);
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: background-color var(--motion-duration-quick) var(--motion-ease-out), border-color var(--motion-duration-quick) var(--motion-ease-out), color var(--motion-duration-quick) var(--motion-ease-out), transform var(--motion-duration-quick) var(--motion-ease-out);
}

.secondary-btn {
  display: inline-flex;
  justify-content: center;
}

.secondary-btn:hover:not(:disabled),
.btn:hover:not(:disabled) {
  border-color: var(--color-primary-300);
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

.secondary-btn:disabled,
.btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.btn-primary {
  border-color: var(--color-primary-700);
  background: linear-gradient(135deg, var(--color-primary-500), var(--color-primary-600));
  color: var(--text-inverse);
}

.btn-default {
  background: var(--bg-elevated);
}

.primary-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  min-height: 38px;
  padding: 0 14px;
  border: 1px solid var(--color-primary-700);
  border-radius: 10px;
  background: linear-gradient(135deg, var(--color-primary-500), var(--color-primary-600));
  color: var(--text-inverse);
  font-family: var(--font-sans, Inter, ui-sans-serif, system-ui, sans-serif);
  font-size: 0.875rem;
  font-weight: 600;
  line-height: 1;
  white-space: nowrap;
  cursor: pointer;
  transition: transform var(--motion-duration-quick) var(--motion-ease-out), border-color var(--motion-duration-quick) var(--motion-ease-out), background var(--motion-duration-quick) var(--motion-ease-out), box-shadow var(--motion-duration-quick) var(--motion-ease-out);
}

.primary-btn:hover:not(:disabled) {
  transform: translateY(-1px);
  border-color: color-mix(in srgb, var(--color-primary-700) 82%, black);
  background: linear-gradient(135deg, var(--color-primary-600), var(--color-primary-700));
  box-shadow: 0 5px 14px color-mix(in srgb, var(--color-primary-500) 28%, transparent);
}

.primary-btn:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.empty-primary {
  margin: 0 auto;
}

.resource-name {
  min-width: 0;
  gap: 10px;
}

.resource-kind {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 32px;
  width: 32px;
  height: 32px;
  border-radius: 9px;
  color: var(--color-primary-700);
  background: var(--color-primary-100);
}

.resource-name-copy {
  flex-direction: column;
  align-items: flex-start;
  min-width: 0;
  line-height: 1.25;
}

.resource-name-copy strong,
.resource-name-copy small {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.resource-name-copy small {
  margin-top: 3px;
}

.cron-code,
.mono-cell {
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 0.75rem;
  color: var(--text-secondary);
}

.cron-code {
  padding: 3px 7px;
  border-radius: 5px;
  background: var(--bg-tertiary);
}

.cell-ellipsis {
  overflow: hidden;
  color: var(--text-secondary);
  font-size: 0.8125rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.empty-cell {
  color: var(--text-tertiary);
}

.time-cell {
  flex-wrap: wrap;
  gap: 5px 7px;
  min-width: 0;
  color: var(--text-secondary);
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 0.72rem;
}

.type-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 10px;
  font-size: 0.75rem;
  background: var(--color-primary-50, var(--bg-tertiary));
  color: var(--color-primary-600, var(--text-primary));
  white-space: nowrap;
}

.type-badge.type-build_cache_prune {
  background: var(--color-warning-50, var(--bg-tertiary));
  color: var(--color-warning-600, var(--text-primary));
}
.type-badge.type-image_update_check {
  background: var(--color-accent-50, var(--bg-tertiary));
  color: var(--color-accent-600, var(--text-primary));
}
.type-badge.type-protection_backup {
  background: var(--color-success-50, var(--bg-tertiary));
  color: var(--color-success-600, var(--text-primary));
}

.status-badge {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 8px;
  font-size: 0.6875rem;
  white-space: nowrap;
}
.status-badge.status-success, .status-badge.status-completed {
  background: var(--color-success-100);
  color: var(--color-success-700);
}
.status-badge.status-failed, .status-badge.status-error {
  background: var(--color-danger-100);
  color: var(--color-danger-700);
}
.status-badge.status-running {
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

.enable-toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 10px;
  border-radius: 999px;
  border: 1px solid var(--border-default);
  background: var(--bg-tertiary);
  cursor: pointer;
  font-size: 0.75rem;
  color: var(--text-tertiary);
}
.enable-toggle.enabled {
  background: var(--color-success-100);
  color: var(--color-success-700);
  border-color: var(--color-success-200);
}
.toggle-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--text-tertiary);
}
.enable-toggle.enabled .toggle-dot {
  background: var(--color-success-500);
}

.cell-actions {
  display: flex;
  gap: 4px;
  white-space: nowrap;
  width: 100%;
}

.table-btn {
  width: 30px;
  height: 30px;
  border-radius: 6px;
  border: 1px solid transparent;
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}
.table-btn:hover {
  background: var(--bg-tertiary);
  color: var(--text-primary);
}
.table-btn.danger:hover {
  background: var(--color-danger-100);
  color: var(--color-danger-600);
}
.table-btn.primary:hover {
  background: var(--color-primary-100);
  color: var(--color-primary-600);
}

/* 表单 */
.task-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-label {
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--text-primary);
}

.form-input, .form-select {
  padding: 8px 12px;
  border: 1px solid var(--border-default);
  border-radius: 8px;
  background: var(--bg-primary);
  color: var(--text-primary);
  font-size: 0.875rem;
  outline: none;
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out);
}
.form-input:focus, .form-select:focus {
  border-color: var(--color-primary-500, var(--color-primary));
}

.form-hint {
  font-size: 0.75rem;
  color: var(--text-tertiary);
  margin: 0;
}
.cron-next {
  color: var(--color-primary-600, var(--color-primary));
  margin-left: 8px;
}

.cron-presets {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.preset-btn {
  padding: 4px 10px;
  border-radius: 999px;
  border: 1px solid var(--border-default);
  background: var(--bg-secondary);
  color: var(--text-secondary);
  font-size: 0.75rem;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}
.preset-btn:hover {
  border-color: var(--color-primary-500, var(--color-primary));
  color: var(--color-primary-600, var(--color-primary));
}
.preset-btn.active {
  background: var(--color-primary-500, var(--color-primary));
  color: var(--text-inverse);
  border-color: transparent;
}

.form-checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 0.8125rem;
  color: var(--text-primary);
  cursor: pointer;
}
.form-checkbox-label input[type="checkbox"] {
  width: 16px;
  height: 16px;
  cursor: pointer;
}

/* 容器选择 */
.container-select {
  max-height: 200px;
  overflow-y: auto;
  border: 1px solid var(--border-default);
  border-radius: 8px;
  padding: 4px;
}
.container-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.container-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.8125rem;
}
.container-item:hover {
  background: var(--bg-secondary);
}
.container-item.selected {
  background: var(--color-primary-50, var(--bg-tertiary));
}
.container-item input[type="checkbox"] {
  width: 16px;
  height: 16px;
}
.container-name {
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
}
.container-loading, .container-empty {
  padding: 12px;
  text-align: center;
  color: var(--text-tertiary);
  font-size: 0.8125rem;
}

/* 执行历史 */
.runs-list {
  max-height: 400px;
  overflow-y: auto;
}
.run-items {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.run-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  font-size: 0.8125rem;
}
.run-time {
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  color: var(--text-secondary);
}
.run-error {
  color: var(--color-danger-600);
  font-size: 0.75rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
}
.run-status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-tertiary);
  flex-shrink: 0;
}
.run-status-dot.dot-success, .run-status-dot.dot-completed {
  background: var(--color-success-500);
}
.run-status-dot.dot-failed, .run-status-dot.dot-error {
  background: var(--color-danger-500);
}
.run-status-dot.dot-running {
  background: var(--color-primary-500, var(--color-primary));
}

.loading-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 60px 20px;
  color: var(--text-tertiary);
}
.loading-state.small {
  padding: 30px 20px;
}

@media (max-width: 768px) {
  .scheduled-tasks-workbench {
    --resource-toolbar-search-width: 100%;
  }

  .toolbar-right {
    width: 100%;
  }

  .toolbar-right > * {
    flex: 1 1 0;
  }
}
</style>
