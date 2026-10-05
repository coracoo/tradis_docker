<template>
  <Modal
    v-model:visible="localVisible"
    title="项目更新"
    width="800px"
    :show-close="true"
    :close-on-esc="true"
    @close="handleClose"
  >
    <template #footer>
      <button 
        v-ripple
        class="action-btn" 
        @click="handleClose"
      >
        {{ streaming ? '后台运行并关闭' : '关闭' }}
      </button>
    </template>
    
    <div ref="logsContainer" class="update-logs ops-console ops-console__body ops-console--standalone">
      <div v-if="logs.length === 0" class="logs-placeholder">
        正在连接更新服务...
      </div>
      <div
        v-for="(log, index) in logs"
        :key="index"
        :class="['update-log-line', log.type]"
      >
        <span class="log-time">[{{ formatLogTime(log.time) }}]</span>
        <span class="log-message">{{ log.message }}</span>
      </div>
    </div>
  </Modal>
</template>

<script setup>
import { ref, watch, nextTick, computed, onBeforeUnmount } from 'vue'
import Modal from '@/components/feedback/Modal.vue'
import { compose } from '@edition/api'
import { formatTime } from '@/utils/format.js'

const props = defineProps({
  visible: { type: Boolean, default: false },
  projectName: { type: String, default: '' }
})

const emit = defineEmits(['update:visible', 'completed', 'task-started'])

// 本地visible状态，用于v-model
const localVisible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val)
})

// 本地状态
const logs = ref([])
const streaming = ref(false)
const logsContainer = ref(null)
let eventSource = null

// 监听visible变化
watch(() => props.visible, (newVal) => {
  if (newVal && props.projectName) {
    startUpdateStream()
  } else if (!newVal) {
    cleanup()
  }
})

function handleClose() {
  cleanup()
  emit('update:visible', false)
}

function cleanup() {
  streaming.value = false
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
}

// 组件卸载时关闭 SSE 连接，避免依赖父组件手动调用 cleanup() 造成泄漏
onBeforeUnmount(() => {
  cleanup()
})

function formatLogTime(timestamp) {
  if (!timestamp) return '—'
  return formatTime(timestamp)
}

async function startUpdateStream() {
  logs.value = []
  streaming.value = true
  
  await nextTick()
  
  // 关闭之前的连接
  if (eventSource) {
    eventSource.close()
  }
  
  let taskId = ''
  try {
    const res = await compose.updateTask(props.projectName, { forcePull: true })
    taskId = String(res?.taskId || '')
    if (!taskId) throw new Error('未获取到任务ID')
    emit('task-started', { name: props.projectName, taskId })
  } catch (error) {
    streaming.value = false
    logs.value.push({
      type: 'error',
      message: '提交更新任务失败: ' + (error.message || '未知错误'),
      time: Date.now()
    })
    return
  }

  const url = compose.getTaskEventsUrl(taskId)

  eventSource = new EventSource(url)
  
  eventSource.onopen = () => {
    logs.value.push({
      type: 'info',
      message: '连接成功，开始拉取镜像并更新容器，保持项目原有运行状态...',
      time: Date.now()
    })
  }
  
  // 标记更新是否已完成
  let updateCompleted = false
  
  eventSource.onmessage = (event) => {
    let payload
    try {
      payload = JSON.parse(event.data || '{}')
    } catch {
      payload = { message: String(event.data || '') }
    }

    if (payload.type === 'result') {
      if (updateCompleted) return
      streaming.value = false
      updateCompleted = true
      eventSource.close()
      const succeeded = payload.status === 'success'
      const pullFailures = Array.isArray(payload.result?.pullFailures) ? payload.result.pullFailures : []
      let type = succeeded ? (pullFailures.length ? 'warning' : 'success') : 'error'
      let message = succeeded
          ? (pullFailures.length
              ? `更新任务完成，但部分镜像拉取失败（${pullFailures.length} 项），对应服务使用本地镜像`
              : '更新任务完成')
          : `更新任务失败: ${payload.error || ''}`
      const serviceUpdates = Array.isArray(payload.result?.serviceUpdates) ? payload.result.serviceUpdates : []
      if (serviceUpdates.length) {
        const labels = {
          updated: '已更新', unchanged: '无需更新', applied: '已应用',
          pull_failed: '拉取失败', blocked: '更新受阻', apply_failed: '应用失败', unknown: '结果未确认'
        }
        const counts = {}
        for (const service of serviceUpdates) {
          const status = Object.hasOwn(labels, service.status) ? service.status : 'unknown'
          counts[status] = (counts[status] || 0) + 1
        }
        const completedCount = (counts.updated || 0) + (counts.unchanged || 0) + (counts.applied || 0)
        const incomplete = completedCount !== serviceUpdates.length || pullFailures.length > 0
        const partial = completedCount > 0 && (incomplete || !succeeded)
        type = partial || (succeeded && incomplete) ? 'warning' : (succeeded ? 'success' : 'error')
        const summary = Object.entries(labels)
          .filter(([status]) => counts[status])
          .map(([status, label]) => `${label} ${counts[status]} 个${status === 'applied' ? '（版本变化未确认）' : ''}`)
          .join('，')
        message = `${partial ? '更新部分完成' : (succeeded ? '更新任务完成' : '更新任务失败')}：${summary}`
        if (pullFailures.length && !counts.pull_failed) message += `；部分镜像拉取失败（${pullFailures.length} 项）`
        if (payload.error) message += `。${payload.error}`
      }
      logs.value.push({ type, message, time: Date.now() })
      emit('completed')
      return
    }

    const str = String(payload.message || '')
    
    const hasStructuredType = ['error', 'warning', 'info', 'success'].includes(payload.type)
    let type = hasStructuredType ? payload.type : 'info'
    let message = str

    if (!hasStructuredType) {
      const prefix = str.match(/^(error|warning|warn|info|success):\s*/)
      if (prefix) {
        type = prefix[1] === 'warn' ? 'warning' : prefix[1]
        message = str.substring(prefix[0].length).trim()
      }
    }
    
    logs.value.push({ type, message, time: Date.now() })
    
    // 自动滚动
    nextTick(() => {
      if (logsContainer.value) {
        logsContainer.value.scrollTop = logsContainer.value.scrollHeight
      }
    })
  }
  
  eventSource.onerror = () => {
    if (updateCompleted) return
    streaming.value = false
    eventSource.close()
    logs.value.push({
      type: 'warning',
      message: '任务日志连接已断开，可稍后在任务中心查看结果',
      time: Date.now()
    })
  }
  
  eventSource.addEventListener('close', () => {
    streaming.value = false
    eventSource.close()
  })
}

// 暴露方法给父组件
defineExpose({
  cleanup
})
</script>

<style scoped>
.update-logs {
  padding: 14px;
  max-height: 400px;
  overflow-y: auto;
}

.logs-placeholder {
  color: var(--text-tertiary);
  text-align: center;
  padding: 40px;
}

.update-log-line {
  margin-bottom: 4px;
}

.update-log-line.error {
  color: var(--color-danger-500);
}

.update-log-line.warning {
  color: var(--color-warning-500);
}

.update-log-line.success {
  color: var(--color-success-500);
}

.log-time {
  color: var(--text-tertiary);
  margin-right: 8px;
}

.log-message {
  word-break: break-all;
}

.action-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 38px;
  padding: 0 14px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: 8px;
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.action-btn:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
}
</style>
