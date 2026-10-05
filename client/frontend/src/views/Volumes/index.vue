<template>
  <ResourceWorkbench class="volumes-page volumes-workbench" role="main" aria-label="数据卷管理" remote-context>
    <template #rail>
      <section class="resource-rail" aria-label="数据卷概览">
        <article v-for="item in volumeRail" :key="item.label" class="rail-card" :class="item.tone">
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
          placeholder="搜索卷名称..."
          @search="handleSearch"
        />
        <div class="sort-control">
          <DynamicIcon name="arrow-up-down" :size="15" />
          <select v-model="sortField" aria-label="数据卷排序字段">
            <option value="">默认排序</option>
            <option value="name">按名称</option>
            <option value="driver">按驱动</option>
            <option value="mountpoint">按挂载点</option>
            <option value="containers">按容器数</option>
            <option value="created">按创建时间</option>
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
          v-model="volumeFilters.usage"
          :options="usageFilterOptions"
          aria-label="使用筛选"
          compact
        />
      </div>
      <div class="toolbar-right">
        <button
          v-if="viewMode !== 'grid' && checkedVolumes.length > 0"
          v-ripple
          data-remote-write
          class="secondary-btn batch-danger"
          type="button"
          :disabled="batchWorking"
          title="批量删除已勾选的数据卷"
          @click="batchRemoveVolumes"
        >
          <DynamicIcon name="trash-2" :size="16" />
          批量删除（{{ checkedVolumes.length }}）
        </button>
        <ViewToggle v-model="viewMode" />
        <button
          v-ripple
          class="icon-btn resource-refresh-btn"
          :class="{ 'is-spinning': volumesBusy }"
          :disabled="volumesBusy"
          title="刷新数据卷列表"
          @click="refreshVolumes"
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
            <div class="dropdown-item danger" @click="pruneVolumes">
              <DynamicIcon name="trash-2" :size="16" />
              清理无用卷
            </div>
          </div>
        </div>
        <button class="primary-btn" data-remote-write @click="openCreateDialog">
          <DynamicIcon name="plus" :size="16" />
          新建卷
        </button>
      </div>
      </div>
    </template>

    <ResourceBoard>
      <ResourcePanel>
        <CardGrid
          v-if="viewMode === 'grid'"
          :items="paginatedVolumes"
          :loading="loading"
          :skeleton-count="12"
          :empty-props="emptyProps"
        >
          <template #default="{ items }">
            <VolumeCard
              v-for="volume in items"
              :key="volume.Name"
              :volume="volume"
              :selected="isSelectedVolume(volume)"
              @select="selectVolume(volume)"
              @browse="handleBrowse(volume)"
              @remove="handleRemove(volume)"
            />
          </template>
        </CardGrid>

        <ResourceTable
          v-else
          :columns="volumeColumns"
          :rows="paginatedVolumes"
          table-key="volumes-v2"
          row-key="Name"
          :selected-key="selectedVolumeName"
          :loading="loading"
          aria-label="数据卷列表"
          @select="selectVolume"
          @sort="handleSort"
        >
          <template #header-check>
            <input
              class="volume-batch-check"
              type="checkbox"
              :checked="allVisibleChecked"
              :indeterminate.prop="partiallyChecked"
              :disabled="paginatedVolumes.length === 0"
              title="全选/取消当前列表全部数据卷"
              @click.stop
              @change="toggleVolumeCheckAll"
            />
          </template>

          <template #cell-check="{ row: volume }">
            <input
              class="volume-batch-check"
              type="checkbox"
              :checked="checkedVolumeNames.includes(volume.Name)"
              title="勾选后可批量删除"
              @click.stop
              @change="toggleVolumeCheck(volume.Name)"
            />
          </template>

          <template v-for="column in volumeSortableColumns" :key="column.key" #[`header-${column.key}`]>
            {{ column.label }}
            <span class="sort-icon" :class="getSortIcon(column.key)">
              <DynamicIcon v-if="getSortIcon(column.key) === 'asc'" name="arrow-up" :size="14" />
              <DynamicIcon v-else-if="getSortIcon(column.key) === 'desc'" name="arrow-down" :size="14" />
              <DynamicIcon v-else name="arrow-up-down" :size="14" />
            </span>
          </template>

          <template #cell-name="{ row: volume }">
            <button class="resource-name-button" type="button" @click.stop="selectVolume(volume)">
              <span class="resource-kind volume">
                <DynamicIcon class="volume-icon-small" name="hard-drive" :size="16" />
              </span>
              <span class="resource-name-copy">
                <strong>{{ volume.Name }}</strong>
                <small>{{ volume.Driver || 'local' }} · {{ shortMountpoint(volume.Mountpoint) || '-' }}</small>
              </span>
            </button>
          </template>
          <template #cell-driver="{ row: volume }">{{ volume.Driver || 'local' }}</template>
          <template #cell-mountpoint="{ row: volume }">
            <button
              v-if="volume.Mountpoint"
              class="copyable-text cell-ellipsis"
              type="button"
              :title="`点击复制：${volume.Mountpoint}`"
              @click.stop="copyMountpoint(volume.Mountpoint)"
            >{{ shortMountpoint(volume.Mountpoint) }}</button>
            <span v-else>-</span>
          </template>
          <template #cell-containers="{ row: volume }">
            <StatusBadge :status="getContainerCount(volume) > 0 ? 'used' : 'unused'" variant="minimal" />
            <span v-if="getContainerCount(volume) > 0" class="container-count">({{ getContainerCount(volume) }})</span>
          </template>
          <template #cell-created="{ row: volume }">{{ formatDate(volume.CreatedAt) }}</template>
          <template #cell-actions="{ row: volume }">
            <div class="cell-actions" data-remote-write>
              <button class="table-btn info" title="浏览文件" @click.stop="handleBrowse(volume)">
                <DynamicIcon name="file-text" :size="14" />
              </button>
              <button
                class="table-btn danger"
                :disabled="getContainerCount(volume) > 0"
                :title="getContainerCount(volume) > 0 ? '使用中，无法删除' : '删除'"
                @click.stop="handleRemove(volume)"
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
            :total="filteredVolumes.length"
          />
        </template>
      </ResourcePanel>

      <template #context>
        <ResourceContextPanel :motion-key="selectedSummaryVolume?.Name || 'overview'" aria-label="数据卷上下文">
      <div class="context-header">
        <span class="resource-kind volume">
          <DynamicIcon name="hard-drive" :size="18" />
        </span>
        <div>
          <strong>{{ selectedSummaryVolume ? selectedSummaryVolume.Name : '数据卷概览' }}</strong>
          <span v-if="selectedSummaryVolume">{{ selectedSummaryVolume.Mountpoint || '-' }}</span>
          <span v-else>{{ filteredVolumes.length }} 个匹配数据卷</span>
        </div>
      </div>

      <div v-if="selectedSummaryVolume" class="context-actions" data-remote-write>
        <button type="button" class="detail-action info" @click="handleBrowse(selectedSummaryVolume)">
          <DynamicIcon name="file-text" :size="15" />
          浏览文件
        </button>
        <button
          type="button"
          class="detail-action danger"
          :disabled="getContainerCount(selectedSummaryVolume) > 0"
          @click="handleRemove(selectedSummaryVolume)"
        >
          <DynamicIcon name="trash-2" :size="15" />
          删除
        </button>
      </div>

      <div class="context-grid">
        <div>
          <span>驱动</span>
          <strong>{{ selectedSummaryVolume?.Driver || 'local' }}</strong>
        </div>
        <div>
          <span>容器</span>
          <strong>{{ selectedSummaryVolume ? getContainerCount(selectedSummaryVolume) : volumeContextCounts.used }}</strong>
        </div>
        <div>
          <span>状态</span>
          <strong>{{ selectedSummaryVolume ? (getContainerCount(selectedSummaryVolume) > 0 ? '使用中' : '未使用') : '全部' }}</strong>
        </div>
        <div>
          <span>创建</span>
          <strong>{{ selectedSummaryVolume ? formatDate(selectedSummaryVolume.CreatedAt) || '-' : '-' }}</strong>
        </div>
      </div>

      <div v-if="selectedSummaryVolume" class="context-section selected-detail">
        <p class="section-label">挂载 / 配置</p>
        <div class="detail-lines">
          <span>
            <b>PATH</b>
            {{ selectedSummaryVolume.Mountpoint || '-' }}
          </span>
          <span>
            <b>LABELS</b>
            {{ objectKeyCount(selectedSummaryVolume.Labels) }}
          </span>
          <span>
            <b>OPTIONS</b>
            {{ objectKeyCount(selectedSummaryVolume.Options) }}
          </span>
        </div>
      </div>

      <ResourceRelationList
        title="使用关系"
        :items="volumeContainerRelations"
        clickable
        empty-text="无关联容器"
        @select="openContainerRelation"
      />
        </ResourceContextPanel>
      </template>
    </ResourceBoard>

    <!-- 创建卷对话框 -->
    <Modal
      v-model:visible="showCreateDialog"
      title="新建卷"
      width="480px"
    >
      <template #footer>
        <button class="action-btn secondary" @click="showCreateDialog = false">取消</button>
        <button class="action-btn primary" @click="handleCreate">创建</button>
      </template>
      <div class="form-group">
        <label class="form-label">
          卷名称
          <span class="required">*</span>
        </label>
        <input
          v-model="createForm.name"
          type="text"
          class="form-input"
          placeholder="输入卷名称"
        />
      </div>
      <div class="form-group">
        <label class="form-label">驱动</label>
        <select v-model="createForm.driver" class="form-input">
          <option value="local">local</option>
          <option value="nfs">nfs (local driver + nfs opts)</option>
          <option value="cifs">cifs (local driver + cifs opts)</option>
          <option value="custom">自定义驱动</option>
        </select>
      </div>
      <div v-if="createForm.driver === 'custom'" class="form-group">
        <label class="form-label">自定义驱动名</label>
        <input
          v-model="createForm.customDriver"
          type="text"
          class="form-input"
          placeholder="例如: rexray/efs"
        />
      </div>
      <div class="form-group">
        <label class="form-label">Driver Options</label>
        <textarea
          v-model="createForm.driverOptsText"
          class="form-textarea"
          rows="4"
          :placeholder="driverOptionsPlaceholder"
        ></textarea>
      </div>
      <div class="form-group">
        <label class="form-label">Labels</label>
        <textarea
          v-model="createForm.labelsText"
          class="form-textarea"
          rows="3"
          placeholder="每行一个 key=value"
        ></textarea>
      </div>
    </Modal>

    <!-- 文件浏览器对话框 -->
    <Modal
      v-model:visible="showBrowseDialog"
      title="卷文件浏览器"
      width="90%"
      @close="closeBrowseDialog"
    >
      <div class="browse-container">
        <iframe
          v-if="browseUrl"
          :src="browseUrl"
          class="browse-iframe"
          frameborder="0"
        ></iframe>
        <div v-else class="browse-loading">
          <Loading />
          <p>正在启动文件浏览器...</p>
        </div>
      </div>
    </Modal>
  </ResourceWorkbench>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useDockerResourcesStore } from '@/stores/dockerResources.js'
import CardGrid from '@/components/data-display/CardGrid.vue'
import VolumeCard from '@/components/data-display/VolumeCard.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import ViewToggle from '@/components/ui/ViewToggle.vue'
import Pagination from '@/components/ui/Pagination.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import Modal from '@/components/feedback/Modal.vue'
import Loading from '@/components/feedback/Loading.vue'
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
import { volumes } from '@edition/api'
import { useUiStore } from '@/stores/ui.js'
import { useViewMode } from '@/composables/useViewMode.js'
import { useSort } from '@/composables/useSort.js'
import { formatTime } from '@/utils/format.js'
import { copyToClipboard } from '@/utils/helpers.js'
import { matchesBooleanFilter } from '@/utils/resourceFilters.js'
import { notifyBatchResult } from '@/utils/batchFeedback.js'

const dockerResources = useDockerResourcesStore()
const uiStore = useUiStore()
const router = useRouter()

// 视图模式
const { viewMode } = useViewMode('volumes_view_mode', 'table')

// 排序
const { sortState, handleSort, setSort, sortData, getSortIcon } = useSort('sort_volumes')
const volumeColumns = [
  { key: 'check', label: '', cellClass: 'cell-check', width: 40, minWidth: 36 },
  { key: 'name', label: '名称', sortable: true, cellClass: 'cell-name', width: 210, minWidth: 150, flex: true },
  { key: 'driver', label: '驱动', sortable: true, width: 74, minWidth: 64 },
  { key: 'mountpoint', label: '挂载点', sortable: true, cellClass: 'cell-mono', width: 170, minWidth: 120 },
  { key: 'containers', label: '容器数', sortable: true, width: 96, minWidth: 88 },
  { key: 'created', label: '创建时间', sortable: true, width: 116, minWidth: 96 },
  { key: 'actions', label: '操作', cellClass: 'cell-actions-column', width: 116, minWidth: 106 }
]
const volumeSortableColumns = volumeColumns.filter(column => column.sortable)
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
const loading = computed(() => {
  const resource = dockerResources.resources.volumes
  return resource.loading || resource.refreshing
})
const volumesBusy = computed(() => loading.value)
const volumesList = computed(() => dockerResources.volumeList)
const searchQuery = ref('')
const volumeFilters = ref({ usage: 'all' })
const usageFilterOptions = [
  { value: 'all', label: '全部' },
  { value: 'yes', label: '已使用' },
  { value: 'no', label: '未使用' }
]
const currentPage = ref(1)
const pageSize = ref(20)
const showMoreMenu = ref(false)
const selectedVolumeName = ref('')

// 批量删除：仅表格视图提供勾选，勾选键为卷名称。
const checkedVolumeNames = ref([])
const batchWorking = ref(false)
// 批量目标取当前过滤结果中被勾选的卷，保证计数与用户可见范围一致。
const checkedVolumes = computed(() =>
  filteredVolumes.value.filter(volume => checkedVolumeNames.value.includes(volume.Name))
)
const allVisibleChecked = computed(() =>
  paginatedVolumes.value.length > 0 &&
  paginatedVolumes.value.every(volume => checkedVolumeNames.value.includes(volume.Name))
)
const partiallyChecked = computed(() =>
  !allVisibleChecked.value &&
  paginatedVolumes.value.some(volume => checkedVolumeNames.value.includes(volume.Name))
)

function toggleVolumeCheck(name) {
  const next = new Set(checkedVolumeNames.value)
  if (next.has(name)) next.delete(name)
  else next.add(name)
  checkedVolumeNames.value = [...next]
}

// 表头全选：勾选/清空当前页条目，不影响其它页已勾选的数据卷。
function toggleVolumeCheckAll() {
  const visible = paginatedVolumes.value.map(volume => volume.Name)
  const visibleSet = new Set(visible)
  const allChecked = visible.length > 0 && visible.every(name => checkedVolumeNames.value.includes(name))
  checkedVolumeNames.value = allChecked
    ? checkedVolumeNames.value.filter(name => !visibleSet.has(name))
    : [...new Set([...checkedVolumeNames.value, ...visible])]
}

// 刷新后剔除已不存在的勾选，避免对已删除卷执行批量操作。
function pruneCheckedVolumes() {
  const existing = new Set(volumesList.value.map(volume => volume.Name))
  const next = checkedVolumeNames.value.filter(name => existing.has(name))
  if (next.length !== checkedVolumeNames.value.length) checkedVolumeNames.value = next
}

// 批量删除：确认后串行逐项调用单卷删除接口，逐条隔离失败并汇总；
// 正在被容器使用的卷由后端报错计入失败，前端不预检。
async function batchRemoveVolumes() {
  const targets = checkedVolumes.value
  if (batchWorking.value || targets.length === 0) return
  const confirmed = await uiStore.confirm({
    type: 'danger',
    title: '批量删除数据卷',
    message: `确定删除勾选的 ${targets.length} 个数据卷吗？将逐条删除，单个失败后继续删除其余数据卷；正在被容器使用的数据卷会删除失败并计入失败数。`,
    confirmText: '删除'
  })
  if (!confirmed) {
    return
  }
  batchWorking.value = true
  let succeeded = 0
  const failures = []
  for (const volume of targets) {
    try {
      await volumes.remove(volume.Name)
      succeeded += 1
    } catch (error) {
      failures.push({ name: volume.Name, reason: error.message || '' })
    }
  }
  batchWorking.value = false
  checkedVolumeNames.value = []
  await refreshVolumes()
  notifyBatchResult(uiStore, { label: '批量删除', succeeded, failures })
}

// 对话框状态
const showCreateDialog = ref(false)
const showBrowseDialog = ref(false)
const browseUrl = ref('')
const browseSessionId = ref('')
const currentBrowseVolume = ref(null)

// 表单数据
const createForm = ref({
  name: '',
  driver: 'local',
  customDriver: '',
  driverOptsText: '',
  labelsText: ''
})

// 空状态配置
const emptyProps = {
  title: '暂无卷',
  description: '点击"新建卷"按钮创建第一个卷',
  icon: 'volume'
}

const driverOptionsPlaceholder = computed(() => {
  switch (createForm.value.driver) {
    case 'nfs':
      return 'type=nfs\no=addr=192.168.1.10,rw,nfsvers=4\ndevice=:/export/path'
    case 'cifs':
      return 'type=cifs\no=username=user,password=pass,vers=3.0\ndevice=//192.168.1.10/share'
    default:
      return '每行一个 key=value'
  }
})

const volumeRail = computed(() => {
  const used = volumesList.value.filter(volume => getContainerCount(volume) > 0).length
  const unused = Math.max(0, volumesList.value.length - used)
  const local = volumesList.value.filter(volume => (volume.Driver || 'local') === 'local').length
  return [
    {
      label: '数据卷',
      value: volumesList.value.length,
      note: `${filteredVolumes.value.length} 个匹配`,
      icon: 'hard-drive',
      tone: 'neutral'
    },
    {
      label: '使用中',
      value: used,
      note: '被容器挂载',
      icon: 'container',
      tone: 'success'
    },
    {
      label: '未使用',
      value: unused,
      note: '可清理候选',
      icon: 'trash-2',
      tone: unused > 0 ? 'warning' : 'muted'
    },
    {
      label: 'local 驱动',
      value: local,
      note: '本机存储',
      icon: 'database',
      tone: 'info'
    }
  ]
})

const selectedSummaryVolume = computed(() => {
  if (selectedVolumeName.value) {
    const selected = volumesList.value.find(volume => volume.Name === selectedVolumeName.value)
    if (selected) return selected
  }
  return paginatedVolumes.value[0] || null
})

const volumeContextCounts = computed(() => {
  const used = volumesList.value.filter(volume => getContainerCount(volume) > 0).length
  return {
    used,
    unused: Math.max(0, volumesList.value.length - used)
  }
})

const volumeContainerRelations = computed(() => {
  const volume = selectedSummaryVolume.value
  if (!volume) return []
  return Object.entries(volume.Containers || {}).map(([containerId, container]) => ({
    id: containerId,
    name: container?.Name || container?.NameID || containerId.slice(0, 12) || '未命名容器',
    meta: containerId.slice(0, 12) || '-'
  }))
})

function selectVolume(volume) {
  selectedVolumeName.value = volume?.Name || ''
}

function isSelectedVolume(volume) {
  return Boolean(volume?.Name && selectedSummaryVolume.value?.Name === volume.Name)
}

function objectKeyCount(value) {
  return Object.keys(value || {}).length
}

// 获取容器数量
function getContainerCount(volume) {
  if (!volume.Containers) return 0
  return Object.keys(volume.Containers).length
}

// 短挂载点
function shortMountpoint(path) {
  if (!path) return ''
  if (path.length > 40) {
    return '...' + path.slice(-37)
  }
  return path
}

// 点击复制挂载点路径到剪贴板
async function copyMountpoint(path) {
  if (!path) return
  const ok = await copyToClipboard(path)
  if (ok) {
    uiStore.toastSuccess('已复制挂载点路径')
  } else {
    uiStore.toastError('复制失败，请手动复制')
  }
}

// 格式化日期
function formatDate(dateStr) {
  if (!dateStr) return ''
  return formatTime(dateStr)
}

// 计算属性
const filteredVolumes = computed(() => {
  let list = volumesList.value.filter(volume => (
    matchesBooleanFilter(getContainerCount(volume) > 0, volumeFilters.value.usage)
  ))
  
  // 搜索过滤
  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    list = list.filter(volume => {
      const name = (volume.Name || '').toLowerCase()
      const driver = (volume.Driver || '').toLowerCase()
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
          return item.Driver || 'local'
        case 'mountpoint':
          return item.Mountpoint || ''
        case 'containers':
          return getContainerCount(item)
        case 'created':
          return item.CreatedAt || ''
        default:
          return ''
      }
    })
  }
  
  // 默认排序：按名称字母顺序
  return [...list].sort((a, b) => {
    const nameA = a.Name || ''
    const nameB = b.Name || ''
    return nameA.localeCompare(nameB)
  })
})

watch(volumeFilters, () => {
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
const paginatedVolumes = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  const end = start + pageSize.value
  return filteredVolumes.value.slice(start, end)
})

// 批量删除或筛选导致总页数收缩时，把当前页钳制到最后一个非空页。
watch(() => Math.max(1, Math.ceil(filteredVolumes.value.length / pageSize.value)), pages => {
  if (currentPage.value > pages) currentPage.value = pages
})

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

async function fetchVolumes() {
  try {
    await dockerResources.loadVolumes()
    pruneCheckedVolumes()
  } catch (error) {
    console.error('获取卷列表失败:', error)
  }
}

async function refreshVolumes() {
  if (volumesBusy.value) return
  try {
    await dockerResources.loadVolumes({ force: true })
    pruneCheckedVolumes()
  } catch (error) {
    console.error('刷新卷列表失败:', error)
  }
}

function openCreateDialog() {
  createForm.value = {
    name: '',
    driver: 'local',
    customDriver: '',
    driverOptsText: '',
    labelsText: ''
  }
  showCreateDialog.value = true
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

async function handleCreate() {
  if (!createForm.value.name.trim()) {
    uiStore.toastWarning('请输入卷名称')
    return
  }
  
  try {
    const requestedDriver = createForm.value.driver
    const driver = requestedDriver === 'custom' ? createForm.value.customDriver.trim() : 'local'
    if (!driver) {
      uiStore.toastWarning('请输入自定义驱动名')
      return
    }
    const driverOpts = parseKeyValueText(createForm.value.driverOptsText)
    const labels = parseKeyValueText(createForm.value.labelsText)
    await volumes.create({
      name: createForm.value.name.trim(),
      driver,
      driverOpts,
      labels
    })
    showCreateDialog.value = false
    await refreshVolumes()
  } catch (error) {
    console.error('创建卷失败:', error)
    uiStore.toastError('创建失败: ' + (error.message || '未知错误'))
  }
}

async function handleRemove(volume) {
  const containerCount = getContainerCount(volume)
  if (containerCount > 0) {
    uiStore.toastWarning(`该卷有 ${containerCount} 个容器正在使用，无法删除`)
    return
  }

  const confirmed = await uiStore.confirm({
    type: 'danger',
    title: '删除卷',
    message: `确定删除卷 "${volume.Name}" 吗？`,
    confirmText: '删除'
  })
  if (!confirmed) {
    return
  }

  try {
    await volumes.remove(volume.Name)
    await refreshVolumes()
  } catch (error) {
    console.error('删除卷失败:', error)
    uiStore.toastError('删除失败: ' + (error.message || '未知错误'))
  }
}

async function pruneVolumes() {
  closeMoreMenu()
  const confirmed = await uiStore.confirm({
    type: 'danger',
    title: '清理无用卷',
    message: '确定清理所有无用的卷吗？此操作不可恢复。',
    confirmText: '清理'
  })
  if (!confirmed) {
    return
  }
  try {
    await volumes.prune()
    await refreshVolumes()
  } catch (error) {
    console.error('清理卷失败:', error)
    uiStore.toastError('清理失败: ' + (error.message || '未知错误'))
  }
}

async function handleBrowse(volume) {
  selectVolume(volume)
  currentBrowseVolume.value = volume
  showBrowseDialog.value = true
  browseUrl.value = ''
  browseSessionId.value = ''
  
  try {
    const res = await volumes.browseStart(volume.Name)
    // 后端返回 { sessionId, url, readOnly }
    browseSessionId.value = res?.sessionId || ''
    if (res?.url) {
      browseUrl.value = new URL(res.url, window.location.origin).toString()
    }
  } catch (error) {
    console.error('启动文件浏览器失败:', error)
    uiStore.toastError('启动文件浏览器失败: ' + (error.message || '未知错误'))
  }
}

function closeBrowseDialog() {
  if (browseSessionId.value) {
    volumes.browseClose(browseSessionId.value).catch(error => {
      console.warn('关闭文件浏览器会话失败:', error)
    })
  }
  showBrowseDialog.value = false
  browseUrl.value = ''
  browseSessionId.value = ''
  currentBrowseVolume.value = null
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
  fetchVolumes()
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  if (browseSessionId.value) {
    closeBrowseDialog()
  }
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.volume-batch-check {
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

.volumes-hero,
.resource-rail,
.toolbar {
  max-width: 1680px;
  width: 100%;
  margin-left: auto;
  margin-right: auto;
}

.volumes-hero {
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

.name-button {
  width: 100%;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--ops-ink);
  cursor: pointer;
  font: inherit;
  text-align: left;
}

.volume-icon-small {
  color: currentColor;
  flex-shrink: 0;
}

.cell-mono {
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.8125rem;
}

/* 可点击复制的文本：外观像普通文本，hover 时变主色提示可交互 */
.copyable-text {
  font-family: inherit;
  font-size: inherit;
  color: inherit;
  background: transparent;
  border: none;
  padding: 0;
  cursor: pointer;
  border-bottom: 1px dashed transparent;
  transition: color var(--motion-duration-quick) var(--motion-ease-out), border-color var(--motion-duration-quick) var(--motion-ease-out);
}
.copyable-text:hover {
  color: var(--color-primary-600, var(--color-primary));
  border-bottom-color: var(--color-primary-400, var(--color-primary));
}

.copyable-detail {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.copy-btn-inline {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 6px;
  border: 1px solid var(--border-default);
  background: var(--bg-secondary);
  color: var(--text-tertiary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}
.copy-btn-inline:hover {
  color: var(--color-primary-600, var(--color-primary));
  border-color: var(--color-primary-400, var(--color-primary));
}

.container-count {
  margin-left: 4px;
  color: var(--text-tertiary);
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

/* 浏览 - 浅蓝色 */
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
.form-textarea {
  width: 100%;
  padding: 10px 12px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--bg-elevated);
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
.form-textarea:focus {
  outline: none;
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.form-input::placeholder,
.form-textarea::placeholder {
  color: var(--text-tertiary);
}

select.form-input {
  cursor: pointer;
}

/* Modal 底部按钮 */
.action-btn {
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  border: none;
}

.action-btn.primary {
  background: var(--color-primary-500);
  color: var(--text-inverse);
}

.action-btn.primary:hover {
  background: var(--color-primary-600);
}

.action-btn.secondary {
  background: var(--bg-tertiary);
  color: var(--text-secondary);
}

.action-btn.secondary:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

/* 文件浏览器 */
.browse-container {
  height: 70vh;
  min-height: 500px;
  background: var(--bg-secondary);
  border-radius: 8px;
  overflow: hidden;
}

.browse-iframe {
  width: 100%;
  height: 100%;
  border: none;
}

.browse-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  gap: 16px;
  color: var(--text-secondary);
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

.container-id {
  font-family: 'JetBrains Mono', monospace;
  color: var(--text-secondary);
  font-size: 0.75rem;
}

/* Compose/容器工作台基准覆盖 */
.volumes-page {
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

.volumes-hero {
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

.content-scroll :deep(.volume-card) {
  min-height: 236px;
  aspect-ratio: auto;
  border-color: var(--ops-line);
  border-radius: 12px;
}

.content-scroll :deep(.volume-card.is-selected) {
  border-color: color-mix(in srgb, var(--ops-green) 62%, var(--ops-line));
  box-shadow:
    inset 4px 0 0 var(--ops-green),
    0 18px 42px color-mix(in srgb, var(--ops-green) 14%, transparent);
  animation: volume-card-select var(--motion-duration-fast) var(--motion-ease-out);
}

.data-table {
  min-width: 0;
  width: 100%;
  table-layout: fixed;
  font-size: 0.875rem;
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
  grid-template-columns: var(--resource-grid-template, minmax(170px, 1.35fr) 70px minmax(150px, 1fr) 92px 120px 82px);
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

.resource-row:active {
  background: var(--ops-row-active);
  transform: scale(0.992);
}

.resource-row.selected {
  animation: volume-row-select var(--motion-duration-fast) var(--motion-ease-out);
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

@keyframes volume-row-compress {
  0% { transform: none; }
  42% { transform: none; }
  100% { transform: none; }
}

@keyframes volume-row-select {
  0% { transform: scale(1); background: var(--ops-row-hover); }
  44% { transform: scale(0.988); background: var(--ops-row-hover); }
  100% { transform: scale(1); background: var(--ops-row-hover); }
}

@keyframes volume-card-select {
  0% { transform: translateY(0) scale(0.985); }
  70% { transform: translateY(0) scale(1.006); }
  100% { transform: translateY(0) scale(1); }
}

@media (prefers-reduced-motion: reduce) {
  .resource-row.selected,
  .content-scroll :deep(.volume-card.is-selected) {
    animation: none;
  }
}

.cell-mono,
.container-count {
  color: var(--ops-muted);
}

.volume-icon-small {
  color: var(--ops-green-strong);
}

.table-btn {
  width: 32px;
  height: 32px;
  border-radius: 8px;
}

/* 紧凑资源页头：全局 Header 已经承担页面定位 */
.volumes-hero {
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
