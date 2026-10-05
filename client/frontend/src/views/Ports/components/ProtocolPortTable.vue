<template>
  <section class="protocol-port-table" :class="protocol" :aria-label="`${protocolLabel} 端口列表`">
    <header class="protocol-panel-header" :class="protocol">
      <div class="protocol-heading">
        <span class="protocol-mark">{{ protocolLabel }}</span>
        <div>
          <strong>{{ protocolLabel }} 端口</strong>
          <small>{{ protocolDescription }}</small>
        </div>
      </div>
      <span class="protocol-count">
        <strong>{{ ports.length }}</strong>
        <span>/ {{ total }}</span>
      </span>
    </header>

    <ResourceTable
      :columns="columns"
      :rows="ports"
      :table-key="`ports-${protocol}-v3`"
      :row-key="portRowKey"
      :loading="loading && ports.length === 0"
      :interactive="false"
      :row-class="getRowClass"
      :aria-label="`${protocolLabel} 端口表格`"
      @scroll="handleScroll"
    >
      <template #cell-port="{ row }">
        <div class="port-identity">
          <strong>{{ formatPortRange(row) }}</strong>
        </div>
      </template>

      <template #cell-source="{ row }">
        <span class="source-badge" :class="`source-${String(row.type || 'unknown').toLowerCase()}`">
          {{ row.type || 'Unknown' }}
        </span>
      </template>

      <template #cell-status="{ row }">
        <StatusBadge :status="row.used ? 'used' : 'unused'" variant="minimal" />
      </template>

      <template #cell-service="{ row }">
        <span class="port-service" :title="row.service || '-'">{{ row.service || '-' }}</span>
      </template>

      <template #cell-note="{ row }">
        <input
          :value="row.note || ''"
          type="text"
          class="port-note-input"
          placeholder="添加备注..."
          :aria-label="`${protocolLabel} ${formatPortRange(row)} 端口备注`"
          @click.stop
          @input="$emit('update-note', row, protocol, $event.target.value)"
          @blur="$emit('save-note', row, protocol)"
          @keydown.enter="$event.currentTarget.blur()"
        />
      </template>

      <template #cell-action="{ row }">
        <div class="port-action-buttons">
          <button
            v-ripple
            type="button"
            class="port-action-btn copy-port-btn"
            title="复制访问地址"
            :aria-label="`复制 ${formatPortRange(row)} 的访问地址`"
            @click.stop="$emit('copy-port', row, protocol)"
          >
            <DynamicIcon name="copy" :size="14" />
          </button>
          <button
            v-if="isReservedPort(row)"
            v-ripple
            type="button"
            class="port-action-btn release-port-btn"
            :disabled="isReleasing(row)"
            title="释放预留端口"
            @click.stop="$emit('release', row, protocol)"
          >
            <DynamicIcon :name="isReleasing(row) ? 'loader-2' : 'trash-2'" :size="14" />
          </button>
        </div>
      </template>

      <template #empty>
        <EmptyState icon="box" :title="emptyTitle" :description="emptyDescription" />
      </template>
    </ResourceTable>

    <div v-if="loading && ports.length > 0" class="protocol-loading-more" role="status">
      <DynamicIcon name="loader-2" :size="15" />
      正在载入
    </div>
  </section>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import ResourceTable from '@/components/resource-workbench/ResourceTable.vue'

const props = defineProps({
  protocol: {
    type: String,
    required: true,
    validator: value => ['tcp', 'udp'].includes(value)
  },
  ports: {
    type: Array,
    default: () => []
  },
  total: {
    type: Number,
    default: 0
  },
  loading: {
    type: Boolean,
    default: false
  },
  releasingKeys: {
    type: Object,
    default: () => new Set()
  },
  emptyTitle: {
    type: String,
    default: '暂无端口数据'
  },
  emptyDescription: {
    type: String,
    default: '当前筛选范围内没有端口记录'
  }
})

const emit = defineEmits(['update-note', 'save-note', 'copy-port', 'release', 'load-more'])
const requestingMore = ref(false)

const protocolLabel = computed(() => props.protocol.toUpperCase())
const protocolDescription = computed(() => (
  props.protocol === 'tcp' ? '传输控制协议' : '用户数据报协议'
))

const columns = [
  { key: 'port', label: '端口', width: 76, minWidth: 66 },
  { key: 'source', label: '来源', width: 68, minWidth: 60 },
  { key: 'status', label: '状态', width: 72, minWidth: 68 },
  { key: 'service', label: '服务 / 进程', width: 90, minWidth: 76, flex: true },
  { key: 'note', label: '备注', width: 72, minWidth: 64 },
  { key: 'action', label: '操作', width: 64, minWidth: 60 }
]

watch(
  () => [props.loading, props.ports.length],
  ([loading]) => {
    if (!loading) requestingMore.value = false
  }
)

function portRowKey(port, index) {
  return `${props.protocol}-${port.port}-${port.end_port || port.port}-${port.type || ''}-${index}`
}

function getRowClass(port) {
  return `port-data-row protocol-${props.protocol}${port.used ? ' is-used' : ''}`
}

function formatPortRange(port) {
  return port.end_port > port.port ? `${port.port}-${port.end_port}` : `${port.port}`
}

function isReservedPort(port) {
  return String(port?.type || '').toLowerCase() === 'reserved'
}

function releaseKey(port) {
  return `${props.protocol}-${port.port}-${port.end_port || port.port}`
}

function isReleasing(port) {
  return props.releasingKeys.has(releaseKey(port))
}

function handleScroll(event) {
  const target = event.currentTarget
  if (
    props.loading
    || requestingMore.value
    || props.ports.length >= props.total
    || target.scrollHeight - target.scrollTop - target.clientHeight >= 120
  ) return

  requestingMore.value = true
  emit('load-more', props.protocol)
}
</script>

<style scoped>
.protocol-port-table {
  position: relative;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.protocol-panel-header {
  box-sizing: border-box;
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex: 0 0 auto;
  min-height: 54px;
  padding: 9px 12px;
  border-bottom: 1px solid var(--resource-line, var(--border-subtle));
  background: var(--resource-surface, var(--bg-elevated));
}

.protocol-panel-header.tcp {
  box-shadow: inset 4px 0 0 var(--color-info-500);
}

.protocol-panel-header.udp {
  box-shadow: inset 4px 0 0 var(--color-warning-500);
}

.protocol-heading {
  display: flex;
  align-items: center;
  gap: 9px;
  min-width: 0;
}

.protocol-heading > div {
  display: grid;
  gap: 2px;
  min-width: 0;
}

.protocol-heading strong {
  color: var(--resource-ink, var(--text-primary, #0f172a));
  font-size: 0.8125rem;
  font-weight: 800;
  line-height: 1.1;
}

.protocol-heading small,
.protocol-count {
  color: var(--text-secondary, #64748b);
  font-size: 0.6875rem;
}

.protocol-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 46px;
  height: 26px;
  border: 1px solid transparent;
  border-radius: 7px;
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 0.6875rem;
  font-weight: 700;
}

.tcp .protocol-mark {
  border-color: var(--color-info-300);
  background: var(--color-info-100);
  color: var(--color-info-700);
}

.udp .protocol-mark {
  border-color: var(--color-warning-300);
  background: var(--color-warning-100);
  color: var(--color-warning-700);
}

.protocol-count {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
  flex: 0 0 auto;
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
}

.protocol-count strong {
  color: var(--resource-ink, var(--text-primary, #0f172a));
  font-size: 0.875rem;
}

.protocol-port-table :deep(.resource-table-frame) {
  flex: 1 1 auto;
  height: auto;
  min-height: 0;
}

.protocol-port-table :deep(.resource-table__row),
.protocol-port-table :deep(.resource-table__cell) {
  min-height: 58px;
}

.port-identity {
  display: grid;
  gap: 2px;
  min-width: 0;
}

.port-identity strong {
  overflow: hidden;
  color: var(--resource-ink, var(--text-primary, #0f172a));
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 0.8125rem;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.source-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  max-width: 100%;
  min-height: 24px;
  padding: 0 7px;
  overflow: hidden;
  border: 1px solid var(--border-subtle);
  border-radius: 7px;
  background: var(--bg-tertiary);
  color: var(--text-secondary, #64748b);
  font-size: 0.6875rem;
  font-weight: 600;
  text-overflow: ellipsis;
  text-transform: capitalize;
  white-space: nowrap;
}

.source-badge.source-host {
  border-color: var(--color-primary-300);
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

.source-badge.source-container {
  border-color: var(--color-success-300);
  background: var(--color-success-100);
  color: var(--color-success-700);
}

.source-badge.source-reserved {
  border-color: var(--color-warning-300);
  background: var(--color-warning-100);
  color: var(--color-warning-700);
}

.port-service {
  display: block;
  min-width: 0;
  overflow: hidden;
  color: var(--text-secondary, #64748b);
  font-size: 0.8125rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:deep(.port-data-row.is-used) .port-service {
  color: var(--resource-ink, var(--text-primary, #0f172a));
}

.port-note-input {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  height: 30px;
  padding: 0 7px;
  border: 1px solid transparent;
  border-radius: 7px;
  outline: none;
  background: transparent;
  color: var(--resource-ink, var(--text-primary, #0f172a));
  font: inherit;
  font-size: 0.75rem;
}

.port-note-input:hover {
  border-color: var(--resource-line-strong, var(--border-default));
  background: var(--bg-elevated);
}

.port-note-input:focus {
  border-color: var(--color-primary-500);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.port-note-input::placeholder {
  color: var(--text-tertiary, #94a3b8);
}

.port-action-buttons {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
  width: 100%;
}

.port-action-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 28px;
  width: 28px;
  height: 28px;
  padding: 0;
  border-radius: 8px;
  cursor: pointer;
}

.copy-port-btn {
  border: 1px solid var(--color-primary-200);
  background: var(--color-primary-100);
  color: var(--color-primary-600);
}

.copy-port-btn:hover {
  border-color: var(--color-primary-300);
  background: var(--color-primary-200);
  color: var(--color-primary-700);
}

.release-port-btn {
  border: 1px solid var(--color-danger-200);
  background: var(--color-danger-100);
  color: var(--color-danger-600);
}

.release-port-btn:hover:not(:disabled) {
  border-color: var(--color-danger-300);
  background: var(--color-danger-200);
  color: var(--color-danger-700);
}

.release-port-btn:disabled {
  cursor: wait;
  opacity: 0.7;
}

.protocol-loading-more {
  position: absolute;
  z-index: 4;
  right: 12px;
  bottom: 12px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 30px;
  padding: 0 10px;
  border: 1px solid var(--resource-line, var(--border-subtle));
  border-radius: 8px;
  background: var(--resource-panel, var(--bg-primary));
  box-shadow: var(--shadow-sm);
  color: var(--text-secondary, #64748b);
  font-size: 0.75rem;
}

.protocol-loading-more :deep(svg),
.release-port-btn:disabled :deep(svg) {
  animation: resource-refresh-spin 0.75s linear infinite;
}
</style>
