<template>
  <section class="live-log-preview" :aria-label="title">
    <header class="live-log-header">
      <p class="section-label">{{ title }}</p>
      <div class="live-log-header-actions">
        <span v-if="source?.running" class="live-log-status" :class="`is-${connectionState}`">
          <i></i>
          {{ connectionLabel }}
        </span>
        <button
          v-if="expandable && source?.id"
          type="button"
          class="live-log-expand"
          title="打开完整日志"
          @click="$emit('open')"
        >
          <DynamicIcon name="maximize" :size="13" />
        </button>
      </div>
    </header>

    <div v-if="emptyText" class="live-log-empty">{{ emptyText }}</div>
    <ol v-else class="live-log-list" aria-live="polite">
      <li
        v-for="line in logLines"
        :key="line.id"
        :class="`is-${line.tone}`"
        :style="lineSourceStyle(line)"
        :title="lineTooltip(line)"
      >
        <b :title="line.service" :style="lineSourceLabelStyle(line)">{{ line.service }}</b>
        <time :datetime="line.timestamp" :title="line.timestamp">{{ formatLogTime(line.timestamp) }}</time>
        <code :title="line.text" v-html="renderLogLineHtml(line.text)"></code>
      </li>
    </ol>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { buildApiUrl } from '@/utils/request.js'
import { formatLogTime, parseLogEntry, renderLogLineHtml, sourceColors, stripAnsi } from '@/utils/logPresentation.js'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import { useEnvironmentContext } from '@/composables/useEnvironmentContext.js'

const props = defineProps({
  source: {
    type: Object,
    default: null
  },
  title: {
    type: String,
    default: '实时日志'
  },
  tail: {
    type: Number,
    default: 20
  },
  maxLines: {
    type: Number,
    default: 80
  },
  expandable: {
    type: Boolean,
    default: false
  }
})

defineEmits(['open'])

const logLines = ref([])
const connectionState = ref('idle')
const streamMessage = ref('')
let eventSource = null
let sequence = 0
const { environmentId, isRemoteEnvironment } = useEnvironmentContext()

const streamSignature = computed(() => [
  props.source?.type || '',
  props.source?.id || '',
  props.source?.running ? '1' : '0',
  props.tail,
	environmentId.value
].join('|'))

const connectionLabel = computed(() => {
  if (connectionState.value === 'live') return '实时'
  if (connectionState.value === 'ended') return '已结束'
  if (connectionState.value === 'failed') return '连接失败'
  if (connectionState.value === 'error') return '重连中'
  return '连接中'
})

const emptyText = computed(() => {
  if (!props.source?.running) return '无'
  if (logLines.value.length > 0) return ''
  if (connectionState.value === 'ended') return streamMessage.value || '日志流已结束'
  if (connectionState.value === 'failed') return streamMessage.value || '日志连接失败'
  if (connectionState.value === 'error') return '日志连接中断，正在重试'
  if (connectionState.value === 'connecting') return '正在连接日志流'
  return '暂无日志'
})

function disconnect() {
  eventSource?.close()
  eventSource = null
}

function parseLogLine(raw) {
  const entry = parseLogEntry(raw, new Date())
  return {
    service: entry.source || String(props.source?.label || props.source?.id || 'service'),
    text: entry.text,
    timestamp: entry.timestamp,
    tone: entry.tone
  }
}

function prependLog(raw) {
  const parsed = parseLogLine(raw)
  if (!parsed.text) return
  sequence += 1
  logLines.value = [{ id: sequence, ...parsed }, ...logLines.value].slice(0, props.maxLines)
  connectionState.value = 'live'
}

function lineTooltip(line) {
  return [line.service, line.timestamp, stripAnsi(line.text)].filter(Boolean).join('  ')
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

function connect() {
  disconnect()
  logLines.value = []
  streamMessage.value = ''
  sequence = 0

  const source = props.source
  if (!source?.running || !source.id || typeof EventSource === 'undefined') {
    connectionState.value = 'idle'
    return
  }

  const query = new URLSearchParams({ tail: String(props.tail) })
	if (isRemoteEnvironment.value) query.set('environmentId', environmentId.value)
  const id = encodeURIComponent(String(source.id))
  const path = source.type === 'container'
    ? `/containers/${id}/logs/events?${query.toString()}`
    : `/compose/${id}/logs?${query.toString()}`

  connectionState.value = 'connecting'
  eventSource = new EventSource(buildApiUrl(path))
  eventSource.onopen = () => {
    connectionState.value = 'live'
  }
  eventSource.onmessage = event => prependLog(event.data)
  eventSource.addEventListener('complete', event => {
    streamMessage.value = String(event.data || '日志流已结束')
    connectionState.value = 'ended'
    disconnect()
  })
  eventSource.addEventListener('stream-error', event => {
    streamMessage.value = String(event.data || '日志连接失败')
    connectionState.value = 'failed'
    disconnect()
  })
  eventSource.onerror = () => {
    connectionState.value = 'error'
  }
}

watch(streamSignature, connect, { immediate: true })
onBeforeUnmount(disconnect)
</script>

<style scoped>
.live-log-preview {
  display: flex;
  flex-direction: column;
  min-height: 156px;
  overflow: hidden;
}

.live-log-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  flex: 0 0 auto;
  position: sticky;
  top: 0;
  z-index: 1;
}

.live-log-header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.live-log-expand {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  color: var(--text-secondary);
  background: var(--bg-primary);
  cursor: pointer;
}

.live-log-expand:hover {
  border-color: var(--color-primary-300);
  color: var(--color-primary-600);
  background: var(--color-primary-50);
}

.section-label {
  margin: 0;
  color: var(--text-secondary);
  font-size: 0.6875rem;
  font-weight: 800;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.live-log-status {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: var(--text-tertiary);
  font-size: 0.625rem;
  font-weight: 700;
}

.live-log-status i {
  width: 6px;
  height: 6px;
  border-radius: 999px;
  background: currentColor;
  box-shadow: 0 0 0 3px color-mix(in srgb, currentColor 16%, transparent);
}

.live-log-status.is-live {
  color: var(--color-success-700);
}

.live-log-status.is-error {
  color: var(--color-warning-700);
}

.live-log-status.is-failed {
  color: var(--color-danger-600);
}

.live-log-status.is-ended {
  color: var(--text-tertiary);
}

.live-log-empty {
  display: grid;
  place-items: center;
  flex: 1 1 auto;
  min-height: 108px;
  color: var(--text-tertiary);
  font-size: 0.75rem;
}

.live-log-list {
  display: grid;
  align-content: start;
  gap: 7px;
  flex: 1 1 auto;
  min-height: 0;
  margin: 10px 0 0;
  padding: 0 2px 0 0;
  overflow-y: auto;
  list-style: none;
}

.live-log-list li {
  display: grid;
  grid-template-columns: minmax(58px, 76px) 62px minmax(0, 1fr);
  align-items: start;
  gap: 8px;
  min-width: 0;
  padding: 2px 5px;
  border-left: 2px solid transparent;
  border-radius: 3px;
}

.live-log-list b,
.live-log-list time {
  overflow: hidden;
  font-family: var(--font-mono, 'JetBrains Mono', ui-monospace, monospace);
  font-size: 0.6875rem;
  line-height: 1.45;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.live-log-list code {
  overflow: visible;
  min-width: 0;
  color: var(--text-secondary);
  font-family: var(--font-mono, 'JetBrains Mono', ui-monospace, monospace);
  font-size: 0.6875rem;
  line-height: 1.45;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}

.live-log-list time {
  color: var(--text-tertiary);
  font-variant-numeric: tabular-nums;
}

.live-log-list b {
  color: var(--color-success-700);
  font-weight: 700;
}

.live-log-list li.is-error {
  border-left-color: var(--color-danger-500);
  background: color-mix(in srgb, var(--color-danger-500) 6%, transparent);
}

.live-log-list li.is-warning {
  border-left-color: var(--color-warning-500);
  background: color-mix(in srgb, var(--color-warning-500) 6%, transparent);
}

.live-log-list li.is-success { border-left-color: var(--color-success-500); }
.live-log-list li.is-info { border-left-color: var(--color-info-500); }
.live-log-list li.is-debug { border-left-color: var(--text-tertiary); }

.live-log-list li.is-error code {
  color: color-mix(in srgb, var(--text-primary) 72%, var(--color-danger-500));
}

.live-log-list li.is-warning code {
  color: color-mix(in srgb, var(--text-primary) 76%, var(--color-warning-500));
}

.live-log-list :deep(.log-token-source) {
  color: var(--color-primary-600);
  font-weight: 700;
}

.live-log-list :deep(.log-token-time) {
  color: color-mix(in srgb, var(--text-secondary) 64%, var(--color-info-500));
  font-variant-numeric: tabular-nums;
}
</style>
