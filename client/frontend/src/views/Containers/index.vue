<template>
  <ResourceWorkbench class="containers-page containers-workbench" role="main" aria-label="容器管理" remote-context>
    <template #rail>
      <section class="resource-rail" aria-label="容器状态概览">
        <article v-for="item in resourceRail" :key="item.label" class="rail-card" :class="item.tone">
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
          placeholder="搜索容器名称或镜像..."
          @search="handleSearch"
        />
        <div class="sort-control">
          <DynamicIcon name="arrow-up-down" :size="15" />
          <select v-model="sortField" aria-label="容器排序字段">
            <option value="">默认排序</option>
            <option value="name">按名称</option>
            <option value="image">按镜像</option>
            <option value="status">按状态</option>
          </select>
          <button
            type="button"
            class="sort-direction-btn"
            :class="{ 'is-placeholder': !sortField }"
            :disabled="!sortField"
            :aria-hidden="!sortField"
            :title="sortState.order === 'ascending' ? '当前升序，点击切换降序' : '当前降序，点击切换升序'"
            @click="toggleSortDirection"
          >
            <DynamicIcon :name="sortState.order === 'ascending' ? 'arrow-up' : 'arrow-down'" :size="15" />
          </button>
        </div>
        <SegmentedTabs
          v-model="statusFilter"
          :options="statusTabs"
          aria-label="状态筛选"
          compact
          @change="currentPage = 1"
        />
      </div>
      <div class="toolbar-right">
        <template v-if="viewMode !== 'grid' && checkedContainers.length > 0 && canContainerLifecycle">
          <button
            v-ripple
            class="primary-btn"
            type="button"
            :disabled="batchWorking"
            title="批量启动已勾选的容器"
            @click="batchLifecycle('start', '启动')"
          >
            <DynamicIcon name="play" :size="16" />
            批量启动（{{ checkedContainers.length }}）
          </button>
          <button
            v-ripple
            class="secondary-btn"
            type="button"
            :disabled="batchWorking"
            title="批量停止已勾选的容器"
            @click="batchLifecycle('stop', '停止')"
          >
            <DynamicIcon name="stop" :size="16" />
            批量停止（{{ checkedContainers.length }}）
          </button>
        </template>
        <ViewToggle v-model="viewMode" />
        <button
          v-ripple
          class="icon-btn resource-refresh-btn"
          :class="{ 'is-spinning': loading }"
          :disabled="loading"
          title="刷新容器列表"
          @click="fetchContainers"
        >
          <DynamicIcon class="resource-refresh-icon" name="refresh" :size="18" />
        </button>
        <div class="dropdown-wrapper">
          <button
            class="icon-btn"
            data-remote-write
            :class="{ 'is-active': showMoreMenu }"
            title="更多操作"
            @click="toggleMoreMenu"
          >
            <DynamicIcon name="more-vertical" :size="18" />
          </button>
          <div v-if="showMoreMenu" class="dropdown-menu">
            <div class="dropdown-item danger" @click="pruneContainers">
              <DynamicIcon name="delete" :size="16" />
              清理已停止容器
            </div>
          </div>
        </div>
        <button class="primary-btn" data-remote-write @click="openCreateDialog">
          <DynamicIcon name="plus" :size="16" />
          新建容器
        </button>
      </div>
      </section>
    </template>

    <ResourceBoard>
      <ResourcePanel>
          <CardGrid
            v-if="viewMode === 'grid'"
            :items="paginatedContainers"
            :loading="loading"
            :skeleton-count="12"
            :empty-props="emptyProps"
          >
            <template #default="{ items }">
              <ContainerCard
                v-for="container in items"
                :key="container.Id"
                :container="container"
                :class="{ 'is-selected': isSelectedContainer(container) }"
                @select="selectContainer(container)"
                @start="handleStart(container)"
                @stop="handleStop(container)"
                @restart="handleRestart(container)"
                @logs="handleLogs(container)"
                @terminal="handleTerminal(container)"
                @remove="handleRemove(container)"
                @force-stop="handleForceStop(container)"
              />
            </template>
          </CardGrid>

          <ResourceTable
            v-else
            :columns="containerColumns"
            :rows="paginatedContainers"
            table-key="containers-v2"
            row-key="Id"
            :selected-key="selectedContainerId"
            :row-class="getContainerRowClass"
            :loading="loading"
            aria-label="容器列表"
            @select="selectContainer"
            @sort="handleSort"
          >
            <template #header-check>
              <input
                class="container-batch-check"
                type="checkbox"
                :checked="allVisibleChecked"
                :indeterminate.prop="partiallyChecked"
                :disabled="paginatedContainers.length === 0"
                title="全选/取消当前列表全部容器"
                @click.stop
                @change="toggleContainerCheckAll"
              />
            </template>

            <template #cell-check="{ row: container }">
              <input
                class="container-batch-check"
                type="checkbox"
                :checked="checkedContainerIds.includes(container.Id)"
                title="勾选后可批量启动/停止"
                @click.stop
                @change="toggleContainerCheck(container.Id)"
              />
            </template>

            <template v-for="column in sortableContainerColumns" :key="column.key" #[`header-${column.key}`]>
              {{ column.label }}
              <span class="sort-icon" :class="getSortIcon(column.key)">
                <DynamicIcon v-if="getSortIcon(column.key) === 'asc'" name="chevron-up" :size="14" />
                <DynamicIcon v-else-if="getSortIcon(column.key) === 'desc'" name="chevron-down" :size="14" />
                <DynamicIcon v-else name="arrow-up-down" :size="14" />
              </span>
            </template>

            <template #cell-name="{ row: container }">
                  <div class="name-wrapper">
                    <span class="resource-kind container">
                      <DynamicIcon class="container-icon-small" name="container" :size="16" />
                    </span>
                    <span class="resource-name-copy">
                      <strong>{{ getDisplayContainerName(container) }}</strong>
                      <small>{{ container.Id?.substring(0, 12) || '-' }}</small>
                    </span>
                    <span v-if="hasContainerUpdate(container)" class="container-update-tag">
                      <DynamicIcon name="cloud-download" :size="12" />
                      可更新
                    </span>
                  </div>
            </template>

            <template #cell-image="{ row: container }">
              <span class="cell-mono image-cell" :title="container.Image || '<none>'">{{ formatDockerImageReference(container.Image) || '<none>' }}</span>
            </template>

            <template #cell-status="{ row: container }">
              <StatusBadge :status="getContainerState(container)" :label="getContainerStateLabel(container)" variant="minimal" />
            </template>

            <template #cell-resources="{ row: container }">
              <span v-if="!hasContainerResource(container)" class="resource-empty">-</span>
              <div v-else class="resource-usage-cell" :title="getResourceTitle(container)">
                <span class="resource-metric">
                  <b class="resource-metric-label">CPU</b>
                  <span class="resource-metric-value">{{ formatStatPercent(container.ResourceStats?.cpu_percent) }}</span>
                </span>
                <span class="resource-metric">
                  <b class="resource-metric-label">MEM</b>
                  <span class="resource-metric-value">{{ formatStatPercent(container.ResourceStats?.memory_percent) }}</span>
                </span>
                <span class="resource-metric">
                  <b class="resource-metric-label">NET</b>
                  <span class="resource-metric-value">{{ formatBytesShort(sumStats(container.ResourceStats?.network_rx, container.ResourceStats?.network_tx)) }}</span>
                </span>
                <span class="resource-metric">
                  <b class="resource-metric-label">I/O</b>
                  <span class="resource-metric-value">{{ formatBytesShort(sumStats(container.ResourceStats?.block_read, container.ResourceStats?.block_write)) }}</span>
                </span>
              </div>
            </template>

            <template #cell-ports="{ row: container }">
              <PortLinksCell
                :ports="container.Ports"
                :fallback-text="formatPorts(container.Ports)"
                :fallback-title="formatPortsTitle(container.Ports)"
              />
            </template>

            <template #cell-actions="{ row: container }">
              <div v-if="canContainerLifecycle" class="cell-actions">
                  <div class="action-buttons compact-actions">
                    <button
                      v-if="container.State === 'running'"
                      class="table-btn stop"
                      title="停止"
                      @click.stop="handleStop(container)"
                    >
                      <DynamicIcon name="stop" :size="14" />
                    </button>
                    <button
                      v-else
                      class="table-btn success"
                      title="启动"
                      @click.stop="handleStart(container)"
                    >
                      <DynamicIcon name="play" :size="14" />
                    </button>
                    <button class="table-btn warning" title="重启" @click.stop="handleRestart(container)">
                      <DynamicIcon name="refresh" :size="14" />
                    </button>
                    <button
                      v-if="!isRemoteEnvironment"
                      class="table-btn row-action-more"
                      title="更多"
                      :class="{ 'is-active': openRowMenuId === container.Id }"
                      @click.stop="toggleRowActionMenu(container, $event)"
                    >
                      <DynamicIcon name="more-vertical" :size="14" />
                    </button>
                  </div>
              </div>
            </template>

            <template #empty>
              <EmptyState v-bind="emptyProps" />
            </template>
          </ResourceTable>
        <template #footer>
          <Pagination
            v-model:current="currentPage"
            v-model:page-size="pageSize"
            :total="filteredContainers.length"
          />
        </template>
      </ResourcePanel>

      <template #context>
        <ResourceContextPanel :motion-key="selectedSummaryContainer?.Id || 'overview'" aria-label="容器上下文">
          <div class="context-header">
            <span class="resource-kind container">
              <DynamicIcon name="container" :size="18" />
            </span>
            <div>
              <strong>{{ selectedSummaryContainer ? getDisplayContainerName(selectedSummaryContainer) : '容器概览' }}</strong>
              <span v-if="selectedSummaryContainer">{{ formatDockerImageReference(selectedSummaryContainer.Image) || '<none>' }}</span>
              <span v-else>{{ filteredContainers.length }} 个匹配结果 · {{ activeStatusLabel }}</span>
            </div>
            <span v-if="selectedSummaryContainer && hasContainerUpdate(selectedSummaryContainer)" class="container-update-tag">
              <DynamicIcon name="cloud-download" :size="12" />
              可更新
            </span>
          </div>

          <div v-if="selectedSummaryContainer && (canContainerLifecycle || canContainerLogs)" class="context-actions">
            <button
              v-if="!isRemoteEnvironment && hasContainerUpdate(selectedSummaryContainer)"
              type="button"
              class="detail-action update"
              :disabled="isContainerUpdating(selectedSummaryContainer)"
              @click="handleUpdate(selectedSummaryContainer)"
            >
              <DynamicIcon name="cloud-download" :size="15" />
              更新
            </button>
            <button
              v-if="canContainerLifecycle && selectedSummaryContainer.State === 'running'"
              type="button"
              class="detail-action stop"
              @click="handleStop(selectedSummaryContainer)"
            >
              <DynamicIcon name="stop" :size="15" />
              停止
            </button>
            <button
              v-if="canContainerLifecycle && selectedSummaryContainer.State !== 'running'"
              type="button"
              class="detail-action success"
              @click="handleStart(selectedSummaryContainer)"
            >
              <DynamicIcon name="play" :size="15" />
              启动
            </button>
            <button v-if="canContainerLifecycle" type="button" class="detail-action warning" @click="handleRestart(selectedSummaryContainer)">
              <DynamicIcon name="refresh" :size="15" />
              重启
            </button>
            <button v-if="canContainerLogs" type="button" class="detail-action info" @click="handleLogs(selectedSummaryContainer)">
              <DynamicIcon name="file-text" :size="15" />
              Logs
            </button>
            <button v-if="!isRemoteEnvironment" type="button" class="detail-action terminal" @click="handleTerminal(selectedSummaryContainer)">
              <DynamicIcon name="terminal" :size="15" />
              终端
            </button>
          </div>

          <div class="context-grid">
            <div>
              <span>公开端口</span>
              <strong :title="selectedSummaryContainer ? formatPortsTitle(selectedSummaryContainer.Ports) : ''">{{ selectedSummaryContainer ? dedupePorts(selectedSummaryContainer.Ports || []).length : summaryPortsCount }}</strong>
            </div>
            <div>
              <span>已运行时间</span>
              <strong>{{ selectedSummaryContainer ? formatContainerUptimeCn(selectedSummaryContainer) : '-' }}</strong>
            </div>
          </div>

          <div v-if="selectedSummaryContainer" class="context-section selected-detail">
            <p class="section-label">端口 / 资源</p>
            <div class="detail-lines">
              <span>
                <b>PORT</b>
                {{ formatPorts(selectedSummaryContainer.Ports) }}
              </span>
              <span>
                <b>CPU</b>
                {{ selectedSummaryContainer.ResourceStats ? formatStatPercent(selectedSummaryContainer.ResourceStats.cpu_percent) : '-' }}
              </span>
              <span>
                <b>MEM</b>
                {{ selectedSummaryContainer.ResourceStats ? formatStatPercent(selectedSummaryContainer.ResourceStats.memory_percent) : '-' }}
              </span>
            </div>
          </div>

          <LiveLogPreview
            class="context-section log-stream-preview"
            v-if="selectedSummaryContainer && canContainerLogs"
            :source="selectedContainerLogSource"
            title="容器日志"
            :tail="40"
            :max-lines="80"
          />
        </ResourceContextPanel>
      </template>
    </ResourceBoard>

    <AnchoredMenu
      :open="Boolean(openRowMenuId)"
      :anchor="rowActionMenuAnchor"
      @update:open="handleRowActionMenuOpenChange"
    >
      <template v-if="activeRowActionContainer">
        <button
          v-if="hasContainerUpdate(activeRowActionContainer)"
          type="button"
          role="menuitem"
          :disabled="isContainerUpdating(activeRowActionContainer)"
          @click="handleUpdateFromMenu(activeRowActionContainer)"
        >
          <DynamicIcon name="cloud-download" :size="14" />
          更新容器
        </button>
        <button type="button" role="menuitem" @click="handleLogsFromMenu(activeRowActionContainer)">
          <DynamicIcon name="file-text" :size="14" />
          日志
        </button>
        <button type="button" role="menuitem" @click="handleTerminalFromMenu(activeRowActionContainer)">
          <DynamicIcon name="terminal" :size="14" />
          终端
        </button>
        <button type="button" role="menuitem" @click="handleResetFromMenu(activeRowActionContainer)">
          <DynamicIcon name="refresh" :size="14" />
          重置容器
        </button>
        <button type="button" role="menuitem" @click="handleForceStopFromMenu(activeRowActionContainer)">
          <DynamicIcon name="zap" :size="14" />
          强制停止
        </button>
        <button type="button" role="menuitem" class="danger" @click="handleRemoveFromMenu(activeRowActionContainer)">
          <DynamicIcon name="delete" :size="14" />
          删除
        </button>
      </template>
    </AnchoredMenu>

    <!-- 日志对话框 -->
    <Modal
      v-model:visible="showLogsDialog"
      title="容器日志"
      width="80%"
    >
      <ContainerLogs
        v-if="selectedContainer"
        :container-id="selectedContainer.Id"
        :container-name="(selectedContainer.Names?.[0] || '').replace(/^\//, '')"
      />
    </Modal>

    <!-- 终端对话框 -->
    <Modal
      v-model:visible="showTerminalDialog"
      title="容器终端"
      width="80%"
    >
      <ContainerTerminal
        v-if="showTerminalDialog && selectedContainer"
        :container-id="selectedContainer.Id"
        :container-name="(selectedContainer.Names?.[0] || '').replace(/^\//, '')"
      />
    </Modal>

    <Modal
      v-model:visible="showResetDialog"
      title="重置容器"
      width="640px"
      :show-close="!resettingContainer"
      :close-on-esc="!resettingContainer"
    >
      <div class="reset-container-summary">
        <span class="resource-kind container">
          <DynamicIcon name="container" :size="16" />
        </span>
        <div>
          <strong>{{ resetTargetName }}</strong>
          <span>{{ resetTargetImage }}</span>
        </div>
        <span class="reset-mode">本地镜像</span>
      </div>

      <div class="reset-log ops-console" aria-live="polite">
        <div v-if="resetLogs.length === 0" class="reset-log-empty">正在准备重建容器...</div>
        <div
          v-for="line in resetLogs"
          :key="line.id"
          class="reset-log-line"
          :class="`is-${line.tone}`"
        >
          <time>{{ line.time }}</time>
          <span>{{ line.message }}</span>
        </div>
      </div>

      <template #footer>
        <button class="btn btn-default" :disabled="resettingContainer" @click="showResetDialog = false">
          {{ resettingContainer ? '重置中...' : '关闭' }}
        </button>
      </template>
    </Modal>
  </ResourceWorkbench>
</template>

<script setup>
import { ref, computed, nextTick, onMounted, onUnmounted, defineAsyncComponent, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import CardGrid from '@/components/data-display/CardGrid.vue'
import ContainerCard from '@/components/data-display/ContainerCard.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import ViewToggle from '@/components/ui/ViewToggle.vue'
import Pagination from '@/components/ui/Pagination.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import AnchoredMenu from '@/components/ui/AnchoredMenu.vue'
import SegmentedTabs from '@/components/ui/SegmentedTabs.vue'
import Modal from '@/components/feedback/Modal.vue'
import ContainerLogs from '@/components/container/ContainerLogs.vue'
import LiveLogPreview from '@/components/container/LiveLogPreview.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import { containers, system } from '@edition/api'
import { useUiStore } from '@/stores/ui.js'
import { remoteCapabilityAllowed, useEnvironmentContext } from '@/composables/useEnvironmentContext.js'
import { useViewMode } from '@/composables/useViewMode.js'
import { useSort } from '@/composables/useSort.js'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import ResourceTable from '@/components/resource-workbench/ResourceTable.vue'
import PortLinksCell from './components/PortLinksCell.vue'
import ResourceWorkbench from '@/components/resource-workbench/ResourceWorkbench.vue'
import ResourceBoard from '@/components/resource-workbench/ResourceBoard.vue'
import ResourcePanel from '@/components/resource-workbench/ResourcePanel.vue'
import ResourceContextPanel from '@/components/resource-workbench/ResourceContextPanel.vue'
import { usePageAction } from '@/composables/usePageAction.js'
import { formatBytesCompact, formatDockerImageReference } from '@/utils/format.js'
import { classifyLogLine } from '@/utils/logPresentation.js'
import { classifyContainerStatus, containerDisplayStatus, formatContainerUptimeCn } from '@/utils/resourceStatus.js'
import { notifyBatchResult } from '@/utils/batchFeedback.js'

const ContainerTerminal = defineAsyncComponent(() => import('@/components/container/ContainerTerminal.vue'))

const uiStore = useUiStore()
const { isRemoteEnvironment, environment } = useEnvironmentContext()
const canContainerLifecycle = computed(() => !isRemoteEnvironment.value || remoteCapabilityAllowed(environment.value, 'container.manage'))
const canContainerLogs = computed(() => !isRemoteEnvironment.value || remoteCapabilityAllowed(environment.value, 'container.logs'))
const route = useRoute()
const router = useRouter()

// 视图模式
const { viewMode } = useViewMode('containers_view_mode', 'table')

// 排序
const { sortState, handleSort, setSort, sortData, getSortIcon } = useSort('sort_containers')
const containerColumns = [
  { key: 'check', label: '', cellClass: 'cell-check', width: 40, minWidth: 36 },
  { key: 'name', label: '名称', sortable: true, cellClass: 'cell-name', width: 190, minWidth: 170, flex: true },
  { key: 'image', label: '镜像', sortable: true, width: 180, minWidth: 140 },
  { key: 'status', label: '状态', sortable: true, cellClass: 'status-cell', width: 96, minWidth: 88 },
  { key: 'resources', label: '资源', width: 150, minWidth: 130 },
  { key: 'ports', label: '端口', cellClass: 'ports-cell', width: 100, minWidth: 86 },
  { key: 'actions', label: '操作', cellClass: 'cell-actions', width: 126, minWidth: 126 }
]
const sortableContainerColumns = containerColumns.filter(column => column.sortable)
const sortField = computed({
  get: () => sortState.value.prop || '',
  set: value => setSort(value, sortState.value.order || 'ascending')
})

function toggleSortDirection() {
  if (!sortState.value.prop) return
  setSort(
    sortState.value.prop,
    sortState.value.order === 'ascending' ? 'descending' : 'ascending'
  )
}

// 状态
const loading = ref(false)
const containersList = ref([])
const containerStatsMap = ref(new Map())
const searchQuery = ref('')
const statusFilter = ref('all') // all, running, stopped
const currentPage = ref(1)
const pageSize = ref(20)
const showMoreMenu = ref(false)
const selectedContainerId = ref('')
const openRowMenuId = ref('')

// 批量启动/停止：仅表格视图提供勾选，勾选键为容器 ID。
const checkedContainerIds = ref([])
const batchWorking = ref(false)
// 批量目标取当前过滤结果中被勾选的容器，保证计数与用户可见范围一致。
const checkedContainers = computed(() =>
  filteredContainers.value.filter(container => checkedContainerIds.value.includes(container.Id))
)
const allVisibleChecked = computed(() =>
  paginatedContainers.value.length > 0 &&
  paginatedContainers.value.every(container => checkedContainerIds.value.includes(container.Id))
)
const partiallyChecked = computed(() =>
  !allVisibleChecked.value &&
  paginatedContainers.value.some(container => checkedContainerIds.value.includes(container.Id))
)

function toggleContainerCheck(id) {
  const next = new Set(checkedContainerIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  checkedContainerIds.value = [...next]
}

// 表头全选：勾选/清空当前页条目，不影响其它页已勾选的容器。
function toggleContainerCheckAll() {
  const visible = paginatedContainers.value.map(container => container.Id)
  const visibleSet = new Set(visible)
  const allChecked = visible.length > 0 && visible.every(id => checkedContainerIds.value.includes(id))
  checkedContainerIds.value = allChecked
    ? checkedContainerIds.value.filter(id => !visibleSet.has(id))
    : [...new Set([...checkedContainerIds.value, ...visible])]
}

// 刷新后剔除已不存在的勾选，避免对已删除容器执行批量操作。
function pruneCheckedContainers() {
  const existing = new Set(containersList.value.map(container => container.Id))
  const next = checkedContainerIds.value.filter(id => existing.has(id))
  if (next.length !== checkedContainerIds.value.length) checkedContainerIds.value = next
}

// 批量启动/停止：串行逐项调用现有单容器接口，逐条隔离失败并汇总。
async function batchLifecycle(action, actionLabel) {
  const targets = checkedContainers.value
  if (batchWorking.value || targets.length === 0) return
  batchWorking.value = true
  let succeeded = 0
  const failures = []
  for (const container of targets) {
    try {
      await containers[action](container.Id)
      succeeded += 1
    } catch (error) {
      const name = getDisplayContainerName(container) || container.Id
      failures.push({ name, reason: error.message || '' })
    }
  }
  batchWorking.value = false
  checkedContainerIds.value = []
  await fetchContainers()
  notifyBatchResult(uiStore, { label: `批量${actionLabel}`, succeeded, failures })
}
const activeRowActionContainer = ref(null)
const rowActionMenuAnchor = ref(null)

// 对话框状态
const showLogsDialog = ref(false)
const showTerminalDialog = ref(false)
const showResetDialog = ref(false)
const resettingContainer = ref(false)
const resetTargetName = ref('')
const resetTargetImage = ref('')
const resetLogs = ref([])
const selectedContainer = ref(null)
let resourceStatsTimer = null
let resourceStatsLoading = false
let resetStreamStop = null
let resetLogSequence = 0
const containerUpdatePollTimers = new Map()
const updatingContainerIds = ref(new Set())

// 空状态配置
const emptyProps = {
  title: '暂无容器',
  description: '点击"新建容器"按钮创建第一个容器',
  icon: 'container'
}

const containerCounts = computed(() => {
  const total = containersList.value.length
  const running = containersList.value.filter(c => classifyContainerStatus(c) === 'running').length
  const unhealthy = containersList.value.filter(c => classifyContainerStatus(c) === 'unhealthy').length
  const stopped = containersList.value.filter(c => classifyContainerStatus(c) === 'stopped').length
  const exposedPorts = containersList.value.reduce((sum, container) => sum + dedupePorts(container.Ports || []).length, 0)
  return { total, running, unhealthy, stopped, exposedPorts }
})

// 状态筛选选项
const statusTabs = computed(() => [
  { label: '全部', value: 'all', count: containerCounts.value.total },
  { label: '运行中', value: 'running', count: containerCounts.value.running },
  { label: '其它', value: 'unhealthy', count: containerCounts.value.unhealthy },
  { label: '已停止', value: 'stopped', count: containerCounts.value.stopped }
])

const activeStatusLabel = computed(() => {
  return statusTabs.value.find(tab => tab.value === statusFilter.value)?.label || '全部'
})

const resourceRail = computed(() => [
  {
    label: '容器总数',
    value: containerCounts.value.total,
    note: `${filteredContainers.value.length} 个匹配`,
    icon: 'container',
    tone: 'neutral'
  },
  {
    label: '运行中',
    value: containerCounts.value.running,
    note: '实时采样',
    icon: 'play',
    tone: 'success'
  },
  {
    label: '不健康',
    value: containerCounts.value.unhealthy,
    note: containerCounts.value.unhealthy > 0 ? '需要关注' : '无异常',
    icon: 'alert',
    tone: 'warning'
  },
  {
    label: '已停止',
    value: containerCounts.value.stopped,
    note: '可清理',
    icon: 'stop',
    tone: 'muted'
  },
  {
    label: '公开端口',
    value: containerCounts.value.exposedPorts,
    note: '宿主机映射',
    icon: 'server',
    tone: 'info'
  }
])

const enrichedContainers = computed(() => containersList.value.map(container => ({
  ...container,
  ResourceStats: getContainerResourceStats(container)
})))

// 获取容器状态
function getContainerState(container) {
  return containerDisplayStatus(container)
}

function getContainerRowClass(container) {
  return `state-${getContainerState(container)}`
}

// 格式化端口（列表显示，按 PublicPort:PrivatePort 去重 IPv4/IPv6 重复）
function formatPorts(ports) {
  if (!ports || ports.length === 0) return '-'
  const publicPorts = dedupePorts(ports)
  if (publicPorts.length > 0) {
    return publicPorts.slice(0, 2).map(p => `${p.PublicPort}:${p.PrivatePort}`).join(', ') + (publicPorts.length > 2 ? '...' : '')
  }
  // 没有公开端口但有 EXPOSE 声明的内部端口：显示 :PrivatePort（去掉重复）
  const internalPorts = dedupeInternalPorts(ports)
  if (internalPorts.length === 0) return '-'
  return internalPorts.slice(0, 2).map(p => `:${p.PrivatePort}/${p.Type || 'tcp'}`).join(', ') + (internalPorts.length > 2 ? '...' : '')
}

// 端口悬停提示：列出所有 IP 绑定（IPv4 + IPv6 都展示），包含仅内部端口
function formatPortsTitle(ports) {
  if (!ports || ports.length === 0) return ''
  const seen = new Set()
  const lines = []
  for (const p of ports) {
    if (p.PublicPort) {
      const ip = p.IP || '0.0.0.0'
      const host = ip.includes(':') ? `[${ip}]` : ip
      const line = `${host}:${p.PublicPort} → :${p.PrivatePort}/${p.Type || 'tcp'}`
      if (!seen.has(line)) { seen.add(line); lines.push(line) }
    } else if (p.PrivatePort) {
      const line = `内部 :${p.PrivatePort}/${p.Type || 'tcp'}（仅 EXPOSE，未映射到宿主机）`
      if (!seen.has(line)) { seen.add(line); lines.push(line) }
    }
  }
  return lines.join('\n')
}

// Docker 对同一端口会分别返回 IPv4 (0.0.0.0) 和 IPv6 (::) 两条记录，
// 按 PublicPort:PrivatePort 去重避免列表显示两次，但悬停时仍展示完整 IP 信息。
function dedupePorts(ports) {
  const seen = new Set()
  const result = []
  for (const p of ports) {
    if (!p.PublicPort) continue
    const key = `${p.PublicPort}:${p.PrivatePort}`
    if (seen.has(key)) continue
    seen.add(key)
    result.push(p)
  }
  return result
}

// 仅去重内部端口（无 PublicPort 但有 PrivatePort 的 EXPOSE 声明）
function dedupeInternalPorts(ports) {
  const seen = new Set()
  const result = []
  for (const p of ports) {
    if (p.PublicPort || !p.PrivatePort) continue
    const key = `${p.PrivatePort}/${p.Type || 'tcp'}`
    if (seen.has(key)) continue
    seen.add(key)
    result.push(p)
  }
  return result
}

function getDisplayContainerName(container) {
  return (container.Names?.[0] || '').replace(/^\//, '') || container.Id?.substring(0, 12) || '-'
}

function isContainerRunning(container) {
  return String(container?.State || '').toLowerCase() === 'running'
}

function hasContainerResource(container) {
  return isContainerRunning(container) && Boolean(container?.ResourceStats)
}

function getContainerResourceStats(container) {
  const id = String(container?.Id || '')
  if (!id) return null
  return containerStatsMap.value.get(id) || containerStatsMap.value.get(id.slice(0, 12)) || null
}

function formatStatPercent(value) {
  if (value === undefined || value === null) return '-'
  const num = Number(value)
  if (Number.isNaN(num)) return '-'
  return `${num.toFixed(num >= 10 ? 0 : 1)}%`
}

function getResourceTitle(container) {
  if (!isContainerRunning(container)) return ''
  const stats = container.ResourceStats
  if (!stats) return ''
  const memory = stats.memory_usage ? `内存占用 ${formatBytesCompact(Number(stats.memory_usage || 0))}` : '内存占用 -'
  const net = `网络 I/O ${formatBytesCompact(sumStats(stats.network_rx, stats.network_tx))}`
  const block = `磁盘 I/O ${formatBytesCompact(sumStats(stats.block_read, stats.block_write))}`
  return `CPU ${formatStatPercent(stats.cpu_percent)}\nMEM ${formatStatPercent(stats.memory_percent)}\n${memory}\n${net}\n${block}`
}

function sumStats(...values) {
  return values.reduce((sum, value) => sum + Number(value || 0), 0)
}

function formatBytesShort(value) {
  const num = Number(value || 0)
  if (num <= 0) return '-'
  return formatBytesCompact(num)
}

// 计算属性
const filteredContainers = computed(() => {
  let list = enrichedContainers.value
  
  // 状态筛选
  if (statusFilter.value !== 'all') {
    list = list.filter(c => classifyContainerStatus(c) === statusFilter.value)
  }
  
  // 搜索过滤
  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    list = list.filter(container => {
      const name = (container.Names?.[0] || '').toLowerCase()
      const image = (container.Image || '').toLowerCase()
      return name.includes(query) || image.includes(query)
    })
  }
  
  // 应用用户自定义排序（如果有）
  if (sortState.value.prop && sortState.value.order) {
    return sortData(list, (item, prop) => {
      switch (prop) {
        case 'name':
          return (item.Names?.[0] || '').replace(/^\//, '')
        case 'image':
          return item.Image || ''
        case 'status':
          return getContainerState(item)
        default:
          return ''
      }
    })
  }
  
  // 默认排序：运行中在前，按名称排序
  return [...list].sort((a, b) => {
    if (a.State === 'running' && b.State !== 'running') return -1
    if (a.State !== 'running' && b.State === 'running') return 1
    const nameA = (a.Names?.[0] || '').replace(/^\//, '')
    const nameB = (b.Names?.[0] || '').replace(/^\//, '')
    return nameA.localeCompare(nameB)
  })
})

// 分页后的列表
const paginatedContainers = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  const end = start + pageSize.value
  return filteredContainers.value.slice(start, end)
})

// 批量删除或筛选导致总页数收缩时，把当前页钳制到最后一个非空页。
watch(() => Math.max(1, Math.ceil(filteredContainers.value.length / pageSize.value)), pages => {
  if (currentPage.value > pages) currentPage.value = pages
})

const selectedSummaryContainer = computed(() => {
  if (selectedContainerId.value) {
    const selected = enrichedContainers.value.find(container => container.Id === selectedContainerId.value)
    if (selected) return selected
  }
  return paginatedContainers.value.find(container => container.State === 'running') || paginatedContainers.value[0] || null
})

const selectedContainerLogSource = computed(() => {
  const container = selectedSummaryContainer.value
  return {
    type: 'container',
    id: container?.Id || '',
    label: container ? getDisplayContainerName(container) : 'container',
    running: container?.State === 'running'
  }
})

const summaryPortsCount = computed(() => {
  return filteredContainers.value.reduce((sum, container) => sum + dedupePorts(container.Ports || []).length, 0)
})

function getContainerStateLabel(container) {
  const labels = {
    running: '运行中',
    unhealthy: '不健康',
    'health-starting': '检测中',
    stopped: '已停止',
    paused: '已暂停',
    restarting: '重启中',
    removing: '移除中',
    created: '已创建',
    error: '故障',
    unknown: '未知'
  }
  return labels[getContainerState(container)] || '未知'
}

function hasContainerUpdate(container) {
  return Boolean(container?.UpdateAvailable ?? container?.updateAvailable)
}

function selectContainer(container) {
  selectedContainerId.value = container?.Id || ''
  closeRowActionMenu()
}

function isSelectedContainer(container) {
  return Boolean(container?.Id && selectedSummaryContainer.value?.Id === container.Id)
}

// 方法
function handleSearch() {
  currentPage.value = 1
}

function toggleMoreMenu(e) {
  e.stopPropagation()
  showMoreMenu.value = !showMoreMenu.value
}

function closeMoreMenu() {
  showMoreMenu.value = false
}

function toggleRowActionMenu(container, event) {
  if (openRowMenuId.value === container?.Id) {
    closeRowActionMenu()
    return
  }
  openRowMenuId.value = container?.Id || ''
  activeRowActionContainer.value = container || null
  rowActionMenuAnchor.value = event?.currentTarget || null
}

function closeRowActionMenu() {
  openRowMenuId.value = ''
  activeRowActionContainer.value = null
  rowActionMenuAnchor.value = null
}

function handleRowActionMenuOpenChange(open) {
  if (!open) closeRowActionMenu()
}

async function fetchContainers() {
  loading.value = true
  try {
    const data = await containers.list({ all: true })
    containersList.value = data || []
    pruneCheckedContainers()
    await focusContainerFromRoute()
  } catch (error) {
    console.error('获取容器列表失败:', error)
  } finally {
    loading.value = false
  }
}

async function focusContainerFromRoute() {
  const focusId = String(route.query?.focus || '').trim()
  const focusName = String(route.query?.focusName || '').replace(/^\//, '').trim()
  if (!focusId && !focusName) return

  const target = containersList.value.find(container => {
    const id = String(container?.Id || '')
    const name = getDisplayContainerName(container)
    const matchesId = focusId && (id === focusId || id.startsWith(focusId) || focusId.startsWith(id))
    return matchesId || (focusName && name === focusName)
  })
  if (!target) return

  statusFilter.value = 'all'
  searchQuery.value = ''
  selectedContainerId.value = target.Id
  await nextTick()

  const targetIndex = filteredContainers.value.findIndex(container => container.Id === target.Id)
  if (targetIndex >= 0) {
    currentPage.value = Math.floor(targetIndex / pageSize.value) + 1
  }

  const query = { ...route.query }
  delete query.focus
  delete query.focusName
  await router.replace({ query })
}

watch(
  () => [route.query?.focus, route.query?.focusName],
  () => { void focusContainerFromRoute() },
  { immediate: true }
)

async function fetchResourceStats() {
  if (resourceStatsLoading) return
  resourceStatsLoading = true
  try {
    const data = await system.getStats()
    const map = new Map()
    const statsList = data?.container_stats || []
    for (const item of statsList) {
      if (item.container_id) map.set(String(item.container_id), item)
      if (item.id) map.set(String(item.id), item)
    }
    containerStatsMap.value = map
  } catch (error) {
    console.error('获取容器资源占用失败:', error)
  } finally {
    resourceStatsLoading = false
  }
}

function startResourceStatsRefresh() {
  stopResourceStatsRefresh()
  if (isRemoteEnvironment.value) {
    containerStatsMap.value = new Map()
    return
  }
  fetchResourceStats()
  resourceStatsTimer = setInterval(fetchResourceStats, 5000)
}

function stopResourceStatsRefresh() {
  if (resourceStatsTimer) {
    clearInterval(resourceStatsTimer)
    resourceStatsTimer = null
  }
}

function openCreateDialog() {
  uiStore.toastInfo('新建容器功能开发中...')
}

async function handleStart(container) {
  try {
    await containers.start(container.Id)
    await fetchContainers()
  } catch (error) {
    console.error('启动容器失败:', error)
    uiStore.toastError('启动失败: ' + (error.message || '未知错误'))
  }
}

async function handleStop(container) {
  try {
    await containers.stop(container.Id)
    await fetchContainers()
  } catch (error) {
    console.error('停止容器失败:', error)
    uiStore.toastError('停止失败: ' + (error.message || '未知错误'))
  }
}

async function handleRestart(container) {
  try {
    await containers.restart(container.Id)
    await fetchContainers()
  } catch (error) {
    console.error('重启容器失败:', error)
    uiStore.toastError('重启失败: ' + (error.message || '未知错误'))
  }
}

async function handleForceStop(container) {
  const name = (container.Names?.[0] || '').replace(/^\//, '') || container.Id?.slice(0, 12)
  const confirmed = await uiStore.confirm({
    type: 'danger',
    title: '强制停止容器',
    message: `将对容器 "${name}" 立即发送 SIGKILL，不做优雅停止。容器内未保存的数据可能丢失。确定继续吗？`,
    confirmText: '强制停止'
  })
  if (!confirmed) {
    return
  }
  try {
    await containers.forceStop(container.Id)
    uiStore.toastSuccess?.(`容器 ${name} 已强制停止`)
    await fetchContainers()
  } catch (error) {
    console.error('强制停止容器失败:', error)
    uiStore.toastError('强制停止失败: ' + (error.message || '未知错误'))
  }
}

async function handleRemove(container) {
  const name = (container.Names?.[0] || '').replace(/^\//, '')
  const confirmed = await uiStore.confirm({
    type: 'danger',
    title: '删除容器',
    message: `确定删除容器 "${name}" 吗？`,
    confirmText: '删除'
  })
  if (!confirmed) {
    return
  }
  try {
    await containers.remove(container.Id, { force: container.State === 'running' })
    await fetchContainers()
  } catch (error) {
    console.error('删除容器失败:', error)
    uiStore.toastError('删除失败: ' + (error.message || '未知错误'))
  }
}

async function pruneContainers() {
  closeMoreMenu()
  const confirmed = await uiStore.confirm({
    type: 'danger',
    title: '清理容器',
    message: '确定清理所有已停止的容器吗？此操作不可恢复。',
    confirmText: '清理'
  })
  if (!confirmed) {
    return
  }
  try {
    await containers.prune()
    await fetchContainers()
  } catch (error) {
    console.error('清理容器失败:', error)
    uiStore.toastError('清理失败: ' + (error.message || '未知错误'))
  }
}

function handleLogs(container) {
  selectedContainer.value = container
  showLogsDialog.value = true
}

function handleTerminal(container) {
  selectedContainer.value = container
  showTerminalDialog.value = true
}

function handleLogsFromMenu(container) {
  closeRowActionMenu()
  handleLogs(container)
}

function handleTerminalFromMenu(container) {
  closeRowActionMenu()
  handleTerminal(container)
}

function appendResetLog(rawLine) {
  const raw = String(rawLine || '').trim()
  if (!raw) return

  resetLogSequence += 1
  resetLogs.value.push({
    id: resetLogSequence,
    time: new Date().toLocaleTimeString('zh-CN', { hour12: false }),
    tone: classifyLogLine(raw),
    message: raw.replace(/^(?:info|warn|error|success):\s*/i, '')
  })
  resetLogs.value = resetLogs.value.slice(-100)
}

async function handleReset(container) {
  const name = (container?.Names?.[0] || '').replace(/^\//, '') || container?.Id?.slice(0, 12) || '容器'
  const confirmed = await uiStore.confirm({
    type: 'warning',
    title: '重置容器',
    message: `将保留 "${name}" 的配置和挂载，使用本地 ${formatDockerImageReference(container?.Image) || '当前镜像'} 标签重新创建容器。运行连接会短暂中断。`,
    confirmText: '重置'
  })
  if (!confirmed) return

  resetStreamStop?.()
  resetStreamStop = null
  resetLogSequence = 0
  resetLogs.value = []
  resetTargetName.value = name
  resetTargetImage.value = formatDockerImageReference(container?.Image) || '<none>'
  resettingContainer.value = true
  showResetDialog.value = true

  resetStreamStop = containers.recreateStream(container.Id, {
    pull: false,
    onLog: appendResetLog,
    onComplete: async () => {
      resettingContainer.value = false
      resetStreamStop = null
      uiStore.toastSuccess(`容器 ${name} 已使用最新本地镜像重建`)
      await fetchContainers()
    },
    onError: async (error) => {
      resettingContainer.value = false
      resetStreamStop = null
      if (!resetLogs.value.some(line => line.tone === 'error')) {
        appendResetLog(`error: ${error.message || '重置失败'}`)
      }
      uiStore.toastError('重置失败: ' + (error.message || '未知错误'))
      await fetchContainers()
    }
  })
}

function handleResetFromMenu(container) {
  closeRowActionMenu()
  handleReset(container)
}

async function handleUpdate(container) {
  if (isContainerUpdating(container)) {
    uiStore.toastWarning('该容器已有更新任务正在运行')
    return
  }
  const name = getDisplayContainerName(container)
  const confirmed = await uiStore.confirm({
    type: 'warning',
    title: '更新容器',
    message: `将拉取 "${formatDockerImageReference(container?.Image) || '当前镜像'}" 的最新版本，并保留 "${name}" 的配置和挂载重新创建容器。`,
    confirmText: '后台更新'
  })
  if (!confirmed) return

  try {
    const result = await containers.startUpdateTask(container.Id, { pull: true })
    if (!result?.taskId) throw new Error('未获取到任务 ID')
    setContainerUpdating(container.Id, true)
    uiStore.toastInfo(`容器 ${name} 已转入后台更新`)
    pollContainerUpdateTask(result.taskId, name, container.Id)
  } catch (error) {
    setContainerUpdating(container.Id, false)
    uiStore.toastError('更新容器失败: ' + (error.message || '未知错误'))
  }
}

function handleUpdateFromMenu(container) {
  closeRowActionMenu()
  handleUpdate(container)
}

async function pollContainerUpdateTask(taskId, name, containerId) {
  const normalizedTaskId = String(taskId || '')
  if (!normalizedTaskId) return
  try {
    const task = await containers.getUpdateTask(normalizedTaskId)
    const status = String(task?.status || '').toLowerCase()
    if (['success', 'completed'].includes(status)) {
      containerUpdatePollTimers.delete(normalizedTaskId)
      setContainerUpdating(containerId, false)
      uiStore.toastSuccess?.(`容器 ${name} 更新完成`)
      await fetchContainers()
      return
    }
    if (['error', 'failed'].includes(status)) {
      containerUpdatePollTimers.delete(normalizedTaskId)
      setContainerUpdating(containerId, false)
      uiStore.toastError(`容器 ${name} 更新失败${task?.error ? `: ${task.error}` : ''}`)
      await fetchContainers()
      return
    }
  } catch (error) {
    console.error('读取容器更新任务失败:', error)
  }

  const timer = window.setTimeout(() => {
    containerUpdatePollTimers.delete(normalizedTaskId)
    void pollContainerUpdateTask(normalizedTaskId, name, containerId)
  }, 2000)
  containerUpdatePollTimers.set(normalizedTaskId, timer)
}

function isContainerUpdating(container) {
  return updatingContainerIds.value.has(String(container?.Id || ''))
}

function setContainerUpdating(containerId, updating) {
  const next = new Set(updatingContainerIds.value)
  const key = String(containerId || '')
  if (!key) return
  if (updating) next.add(key)
  else next.delete(key)
  updatingContainerIds.value = next
}

function handleRemoveFromMenu(container) {
  closeRowActionMenu()
  handleRemove(container)
}

function handleForceStopFromMenu(container) {
  closeRowActionMenu()
  handleForceStop(container)
}

// 点击外部关闭下拉菜单
function handleClickOutside(e) {
  const dropdown = document.querySelector('.dropdown-wrapper')
  if (dropdown && !dropdown.contains(e.target)) {
    showMoreMenu.value = false
  }
}

usePageAction({ create: openCreateDialog })

onMounted(() => {
  fetchContainers()
  startResourceStatsRefresh()
  document.addEventListener('click', handleClickOutside)
})

watch(isRemoteEnvironment, startResourceStatsRefresh)

onUnmounted(() => {
  stopResourceStatsRefresh()
  resetStreamStop?.()
  for (const timer of containerUpdatePollTimers.values()) {
    window.clearTimeout(timer)
  }
  containerUpdatePollTimers.clear()
  updatingContainerIds.value = new Set()
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.container-batch-check {
  width: 15px;
  height: 15px;
  cursor: pointer;
  accent-color: var(--color-primary-600, #2563eb);
}

.containers-hero {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  max-width: 1680px;
  width: 100%;
  margin: 0 auto;
}

.hero-copy {
  min-width: 0;
}

.eyebrow {
  margin: 0 0 4px;
  color: var(--ops-muted);
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.hero-copy h1 {
  margin: 0;
  color: var(--ops-ink);
  font-size: clamp(1.65rem, 1.75vw, 2.25rem);
  line-height: 1.12;
  font-weight: 800;
  letter-spacing: 0;
}

.hero-copy p:last-child {
  margin: 6px 0 0;
  color: var(--ops-muted);
  font-size: 0.95rem;
}

.hero-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.resource-rail {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 10px;
  max-width: 1680px;
  width: 100%;
  margin: 0 auto;
}

.rail-card {
  display: grid;
  grid-template-columns: 38px 1fr;
  grid-template-areas:
    "icon copy"
    "icon note";
  align-items: center;
  gap: 2px 10px;
  min-width: 0;
  padding: 12px;
  border: 1px solid var(--ops-line);
  border-radius: 12px;
  background: color-mix(in srgb, var(--ops-panel) 78%, transparent);
  box-shadow: 0 18px 48px color-mix(in srgb, var(--ops-ink) 7%, transparent);
  backdrop-filter: blur(12px);
}

.rail-icon {
  grid-area: icon;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  border-radius: 10px;
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

.rail-card.muted .rail-icon {
  color: var(--ops-muted);
  background: var(--ops-muted-soft);
}

.rail-card.info .rail-icon {
  color: var(--ops-cyan);
  background: var(--ops-cyan-soft);
}

.rail-copy {
  grid-area: copy;
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}

.rail-copy strong {
  color: var(--ops-ink);
  font-size: 1.35rem;
  line-height: 1;
  font-weight: 800;
}

.rail-copy span,
.rail-card small {
  overflow: hidden;
  color: var(--ops-muted);
  font-size: 0.75rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rail-card small {
  grid-area: note;
  color: var(--ops-muted);
}

.workbench-toolbar {
  max-width: 1680px;
  width: 100%;
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
  grid-template-columns: minmax(220px, 1fr) 190px minmax(240px, 1.15fr);
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
  margin-left: auto;
}

.sort-control {
  width: 190px;
  box-sizing: border-box;
  min-height: 38px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 8px 0 10px;
  border: 1px solid var(--ops-line);
  border-radius: 10px;
  background: color-mix(in srgb, var(--ops-panel) 82%, transparent);
  color: var(--ops-muted);
}

.sort-control select {
  flex: 1;
  width: 0;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--ops-ink);
  font: inherit;
  font-size: 0.8125rem;
  font-weight: 700;
  cursor: pointer;
}

.sort-control select option {
  background: var(--ops-panel);
  color: var(--ops-ink);
}

.sort-direction-btn {
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 7px;
  background: var(--ops-muted-soft);
  color: var(--ops-muted);
  cursor: pointer;
}

.sort-direction-btn:hover {
  color: var(--ops-green-strong);
  background: var(--ops-green-soft);
}

.sort-direction-btn.is-placeholder {
  visibility: hidden;
  pointer-events: none;
}

.icon-btn,
.table-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid transparent;
  cursor: pointer;
  transition: background var(--motion-duration-quick) var(--motion-ease-out), border-color var(--motion-duration-quick) var(--motion-ease-out), color var(--motion-duration-quick) var(--motion-ease-out), transform var(--motion-duration-quick) var(--motion-ease-out);
}

.icon-btn {
  width: 38px;
  height: 38px;
  border-radius: 10px;
  border-color: var(--border-subtle);
  background: var(--bg-elevated);
  color: var(--text-secondary);
}

.icon-btn:hover,
.icon-btn.is-active {
  color: var(--text-primary);
  border-color: var(--border-default);
  background: var(--bg-secondary);
}

.icon-btn.is-spinning svg {
  animation: resource-refresh-spin 0.75s linear infinite;
}

.dropdown-wrapper {
  position: relative;
}

.dropdown-menu {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  z-index: 100;
  min-width: 170px;
  padding: 6px;
  border: 1px solid var(--ops-line);
  border-radius: 10px;
  background: var(--ops-panel);
  box-shadow: var(--shadow-lg);
}

.dropdown-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 12px;
  border-radius: 8px;
  color: var(--ops-muted);
  cursor: pointer;
  font-size: 0.8125rem;
  white-space: nowrap;
}

.dropdown-item:hover {
  color: var(--ops-ink);
  background: color-mix(in srgb, var(--ops-muted) 12%, transparent);
}

.dropdown-item.danger {
  color: var(--color-danger-600);
}

.dropdown-item.danger:hover {
  background: var(--color-danger-100);
}

.content-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding-bottom: 92px;
}

.resource-board {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(280px, 300px);
  gap: 12px;
  max-width: 1680px;
  width: 100%;
  margin: 0 auto;
  align-items: stretch;
}

.resource-list-card,
.resource-context-panel {
  min-width: 0;
  border: 1px solid var(--ops-line);
  border-radius: 14px;
  background: color-mix(in srgb, var(--ops-surface) 90%, transparent);
  box-shadow: 0 18px 48px color-mix(in srgb, var(--ops-ink) 7%, transparent);
  backdrop-filter: blur(14px);
}

.resource-list-card {
  overflow: hidden;
  min-height: 560px;
}

.resource-list-card :deep(.card-grid-wrapper) {
  height: 100%;
  padding: 16px;
}

.resource-list-card :deep(.card-grid) {
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 16px;
  padding: 0;
}

.resource-list-card :deep(.container-card) {
  min-height: 236px;
  aspect-ratio: auto;
  border-color: var(--ops-line);
  border-radius: 12px;
}

.resource-list-card :deep(.container-card.is-selected) {
  border-color: color-mix(in srgb, var(--ops-green) 62%, var(--ops-line));
  box-shadow:
    inset 4px 0 0 var(--ops-green),
    0 18px 42px color-mix(in srgb, var(--ops-green) 14%, transparent);
  animation: resource-card-select var(--motion-duration-fast) var(--motion-ease-out);
}

.resource-list-card :deep(.container-info) {
  gap: 8px;
}

.resource-list-card :deep(.card-actions) {
  display: grid;
  grid-template-columns: repeat(6, minmax(28px, 1fr));
  gap: 6px;
}

.resource-list-card :deep(.action-btn) {
  width: 100%;
  min-width: 0;
}

.list-view {
  overflow-x: hidden;
  overflow-y: hidden;
}

.data-table thead {
  position: sticky;
  top: 0;
  z-index: 1;
}

.list-loading {
  padding: 16px;
}

.list-skeleton {
  display: flex;
  gap: 16px;
  padding: 14px 0;
  border-bottom: 1px solid var(--ops-line);
}

.skeleton-cell {
  height: 16px;
  border-radius: 4px;
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}

.list-empty {
  padding: 48px 0;
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
  grid-template-columns: var(--resource-grid-template, minmax(165px, 1.3fr) minmax(130px, 0.95fr) 92px minmax(145px, 1fr) minmax(72px, 0.55fr) 154px);
  align-items: center;
  gap: 8px;
}

.table-head {
  position: sticky;
  top: 0;
  z-index: 2;
  min-height: 42px;
  padding: 0 8px;
  border-bottom: 1px solid var(--ops-line);
  background: rgba(238, 243, 247, 0.96);
}

.table-head > span,
.table-head > button {
  position: relative;
  min-width: 0;
  min-height: 100%;
  display: inline-flex;
  align-items: center;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--ops-muted);
  font-size: 0.72rem;
  font-weight: 800;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  text-align: left;
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
  background: var(--ops-strong-line);
}

.table-head .sortable {
  cursor: pointer;
  user-select: none;
}

.table-head .sortable:hover {
  color: var(--ops-ink);
  background: transparent;
}

.sort-icon {
  display: inline-flex;
  align-items: center;
  margin-left: 4px;
  color: var(--text-tertiary);
  vertical-align: middle;
}

.sort-icon.asc,
.sort-icon.desc {
  color: var(--color-primary-500);
}

.resource-row > [role="cell"] {
  min-height: 68px;
  display: flex;
  align-items: center;
  color: var(--ops-ink);
  min-width: 0;
  overflow: hidden;
}

.resource-row .status-badge {
  flex: 0 0 auto;
  min-width: 64px;
}

.resource-row {
  width: 100%;
  min-height: 68px;
  padding: 0 8px;
  border: 0;
  border-bottom: 1px solid var(--ops-line);
  background: transparent;
  color: var(--ops-ink);
  text-align: left;
  cursor: pointer;
  outline: none;
  transform-origin: center;
  transition:
    transform var(--motion-duration-quick) var(--motion-ease-out),
    background var(--motion-duration-quick) var(--motion-ease-out),
    box-shadow var(--motion-duration-quick) var(--motion-ease-out);
  box-sizing: border-box;
}

.resource-row:hover {
  background: var(--ops-row-hover);
}

.resource-row:active {
  background: var(--ops-row-active);
  transform: scale(0.992);
}

.resource-row.selected {
  animation: resource-row-select var(--motion-duration-fast) var(--motion-ease-out);
  background: var(--ops-row-hover);
  box-shadow: inset 4px 0 0 var(--ops-green);
}

.resource-row.selected .resource-kind {
  animation: resource-kind-pop var(--motion-duration-fast) var(--motion-ease-out);
}

@keyframes resource-row-compress {
  0% {
    transform: none;
  }
  42% {
    transform: none;
  }
  100% {
    transform: none;
  }
}

@keyframes resource-row-select {
  0% {
    transform: scale(1);
    background: var(--ops-row-hover);
  }
  44% {
    transform: scale(0.988);
    background: var(--ops-row-hover);
  }
  100% {
    transform: scale(1);
    background: var(--ops-row-hover);
  }
}

@keyframes resource-kind-pop {
  0% {
    transform: scale(0.94);
  }
  60% {
    transform: scale(1.04);
  }
  100% {
    transform: scale(1);
  }
}

@keyframes resource-card-select {
  0% {
    transform: translateY(0) scale(0.985);
  }
  70% {
    transform: translateY(0) scale(1.006);
  }
  100% {
    transform: translateY(0) scale(1);
  }
}

@media (prefers-reduced-motion: reduce) {
  .resource-row.selected,
  .resource-row.selected .resource-kind,
  .resource-list-card :deep(.container-card.is-selected) {
    animation: none;
  }
}

.resource-row:focus-visible {
  background: var(--ops-row-focus);
  outline: 2px solid color-mix(in srgb, var(--ops-green) 36%, transparent);
  outline-offset: -2px;
}

.resource-row.state-stopped,
.resource-row.state-created {
  color: var(--ops-muted);
}

.name-wrapper {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.container-update-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex: 0 0 auto;
  padding: 3px 7px;
  border: 1px solid color-mix(in srgb, var(--ops-amber) 40%, var(--ops-line));
  border-radius: 999px;
  background: var(--ops-amber-soft);
  color: var(--ops-amber);
  font-size: 0.6875rem;
  font-weight: 700;
  line-height: 1;
}

.resource-kind {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 34px;
  height: 34px;
  border-radius: 10px;
  color: var(--ops-green-strong);
  background: var(--ops-green-soft);
}

.name-stack {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.name-stack strong,
.name-stack small,
.image-cell {
  position: relative;
  cursor: pointer;
}

.ports-cell {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.name-stack strong {
  color: var(--ops-ink);
  font-size: 0.875rem;
  font-weight: 600;
}

.name-stack small {
  color: var(--ops-muted);
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.6875rem;
}

.cell-mono {
  color: var(--ops-muted);
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
  color: var(--text-secondary);
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.6875rem;
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
  overflow: hidden;
  min-width: 0;
  border-left: 2px solid var(--color-info-500);
  border-radius: 3px;
  background: color-mix(in srgb, var(--bg-tertiary) 72%, transparent);
  white-space: nowrap;
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

.cell-actions {
  white-space: nowrap;
}

.action-buttons {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 6px;
  min-height: 44px;
}

.compact-actions {
  width: max-content;
  max-width: 100%;
}

.table-btn {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
}

.table-btn:hover:not(:disabled) {
  color: var(--text-primary);
  background: var(--bg-secondary);
}

.table-btn.is-active {
  color: var(--ops-ink);
  border-color: var(--ops-strong-line);
  background: color-mix(in srgb, var(--ops-panel) 70%, var(--ops-green) 10%);
}

.table-btn.success {
  color: var(--color-success-600);
  background: var(--color-success-100);
}

.table-btn.success:hover:not(:disabled) {
  color: var(--color-success-700);
  background: var(--color-success-200);
}

.table-btn.stop,
.table-btn.danger {
  color: var(--color-danger-600);
  background: var(--color-danger-100);
}

.table-btn.stop:hover:not(:disabled),
.table-btn.danger:hover:not(:disabled) {
  color: var(--color-danger-700);
  background: var(--color-danger-200);
}

.table-btn.warning {
  color: var(--color-warning-700);
  background: var(--color-warning-100);
}

.table-btn.warning:hover:not(:disabled) {
  color: var(--color-warning-700);
  background: var(--color-warning-200);
}

.table-btn.info {
  color: var(--color-primary-600);
  background: var(--color-primary-100);
}

.table-btn.info:hover:not(:disabled) {
  color: var(--color-primary-700);
  background: var(--color-primary-200);
}

.resource-context-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 16px;
  position: sticky;
  top: 0;
  align-self: start;
  min-height: 560px;
  height: 100%;
}

.context-header {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.context-header div {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.context-header strong {
  overflow: hidden;
  color: var(--ops-ink);
  font-size: 1.05rem;
  line-height: 1.1;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.context-header span {
  color: var(--ops-muted);
  font-size: 0.75rem;
}

.context-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.context-grid div {
  display: grid;
  gap: 4px;
  padding: 12px;
  border: 1px solid var(--ops-line);
  border-radius: 10px;
  background: color-mix(in srgb, var(--ops-panel) 72%, transparent);
}

.context-grid span,
.section-label {
  color: var(--ops-muted);
  font-size: 0.71875rem;
  font-weight: 800;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.context-grid strong {
  overflow: hidden;
  color: var(--ops-ink);
  font-size: 1rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.context-section {
  display: grid;
  gap: 8px;
}

.section-label {
  margin: 0;
}

.view-summary {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.view-summary span {
  padding: 5px 8px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--ops-muted) 13%, transparent);
  color: var(--ops-muted);
  font-size: 0.75rem;
}

.view-summary span.active {
  color: var(--ops-green-strong);
  background: var(--ops-green-soft);
  font-weight: 800;
}

.detail-lines {
  display: grid;
  gap: 6px;
}

.detail-lines span {
  display: grid;
  grid-template-columns: 48px minmax(0, 1fr);
  gap: 8px;
  min-height: 28px;
  align-items: center;
  align-content: center;
  line-height: 1.5;
  padding: 0 9px;
  border: 1px solid var(--ops-line);
  border-radius: 8px;
  background: color-mix(in srgb, var(--ops-panel) 76%, transparent);
  color: var(--ops-ink);
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-size: 0.75rem;
}

.detail-lines b {
  align-self: center;
  color: var(--ops-muted);
  font-size: 0.6875rem;
}

.log-stream-preview {
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  max-width: 100%;
  flex: 1 1 auto;
  min-height: 156px;
  padding: 12px;
  border: 1px solid var(--ops-line);
  border-radius: 12px;
  background:
    linear-gradient(90deg, color-mix(in srgb, var(--ops-ink) 5%, transparent) 1px, transparent 1px),
    linear-gradient(180deg, color-mix(in srgb, var(--ops-ink) 5%, transparent) 1px, transparent 1px),
    color-mix(in srgb, var(--ops-panel) 78%, transparent);
  background-size: 18px 18px;
  overflow: hidden;
}

.reset-container-summary {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) max-content;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
  padding: 10px 12px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
}

.reset-container-summary > div {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.reset-container-summary strong,
.reset-container-summary span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.reset-container-summary strong {
  color: var(--text-primary);
  font-size: 0.8125rem;
}

.reset-container-summary > div > span {
  color: var(--text-tertiary);
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 0.6875rem;
}

.reset-mode {
  padding: 3px 7px;
  color: var(--color-info-700);
  background: color-mix(in srgb, var(--color-info-500) 10%, var(--bg-elevated));
  border: 1px solid color-mix(in srgb, var(--color-info-500) 24%, var(--border-subtle));
  border-radius: 5px;
  font-size: 0.6875rem;
  font-weight: 600;
}

.reset-log {
  min-height: 220px;
  max-height: 340px;
  padding: 10px 12px;
  overflow-y: auto;
  background: var(--bg-primary);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 0.75rem;
}

.reset-log-line {
  display: grid;
  grid-template-columns: 68px minmax(0, 1fr);
  gap: 10px;
  padding: 4px 7px;
  border-left: 2px solid transparent;
  line-height: 1.45;
}

.reset-log-line time {
  color: var(--text-tertiary);
  font-variant-numeric: tabular-nums;
}

.reset-log-line span {
  min-width: 0;
  color: var(--text-secondary);
  overflow-wrap: anywhere;
}

.reset-log-line.is-error { border-left-color: var(--color-danger-500); }
.reset-log-line.is-warning { border-left-color: var(--color-warning-500); }
.reset-log-line.is-success { border-left-color: var(--color-success-500); }
.reset-log-line.is-info { border-left-color: var(--color-info-500); }

.reset-log-empty {
  display: grid;
  min-height: 196px;
  place-items: center;
  color: var(--text-tertiary);
}

.pagination-wrapper {
  position: fixed;
  right: 0;
  bottom: 0;
  left: 260px;
  z-index: 100;
  padding: 12px 24px;
  border-top: 1px solid var(--border-subtle);
  background: var(--bg-primary);
}

/* 紧凑资源页头：全局 Header 已经承担页面定位 */
.resource-rail {
  gap: 8px;
}

.workbench-toolbar {
  padding: 12px;
  border: 1px solid var(--ops-line);
  border-radius: 12px;
  background: color-mix(in srgb, var(--ops-surface) 90%, transparent);
  box-shadow: 0 18px 48px color-mix(in srgb, var(--ops-ink) 7%, transparent);
}

.rail-card {
  min-height: 64px;
  padding: 10px 12px;
}

.rail-icon {
  width: 34px;
  height: 34px;
  border-radius: 9px;
}

.rail-copy strong {
  font-size: 1.18rem;
}

.rail-copy span,
.rail-card small {
  font-size: 0.7rem;
}

@media (max-width: 1280px) {
  .resource-rail {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .resource-board {
    grid-template-columns: 1fr;
  }

  .resource-context-panel {
    position: static;
    grid-template-columns: 1fr 1fr;
  }

  .context-section {
    grid-column: 1 / -1;
  }
}

@media (max-width: 900px) {
  .containers-page {
    padding: 16px 16px 0;
  }

  .containers-hero,
  .hero-actions {
    align-items: stretch;
    flex-direction: column;
  }

  .hero-actions {
    flex-direction: row;
    justify-content: flex-start;
  }

  .resource-rail,
  .toolbar-left,
  .resource-context-panel {
    grid-template-columns: 1fr;
  }

  .pagination-wrapper {
    left: 0 !important;
  }
}
</style>
