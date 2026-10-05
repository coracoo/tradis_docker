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
      message: '连接成功，开始拉取镜像并创建容器（不会自动启动）...',
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
      payload = { type: 'info', message: String(event.data || '') }
    }

    if (payload.type === 'result') {
      streaming.value = false
      updateCompleted = true
      eventSource.close()
      logs.value.push({
        type: payload.status === 'success' ? 'success' : 'error',
        message: payload.status === 'success' ? '更新任务完成' : `更新任务失败: ${payload.error || ''}`,
        time: Date.now()
      })
      emit('completed')
      return
    }

    const str = String(payload.message || '')
    
    let type = 'info'
    let message = str
    
    if (str.startsWith('error:')) {
      type = 'error'
      message = str.substring(6).trim()
    } else if (str.startsWith('warn:')) {
      type = 'warning'
      message = str.substring(5).trim()
    } else if (str.startsWith('info:')) {
      type = 'info'
      message = str.substring(5).trim()
    } else if (str.startsWith('success:')) {
      type = 'success'
      message = str.substring(8).trim()
      if (message.includes('完成') || message.includes('成功')) {
        updateCompleted = true
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
    streaming.value = false
    eventSource.close()
    if (!updateCompleted) {
      logs.value.push({
        type: 'warning',
        message: '连接异常断开',
        time: Date.now()
      })
    }
    emit('completed')
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
