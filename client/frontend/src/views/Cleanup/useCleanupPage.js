import { computed, onScopeDispose, ref, shallowRef } from 'vue'
import cleanupApi from '../../api/cleanup.js'

const TERMINAL_TASK_STATUSES = new Set(['success', 'error', 'failed', 'completed', 'cancelled', 'canceled'])

export function useCleanupPage({ api = cleanupApi, EventSourceImpl = globalThis.EventSource } = {}) {
  const evaluation = ref(emptyEvaluation())
  const query = ref('')
  const riskFilter = ref('all')
  const selectedKeys = shallowRef(new Set())
  const activeTask = ref(null)
  const taskLogs = ref([])
  const loading = ref(false)
  const evaluating = ref(false)
  const error = ref('')

  let taskSource = null

  const allItems = computed(() => (
    evaluation.value.categories.flatMap(category => category.items || [])
  ))

  const filteredCategories = computed(() => {
    const term = query.value.trim().toLowerCase()
    return evaluation.value.categories
      .filter(category => riskFilter.value === 'all' || category.risk === riskFilter.value)
      .map(category => ({
        ...category,
        items: (category.items || []).filter(item => {
          if (!term) return true
          return [item.name, item.detail, item.id, category.label]
            .filter(Boolean)
            .join(' ')
            .toLowerCase()
            .includes(term)
        })
      }))
      .filter(category => category.items.length > 0)
  })

  const selectedItems = computed(() => (
    allItems.value.filter(item => selectedKeys.value.has(item.key))
  ))

  const selectedKnownBytes = computed(() => (
    selectedItems.value.reduce((total, item) => (
      item.sizeState === 'known' ? total + Number(item.sizeBytes || 0) : total
    ), 0)
  ))

  const selectedUnknownSizeCount = computed(() => (
    selectedItems.value.filter(item => item.sizeState === 'unknown').length
  ))

  const requiresHighRiskConfirmation = computed(() => (
    selectedItems.value.some(item => item.category === 'unused_volume')
  ))

  const taskRunning = computed(() => {
    const status = String(activeTask.value?.status || '').toLowerCase()
    return status === 'pending' || status === 'running'
  })

  async function loadEvaluation({ resetSelection = true, force = false } = {}) {
    evaluating.value = true
    error.value = ''
    try {
      const result = normalizeEvaluation(await api.getEvaluation({ refresh: force }))
      evaluation.value = result
      if (resetSelection) {
        selectedKeys.value = new Set()
      } else {
        const available = new Set(result.categories.flatMap(category => category.items.map(item => item.key)))
        selectedKeys.value = new Set([...selectedKeys.value].filter(key => available.has(key)))
      }
      return result
    } catch (reason) {
      error.value = readableError(reason, '评估 Docker 空间失败')
      throw reason
    } finally {
      evaluating.value = false
    }
  }

  function toggleItem(item) {
    const next = new Set(selectedKeys.value)
    if (next.has(item.key)) next.delete(item.key)
    else next.add(item.key)
    selectedKeys.value = next
  }

  function toggleCategory(category) {
    const items = category.items || []
    const next = new Set(selectedKeys.value)
    const allSelected = items.length > 0 && items.every(item => next.has(item.key))
    for (const item of items) {
      if (allSelected) next.delete(item.key)
      else next.add(item.key)
    }
    selectedKeys.value = next
  }

  async function submitCleanup({ confirmHighRisk = false } = {}) {
    if (selectedItems.value.length === 0 || taskRunning.value) return null
    loading.value = true
    error.value = ''
    disconnectTaskStream()
    taskLogs.value = []
    try {
      const response = await api.startTask({
        items: selectedItems.value.map(item => ({ category: item.category, id: item.id })),
        confirmHighRisk
      })
      activeTask.value = {
        ...response,
        id: response.id || response.taskId,
        status: response.status || 'pending'
      }
      connectTaskStream(activeTask.value)
      return activeTask.value
    } catch (reason) {
      error.value = readableError(reason, '启动空间清理失败')
      throw reason
    } finally {
      loading.value = false
    }
  }

  async function resumeActiveTask() {
    try {
      const tasks = await api.listTasks({ statuses: 'pending,running', limit: 10 })
      const task = Array.isArray(tasks) ? tasks[0] : null
      if (!task) return null
      activeTask.value = task
      connectTaskStream(task)
      return task
    } catch (reason) {
      error.value = readableError(reason, '恢复空间清理任务失败')
      return null
    }
  }

  async function initialize() {
    await Promise.allSettled([loadEvaluation(), resumeActiveTask()])
  }

  function connectTaskStream(task) {
    const taskID = task?.id || task?.taskId
    if (!taskID || typeof EventSourceImpl !== 'function') return
    disconnectTaskStream()
    const source = new EventSourceImpl(api.getTaskEventsUrl(taskID))
    taskSource = source
    source.onmessage = async event => {
      const payload = parseTaskEvent(event.data)
      if (payload.type !== 'result') {
        taskLogs.value = [...taskLogs.value, payload]
        return
      }
      disconnectTaskStream()
      try {
        activeTask.value = await api.getTask(taskID)
      } catch {
        activeTask.value = { ...task, id: taskID, status: payload.status, error: payload.error || '' }
      }
      await loadEvaluation({ force: true })
    }
    source.onerror = () => {
      if (TERMINAL_TASK_STATUSES.has(String(activeTask.value?.status || '').toLowerCase())) {
        disconnectTaskStream()
      }
    }
  }

  function disconnectTaskStream() {
    taskSource?.close?.()
    taskSource = null
  }

  onScopeDispose(disconnectTaskStream)

  return {
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
    submitCleanup,
    resumeActiveTask,
    disconnectTaskStream
  }
}

function emptyEvaluation() {
  return {
    evaluatedAt: '',
    summary: { candidateCount: 0, knownReclaimableBytes: 0, unknownSizeCount: 0 },
    categories: []
  }
}

function normalizeEvaluation(value) {
  const result = value && typeof value === 'object' ? value : emptyEvaluation()
  return {
    evaluatedAt: result.evaluatedAt || '',
    summary: {
      candidateCount: Number(result.summary?.candidateCount || 0),
      knownReclaimableBytes: Number(result.summary?.knownReclaimableBytes || 0),
      unknownSizeCount: Number(result.summary?.unknownSizeCount || 0)
    },
    categories: Array.isArray(result.categories)
      ? result.categories.map(category => ({ ...category, items: Array.isArray(category.items) ? category.items : [] }))
      : []
  }
}

function parseTaskEvent(raw) {
  if (raw && typeof raw === 'object') return raw
  try {
    return JSON.parse(raw)
  } catch {
    return { type: 'info', message: String(raw || '') }
  }
}

function readableError(reason, fallback) {
  return String(reason?.message || reason?.error || fallback)
}
