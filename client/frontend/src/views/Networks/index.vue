<template>
  <ResourceWorkbench class="networks-page networks-workbench" role="main" aria-label="网络管理" remote-context>
    <template #rail>
      <section class="resource-rail" aria-label="网络概览">
        <article v-for="item in networkRail" :key="item.label" class="rail-card" :class="item.tone">
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
      <div class="toolbar workbench-toolbar">
      <div class="toolbar-left workbench-toolbar-left">
        <SearchInput
          v-model="searchQuery"
          placeholder="搜索网络名称..."
          @search="handleSearch"
        />
        <div class="sort-control">
          <DynamicIcon name="arrow-up-down" :size="15" />
          <select v-model="sortField" aria-label="网络排序字段">
            <option value="">默认排序</option>
            <option value="name">按名称</option>
            <option value="driver">按类型</option>
            <option value="subnet">按子网</option>
            <option value="gateway">按网关</option>
            <option value="containers">按容器数</option>
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
          v-model="networkFilters.usage"
          :options="usageFilterOptions"
          aria-label="使用筛选"
          compact
        />
      </div>
      <div class="toolbar-right">
        <button
          v-if="viewMode !== 'grid' && checkedNetworks.length > 0"
          v-ripple
          data-remote-write
          class="secondary-btn batch-danger"
          type="button"
          :disabled="batchWorking"
          title="批量删除已勾选的网络"
          @click="batchRemoveNetworks"
        >
          <DynamicIcon name="trash-2" :size="16" />
          批量删除（{{ checkedNetworks.length }}）
        </button>
        <ViewToggle v-model="viewMode" />
        <button
          v-ripple
          class="icon-btn resource-refresh-btn"
          :class="{ 'is-spinning': networksBusy }"
          :disabled="networksBusy"
          title="刷新网络列表"
          @click="refreshNetworks"
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
          <!-- 更多操作下拉菜单 -->
          <div v-if="showMoreMenu" class="dropdown-menu">
            <div class="dropdown-item danger" @click="pruneNetworks">
              <DynamicIcon name="trash-2" :size="16" />
              清理无用网络
            </div>
          </div>
        </div>
        <button class="primary-btn" data-remote-write @click="openCreateDialog">
          <DynamicIcon name="plus" :size="16" />
          新建网络
        </button>
      </div>
      </div>
    </template>

    <ResourceBoard>
      <ResourcePanel>
        <CardGrid
          v-if="viewMode === 'grid'"
          :items="paginatedNetworks"
          :loading="loading"
          :skeleton-count="12"
          :empty-props="emptyProps"
        >
          <template #default="{ items }">
            <NetworkCard
              v-for="network in items"
              :key="network.Id"
              :network="network"
              :selected="isSelectedNetwork(network)"
              @select="selectNetwork(network)"
              @edit="handleEdit(network)"
              @remove="handleRemove(network)"
            />
          </template>
        </CardGrid>

        <ResourceTable
          v-else
          :columns="networkColumns"
          :rows="paginatedNetworks"
          table-key="networks-v2"
          row-key="Id"
          :selected-key="selectedNetworkId"
          :loading="loading"
          aria-label="网络列表"
          @select="selectNetwork"
          @sort="handleSort"
        >
          <template #header-check>
            <input
              class="network-batch-check"
              type="checkbox"
              :checked="allVisibleChecked"
              :indeterminate.prop="partiallyChecked"
              :disabled="paginatedNetworks.length === 0"
              title="全选/取消当前列表全部网络"
              @click.stop
              @change="toggleNetworkCheckAll"
            />
          </template>

          <template #cell-check="{ row: network }">
            <input
              class="network-batch-check"
              type="checkbox"
              :checked="checkedNetworkIds.includes(network.Id)"
              title="勾选后可批量删除"
              @click.stop
              @change="toggleNetworkCheck(network.Id)"
            />
          </template>

          <template v-for="column in networkSortableColumns" :key="column.key" #[`header-${column.key}`]>
            {{ column.label }}
            <span class="sort-icon" :class="getSortIcon(column.key)">
              <DynamicIcon v-if="getSortIcon(column.key) === 'asc'" name="arrow-up" :size="14" />
              <DynamicIcon v-else-if="getSortIcon(column.key) === 'desc'" name="arrow-down" :size="14" />
              <DynamicIcon v-else name="arrow-up-down" :size="14" />
            </span>
          </template>

          <template #cell-name="{ row: network }">
            <button class="resource-name-button" type="button" @click.stop="selectNetwork(network)">
              <span class="resource-kind network">
                <DynamicIcon class="network-icon-small" name="network" :size="16" />
              </span>
              <span class="resource-name-copy">
                <span class="network-name-line">
                  <strong>{{ network.Name }}</strong>
                  <StatusBadge
                    v-if="isSystemNetwork(network.Name)"
                    class="network-system-badge"
                    status="system"
                    variant="minimal"
                  />
                </span>
                <small>{{ getNetworkDriver(network) }} · {{ getSubnet(network) || 'no subnet' }}</small>
              </span>
            </button>
          </template>
          <template #cell-driver="{ row: network }">{{ getNetworkDriver(network) }}</template>
          <template #cell-subnet="{ row: network }">
            <span class="cell-ellipsis" :title="getSubnet(network) || '-'">{{ getSubnet(network) || '-' }}</span>
          </template>
          <template #cell-gateway="{ row: network }">
            <span class="cell-ellipsis" :title="getGateway(network) || '-'">{{ getGateway(network) || '-' }}</span>
          </template>
          <template #cell-containers="{ row: network }">{{ getContainerCount(network) }} 个</template>
          <template #cell-actions="{ row: network }">
            <div class="cell-actions" data-remote-write>
              <button
                v-if="!isSystemNetwork(network.Name)"
                class="table-btn info"
                title="编辑"
                @click.stop="handleEdit(network)"
              >
                <DynamicIcon name="edit" :size="14" />
              </button>
              <button
                v-if="!isSystemNetwork(network.Name)"
                class="table-btn danger"
                :disabled="getContainerCount(network) > 0"
                :title="getContainerCount(network) > 0 ? '有容器使用，无法删除' : '删除'"
                @click.stop="handleRemove(network)"
              >
                <DynamicIcon name="trash-2" :size="14" />
              </button>
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
            :total="filteredNetworks.length"
          />
        </template>
      </ResourcePanel>

      <template #context>
        <ResourceContextPanel :motion-key="selectedSummaryNetwork?.Id || 'overview'" aria-label="网络上下文">
      <div class="context-header">
        <span class="resource-kind network">
          <DynamicIcon name="network" :size="18" />
        </span>
        <div>
          <strong>{{ selectedSummaryNetwork ? selectedSummaryNetwork.Name : '网络概览' }}</strong>
          <span v-if="selectedSummaryNetwork">{{ getNetworkDriver(selectedSummaryNetwork) }}</span>
          <span v-else>{{ filteredNetworks.length }} 个匹配网络</span>
        </div>
      </div>

      <div v-if="selectedSummaryNetwork" class="context-actions" data-remote-write>
        <button
          type="button"
          class="detail-action info"
          :disabled="isSystemNetwork(selectedSummaryNetwork.Name)"
          @click="handleEdit(selectedSummaryNetwork)"
        >
          <DynamicIcon name="edit" :size="15" />
          编辑
        </button>
        <button
          type="button"
          class="detail-action danger"
          :disabled="isSystemNetwork(selectedSummaryNetwork.Name) || getContainerCount(selectedSummaryNetwork) > 0"
          @click="handleRemove(selectedSummaryNetwork)"
        >
          <DynamicIcon name="trash-2" :size="15" />
          删除
        </button>
      </div>

      <div class="context-grid">
        <div>
          <span>驱动</span>
          <strong>{{ selectedSummaryNetwork ? getNetworkDriver(selectedSummaryNetwork) : 'bridge' }}</strong>
        </div>
        <div>
          <span>容器</span>
          <strong>{{ selectedSummaryNetwork ? getContainerCount(selectedSummaryNetwork) : networkContextCounts.attached }}</strong>
        </div>
        <div>
          <span>类型</span>
          <strong>{{ selectedSummaryNetwork ? (isSystemNetwork(selectedSummaryNetwork.Name) ? '系统' : '自定义') : '全部' }}</strong>
        </div>
        <div>
          <span>IPv6</span>
          <strong>{{ selectedSummaryNetwork?.EnableIPv6 ? '启用' : '-' }}</strong>
        </div>
      </div>

      <div v-if="selectedSummaryNetwork" class="context-section selected-detail">
        <p class="section-label">地址 / 网关</p>
        <div class="detail-lines">
          <span>
            <b>SUBNET</b>
            {{ getSubnet(selectedSummaryNetwork) || '-' }}
          </span>
          <span>
            <b>GATEWAY</b>
            {{ getGateway(selectedSummaryNetwork) || '-' }}
          </span>
          <span>
            <b>SCOPE</b>
            {{ selectedSummaryNetwork.Scope || 'local' }}
          </span>
        </div>
      </div>

      <ResourceRelationList
        title="连接关系"
        :items="networkContainerRelations"
        clickable
        empty-text="无关联容器"
        @select="openContainerRelation"
      />
        </ResourceContextPanel>
      </template>
    </ResourceBoard>

    <!-- 创建/编辑网络对话框 -->
    <Modal v-model:visible="showEditDialog" :title="isEditMode ? '编辑网络' : '新建网络'" width="500px">
      <div class="form-group">
        <label class="form-label">网络名称 <span class="required">*</span></label>
        <input v-model="editForm.name" class="form-input" placeholder="例如: my-network" :disabled="isEditMode" />
      </div>
      <div class="form-group">
        <label class="form-label">驱动模式</label>
        <select v-model="editForm.driver" class="form-select" :disabled="isEditMode">
          <option value="bridge">bridge (桥接)</option>
          <option value="host">host (主机)</option>
          <option value="none">none (无网络)</option>
          <option value="macvlan">macvlan</option>
        </select>
      </div>
      <div v-if="editForm.driver === 'bridge' || editForm.driver === 'macvlan'" class="form-group">
        <label class="form-label">IPv4 子网</label>
        <input v-model="editForm.ipv4Subnet" class="form-input" placeholder="例如: 192.168.1.0/24" />
      </div>
      <div v-if="editForm.driver === 'bridge' || editForm.driver === 'macvlan'" class="form-group">
        <label class="form-label">IPv4 网关</label>
        <input v-model="editForm.ipv4Gateway" class="form-input" placeholder="例如: 192.168.1.1" />
      </div>
      <div v-if="editForm.driver === 'bridge' || editForm.driver === 'macvlan'" class="form-group">
        <label class="form-label">IPv4 IP Range</label>
        <input v-model="editForm.ipv4IpRange" class="form-input" placeholder="可选，例如: 192.168.1.128/25" />
      </div>
      <div v-if="editForm.driver === 'macvlan'" class="form-group">
        <label class="form-label">父接口</label>
        <input v-model="editForm.parent" class="form-input" placeholder="例如: eth0" />
      </div>
      <div v-if="editForm.driver === 'bridge' || editForm.driver === 'macvlan'" class="form-group">
        <label class="checkbox-label">
          <input v-model="editForm.enableIPv6" type="checkbox" />
          <span>启用 IPv6</span>
        </label>
      </div>
      <template v-if="editForm.enableIPv6 && (editForm.driver === 'bridge' || editForm.driver === 'macvlan')">
        <div class="form-group">
          <label class="form-label">IPv6 子网</label>
          <input v-model="editForm.ipv6Subnet" class="form-input" placeholder="例如: fd00:1::/64" />
        </div>
        <div class="form-group">
          <label class="form-label">IPv6 网关</label>
          <input v-model="editForm.ipv6Gateway" class="form-input" placeholder="例如: fd00:1::1" />
        </div>
        <div class="form-group">
          <label class="form-label">IPv6 IP Range</label>
          <input v-model="editForm.ipv6IpRange" class="form-input" placeholder="可选，例如: fd00:1::/80" />
        </div>
      </template>
      <div class="form-group form-options-row">
        <label class="checkbox-label">
          <input v-model="editForm.internal" type="checkbox" />
          <span>内部网络</span>
        </label>
        <label class="checkbox-label">
          <input v-model="editForm.attachable" type="checkbox" />
          <span>Attachable</span>
        </label>
      </div>
      <div class="form-group">
        <label class="form-label">Options</label>
        <textarea v-model="editForm.optionsText" class="form-textarea" rows="3" placeholder="每行一个 key=value"></textarea>
      </div>
      <div class="form-group">
        <label class="form-label">Labels</label>
        <textarea v-model="editForm.labelsText" class="form-textarea" rows="3" placeholder="每行一个 key=value"></textarea>
      </div>
      <template #footer>
        <button class="btn btn-default" @click="showEditDialog = false">取消</button>
        <button class="btn btn-primary" :disabled="!editForm.name" @click="saveNetwork">
          {{ isEditMode ? '保存' : '创建' }}
        </button>
      </template>
    </Modal>

  </ResourceWorkbench>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useDockerResourcesStore } from '@/stores/dockerResources.js'
import { useUiStore } from '@/stores/ui.js'
import { networks } from '@edition/api'
import { useSort } from '@/composables/useSort.js'
import { useViewMode } from '@/composables/useViewMode.js'

// 组件
import SearchInput from '@/components/ui/SearchInput.vue'
import ViewToggle from '@/components/ui/ViewToggle.vue'
import CardGrid from '@/components/data-display/CardGrid.vue'
import NetworkCard from '@/components/data-display/NetworkCard.vue'
import Pagination from '@/components/ui/Pagination.vue'
import Modal from '@/components/feedback/Modal.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import SegmentedTabs from '@/components/ui/SegmentedTabs.vue'
import ResourceWorkbench from '@/components/resource-workbench/ResourceWorkbench.vue'
import ResourceBoard from '@/components/resource-workbench/ResourceBoard.vue'
import ResourcePanel from '@/components/resource-workbench/ResourcePanel.vue'
import ResourceContextPanel from '@/components/resource-workbench/ResourceContextPanel.vue'
import ResourceTable from '@/components/resource-workbench/ResourceTable.vue'
import ResourceRelationList from '@/components/resource-workbench/ResourceRelationList.vue'
import { usePageAction } from '@/composables/usePageAction.js'
import { matchesBooleanFilter } from '@/utils/resourceFilters.js'
import { notifyBatchResult } from '@/utils/batchFeedback.js'

const dockerResources = useDockerResourcesStore()
const uiStore = useUiStore()
const router = useRouter()

// 系统网络列表（排序优先级：host > bridge > none）
const systemNetworks = ['host', 'bridge', 'none']

// 状态
const loading = computed(() => {
  const resource = dockerResources.resources.networks
  return resource.loading || resource.refreshing
})
const networksBusy = computed(() => loading.value)
const networksList = computed(() => dockerResources.networkList)
const searchQuery = ref('')
const networkFilters = ref({ usage: 'all' })
const usageFilterOptions = [
  { value: 'all', label: '全部' },
  { value: 'yes', label: '已使用' },
  { value: 'no', label: '未使用' }
]
const { viewMode } = useViewMode('networks_view_mode', 'table')
const networkColumns = [
  { key: 'check', label: '', cellClass: 'cell-check', width: 40, minWidth: 36 },
  { key: 'name', label: '名称', sortable: true, cellClass: 'cell-name', width: 210, minWidth: 150, flex: true },
  { key: 'driver', label: '类型', sortable: true, width: 76, minWidth: 68 },
  { key: 'subnet', label: '子网', sortable: true, cellClass: 'cell-mono', width: 150, minWidth: 120 },
  { key: 'gateway', label: '网关', sortable: true, cellClass: 'cell-mono', width: 120, minWidth: 100 },
  { key: 'containers', label: '容器数', sortable: true, width: 86, minWidth: 78 },
  { key: 'actions', label: '操作', cellClass: 'cell-actions-column', width: 116, minWidth: 106 }
]
const networkSortableColumns = networkColumns.filter(column => column.sortable)
const currentPage = ref(1)
const pageSize = ref(20)
const showMoreMenu = ref(false)
const selectedNetworkId = ref('')

// 批量删除：仅表格视图提供勾选，勾选键为网络 ID。
const checkedNetworkIds = ref([])
const batchWorking = ref(false)
// 批量目标取当前过滤结果中被勾选的网络，保证计数与用户可见范围一致。
const checkedNetworks = computed(() =>
  filteredNetworks.value.filter(network => checkedNetworkIds.value.includes(network.Id))
)
const allVisibleChecked = computed(() =>
  paginatedNetworks.value.length > 0 &&
  paginatedNetworks.value.every(network => checkedNetworkIds.value.includes(network.Id))
)
const partiallyChecked = computed(() =>
  !allVisibleChecked.value &&
  paginatedNetworks.value.some(network => checkedNetworkIds.value.includes(network.Id))
)

function toggleNetworkCheck(id) {
  const next = new Set(checkedNetworkIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  checkedNetworkIds.value = [...next]
}

// 表头全选：勾选/清空当前页条目，不影响其它页已勾选的网络。
function toggleNetworkCheckAll() {
  const visible = paginatedNetworks.value.map(network => network.Id)
  const visibleSet = new Set(visible)
  const allChecked = visible.length > 0 && visible.every(id => checkedNetworkIds.value.includes(id))
  checkedNetworkIds.value = allChecked
    ? checkedNetworkIds.value.filter(id => !visibleSet.has(id))
    : [...new Set([...checkedNetworkIds.value, ...visible])]
}

// 刷新后剔除已不存在的勾选，避免对已删除网络执行批量操作。
function pruneCheckedNetworks() {
  const existing = new Set(networksList.value.map(network => network.Id))
  const next = checkedNetworkIds.value.filter(id => existing.has(id))
  if (next.length !== checkedNetworkIds.value.length) checkedNetworkIds.value = next
}

// 批量删除：确认后串行逐项调用单网络删除接口，逐条隔离失败并汇总；
// 系统网络和有容器连接的网络由后端报错计入失败，前端不预检。
async function batchRemoveNetworks() {
  const targets = checkedNetworks.value
  if (batchWorking.value || targets.length === 0) return
  const confirmed = await uiStore.confirm({
    type: 'danger',
    title: '批量删除网络',
    message: `确定删除勾选的 ${targets.length} 个网络吗？将逐条删除，单个失败后继续删除其余网络；系统网络和有容器连接的网络会删除失败并计入失败数。`,
    confirmText: '删除'
  })
  if (!confirmed) {
    return
  }
  batchWorking.value = true
  let succeeded = 0
  const failures = []
  for (const network of targets) {
    try {
      await networks.remove(network.Id)
      succeeded += 1
    } catch (error) {
      failures.push({ name: network.Name || network.Id, reason: error.message || '' })
    }
  }
  batchWorking.value = false
  checkedNetworkIds.value = []
  await refreshNetworks()
  notifyBatchResult(uiStore, { label: '批量删除', succeeded, failures })
}

// 排序
const { sortState, handleSort, setSort, sortData, getSortIcon } = useSort('sort_networks')
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

// 对话框状态
const showEditDialog = ref(false)
const isEditMode = ref(false)

// 表单数据
const editForm = ref({
  id: '',
  name: '',
  driver: 'bridge',
  ipv4Subnet: '',
  ipv4Gateway: '',
  ipv4IpRange: '',
  enableIPv6: false,
  ipv6Subnet: '',
  ipv6Gateway: '',
  ipv6IpRange: '',
  parent: '',
  internal: false,
  attachable: false,
  optionsText: '',
  labelsText: ''
})

// 空状态配置
const emptyProps = {
  title: '暂无网络',
  description: '点击"新建网络"按钮创建第一个网络',
  icon: 'network'
}

const networkRail = computed(() => {
  const custom = networksList.value.filter(network => !isSystemNetwork(network.Name)).length
  const attached = networksList.value.reduce((sum, network) => sum + getContainerCount(network), 0)
  const bridge = networksList.value.filter(network => getNetworkDriver(network) === 'bridge').length
  return [
    {
      label: '网络总数',
      value: networksList.value.length,
      note: `${filteredNetworks.value.length} 个匹配`,
      icon: 'network',
      tone: 'neutral'
    },
    {
      label: '自定义',
      value: custom,
      note: '非系统网络',
      icon: 'settings',
      tone: 'success'
    },
    {
      label: '连接数',
      value: attached,
      note: '容器挂载',
      icon: 'container',
      tone: 'info'
    },
    {
      label: 'bridge',
      value: bridge,
      note: '桥接网络',
      icon: 'share',
      tone: 'muted'
    }
  ]
})

const selectedSummaryNetwork = computed(() => {
  if (selectedNetworkId.value) {
    const selected = networksList.value.find(network => network.Id === selectedNetworkId.value)
    if (selected) return selected
  }
  return paginatedNetworks.value[0] || null
})

const networkContextCounts = computed(() => ({
  attached: networksList.value.reduce((sum, network) => sum + getContainerCount(network), 0)
}))

const networkContainerRelations = computed(() => {
  const network = selectedSummaryNetwork.value
  if (!network) return []
  return Object.entries(network.Containers || {}).map(([containerId, container]) => ({
    id: containerId,
    name: container?.Name || containerId.slice(0, 12) || '未命名容器',
    meta: container?.IPv4Address || container?.IPv6Address || '未分配 IP'
  }))
})

function selectNetwork(network) {
  selectedNetworkId.value = network?.Id || ''
}

function isSelectedNetwork(network) {
  return Boolean(network?.Id && selectedSummaryNetwork.value?.Id === network.Id)
}

// 计算属性
const filteredNetworks = computed(() => {
  let list = networksList.value.filter(network => (
    matchesBooleanFilter(getContainerCount(network) > 0, networkFilters.value.usage)
  ))
  
  // 搜索过滤
  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    list = list.filter(network => {
      const name = (network.Name || '').toLowerCase()
      const driver = getNetworkDriver(network).toLowerCase()
      return name.includes(query) || driver.includes(query)
    })
  }
  
  // 应用用户自定义排序（如果有）
  if (sortState.value.prop && sortState.value.order) {
    return sortData(list, (item, prop) => {
      switch (prop) {
        case 'name':
          return item.Name || ''
        case 'driver':
          return item.Driver || ''
        case 'subnet':
          return getSubnet(item)
        case 'gateway':
          return getGateway(item)
        case 'containers':
          return getContainerCount(item)
        default:
          return ''
      }
    })
  }

  // 默认排序：host > bridge > 其他按英文排序
  list = [...list].sort((a, b) => {
    const nameA = (a.Name || '').toLowerCase()
    const nameB = (b.Name || '').toLowerCase()
    
    // host 排第一
    if (nameA === 'host') return -1
    if (nameB === 'host') return 1
    
    // bridge 排第二
    if (nameA === 'bridge') return -1
    if (nameB === 'bridge') return 1

    // none 排第三
    if (nameA === 'none') return -1
    if (nameB === 'none') return 1
    
    // 其他按英文名字排序
    return nameA.localeCompare(nameB)
  })
  
  return list
})

watch(networkFilters, () => {
  currentPage.value = 1
}, { deep: true })

function openContainerRelation(item) {
  if (!item?.id && !item?.name) return
  router.push({
    name: 'Containers',
    query: { focus: item.id || '', focusName: item.name || '' }
  })
}

// 分页后的列表
const paginatedNetworks = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  const end = start + pageSize.value
  return filteredNetworks.value.slice(start, end)
})

// 批量删除或筛选导致总页数收缩时，把当前页钳制到最后一个非空页。
watch(() => Math.max(1, Math.ceil(filteredNetworks.value.length / pageSize.value)), pages => {
  if (currentPage.value > pages) currentPage.value = pages
})

// 方法
function isSystemNetwork(name) {
  return systemNetworks.includes(name)
}

function getNetworkDriver(network) {
  const driver = String(network?.Driver || '').trim()
  if (driver && driver !== 'null') return driver
  return network?.Name === 'none' ? 'none' : 'bridge'
}

function getSubnet(network) {
  return getIPAMConfig(network, false)?.Subnet || ''
}

function getGateway(network) {
  return getIPAMConfig(network, false)?.Gateway || ''
}

function getIPAMConfig(network, ipv6 = false) {
  const configs = network?.IPAM?.Config || []
  return configs.find(config => {
    const subnet = config?.Subnet || ''
    return ipv6 ? subnet.includes(':') : !subnet.includes(':')
  }) || null
}

function getContainerCount(network) {
  if (!network.Containers) return 0
  return Object.keys(network.Containers).length
}

async function fetchNetworks() {
  try {
    await dockerResources.loadNetworks()
    pruneCheckedNetworks()
  } catch (error) {
    console.error('获取网络列表失败:', error)
  }
}

async function refreshNetworks() {
  if (networksBusy.value) return
  try {
    await dockerResources.loadNetworks({ force: true })
    pruneCheckedNetworks()
  } catch (error) {
    console.error('刷新网络列表失败:', error)
  }
}

function handleSearch() {
  currentPage.value = 1
}



function toggleMoreMenu() {
  showMoreMenu.value = !showMoreMenu.value
}

function closeMoreMenu() {
  showMoreMenu.value = false
}

function openCreateDialog() {
  isEditMode.value = false
  editForm.value = {
    id: '',
    name: '',
    driver: 'bridge',
    ipv4Subnet: '',
    ipv4Gateway: '',
    ipv4IpRange: '',
    enableIPv6: false,
    ipv6Subnet: '',
    ipv6Gateway: '',
    ipv6IpRange: '',
    parent: '',
    internal: false,
    attachable: false,
    optionsText: '',
    labelsText: ''
  }
  showEditDialog.value = true
}

async function loadNetworkDetail(network) {
  if (!network?.Id) return network
  try {
    return await networks.get(network.Id)
  } catch (error) {
    console.error('获取网络详情失败:', error)
    return network
  }
}

async function handleEdit(network) {
  selectNetwork(network)
  if (isSystemNetwork(network.Name)) {
    uiStore.toastWarning('系统网络不可编辑')
    return
  }
  const detail = await loadNetworkDetail(network)
  const ipv4Config = getIPAMConfig(detail, false)
  const ipv6Config = getIPAMConfig(detail, true)
  isEditMode.value = true
  editForm.value = {
    id: detail.Id,
    name: detail.Name,
    driver: detail.Driver || 'bridge',
    ipv4Subnet: ipv4Config?.Subnet || '',
    ipv4Gateway: ipv4Config?.Gateway || '',
    ipv4IpRange: ipv4Config?.IPRange || '',
    enableIPv6: !!detail.EnableIPv6 || !!ipv6Config,
    ipv6Subnet: ipv6Config?.Subnet || '',
    ipv6Gateway: ipv6Config?.Gateway || '',
    ipv6IpRange: ipv6Config?.IPRange || '',
    parent: detail.Options?.parent || '',
    internal: !!detail.Internal,
    attachable: !!detail.Attachable,
    optionsText: mapToKeyValueText(detail.Options),
    labelsText: mapToKeyValueText(detail.Labels)
  }
  showEditDialog.value = true
}

async function saveNetwork() {
  if (!editForm.value.name) {
    uiStore.toastWarning('请输入网络名称')
    return
  }

  try {
    const data = {
      name: editForm.value.name,
      driver: editForm.value.driver,
      ipv4Subnet: editForm.value.ipv4Subnet,
      ipv4Gateway: editForm.value.ipv4Gateway,
      ipv4IpRange: editForm.value.ipv4IpRange,
      enableIPv6: editForm.value.enableIPv6,
      ipv6Subnet: editForm.value.ipv6Subnet,
      ipv6Gateway: editForm.value.ipv6Gateway,
      ipv6IpRange: editForm.value.ipv6IpRange,
      parent: editForm.value.parent,
      internal: editForm.value.internal,
      attachable: editForm.value.attachable,
      options: parseKeyValueText(editForm.value.optionsText),
      labels: parseKeyValueText(editForm.value.labelsText)
    }

    if (isEditMode.value) {
      await networks.update(editForm.value.id, data)
    } else {
      await networks.create(data)
    }

    showEditDialog.value = false
    await refreshNetworks()
  } catch (error) {
    console.error(isEditMode.value ? '更新网络失败:' : '创建网络失败:', error)
    uiStore.toastError((isEditMode.value ? '更新' : '创建') + '失败: ' + (error.message || '未知错误'))
  }
}

function parseKeyValueText(text) {
  const result = {}
  String(text || '').split('\n').forEach(line => {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) return
    const idx = trimmed.indexOf('=')
    if (idx <= 0) return
    const key = trimmed.slice(0, idx).trim()
    const value = trimmed.slice(idx + 1).trim()
    if (key) result[key] = value
  })
  return result
}

function mapToKeyValueText(value) {
  return Object.entries(value || {})
    .map(([key, val]) => `${key}=${val}`)
    .join('\n')
}

async function handleRemove(network) {
  if (isSystemNetwork(network.Name)) {
    uiStore.toastWarning('系统网络不可删除')
    return
  }

  const containerCount = getContainerCount(network)
  if (containerCount > 0) {
    uiStore.toastWarning(`该网络有 ${containerCount} 个容器正在使用，无法删除`)
    return
  }

  const confirmed = await uiStore.confirm({
    type: 'danger',
    title: '删除网络',
    message: `确定删除网络 "${network.Name}" 吗？`,
    confirmText: '删除'
  })
  if (!confirmed) {
    return
  }

  try {
    await networks.remove(network.Id)
    await refreshNetworks()
  } catch (error) {
    console.error('删除网络失败:', error)
    uiStore.toastError('删除失败: ' + (error.message || '未知错误'))
  }
}

async function pruneNetworks() {
  closeMoreMenu()
  const confirmed = await uiStore.confirm({
    type: 'danger',
    title: '清理无用网络',
    message: '确定清理所有无用的网络吗？此操作不可恢复。',
    confirmText: '清理'
  })
  if (!confirmed) {
    return
  }
  try {
    await networks.prune()
    await refreshNetworks()
  } catch (error) {
    console.error('清理网络失败:', error)
    uiStore.toastError('清理失败: ' + (error.message || '未知错误'))
  }
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
  fetchNetworks()
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.network-batch-check {
  width: 15px;
  height: 15px;
  cursor: pointer;
  accent-color: var(--color-primary-600, #2563eb);
}

.batch-danger {
  color: var(--color-danger-600);
}

.content-scroll {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
  padding-bottom: 24px;
  max-width: 1680px;
  width: 100%;
  margin: 0 auto;
}

.networks-hero,
.resource-rail,
.toolbar {
  max-width: 1680px;
  width: 100%;
  margin-left: auto;
  margin-right: auto;
}

.networks-hero {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.hero-copy {
  min-width: 0;
}

.eyebrow {
  margin: 0 0 6px;
  color: var(--ops-muted);
  font-size: 0.75rem;
  font-weight: 800;
  letter-spacing: 0.08em;
}

.hero-copy h1 {
  margin: 0;
  color: var(--ops-ink);
  font-size: 1.55rem;
  line-height: 1.15;
  font-weight: 900;
  letter-spacing: 0;
}

.hero-copy p:last-child {
  margin: 7px 0 0;
  color: var(--ops-muted);
  font-size: 0.875rem;
}

.resource-rail {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 14px;
}

.rail-card {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 10px;
  min-height: 70px;
  padding: 12px;
  border: 1px solid var(--ops-line);
  border-radius: 10px;
  background: color-mix(in srgb, var(--ops-panel) 78%, transparent);
  box-shadow: 0 18px 48px color-mix(in srgb, var(--ops-ink) 7%, transparent);
}

.rail-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border-radius: 9px;
}

.rail-card.neutral .rail-icon { color: var(--ops-blue); background: var(--ops-blue-soft); }
.rail-card.success .rail-icon { color: var(--ops-green-strong); background: var(--ops-green-soft); }
.rail-card.warning .rail-icon { color: var(--ops-amber); background: var(--ops-amber-soft); }
.rail-card.info .rail-icon { color: var(--ops-cyan); background: var(--ops-cyan-soft); }
.rail-card.muted .rail-icon { color: var(--ops-muted); background: var(--ops-muted-soft); }

.rail-copy {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.rail-copy strong {
  color: var(--ops-ink);
  font-size: 1.05rem;
  font-weight: 900;
  line-height: 1.1;
}

.rail-copy span,
.rail-card small {
  color: var(--ops-muted);
  font-size: 0.72rem;
  font-weight: 700;
}

/* 工具栏 */
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
  flex-wrap: wrap;
  padding: 12px;
  border: 1px solid var(--ops-line);
  border-radius: 12px;
  background: color-mix(in srgb, var(--ops-surface) 90%, transparent);
  box-shadow: 0 18px 48px color-mix(in srgb, var(--ops-ink) 7%, transparent);
}

.toolbar-left {
  display: grid;
  grid-template-columns: minmax(220px, 360px) 190px;
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

.icon-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  border-radius: 10px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.icon-btn:hover,
.icon-btn.is-active {
  background: var(--bg-secondary);
  border-color: var(--border-default);
  color: var(--text-primary);
}

.icon-btn svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.icon-btn.is-spinning svg {
  animation: resource-refresh-spin 0.75s linear infinite;
}

/* 下拉菜单 */
.dropdown-wrapper {
  position: relative;
}

.dropdown-menu {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  box-shadow: var(--shadow-lg);
  padding: 6px;
  min-width: 170px;
  z-index: 100;
}

.dropdown-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 12px;
  border-radius: 8px;
  font-size: 0.8125rem;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  white-space: nowrap;
}

.dropdown-item:hover {
  background: var(--bg-tertiary);
  color: var(--text-primary);
}

.dropdown-item.danger {
  color: var(--color-danger-600);
}

.dropdown-item.danger:hover {
  background: var(--color-danger-100);
}

.dropdown-item svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
  flex-shrink: 0;
}

/* 列表视图 */
.list-view {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  overflow: hidden;
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
  padding: 12px 0;
  border-bottom: 1px solid var(--border-subtle);
}

.skeleton-cell {
  height: 16px;
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
  border-radius: 4px;
}

.list-empty {
  padding: 48px 0;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.875rem;
}

.data-table th {
  text-align: left;
  padding: 12px 16px;
  font-weight: 600;
  color: var(--text-secondary);
  background: var(--bg-elevated);
  border-bottom: 1px solid var(--border-subtle);
  white-space: nowrap;
}

.data-table th.sortable {
  cursor: pointer;
  user-select: none;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.data-table th.sortable:hover {
  color: var(--text-primary);
  background: var(--bg-secondary);
}

.sort-icon {
  display: inline-flex;
  align-items: center;
  margin-left: 4px;
  vertical-align: middle;
  color: var(--text-tertiary);
}

.sort-icon.asc,
.sort-icon.desc {
  color: var(--color-primary-500);
}

.sort-icon.asc,
.sort-icon.desc {
  color: var(--color-primary-500);
}

.data-table td {
  padding: 12px 16px;
  border-bottom: 1px solid var(--border-subtle);
  color: var(--text-primary);
  vertical-align: middle;
}

.name-wrapper {
  display: flex;
  align-items: center;
  gap: 8px;
}

.network-icon-small {
  color: var(--color-primary-500);
  flex-shrink: 0;
}

.cell-mono {
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.8125rem;
}

.cell-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 100%;
}

.table-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: var(--bg-tertiary);
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

/* 查看/编辑 - 浅蓝色 */
.table-btn.info {
  background: var(--color-primary-100);
  color: var(--color-primary-600);
}

.table-btn.info svg {
  stroke: var(--color-primary-600);
}

.table-btn.info:hover:not(:disabled) {
  background: var(--color-primary-200);
  color: var(--color-primary-700);
}

.table-btn.info:hover:not(:disabled) svg {
  stroke: var(--color-primary-700);
}

/* 删除 - 粉红色 */
.table-btn.danger {
  background: var(--color-danger-100);
  color: var(--color-danger-600);
}

.table-btn.danger svg {
  stroke: var(--color-danger-600);
}

.table-btn.danger:hover:not(:disabled) {
  background: var(--color-danger-200);
  color: var(--color-danger-700);
}

.table-btn.danger:hover:not(:disabled) svg {
  stroke: var(--color-danger-700);
}

.table-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.table-btn svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

/* 分页 */
.pagination-wrapper {
  position: fixed;
  bottom: 0;
  left: 260px;
  right: 0;
  background: var(--bg-primary);
  border-top: 1px solid var(--border-subtle);
  padding: 12px 24px;
  z-index: 100;
}


/* 表单样式 */
.form-group {
  margin-bottom: 16px;
}

.form-label {
  display: block;
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--text-primary);
  margin-bottom: 6px;
}

.form-label .required {
  color: var(--color-danger-500);
}

.form-input,
.form-select,
.form-textarea {
  width: 100%;
  padding: 10px 12px;
  border: 1px solid var(--border-default);
  border-radius: 8px;
  background: var(--bg-primary);
  color: var(--text-primary);
  font-size: 0.875rem;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.form-textarea {
  resize: vertical;
  min-height: 82px;
  line-height: 1.5;
}

.form-input:focus,
.form-select:focus,
.form-textarea:focus {
  outline: none;
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.form-input:disabled,
.form-select:disabled,
.form-textarea:disabled {
  background: var(--bg-tertiary);
  color: var(--text-tertiary);
  cursor: not-allowed;
}

.form-options-row {
  display: flex;
  align-items: center;
  gap: 16px;
}

.checkbox-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--text-secondary);
  font-size: 0.875rem;
  cursor: pointer;
  user-select: none;
}

.checkbox-label input {
  width: 16px;
  height: 16px;
  accent-color: var(--color-primary-500);
}

/* 详情查看 */
.view-details {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.detail-row {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.detail-label {
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--text-secondary);
  min-width: 80px;
}

.detail-value {
  font-size: 0.875rem;
  color: var(--text-primary);
  word-break: break-all;
}

.detail-value.mono {
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.8125rem;
}

.detail-value.text-muted {
  color: var(--text-tertiary);
}

.detail-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 8px;
  padding-top: 16px;
  border-top: 1px solid var(--border-subtle);
}

.containers-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.container-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  background: var(--bg-tertiary);
  border-radius: 8px;
  font-size: 0.8125rem;
}

.container-name {
  font-weight: 500;
  color: var(--text-primary);
  flex: 1;
}

.container-ipv4,
.container-ipv6 {
  font-family: 'JetBrains Mono', monospace;
  color: var(--text-secondary);
  font-size: 0.75rem;
}

/* 对话框按钮 */
.btn {
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  border: none;
}

.btn-default {
  background: var(--bg-tertiary);
  color: var(--text-secondary);
}

.btn-default:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

.btn-primary {
  background: linear-gradient(135deg, var(--color-primary-500), var(--color-primary-600));
  color: var(--text-inverse);
}

.btn-primary:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px var(--color-primary-500-30);
}

.btn-primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

/* Compose/容器工作台基准覆盖 */
.networks-page {
  padding: 22px 24px 0;
  gap: 14px;
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

.content-scroll {
  padding-bottom: 92px;
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(280px, 300px);
  align-items: start;
  gap: 12px;
}

.networks-hero {
  align-items: flex-start;
  gap: 24px;
  margin-bottom: 0;
}

.eyebrow {
  margin: 0 0 4px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.hero-copy h1 {
  font-size: clamp(1.65rem, 1.75vw, 2.25rem);
  line-height: 1.12;
  font-weight: 800;
}

.hero-copy p:last-child {
  margin: 6px 0 0;
  font-size: 0.95rem;
}

.resource-rail {
  gap: 10px;
  margin-bottom: 0;
}

.rail-card {
  grid-template-columns: 38px 1fr;
  grid-template-areas:
    "icon copy"
    "icon note";
  gap: 2px 10px;
  min-height: 64px;
  border-radius: 12px;
  backdrop-filter: blur(12px);
}

.rail-icon {
  grid-area: icon;
  width: 38px;
  height: 38px;
  border-radius: 10px;
}

.rail-copy {
  grid-area: copy;
  flex-direction: row;
  align-items: baseline;
  gap: 8px;
}

.rail-copy strong {
  font-size: 1.35rem;
  line-height: 1;
  font-weight: 800;
}

.rail-copy span,
.rail-card small {
  overflow: hidden;
  font-size: 0.75rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rail-card small {
  grid-area: note;
}

.toolbar {
  margin-bottom: 0;
}

.list-view {
  border-color: var(--ops-line);
  border-radius: 14px;
  background: color-mix(in srgb, var(--ops-surface) 90%, transparent);
  box-shadow: 0 18px 48px color-mix(in srgb, var(--ops-ink) 7%, transparent);
  overflow: hidden;
  min-height: 560px;
}

.content-scroll :deep(.card-grid-wrapper) {
  height: 100%;
  min-height: 560px;
  padding: 16px;
  border: 1px solid var(--ops-line);
  border-radius: 14px;
  background: color-mix(in srgb, var(--ops-surface) 90%, transparent);
  box-shadow: 0 18px 48px color-mix(in srgb, var(--ops-ink) 7%, transparent);
}

.content-scroll :deep(.card-grid) {
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 16px;
  padding: 0;
}

.content-scroll :deep(.network-card) {
  min-height: 236px;
  aspect-ratio: auto;
  border-color: var(--ops-line);
  border-radius: 12px;
}

.content-scroll :deep(.network-card.is-selected) {
  border-color: color-mix(in srgb, var(--ops-green) 62%, var(--ops-line));
  box-shadow:
    inset 4px 0 0 var(--ops-green),
    0 18px 42px color-mix(in srgb, var(--ops-green) 14%, transparent);
  animation: network-card-select var(--motion-duration-fast) var(--motion-ease-out);
}

.data-table {
  min-width: 0;
  width: 100%;
  table-layout: fixed;
  font-size: 0.875rem;
}

.col-name { width: 30%; }
.col-driver { width: 10%; }
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
  grid-template-columns: var(--resource-grid-template, minmax(170px, 1.35fr) 72px minmax(135px, 1fr) minmax(110px, 0.85fr) 82px 82px);
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

.resource-row > [role="cell"] {
  min-height: 68px;
  display: flex;
  align-items: center;
  color: var(--ops-ink);
  min-width: 0;
  overflow: hidden;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.resource-row .status-badge {
  flex: 0 0 auto;
  min-width: 54px;
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

.resource-row:active {
  background: var(--ops-row-active);
  transform: scale(0.992);
}

.resource-row.selected {
  animation: network-row-select var(--motion-duration-fast) var(--motion-ease-out);
  background: var(--ops-row-hover);
  box-shadow: inset 4px 0 0 var(--ops-green);
}

.resource-row:hover {
  background: var(--ops-row-hover);
}

.resource-row:focus-visible {
  background: var(--ops-row-focus);
  outline: 2px solid color-mix(in srgb, var(--ops-green) 36%, transparent);
  outline-offset: -2px;
}

.resource-context-panel {
  position: sticky;
  top: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
  min-height: 560px;
  padding: 16px;
  border: 1px solid var(--ops-line);
  border-radius: 14px;
  background: color-mix(in srgb, var(--ops-surface) 90%, transparent);
  box-shadow: 0 18px 48px color-mix(in srgb, var(--ops-ink) 7%, transparent);
  backdrop-filter: blur(14px);
}

.context-header {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.context-header > div {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.context-header strong {
  overflow: hidden;
  color: var(--ops-ink);
  font-size: 0.98rem;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.context-header span {
  overflow: hidden;
  color: var(--ops-muted);
  font-size: 0.75rem;
  text-overflow: ellipsis;
  white-space: nowrap;
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

.context-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.context-grid > div,
.context-section {
  padding: 10px;
  border: 1px solid var(--ops-line);
  border-radius: 10px;
  background: color-mix(in srgb, var(--ops-panel) 76%, transparent);
}

.context-grid span,
.section-label {
  margin: 0;
  color: var(--ops-muted);
  font-size: 0.68rem;
  font-weight: 800;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.context-grid strong {
  display: block;
  overflow: hidden;
  margin-top: 4px;
  color: var(--ops-ink);
  font-size: 0.92rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-lines,
.log-stream-preview ol {
  display: grid;
  gap: 8px;
  margin: 10px 0 0;
  padding: 0;
  list-style: none;
}

.detail-lines span,
.log-stream-preview li {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
  color: var(--ops-ink);
  font-size: 0.76rem;
}

.detail-lines b {
  flex: 0 0 auto;
  color: var(--ops-muted);
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-size: 0.66rem;
}

.log-stream-preview li span {
  width: 7px;
  height: 7px;
  flex: 0 0 auto;
  border-radius: 999px;
  background: var(--ops-green);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--ops-green) 16%, transparent);
}

.log-stream-preview code {
  overflow: hidden;
  color: var(--ops-ink);
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-size: 0.75rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@keyframes network-row-compress {
  0% { transform: none; }
  42% { transform: none; }
  100% { transform: none; }
}

@keyframes network-row-select {
  0% { transform: scale(1); background: var(--ops-row-hover); }
  44% { transform: scale(0.988); background: var(--ops-row-hover); }
  100% { transform: scale(1); background: var(--ops-row-hover); }
}

@keyframes network-card-select {
  0% { transform: translateY(0) scale(0.985); }
  70% { transform: translateY(0) scale(1.006); }
  100% { transform: translateY(0) scale(1); }
}

@media (prefers-reduced-motion: reduce) {
  .resource-row.selected,
  .content-scroll :deep(.network-card.is-selected) {
    animation: none;
  }
}

.cell-mono {
  color: var(--ops-muted);
}

.network-icon-small {
  color: var(--ops-green-strong);
}

.name-wrapper {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.name-button {
  display: flex;
  align-items: center;
  gap: 8px;
  max-width: 100%;
  width: 100%;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--ops-ink);
  cursor: pointer;
  font: inherit;
  text-align: left;
}

.network-name-line {
  display: flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
  overflow: hidden;
}

.network-name-line strong {
  flex: 0 1 auto;
  min-width: 0;
}

.network-system-badge {
  flex: 0 0 auto;
}

.table-btn {
  width: 32px;
  height: 32px;
  border-radius: 8px;
}

/* 紧凑资源页头：全局 Header 已经承担页面定位 */
.networks-hero {
  display: none;
}

.resource-rail {
  gap: 8px;
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
  .content-scroll {
    grid-template-columns: 1fr;
  }

  .resource-context-panel {
    position: static;
    min-height: 360px;
  }
}
</style>
