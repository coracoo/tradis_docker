<template>
  <Teleport to="body">
    <!-- 遮罩 -->
    <Transition name="overlay-fade">
      <div
        v-if="visible"
        class="search-overlay"
        @click="close"
      ></div>
    </Transition>
    
    <!-- 搜索面板 -->
    <Transition name="search">
      <div 
        v-if="visible" 
        class="global-search"
        role="dialog"
        aria-modal="true"
        aria-label="全局搜索"
      >
        <div class="search-box">
          <DynamicIcon name="search" :size="20" class="search-icon" />
          <input
            ref="inputRef"
            v-model="query"
            type="search"
            class="search-input"
            name="tradis-global-query"
            autocomplete="one-time-code"
            autocapitalize="none"
            spellcheck="false"
            data-1p-ignore
            data-lpignore="true"
            data-form-type="other"
            placeholder="搜索页面、功能或 Docker 资源..."
            aria-label="搜索关键词"
            aria-autocomplete="list"
            :aria-controls="results.length > 0 ? 'search-results-list' : null"
            :aria-activedescendant="selectedIndex >= 0 ? `result-item-${selectedIndex}` : null"
            @keyup.enter="handleEnter"
            @keyup.esc="close"
            @keydown.down.prevent="moveSelection(1)"
            @keydown.up.prevent="moveSelection(-1)"
          />
          <div class="shortcut-hint">
            <span class="key">ESC</span>
            <span>关闭</span>
          </div>
        </div>
        
        <div class="search-content">
          <!-- 搜索中 -->
          <div v-if="loading && results.length === 0" class="loading-state" role="status" aria-live="polite">
            <MatrixLoader variant="scan" label="正在搜索" />
            <span>搜索中...</span>
          </div>
          
          <!-- 空状态 -->
          <div v-else-if="query && results.length === 0" class="empty-state" role="status" aria-live="polite">
            <DynamicIcon name="search" :size="48" />
            <p>未找到结果</p>
            <span class="empty-hint">尝试其他关键词</span>
          </div>
          
          <!-- 快捷链接 -->
          <div v-else-if="!query" class="quick-links">
            <div class="section-title">快捷链接</div>
            <div class="links-grid">
              <button 
                v-for="link in quickLinks" 
                :key="link.id"
                class="link-item"
                @click="navigate(link.route)"
              >
                <DynamicIcon :name="link.icon" :size="18" />
                <span>{{ link.label }}</span>
              </button>
            </div>
          </div>
          
          <!-- 搜索结果 -->
          <div v-else class="search-results" id="search-results-list" role="listbox" aria-label="搜索结果">
            <div
              v-for="(group, index) in groupedResults"
              :key="group.type"
              class="result-group"
              role="group"
              :aria-label="group.label"
            >
              <div class="group-title">{{ group.label }}</div>
              <div class="group-items">
                <button
                  v-for="(item, itemIndex) in group.items"
                  :key="item.id"
                  :id="`result-item-${getGlobalIndex(index, itemIndex)}`"
                  class="result-item"
                  :class="{ 
                    active: selectedIndex === getGlobalIndex(index, itemIndex),
                    highlight: item.matched 
                  }"
                  role="option"
                  :aria-selected="selectedIndex === getGlobalIndex(index, itemIndex)"
                  @click="selectItem(item)"
                  @mouseenter="selectedIndex = getGlobalIndex(index, itemIndex)"
                >
                  <DynamicIcon :name="item.icon || getItemIcon(group.type)" :size="16" />
                  <div class="item-info">
                    <span class="item-name" v-html="highlightText(item.name)"></span>
                    <span v-if="item.description" class="item-desc">{{ item.description }}</span>
                  </div>
                  <span class="item-type">{{ group.label }}</span>
                </button>
              </div>
            </div>
          </div>
        </div>
        
        <div class="search-footer">
          <div class="footer-hints">
            <div class="hint">
              <span class="key">↑↓</span>
              <span>选择</span>
            </div>
            <div class="hint">
              <span class="key">↵</span>
              <span>打开</span>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import DynamicIcon from '../ui/DynamicIcon.vue'
import MatrixLoader from '../ui/MatrixLoader.vue'
import { listContainers } from '@/api/containers.js'
import { listImages } from '@/api/images.js'
import { listVolumes } from '@/api/volumes.js'
import { listNetworks } from '@/api/networks.js'
import { listCompose } from '@/api/compose.js'
import { listNavigation } from '@/api/navigation.js'
import { listScheduledTasks } from '@/api/scheduledTasks.js'
import {
  GLOBAL_SEARCH_PAGES,
  GLOBAL_SEARCH_TYPE_ICONS,
  GLOBAL_SEARCH_TYPE_LABELS,
  loadEditionSearchData,
  searchEditionResources,
  searchGlobalCatalog
} from '@edition/global-search-catalog'
import { searchEditionManagedResources, searchLocalManagedResources } from '@edition/global-search-resources'

const props = defineProps({
  visible: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['close'])

const router = useRouter()
const SEARCH_CACHE_TTL = 30 * 1000

const query = ref('')
const loading = ref(false)
const results = ref([])
const selectedIndex = ref(0)
const inputRef = ref(null)
const searchDataset = ref({
  containers: [],
  images: [],
  volumes: [],
  networks: [],
  composes: [],
  nasDevices: [],
  tutorials: [],
  navigationApps: [],
  protectionPolicies: [],
    scheduledTasks: []
})
const searchDatasetFetchedAt = ref(0)
let searchTimeout = null
let preloadPromise = null

// 快捷链接
const quickLinkIds = new Set([
  'page-containers',
  'page-compose',
  'page-images',
  'page-volumes',
  'page-networks',
  'page-appstore',
  'page-navigation',
  'page-settings'
])
const quickLinks = GLOBAL_SEARCH_PAGES
  .filter(item => quickLinkIds.has(item.id))
  .map(item => ({ ...item, label: item.name }))

// 分组结果
const groupedResults = computed(() => {
  const groups = {}
  
  results.value.forEach(item => {
    if (!groups[item.type]) {
      groups[item.type] = {
        type: item.type,
        label: getTypeLabel(item.type),
        items: []
      }
    }
    groups[item.type].items.push(item)
  })
  
  return Object.values(groups)
})

// 获取类型标签
function getTypeLabel(type) {
  return GLOBAL_SEARCH_TYPE_LABELS[type] || type
}

// 获取项目图标
function getItemIcon(type) {
  return GLOBAL_SEARCH_TYPE_ICONS[type] || 'box'
}

// 高亮匹配文本
function highlightText(text) {
  if (!query.value) return text
  const regex = new RegExp(`(${escapeRegex(query.value)})`, 'gi')
  return text.replace(regex, '<mark>$1</mark>')
}

function escapeRegex(str) {
  return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// 获取全局索引
function getGlobalIndex(groupIndex, itemIndex) {
  let index = 0
  for (let i = 0; i < groupIndex; i++) {
    index += groupedResults.value[i].items.length
  }
  return index + itemIndex
}

// 移动选择
function moveSelection(delta) {
  const total = results.value.length
  if (total === 0) return
  
  selectedIndex.value = (selectedIndex.value + delta + total) % total
}

async function ensureSearchDataset(force = false) {
  const now = Date.now()
  if (!force && searchDatasetFetchedAt.value && now - searchDatasetFetchedAt.value < SEARCH_CACHE_TTL) {
    return searchDataset.value
  }
  if (!force && preloadPromise) {
    return preloadPromise
  }

  preloadPromise = Promise.all([
    listContainers({ all: true }).catch(() => []),
    listImages().catch(() => []),
    listVolumes().catch(() => ({ volumes: [] })),
    listNetworks().catch(() => []),
    listCompose().catch(() => []),
    listNavigation({ include_deleted: true }).catch(() => []),
    listScheduledTasks().catch(() => ({ jobs: [] })),
    loadEditionSearchData()
  ])
    .then(([
      containers,
      images,
      volumes,
      networks,
      composes,
      navigationApps,
      scheduledTasks,
      editionData
    ]) => {
      searchDataset.value = {
        containers: Array.isArray(containers) ? containers : [],
        images: Array.isArray(images) ? images : [],
        volumes: Array.isArray(volumes?.volumes || volumes?.Volumes)
          ? (volumes.volumes || volumes.Volumes)
          : [],
        networks: Array.isArray(networks) ? networks : [],
        composes: Array.isArray(composes) ? composes : [],
        navigationApps: Array.isArray(navigationApps) ? navigationApps : [],
        scheduledTasks: Array.isArray(scheduledTasks?.jobs) ? scheduledTasks.jobs : [],
        ...editionData
      }
      searchDatasetFetchedAt.value = Date.now()
      return searchDataset.value
    })
    .finally(() => {
      preloadPromise = null
    })

  return preloadPromise
}

async function doSearch() {
  const q = query.value.trim().toLowerCase()
  if (!q) {
    results.value = []
    return
  }
  
  loading.value = true
  const catalogItems = searchGlobalCatalog(q)
  results.value = catalogItems
  
  try {
    const dataset = await ensureSearchDataset()
    const containers = searchContainers(dataset.containers, q)
    const images = searchImages(dataset.images, q)
    const volumes = searchVolumes(dataset.volumes, q)
    const networks = searchNetworks(dataset.networks, q)
    const composes = searchCompose(dataset.composes, q)
    const managedResources = [
      ...searchLocalManagedResources(dataset, q),
      ...searchEditionManagedResources(dataset, q)
    ]
    const editionResults = searchEditionResources(dataset, q)

    results.value = [
      ...catalogItems,
      ...containers,
      ...composes,
      ...images,
      ...volumes,
      ...networks,
      ...editionResults,
      ...managedResources
    ]
    
    selectedIndex.value = 0
  } catch (e) {
    console.error('搜索失败:', e)
  } finally {
    loading.value = false
  }
}

function searchContainers(list, q) {
  if (!Array.isArray(list)) return []
  return list
    .filter(c => c.Names?.[0]?.toLowerCase().includes(q) || c.Image?.toLowerCase().includes(q))
    .slice(0, 5)
    .map(c => ({
      id: c.Id,
      type: 'container',
      name: c.Names?.[0]?.replace(/^\//, '') || c.Id?.slice(0, 12),
      description: c.Image,
      status: c.State,
      route: { path: '/containers', query: { focus: c.Id } }
    }))
}

function searchCompose(list, q) {
  if (!Array.isArray(list)) return []
  return list
    .filter(p => p.name?.toLowerCase().includes(q))
    .slice(0, 5)
    .map(p => ({
      id: p.id || p.name,
      type: 'compose',
      name: p.name,
      description: p.status || 'Compose 项目',
      link: '/compose'
    }))
}

function searchImages(list, q) {
  if (!Array.isArray(list)) return []
  return list
    .filter(i => i.RepoTags?.[0]?.toLowerCase().includes(q) || i.Id?.toLowerCase().includes(q))
    .slice(0, 5)
    .map(i => ({
      id: i.Id,
      type: 'image',
      name: i.RepoTags?.[0] || i.Id?.slice(7, 19),
      description: formatBytes(i.Size),
      link: '/images'
    }))
}

// 格式化字节
function formatBytes(bytes) {
  if (!bytes || bytes < 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = bytes
  let unitIndex = 0
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024
    unitIndex++
  }
  return `${size.toFixed(1)} ${units[unitIndex]}`
}

function searchVolumes(list, q) {
  if (!Array.isArray(list)) return []
  return list
    .filter(v => v.Name?.toLowerCase().includes(q))
    .slice(0, 5)
    .map(v => ({
      id: v.Name,
      type: 'volume',
      name: v.Name,
      description: v.Driver || 'local',
      link: '/volumes'
    }))
}

function searchNetworks(list, q) {
  if (!Array.isArray(list)) return []
  return list
    .filter(n => n.Name?.toLowerCase().includes(q))
    .slice(0, 5)
    .map(n => ({
      id: n.Id,
      type: 'network',
      name: n.Name,
      description: n.Driver || 'bridge',
      link: '/networks'
    }))
}

// 跳转
function navigate(routeLocation) {
  router.push(routeLocation)
  close()
}

// 选择项目
function selectItem(item) {
  if (item.route) {
    navigate(item.route)
  } else if (item.link) {
    navigate(item.link)
  }
}

// 回车
function handleEnter() {
  const item = results.value[selectedIndex.value]
  if (item) {
    selectItem(item)
  }
}

// 关闭
function close() {
  query.value = ''
  results.value = []
  emit('close')
}

// 监听可见性
watch(() => props.visible, async (val) => {
  if (val) {
    await nextTick()
    inputRef.value?.focus()
    ensureSearchDataset()
    if (query.value.trim()) {
      doSearch()
    }
  }
})

watch(query, () => {
  if (searchTimeout) {
    clearTimeout(searchTimeout)
  }
  searchTimeout = setTimeout(() => {
    doSearch()
  }, 250)
})

// 键盘快捷键
function handleKeydown(e) {
  // Cmd/Ctrl + K 打开搜索
  if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
    e.preventDefault()
    if (!props.visible) {
      emit('open')
    }
  }
}

onMounted(() => {
  document.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  if (searchTimeout) {
    clearTimeout(searchTimeout)
  }
  document.removeEventListener('keydown', handleKeydown)
})
</script>

<style scoped>
.search-overlay {
  position: fixed;
  inset: 0;
  background: var(--overlay-bg);
  backdrop-filter: blur(4px);
  z-index: 299;
}

.global-search {
  position: fixed;
  top: 15%;
  left: 50%;
  transform: translateX(-50%);
  width: 640px;
  max-width: 90vw;
  max-height: 70vh;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 16px;
  box-shadow: 0 25px 50px -12px var(--shadow-lg);
  z-index: 300;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.search-box {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border-subtle);
}

.search-icon {
  color: var(--text-tertiary);
  flex-shrink: 0;
}

.search-input {
  flex: 1;
  font-size: 1.125rem;
  color: var(--text-primary);
  background: transparent;
  border: none;
  outline: none;
}

.search-input::placeholder {
  color: var(--text-tertiary);
}

.shortcut-hint {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 0.75rem;
  color: var(--text-tertiary);
  flex-shrink: 0;
}

.key {
  font-family: 'JetBrains Mono', monospace;
  padding: 2px 6px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  font-size: 0.625rem;
}

.search-content {
  flex: 1;
  overflow-y: auto;
  min-height: 200px;
  max-height: 50vh;
}

.loading-state,
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  color: var(--text-tertiary);
  gap: 12px;
}

.empty-hint {
  font-size: 0.875rem;
}

.quick-links {
  padding: 16px;
}

.section-title {
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-tertiary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 12px;
}

.links-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
}

.link-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 16px 8px;
  border-radius: 10px;
  background: var(--bg-secondary);
  border: none;
  color: var(--text-secondary);
  font-size: 0.8125rem;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  user-select: none;
}

.link-item:hover {
  background: var(--color-primary-50);
  color: var(--color-primary-600);
  transform: translateY(-2px);
}

/* P2: 快捷链接 active 态 */
.link-item:active {
  transform: scale(0.96);
  background: var(--color-primary-100);
  transition-duration: var(--motion-duration-micro);
}

.search-results {
  padding: 8px;
}

.result-group {
  margin-bottom: 8px;
}

.group-title {
  padding: 8px 12px;
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-tertiary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.group-items {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.result-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 8px;
  background: transparent;
  border: none;
  text-align: left;
  cursor: pointer;
  transition: all 0.15s ease;
}

.result-item:hover,
.result-item.active {
  background: var(--bg-secondary);
}

.result-item :deep(mark) {
  background: var(--color-primary-200);
  color: var(--color-primary-800);
  padding: 0 2px;
  border-radius: 2px;
}

.item-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.item-name {
  font-size: 0.875rem;
  color: var(--text-primary);
  font-weight: 500;
}

.item-desc {
  font-size: 0.75rem;
  color: var(--text-tertiary);
}

.item-type {
  font-size: 0.75rem;
  color: var(--text-tertiary);
  padding: 2px 8px;
  background: var(--bg-tertiary);
  border-radius: 4px;
}

.search-footer {
  padding: 12px 16px;
  border-top: 1px solid var(--border-subtle);
  background: var(--bg-secondary);
}

.footer-hints {
  display: flex;
  align-items: center;
  gap: 16px;
  justify-content: center;
}

.hint {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 0.75rem;
  color: var(--text-tertiary);
}

.search-enter-active,
.search-leave-active {
  transition:
    opacity var(--motion-duration-panel) var(--motion-ease-out),
    transform var(--motion-duration-panel) var(--motion-ease-out);
}

.search-enter-from,
.search-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(calc(var(--motion-distance-medium) * -1)) scale(var(--motion-scale-enter));
}

.overlay-fade-enter-active,
.overlay-fade-leave-active {
  transition: opacity var(--motion-duration-quick) var(--motion-ease-out);
}

.overlay-fade-enter-from,
.overlay-fade-leave-to {
  opacity: 0;
}

/* 响应式 */
@media (max-width: 768px) {
  .global-search {
    top: auto;
    bottom: 0;
    left: 0;
    right: 0;
    transform: none;
    width: 100%;
    max-width: 100%;
    max-height: 80vh;
    border-radius: 16px 16px 0 0;
  }

  .search-enter-from,
  .search-leave-to {
    transform: translateY(var(--motion-distance-medium)) scale(var(--motion-scale-enter));
  }
  
  .links-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
