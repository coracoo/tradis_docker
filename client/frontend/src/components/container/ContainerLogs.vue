<template>
  <div class="logs-container ops-console">
    <div class="logs-toolbar ops-console__header">
      <div class="toolbar-left">
        <label class="toolbar-item">
          <input v-model="autoScroll" type="checkbox" />
          <span>自动滚动</span>
        </label>
        <span class="time-standard" title="日志时间统一转换为当前浏览器的本地时间">
          <DynamicIcon name="clock" :size="13" />
          本地时间
        </span>
        <div v-if="isComposeSource" class="log-service-filter">
          <Multiselect v-model="selectedSources" :options="serviceOptions" placeholder="全部容器" />
        </div>
        <!-- 搜索框 -->
        <div class="search-box">
          <DynamicIcon class="search-icon" name="search" :size="14" />
          <input
            v-model="searchQuery"
            type="search"
            class="search-input"
            name="tradis-log-query"
            autocomplete="one-time-code"
            autocapitalize="none"
            spellcheck="false"
            data-1p-ignore
            data-lpignore="true"
            data-form-type="other"
            placeholder="检索日志..."
            @keyup.enter="findNext"
          />
          <span v-if="searchQuery" class="search-count">
            {{ matchCount }} 条
          </span>
          <button v-if="searchQuery" class="search-btn" title="上一个" @click="findPrev">
            <DynamicIcon name="chevron-up" :size="12" />
          </button>
          <button v-if="searchQuery" class="search-btn" title="下一个" @click="findNext">
            <DynamicIcon name="chevron-down" :size="12" />
          </button>
          <button v-if="searchQuery" class="search-btn close" title="清除搜索" @click="searchQuery = ''">
            <DynamicIcon name="x" :size="12" />
          </button>
        </div>
      </div>
      <div class="toolbar-right">
        <button class="toolbar-btn" @click="clearLogs">
          <DynamicIcon name="trash" :size="14" />
          清空
        </button>
        <button class="toolbar-btn" @click="downloadLogs">
          <DynamicIcon name="download" :size="14" />
          下载
        </button>
        <button class="toolbar-btn" :class="{ active: isStreaming }" @click="toggleStream">
          <DynamicIcon :name="isStreaming ? 'pause' : 'play'" :size="14" />
          {{ isStreaming ? '暂停' : '继续' }}
        </button>
      </div>
    </div>
    
    <div ref="logsRef" class="logs-content ops-console__body">
      <div v-if="filteredLogs.length === 0 && logs.length > 0 && (searchQuery || hasServiceFilter)" class="logs-empty">
        {{ searchQuery ? '无匹配结果' : '已过滤全部容器' }}
      </div>
      <template v-else>
        <div
          v-for="line in filteredLogs"
          :key="line.id"
          class="log-line"
          :class="[`is-${line.tone}`, { 'is-match': isMatch(line) }]"
          :style="lineSourceStyle(line)"
          :data-index="line.id"
        >
          <span v-if="searchQuery" class="line-number">{{ getLineNumber(line) }}</span>
          <time class="log-time" :datetime="line.timestamp" :title="line.timestamp">{{ formatLogTime(line.timestamp) }}</time>
          <b v-if="line.source" class="log-source" :title="line.source" :style="lineSourceLabelStyle(line)">{{ line.source }}</b>
          <span class="log-text" v-html="highlightText(line.text)"></span>
        </div>
      </template>
      <div v-if="logs.length === 0 && !searchQuery" class="logs-empty">暂无日志</div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import Multiselect from '@/components/ui/Multiselect.vue'
import { buildApiUrl } from '@/utils/request.js'
import { formatLogTime, parseLogEntry, renderLogLineHtml, sourceColors, stripAnsi } from '@/utils/logPresentation.js'
import { useEnvironmentContext } from '@/composables/useEnvironmentContext.js'

const props = defineProps({
  containerId: { type: String, default: '' },
  source: { type: Object, default: null },
  tail: { type: Number, default: 200 }
})

const logsRef = ref(null)
const logs = ref([])
const autoScroll = ref(true)
const isStreaming = ref(false)
const searchQuery = ref('')
const currentMatchIndex = ref(-1)

let eventSource = null
let logSequence = 0
const { environmentId, isRemoteEnvironment } = useEnvironmentContext()

const resolvedSource = computed(() => {
  if (props.source?.id) {
    return {
      type: props.source.type === 'compose' ? 'compose' : 'container',
      id: String(props.source.id),
      label: String(props.source.label || props.source.id)
    }
  }
  return {
    type: 'container',
    id: String(props.containerId || ''),
    label: String(props.containerId || 'container')
  }
})

const sourceSignature = computed(() => `${resolvedSource.value.type}|${resolvedSource.value.id}|${environmentId.value}`)

const isComposeSource = computed(() => resolvedSource.value.type === 'compose')

// 服务级过滤：selectedSources 为空 = 显示全部；非空 = 仅显示选中的服务
const knownSources = ref([])
const selectedSources = ref([])
const serviceOptions = computed(() => knownSources.value)
const hasServiceFilter = computed(() => selectedSources.value.length > 0)

function rememberSource(source) {
  if (!source || knownSources.value.includes(source)) return
  knownSources.value = [...knownSources.value, source]
}

// Compose 多容器日志用服务色做底色区分来源；error/warning 行保留原有的警示底色
function lineSourceStyle(line) {
  const colors = sourceColors(line?.source)
  if (!colors || line?.tone === 'error' || line?.tone === 'warning') return null
  return { background: colors.tint }
}

function lineSourceLabelStyle(line) {
  const colors = sourceColors(line?.source)
  if (!colors) return null
  return { color: colors.label }
}

function appendLog(raw) {
  const entry = parseLogEntry(raw, new Date())
  rememberSource(entry.source)
  logSequence += 1
  logs.value.push({ id: logSequence, ...entry })

  if (logs.value.length > 2000) {
    logs.value = logs.value.slice(-1000)
  }

  if (autoScroll.value && !searchQuery.value) {
    nextTick(scrollToBottom)
  }
}

// 过滤后的日志
const filteredLogs = computed(() => {
  if (!searchQuery.value.trim() && !hasServiceFilter.value) return logs.value
  const query = searchQuery.value.toLowerCase()
  return logs.value.filter(line => {
    if (hasServiceFilter.value && !selectedSources.value.includes(line.source)) return false
    if (!query) return true
    return `${line.timestamp} ${line.source ? `${line.source} ` : ''}${stripAnsi(line.text)}`.toLowerCase().includes(query)
  })
})

// 匹配数量
const matchCount = computed(() => filteredLogs.value.length)

// 是否是匹配行
const isMatch = (line) => {
  if (!searchQuery.value.trim()) return false
  return `${line.timestamp} ${line.source ? `${line.source} ` : ''}${stripAnsi(line.text)}`.toLowerCase().includes(searchQuery.value.toLowerCase())
}

// 获取原始行号
const getLineNumber = (line) => {
  const originalIndex = logs.value.indexOf(line)
  return originalIndex >= 0 ? originalIndex + 1 : ''
}

// 高亮匹配文本
const highlightText = (text) => {
  return renderLogLineHtml(text, searchQuery.value)
}

// 查找下一个匹配
const findNext = () => {
  if (!searchQuery.value.trim() || filteredLogs.value.length === 0) return
  const matches = logsRef.value?.querySelectorAll('.is-match')
  if (!matches || matches.length === 0) return
  
  currentMatchIndex.value = (currentMatchIndex.value + 1) % matches.length
  matches[currentMatchIndex.value].scrollIntoView({ behavior: 'smooth', block: 'center' })
}

// 查找上一个匹配
const findPrev = () => {
  if (!searchQuery.value.trim() || filteredLogs.value.length === 0) return
  const matches = logsRef.value?.querySelectorAll('.is-match')
  if (!matches || matches.length === 0) return
  
  currentMatchIndex.value = currentMatchIndex.value <= 0 ? matches.length - 1 : currentMatchIndex.value - 1
  matches[currentMatchIndex.value].scrollIntoView({ behavior: 'smooth', block: 'center' })
}

// 连接日志流
const connect = () => {
  if (eventSource) return
  if (!resolvedSource.value.id || typeof EventSource === 'undefined') return
  
  const params = new URLSearchParams({ tail: String(props.tail) })
	if (isRemoteEnvironment.value) params.set('environmentId', environmentId.value)
  const id = encodeURIComponent(resolvedSource.value.id)
  const path = resolvedSource.value.type === 'compose'
    ? `/compose/${id}/logs?${params.toString()}`
    : `/containers/${id}/logs/events?${params.toString()}`
  const url = buildApiUrl(path)

  eventSource = new EventSource(url)
  isStreaming.value = true

  eventSource.onopen = () => {
    isStreaming.value = true
  }
  eventSource.onmessage = event => appendLog(event.data)
  eventSource.addEventListener('complete', () => {
    disconnect()
  })
  eventSource.addEventListener('stream-error', event => {
    appendLog(`ERROR: ${event.data || '日志连接失败'}`)
    disconnect()
  })
  eventSource.onerror = () => {
    isStreaming.value = false
  }
}

// 断开连接
const disconnect = () => {
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
  isStreaming.value = false
}

// 滚动到底部
const scrollToBottom = () => {
  if (logsRef.value) {
    logsRef.value.scrollTop = logsRef.value.scrollHeight
  }
}

// 清空日志
const clearLogs = () => {
  logs.value = []
  currentMatchIndex.value = -1
}

// 下载日志
const downloadLogs = () => {
  const source = searchQuery.value || hasServiceFilter.value ? filteredLogs.value : logs.value
  const content = source
    .map(line => `${line.timestamp} ${line.source ? `${line.source} | ` : ''}${line.text}`)
    .join('\n')
  const blob = new Blob([content], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `logs-${resolvedSource.value.label.slice(0, 24)}-${Date.now()}.txt`
  a.click()
  URL.revokeObjectURL(url)
}

// 切换流
const toggleStream = () => {
  if (isStreaming.value) {
    disconnect()
  } else {
    connect()
  }
}

onMounted(() => {
  connect()
})

onUnmounted(() => {
  disconnect()
})

watch(sourceSignature, () => {
  logs.value = []
  logSequence = 0
  searchQuery.value = ''
  currentMatchIndex.value = -1
  knownSources.value = []
  selectedSources.value = []
  disconnect()
  connect()
})
</script>

<style scoped>
.logs-container {
  display: flex;
  flex-direction: column;
  height: 500px;
  background: var(--bg-primary);
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  overflow: hidden;
  color: var(--text-primary);
  font-family: var(--font-sans, Inter, ui-sans-serif, system-ui, sans-serif);
}

.logs-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  min-height: 52px;
  padding: 9px 12px;
  background: color-mix(in srgb, var(--bg-secondary) 76%, var(--bg-elevated));
  border-bottom: 1px solid var(--border-subtle);
  gap: 12px;
}

.toolbar-left {
  display: flex;
  align-items: center;
  gap: 12px;
  flex: 1;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.toolbar-item {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 0.8125rem;
  color: var(--text-secondary);
  cursor: pointer;
  white-space: nowrap;
}

.time-standard {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: var(--text-tertiary);
  font-size: 0.75rem;
  white-space: nowrap;
}

.log-service-filter {
  flex: 0 0 auto;
  width: 200px;
  min-width: 0;
}

.log-service-filter :deep(.multiselect-input) {
  min-height: 34px;
  padding: 4px 8px;
  background: var(--bg-primary);
}

.log-service-filter :deep(.tag) {
  padding: 2px 7px;
  font-size: 0.75rem;
}

/* 搜索框 */
.search-box {
  display: flex;
  align-items: center;
  gap: 8px;
  background: var(--bg-primary);
  border: 1px solid var(--border-default);
  border-radius: 8px;
  min-height: 34px;
  padding: 0 9px;
  flex: 1;
  max-width: 400px;
  min-width: 200px;
}

.search-icon {
  color: var(--text-tertiary);
  flex-shrink: 0;
}

.search-input {
  flex: 1;
  background: transparent;
  border: none;
  padding: 8px 0;
  font-size: 0.8125rem;
  color: var(--text-primary);
  outline: none;
}

.search-input::placeholder {
  color: var(--text-tertiary);
}

.search-count {
  font-size: 0.6875rem;
  color: var(--text-tertiary);
  white-space: nowrap;
}

.search-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  background: var(--bg-tertiary);
  border: none;
  border-radius: 4px;
  color: var(--text-secondary);
  cursor: pointer;
  flex-shrink: 0;
}

.search-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.search-btn.close:hover {
  background: var(--color-danger-500-20);
  color: var(--color-danger-500);
}

.toolbar-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 32px;
  padding: 0 10px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 7px;
  font-size: 0.8125rem;
  color: var(--text-primary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  white-space: nowrap;
}

.toolbar-btn:hover {
  border-color: var(--color-primary-300);
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

.toolbar-btn.active {
  background: var(--color-primary-500);
  border-color: var(--color-primary-600);
  color: var(--text-inverse);
}

.logs-content {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  background: var(--bg-primary);
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 13px;
  line-height: 1.5;
}

.logs-content::-webkit-scrollbar {
  width: 8px;
}

.logs-content::-webkit-scrollbar-track {
  background: transparent;
}

.logs-content::-webkit-scrollbar-thumb {
  background: var(--border-default);
  border-radius: 4px;
}

.log-line {
  min-height: 23px;
  padding: 2px 8px 2px 10px;
  color: var(--text-primary);
  display: flex;
  gap: 12px;
  border-left: 2px solid transparent;
  border-radius: 3px;
}

.log-line:hover {
  background: color-mix(in srgb, var(--bg-hover) 72%, transparent);
}

.log-line.is-match {
  background: var(--color-primary-500-15);
  border-radius: 2px;
  margin: 0 -8px;
  padding: 1px 8px;
}

.line-number {
  color: var(--text-tertiary);
  min-width: 40px;
  text-align: right;
  flex-shrink: 0;
  user-select: none;
}

.log-text {
  flex: 1;
  white-space: pre-wrap;
  word-break: break-all;
  font-variant-ligatures: none;
}

.log-time {
  flex: 0 0 76px;
  padding-right: 12px;
  border-right: 1px solid var(--border-subtle);
  color: var(--text-tertiary);
  font-size: 0.75rem;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.log-source {
  flex: 0 0 auto;
  max-width: 150px;
  padding-right: 12px;
  border-right: 1px solid var(--border-subtle);
  color: var(--color-success-700);
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 0.75rem;
  font-weight: 700;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.log-line.is-error {
  border-left-color: var(--color-danger-500);
  background: color-mix(in srgb, var(--color-danger-500) 6%, transparent);
}

.log-line.is-warning {
  border-left-color: var(--color-warning-500);
  background: color-mix(in srgb, var(--color-warning-500) 6%, transparent);
}

.log-line.is-success { border-left-color: var(--color-success-500); }
.log-line.is-info { border-left-color: var(--color-info-500); }
.log-line.is-debug { border-left-color: var(--text-tertiary); }

.log-line.is-error .log-text {
  color: color-mix(in srgb, var(--text-primary) 72%, var(--color-danger-500));
}

.log-line.is-warning .log-text {
  color: color-mix(in srgb, var(--text-primary) 76%, var(--color-warning-500));
}

.log-line.is-success .log-text {
  color: color-mix(in srgb, var(--text-primary) 78%, var(--color-success-500));
}

.log-line.is-info .log-text {
  color: color-mix(in srgb, var(--text-primary) 84%, var(--color-info-500));
}

:deep(.highlight) {
  background: var(--color-warning-100);
  color: var(--text-primary);
  border-radius: 2px;
  padding: 0 2px;
}

:deep(.log-token-source) {
  color: var(--color-primary-600);
  font-weight: 700;
}

:deep(.log-token-time) {
  color: color-mix(in srgb, var(--text-secondary) 64%, var(--color-info-500));
  font-variant-numeric: tabular-nums;
}

.logs-empty {
  text-align: center;
  color: var(--text-tertiary);
  padding: 48px 0;
}

@media (max-width: 760px) {
  .logs-toolbar,
  .toolbar-left {
    align-items: stretch;
    flex-direction: column;
  }

  .toolbar-right {
    justify-content: flex-end;
  }

  .search-box {
    width: 100%;
    max-width: none;
    min-width: 0;
  }

  .log-line {
    flex-wrap: wrap;
  }

  .log-time {
    flex-basis: 100%;
    padding-right: 0;
    border-right: 0;
  }
}
</style>
