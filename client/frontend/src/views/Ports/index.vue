<template>
  <ResourceWorkbench class="ports-page ports-workbench" role="main" aria-label="端口管理">
    <template #rail>
      <section class="resource-rail" aria-label="端口概览">
        <article v-for="item in portRail" :key="item.label" class="rail-card" :class="item.tone">
          <span class="rail-icon">
            <DynamicIcon :name="item.icon" :size="18" />
          </span>
          <div class="rail-copy">
            <strong>{{ item.value }}</strong>
            <span>{{ item.label }}</span>
          </div>
          <small>{{ item.note }}</small>
        </article>
      </section>
    </template>

    <template #toolbar>
      <section class="toolbar workbench-toolbar">
        <div class="toolbar-left workbench-toolbar-left">
          <SearchInput
            v-model="searchQuery"
            :loading="loading"
            placeholder="搜索端口号或范围..."
            @search="handleSearch"
            @clear="handleSearch"
          />

          <div class="port-filter-tabs" aria-label="端口筛选">
            <SegmentedTabs
              v-model="statusFilterValue"
              :options="statusFilterOptions"
              aria-label="端口状态"
              compact
              @change="handleFilterChange"
            />
            <SegmentedTabs
              v-model="typeFilterValue"
              :options="typeFilterOptions"
              aria-label="端口来源"
              compact
              @change="handleFilterChange"
            />
          </div>
        </div>

        <div class="toolbar-right">
          <button
            v-ripple
            type="button"
            class="icon-btn resource-refresh-btn"
            :class="{ 'is-spinning': loading }"
            :disabled="loading"
            title="刷新端口数据"
            @click="refreshData"
          >
            <DynamicIcon class="resource-refresh-icon" name="refresh" :size="18" />
          </button>
          <button v-ripple type="button" class="primary-btn" @click="openRangeDialog">
            <DynamicIcon name="settings" :size="16" />
            范围设置
          </button>
        </div>
      </section>
    </template>

    <div class="ports-split-board">
      <ResourcePanel>
        <ProtocolPortTable
          protocol="tcp"
          :ports="tcpPorts"
          :total="tcpTotal"
          :loading="loading || tcpLoading"
          :releasing-keys="releasingPorts"
          :empty-title="emptyTitle"
          :empty-description="emptyDescription"
          @update-note="updateNote"
          @save-note="saveNote"
          @copy-port="copyPort"
          @release="releaseReservedPort"
          @load-more="loadMoreProtocol"
        />
      </ResourcePanel>
      <ResourcePanel>
        <ProtocolPortTable
          protocol="udp"
          :ports="udpPorts"
          :total="udpTotal"
          :loading="loading || udpLoading"
          :releasing-keys="releasingPorts"
          :empty-title="emptyTitle"
          :empty-description="emptyDescription"
          @update-note="updateNote"
          @save-note="saveNote"
          @copy-port="copyPort"
          @release="releaseReservedPort"
          @load-more="loadMoreProtocol"
        />
      </ResourcePanel>
    </div>

    <Modal v-model:visible="rangeDialogVisible" title="端口扫描范围" width="400px">
      <div class="range-form">
        <div class="form-row">
          <div class="form-group">
            <label class="form-label">起始端口</label>
            <input v-model.number="rangeForm.start" type="number" class="form-input" min="0" max="65535" />
          </div>
          <span class="range-separator">-</span>
          <div class="form-group">
            <label class="form-label">结束端口</label>
            <input v-model.number="rangeForm.end" type="number" class="form-input" min="0" max="65535" />
          </div>
        </div>
      </div>
      <template #footer>
        <button v-ripple type="button" class="btn btn-default" @click="resetRange">重置默认</button>
        <button
          v-ripple
          type="button"
          class="btn btn-primary"
          :class="{ 'is-loading': savingRange }"
          :disabled="savingRange"
          @click="saveRange"
        >
          <span v-if="savingRange" class="btn-spinner"></span>
          保存
        </button>
      </template>
    </Modal>
  </ResourceWorkbench>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import SegmentedTabs from '@/components/ui/SegmentedTabs.vue'
import Modal from '@/components/feedback/Modal.vue'
import ResourcePanel from '@/components/resource-workbench/ResourcePanel.vue'
import ResourceWorkbench from '@/components/resource-workbench/ResourceWorkbench.vue'
import ProtocolPortTable from './components/ProtocolPortTable.vue'
import { usePageAction } from '@/composables/usePageAction.js'
import { getPortRange, listPorts, releasePort, savePortNote, updatePortRange } from '@/api/ports.js'
import { useToast } from '@/composables/useToast.js'
import { useUiStore } from '@/stores/ui.js'
import { copyToClipboard } from '@/utils/helpers.js'

const toast = useToast()
const uiStore = useUiStore()

const loading = ref(true)
const tcpLoading = ref(false)
const udpLoading = ref(false)
const searchQuery = ref('')
const statusFilterValue = ref('used')
const typeFilterValue = ref('all')
const statusFilterOptions = [
  { value: 'all', label: '全部' },
  { value: 'used', label: '已使用' },
  { value: 'available', label: '空闲' }
]
const typeFilterOptions = [
  { value: 'all', label: '全部' },
  { value: 'host', label: 'Host' },
  { value: 'container', label: 'Container' }
]

const stats = reactive({ total: 0, used: 0, available: 0 })
const portRange = reactive({ start: 0, end: 65535 })
const tcpPorts = ref([])
const udpPorts = ref([])
const tcpPage = ref(1)
const udpPage = ref(1)
const tcpTotal = ref(0)
const udpTotal = ref(0)
const pageSize = 50

const rangeDialogVisible = ref(false)
const savingRange = ref(false)
const rangeForm = reactive({ start: 0, end: 65535 })
const releasingPorts = ref(new Set())

const emptyTitle = computed(() => {
  if (searchQuery.value) return '无匹配结果'
  return '暂无端口数据'
})

const emptyDescription = computed(() => {
  if (searchQuery.value) return `没有找到 "${searchQuery.value}"`
  return '当前范围内没有数据'
})

const portRail = computed(() => [
  {
    label: '端口记录',
    value: stats.total,
    note: 'TCP + UDP',
    icon: 'server',
    tone: 'neutral'
  },
  {
    label: '已使用',
    value: stats.used,
    note: '当前占用',
    icon: 'activity',
    tone: 'info'
  },
  {
    label: '可用',
    value: stats.available,
    note: '当前空闲',
    icon: 'circle-check',
    tone: 'success'
  },
  {
    label: '扫描范围',
    value: `${portRange.start}-${portRange.end}`,
    note: '当前范围',
    icon: 'settings',
    tone: 'warning'
  }
])

let searchTimeout = null
function handleSearch() {
  clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    resetPages()
    loadPorts()
  }, 300)
}

function resetPages() {
  tcpPage.value = 1
  udpPage.value = 1
}

function handleFilterChange() {
  resetPages()
  loadPorts()
}

async function refreshData() {
  if (loading.value) return
  loading.value = true
  resetPages()
  try {
    await Promise.all([loadPorts(), loadRange()])
    toast.success('数据已刷新')
  } finally {
    loading.value = false
  }
}

async function loadPorts() {
  try {
    const [tcpRes, udpRes] = await Promise.all([
      loadProtocolPorts('tcp', tcpPage.value),
      loadProtocolPorts('udp', udpPage.value)
    ])

    stats.used = (tcpRes?.used || 0) + (udpRes?.used || 0)
    stats.available = (tcpRes?.available || 0) + (udpRes?.available || 0)
    stats.total = stats.used + stats.available
  } catch (err) {
    console.error('加载端口失败:', err)
    toast.error('加载端口失败')
  }
}

async function loadProtocolPorts(protocol, page) {
  if (protocol === 'tcp') tcpLoading.value = true
  if (protocol === 'udp') udpLoading.value = true

  try {
    const res = await listPorts({
      protocol,
      used: getStatusParam(),
      type: getTypeParam(),
      search: searchQuery.value,
      page,
      pageSize,
      start: portRange.start,
      end: portRange.end
    })
    const items = res.items || []
    if (protocol === 'tcp') {
      tcpTotal.value = res.total || 0
      tcpPorts.value = page === 1 ? items : [...tcpPorts.value, ...items]
    } else {
      udpTotal.value = res.total || 0
      udpPorts.value = page === 1 ? items : [...udpPorts.value, ...items]
    }
    return res
  } finally {
    if (protocol === 'tcp') tcpLoading.value = false
    if (protocol === 'udp') udpLoading.value = false
  }
}

async function loadMoreProtocol(protocol) {
  const pageRef = protocol === 'tcp' ? tcpPage : udpPage
  const loadingRef = protocol === 'tcp' ? tcpLoading : udpLoading
  const portsRef = protocol === 'tcp' ? tcpPorts : udpPorts
  const totalRef = protocol === 'tcp' ? tcpTotal : udpTotal
  if (loadingRef.value || portsRef.value.length >= totalRef.value) return

  pageRef.value += 1
  try {
    await loadProtocolPorts(protocol, pageRef.value)
  } catch (err) {
    pageRef.value -= 1
    console.error(`加载 ${protocol.toUpperCase()} 端口失败:`, err)
    toast.error(`加载 ${protocol.toUpperCase()} 端口失败`)
  }
}

function getStatusParam() {
  if (statusFilterValue.value === 'all') return 'all'
  return statusFilterValue.value === 'used' ? 'true' : 'false'
}

function getTypeParam() {
  if (typeFilterValue.value === 'all') return 'all'
  return typeFilterValue.value
}

async function loadRange() {
  try {
    const res = await getPortRange()
    portRange.start = res.start ?? 0
    portRange.end = res.end ?? 65535
    rangeForm.start = portRange.start
    rangeForm.end = portRange.end
  } catch (err) {
    console.error('加载范围失败:', err)
  }
}

function openRangeDialog() {
  rangeForm.start = portRange.start
  rangeForm.end = portRange.end
  rangeDialogVisible.value = true
}

function resetRange() {
  rangeForm.start = 0
  rangeForm.end = 65535
}

async function saveRange() {
  if (rangeForm.end < rangeForm.start) {
    toast.error('结束端口必须大于起始端口')
    return
  }

  savingRange.value = true
  try {
    await updatePortRange({ start: rangeForm.start, end: rangeForm.end })
    portRange.start = rangeForm.start
    portRange.end = rangeForm.end
    rangeDialogVisible.value = false
    toast.success('范围已更新')
    await refreshData()
  } catch (err) {
    toast.error('保存失败')
  } finally {
    savingRange.value = false
  }
}

function isReservedPort(port) {
  return String(port?.type || '').toLowerCase() === 'reserved'
}

function portReleaseKey(port, protocol) {
  return `${protocol}-${port.port}-${port.end_port || port.port}`
}

async function releaseReservedPort(port, protocol) {
  if (!isReservedPort(port)) return
  const start = Number(port.port)
  const end = Number(port.end_port || port.port)
  const label = end > start ? `${start}-${end}` : `${start}`
  const confirmed = await uiStore.confirm({
    type: 'warning',
    title: '释放预留端口',
    message: `确定释放 ${protocol.toUpperCase()} 预留端口 ${label}？`,
    confirmText: '释放'
  })
  if (!confirmed) return

  const key = portReleaseKey(port, protocol)
  releasingPorts.value = new Set([...releasingPorts.value, key])
  try {
    const ports = Array.from({ length: end - start + 1 }, (_, index) => start + index)
    await releasePort({ ports })
    toast.success(`预留端口 ${label} 已释放`)
    resetPages()
    await loadPorts()
  } catch (err) {
    toast.error('释放预留端口失败: ' + (err.message || '未知错误'))
  } finally {
    const next = new Set(releasingPorts.value)
    next.delete(key)
    releasingPorts.value = next
  }
}

function updateNote(port, _protocol, value) {
  port.note = value
}

async function copyPort(port) {
  const start = Number(port.port)
  const end = Number(port.end_port || port.port)
  const label = end > start ? `${start}-${end}` : `${start}`
  const hostname = window.location.hostname || '127.0.0.1'
  const host = hostname.includes(':') && !hostname.startsWith('[') ? `[${hostname}]` : hostname
  const address = `${host}:${label}`
  const copied = await copyToClipboard(address)

  if (copied) {
    toast.success(`已复制地址 ${address}`)
  } else {
    toast.error('复制地址失败')
  }
}

async function saveNote(port, protocol) {
  try {
    await savePortNote({
      port: port.port,
      type: port.type,
      protocol,
      note: port.note || ''
    })
  } catch (err) {
    toast.error('保存备注失败')
  }
}

usePageAction({ range: openRangeDialog })

onMounted(async () => {
  try {
    await loadRange()
    await loadPorts()
  } finally {
    loading.value = false
  }
})

onBeforeUnmount(() => {
  if (searchTimeout) {
    clearTimeout(searchTimeout)
    searchTimeout = null
  }
})
</script>

<style scoped>
.ports-workbench {
  --resource-accent: var(--color-primary-500);
  --resource-accent-strong: var(--color-primary-700);
  --resource-accent-soft: var(--color-primary-100);
  --resource-row-hover: color-mix(in srgb, var(--color-primary-500) 6%, var(--bg-elevated));
}

.resource-rail {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.rail-card.neutral .rail-icon {
  color: var(--ops-blue);
  background: var(--ops-blue-soft);
}

.rail-card.success .rail-icon {
  color: var(--ops-green-strong);
  background: var(--ops-green-soft);
}

.rail-card.warning .rail-icon {
  color: var(--ops-amber);
  background: var(--ops-amber-soft);
}

.rail-card.info .rail-icon {
  color: var(--ops-cyan);
  background: var(--ops-cyan-soft);
}

.workbench-toolbar {
  width: 100%;
  max-width: 1680px;
  margin: 0 auto;
}

.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.toolbar-left {
  display: grid;
  grid-template-columns: var(--resource-toolbar-search-width) minmax(0, 1fr);
  align-items: center;
  gap: 12px;
  flex: 1;
  min-width: 0;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 0 0 auto;
}

.port-filter-tabs {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-width: 0;
  flex-wrap: nowrap;
}

.icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  border: 1px solid transparent;
  border-radius: 10px;
  background: var(--bg-elevated);
  color: var(--text-secondary);
  cursor: pointer;
  transition: background var(--motion-duration-quick) var(--motion-ease-out), border-color var(--motion-duration-quick) var(--motion-ease-out), color var(--motion-duration-quick) var(--motion-ease-out), transform var(--motion-duration-quick) var(--motion-ease-out);
}

.icon-btn:hover:not(:disabled) {
  border-color: var(--border-default);
  color: var(--text-primary);
  background: var(--bg-tertiary);
}

.icon-btn:disabled {
  cursor: wait;
  opacity: 0.68;
}

.icon-btn.is-spinning :deep(svg) {
  animation: resource-refresh-spin 0.75s linear infinite;
}

.ports-split-board {
  box-sizing: border-box;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  align-items: stretch;
  gap: 12px;
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
}

.ports-split-board > :deep(.resource-panel) {
  min-width: 0;
  min-height: 0;
}

.range-form {
  padding: 8px 0;
}

.form-row {
  display: flex;
  align-items: flex-end;
  gap: 12px;
}

.form-group {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-label {
  color: var(--text-primary);
  font-size: 0.8125rem;
  font-weight: 500;
}

.form-input {
  box-sizing: border-box;
  width: 100%;
  height: 40px;
  padding: 0 12px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  outline: none;
  background: var(--bg-elevated);
  color: var(--text-primary);
  font-size: 0.875rem;
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out), box-shadow var(--motion-duration-quick) var(--motion-ease-out);
}

.form-input:focus {
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.range-separator {
  padding-bottom: 10px;
  color: var(--text-secondary);
}

.btn {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 36px;
  padding: 0 16px;
  border-radius: 8px;
  font-size: 0.875rem;
  font-weight: 600;
  cursor: pointer;
}

.btn-default {
  border: 1px solid var(--border-subtle);
  background: var(--bg-secondary);
  color: var(--text-primary);
}

.btn-default:hover {
  background: var(--bg-tertiary);
}

.btn-primary {
  border: 1px solid var(--color-primary-700);
  background: linear-gradient(135deg, var(--color-primary-500), var(--color-primary-600));
  color: var(--text-inverse);
}

.btn-primary:hover:not(:disabled) {
  background: linear-gradient(135deg, var(--color-primary-600), var(--color-primary-700));
}

.btn-primary:disabled {
  cursor: wait;
  opacity: 0.68;
}

.btn.is-loading::before {
  content: '';
  position: absolute;
  right: 9px;
  top: 50%;
  width: 14px;
  height: 14px;
  margin-top: -7px;
  border: 2px solid color-mix(in srgb, var(--text-inverse) 32%, transparent);
  border-top-color: var(--text-inverse);
  border-radius: 50%;
  animation: resource-refresh-spin 0.7s linear infinite;
}

.btn-spinner {
  display: none;
}

@media (max-width: 1240px) {
  .toolbar {
    align-items: stretch;
    flex-wrap: wrap;
  }

  .toolbar-left {
    grid-template-columns: var(--resource-toolbar-search-width) minmax(0, 1fr);
    width: 100%;
  }

  .toolbar-right {
    margin-left: auto;
  }
}

@media (max-width: 900px) {
  .resource-rail {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .toolbar-left {
    grid-template-columns: minmax(0, 1fr);
  }

  .port-filter-tabs {
    flex-wrap: wrap;
  }

  .toolbar-right {
    width: 100%;
    justify-content: flex-end;
  }

  .ports-split-board {
    grid-template-columns: minmax(0, 1fr);
    min-height: 1120px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .icon-btn {
    transition: none;
  }
}
</style>
