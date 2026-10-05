<template>
  <ResourceWorkbench>
    <template #toolbar>
      <div class="toolbar workbench-toolbar">
        <div class="toolbar-left workbench-toolbar-left">
        <SearchInput
          v-model="searchQuery"
          placeholder="搜索应用名称或描述..."
          @search="onSearch"
        />
        <!-- 排序控件 -->
        <div class="sort-control custom-select" v-click-outside="() => sortDropdownOpen = false">
          <div class="select-trigger" @click="sortDropdownOpen = !sortDropdownOpen">
            <span>{{ sortLabel }}</span>
            <svg 
              xmlns="http://www.w3.org/2000/svg" 
              width="16" 
              height="16" 
              viewBox="0 0 24 24" 
              fill="none" 
              stroke="currentColor" 
              stroke-width="2" 
              stroke-linecap="round" 
              stroke-linejoin="round"
              :class="{ 'is-open': sortDropdownOpen }"
            >
              <polyline points="6 9 12 15 18 9"/>
            </svg>
          </div>
          <div v-show="sortDropdownOpen" class="select-dropdown">
            <div 
              v-for="opt in sortOptionsList" 
              :key="opt.value"
              class="select-option"
              :class="{ active: sortValue === opt.value }"
              @click="handleSortSelect(opt.value)"
            >
              <span class="option-label">{{ opt.label }}</span>
              <span v-if="sortValue === opt.value" class="option-check">
                <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
              </span>
            </div>
          </div>
        </div>
        <!-- 分类筛选 -->
        <div class="category-tabs-wrapper">
          <ScrollableTabs
            v-model="selectedCategory"
            :options="categoryFilterOptions"
            aria-label="分类筛选"
            compact
          />
        </div>
        </div>
        <div class="toolbar-right">
        <div
          class="store-status-chip"
          :class="`is-${appStoreIndicatorTone}`"
          :title="appStoreTooltip"
          :aria-label="appStoreTooltip"
        >
          <span class="store-status-dot"></span>
          <span class="store-status-text">商城 {{ appStoreIndicatorLabel }}</span>
        </div>
        <div
          class="store-status-chip meta-status-chip"
          :class="`is-${appStoreMetaTone}`"
          :title="appStoreMetaTooltip"
          :aria-label="appStoreMetaTooltip"
        >
          <span class="store-status-dot"></span>
          <span class="store-status-text">模板 {{ appStoreMetaLabel }}</span>
        </div>
        <button
          v-ripple
          class="refresh-btn resource-refresh-btn"
          :class="{ 'is-spinning': loading }"
          :disabled="loading"
          title="强制刷新应用列表"
          @click="refreshAppStore"
        >
          <DynamicIcon class="resource-refresh-icon" name="refresh" :size="18" />
        </button>
        </div>
      </div>
    </template>

    <div class="appstore-page" role="main" aria-label="应用商店">

    <!-- 应用网格 -->
    <div v-if="loading" class="loading-grid">
      <div v-for="i in 12" :key="i" class="app-card skeleton">
        <div class="app-left">
          <div class="app-skeleton-logo"></div>
          <div class="app-skeleton-deploy"></div>
        </div>
        <div class="app-content">
          <div class="app-header">
            <div class="header-left">
              <div class="app-skeleton-title"></div>
              <div class="app-skeleton-version"></div>
            </div>
            <div class="app-skeleton-category"></div>
          </div>
          <div class="app-skeleton-description">
            <span></span>
            <span></span>
            <span class="short"></span>
          </div>
        </div>
      </div>
    </div>

    <div v-else-if="filteredApps.length === 0" class="empty-state-wrapper">
      <EmptyState
        icon="sparkles"
        title="暂无应用"
        :description="emptyMessage"
      />
    </div>

    <div v-else class="apps-grid-container motion-reveal" @scroll="handleScroll">
      <div class="apps-grid">
        <div
          v-for="(app, index) in filteredApps"
          :key="app.id"
          class="app-card"
          @click="handleOpenAppDetail(app)"
        >
          <!-- 左侧：Icon + 部署按钮 -->
          <div class="app-left">
            <DynamicAppIcon
              :name="app.name"
              :src="app.logo"
              size="lg"
              rounded="10px"
            />
            <button
              class="deploy-btn"
              :class="{ 'is-loading': deployingAppId === app.id }"
              :disabled="deployingAppId"
              :data-tour="index === 0 ? 'appstore-deploy' : undefined"
              @click.stop="goToDeploy(app)"
            >
              <span v-if="deployingAppId === app.id" class="btn-spinner"></span>
              <span class="btn-text">{{ deployingAppId === app.id ? '加载' : '部署' }}</span>
            </button>
          </div>

          <!-- 右侧：头部信息 + 描述 -->
          <div class="app-content">
            <div class="app-header">
              <h3 class="app-name" :title="app.name">{{ app.name }}</h3>
              <span v-if="shouldShowDeployCount(app)" class="deploy-count">
                <DynamicIcon name="download" :size="12" />
                {{ formatDeployCount(app.deployment_count) }}
              </span>
              <span class="app-version">{{ app.version || 'latest' }}</span>
              <span class="app-category">{{ getCategoryCN(app.category) }}</span>
            </div>
            <AppDescriptionTooltip :text="app.description" />
          </div>
        </div>
      </div>
      
      <!-- 加载更多 -->
      <div v-if="hasMore || loadingMore" class="load-more">
        <button v-if="!loadingMore" class="load-more-btn" @click="loadMore">
          加载更多 ({{ filteredApps.length }} / {{ sortedApps.length }})
        </button>
        <div v-else class="loading-spinner">
          <span class="spinner"></span>
          <span>加载中...</span>
        </div>
      </div>
      
      <!-- 已加载全部 -->
      <div v-else-if="sortedApps.length > pageSize" class="load-more all-loaded">
        已加载全部 {{ sortedApps.length }} 个应用
      </div>
    </div>

    <!-- 应用详情对话框 -->
    <Modal
      v-model:visible="showDetailModal"
      :title="detailLoading ? '加载中...' : (selectedApp?.name || '应用详情')"
      width="800px"
    >
      <!-- Loading 状态 -->
      <div v-if="detailLoading" class="detail-loading">
        <div class="detail-skeleton">
          <div class="skeleton-header">
            <div class="skeleton-logo"></div>
            <div class="skeleton-info">
              <div class="skeleton-title"></div>
              <div class="skeleton-meta"></div>
            </div>
          </div>
          <div class="skeleton-body">
            <div class="skeleton-line"></div>
            <div class="skeleton-line"></div>
            <div class="skeleton-line short"></div>
          </div>
        </div>
      </div>
      
      <div v-else-if="selectedApp" class="app-detail">
        <div class="detail-header">
          <div class="header-left">
            <DynamicAppIcon 
              :name="selectedApp.name" 
              :src="selectedApp.logo" 
              size="xl" 
              rounded="12px"
            />
            <div class="detail-info">
              <div class="info-row">
                <h2>{{ selectedApp.name }}</h2>
                <span class="category-badge">{{ getCategoryCN(selectedApp.category) }}</span>
                <span class="version">{{ selectedApp.version || 'latest' }}</span>
                <span v-if="shouldShowDeployCount(selectedApp)" class="deploy-count">
                  <DynamicIcon name="download" :size="14" />
                  {{ formatDeployCount(selectedApp.deployment_count) }}
                </span>
                <a v-if="selectedApp.website" :href="selectedApp.website" target="_blank" class="link-btn">
                  <DynamicIcon name="globe" :size="14" />
                  官网
                </a>
                <a v-if="selectedApp.tutorial" :href="selectedApp.tutorial" target="_blank" class="link-btn">
                  <DynamicIcon name="book" :size="14" />
                  文档
                </a>
              </div>
            </div>
          </div>
          <button class="btn btn-primary btn-deploy" @click="goToDeploy(selectedApp)">
            <DynamicIcon name="rocket" :size="18" />
            立即部署
          </button>
        </div>

        <div class="detail-body">
          <div class="detail-description">
            <h4>应用介绍</h4>
            <p>{{ selectedApp.description || '暂无介绍' }}</p>
          </div>

          <!-- 截图展示 -->
          <div v-if="selectedApp.screenshots?.length" class="detail-screenshots">
            <h4>截图预览</h4>
            <div class="screenshot-list">
              <img
                v-for="(url, idx) in selectedApp.screenshots"
                :key="idx"
                :src="url"
                @click="previewImage(url)"
              />
            </div>
          </div>
        </div>
      </div>

      <template #footer>
        <button class="btn btn-secondary" @click="showDetailModal = false">关闭</button>
      </template>
    </Modal>

    <!-- 图片预览 -->
    <Teleport to="body">
      <div
        v-if="previewImageUrl"
        class="image-preview-overlay"
        :style="{ zIndex: previewOverlayZIndex }"
        role="dialog"
        aria-modal="true"
        aria-label="应用截图预览"
        @click="closeImagePreview"
      >
        <div ref="previewPanelRef" class="image-preview-panel" tabindex="-1" @click.stop>
          <img :src="previewImageUrl" alt="应用截图预览" />
          <button class="close-preview" aria-label="关闭图片预览" @click="closeImagePreview">
            <DynamicIcon name="x" :size="24" />
          </button>
        </div>
      </div>
    </Teleport>
    </div>
  </ResourceWorkbench>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import SearchInput from '@/components/ui/SearchInput.vue'
import ScrollableTabs from '@/components/ui/ScrollableTabs.vue'
import Modal from '@/components/feedback/Modal.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import DynamicAppIcon from '@/components/ui/DynamicAppIcon.vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import ResourceWorkbench from '@/components/resource-workbench/ResourceWorkbench.vue'
import AppDescriptionTooltip from './components/AppDescriptionTooltip.vue'
import { useAppStore } from '@/composables/useAppStore.js'
import { formatTime } from '@/utils/format.js'
import { useOverlayController } from '@/composables/useOverlayController.js'

const router = useRouter()
const previewPanelRef = ref(null)
const previewImageUrl = ref(null)

function closeImagePreview() {
  previewImageUrl.value = null
}

const { overlayZIndex: previewOverlayZIndex } = useOverlayController({
  visible: computed(() => Boolean(previewImageUrl.value)),
  containerRef: previewPanelRef,
  closeOnEscape: true,
  onClose: closeImagePreview
})

// 使用 useAppStore composable
const {
  // 状态
  loading,
  appStoreStatus,
  statusLoading,
  appStoreMetaStatus,
  metaStatusLoading,
  selectedCategory,
  selectedApp,
  detailLoading,
  lastFetchAt,
  // 分页
  pageSize,
  hasMore,
  loadingMore,
  sortedApps,
  // 排序
  sortValue,
  sortOptionsList,
  sortLabel,
  // 计算属性
  categories,
  filteredApps,
  emptyMessage,
  // 方法
  getCategoryCN,
  fetchStatus,
  fetchMetaStatus,
  fetchApps,
  openAppDetail,
  loadMore,
  handleSortChange,
  handleSearch,
  truncateDescription,
  formatDeployCount
} = useAppStore()

// 页面状态
const showDetailModal = ref(false)
const deployingAppId = ref(null)
const sortDropdownOpen = ref(false)
const searchQuery = ref('')

const categoryFilterOptions = computed(() =>
  categories.value.map(cat => ({ value: cat.id, label: cat.name, count: cat.count }))
)

const appStoreIndicatorTone = computed(() => appStoreStatus.value?.level || 'error')

const appStoreIndicatorLabel = computed(() => {
  if (appStoreStatus.value?.connected && appStoreStatus.value?.cdnConfigured) return 'CDN'
  if (appStoreStatus.value?.originConfigured) return '源站'
  if (appStoreStatus.value?.connected) return '缓存'
  return '离线'
})

const appStoreTooltip = computed(() => {
  const parts = [
    `状态：${appStoreStatus.value?.label || '未知'}`,
    `模式：${appStoreStatus.value?.modeLabel || '未知'}`
  ]
  if (appStoreStatus.value?.summary) parts.push(`详情：${appStoreStatus.value.summary}`)
  if (appStoreStatus.value?.cache?.listAppCount > 0) parts.push(`列表缓存：${appStoreStatus.value.cache.listAppCount} 项`)
  if (appStoreStatus.value?.cache?.cachedDetailFileCount > 0) parts.push(`详情缓存：${appStoreStatus.value.cache.cachedDetailFileCount} 项`)
  if (statusLoading.value) parts.push('状态刷新中...')
  return parts.join('\n')
})

function shortHash(value) {
  if (!value) return '无'
  return value.length > 20 ? `${value.slice(0, 12)}…${value.slice(-6)}` : value
}

function shouldShowDeployCount(app) {
  if (!app || Number(app.deployment_count || 0) <= 0) return false
  if (app.show_deployment_count === false) return false
  if (app.show_deployment_count === true) return true
  const source = String(app.source || app.template_source || '').trim().toLowerCase()
  const uid = String(app.template_uid || app.source_template_id || '').trim().toLowerCase()
  if (source !== 'official') return false
  if (uid.startsWith('custom:') || uid.startsWith('local:') || uid.startsWith('remote:')) return false
  return true
}

const appStoreMetaTone = computed(() => {
  if (metaStatusLoading.value) return 'warning'
  const state = appStoreMetaStatus.value?.state
  if (state === 'synced') return 'success'
  if (state === 'empty' || state === 'unknown') return 'error'
  if (appStoreMetaStatus.value?.needsRefresh || state === 'missing_local') return 'warning'
  if (state === 'cache_only') return 'warning'
  return 'error'
})

const appStoreMetaLabel = computed(() => {
  switch (appStoreMetaStatus.value?.state) {
    case 'synced':
      return '已同步'
    case 'outdated':
      return '有更新'
    case 'missing_local':
      return '待缓存'
    case 'cache_only':
      return '仅缓存'
    case 'empty':
      return '无数据'
    case 'unknown':
      return '未获取'
    default:
      return '未知'
  }
})

const appStoreMetaTooltip = computed(() => {
  const local = appStoreMetaStatus.value?.local
  const remote = appStoreMetaStatus.value?.remote
  const parts = [
    `状态：${appStoreMetaLabel.value}`,
    `说明：${appStoreMetaStatus.value?.summary || '尚未获取模板版本信息'}`
  ]
  if (local?.version_hash || local?.versionHash) {
    parts.push(`本地哈希：${shortHash(local.version_hash || local.versionHash)}`)
  }
  if (local?.fetched_at || local?.fetchedAt || lastFetchAt.value) {
    parts.push(`本地更新时间：${formatTime(local?.fetched_at || local?.fetchedAt || lastFetchAt.value)}`)
  }
  if (remote?.version_hash || remote?.versionHash) {
    parts.push(`远端哈希：${shortHash(remote.version_hash || remote.versionHash)}`)
  }
  if (remote?.generated_at || remote?.generatedAt) {
    parts.push(`远端生成时间：${formatTime(remote.generated_at || remote.generatedAt)}`)
  }
  if (remote?.source) {
    parts.push(`远端来源：${remote.source}`)
  }
  if (metaStatusLoading.value) {
    parts.push('模板状态刷新中...')
  }
  return parts.join('\n')
})

// 排序选择处理
function handleSortSelect(value) {
  sortDropdownOpen.value = false
  handleSortChange(value)
}

// v-click-outside 指令
const vClickOutside = {
  mounted(el, binding) {
    el._clickOutside = (e) => {
      if (!el.contains(e.target)) binding.value()
    }
    document.addEventListener('click', el._clickOutside)
  },
  unmounted(el) {
    document.removeEventListener('click', el._clickOutside)
  }
}

// 滚动加载
function handleScroll(e) {
  const container = e.target
  const scrollBottom = container.scrollTop + container.clientHeight
  const threshold = container.scrollHeight - 100
  
  if (scrollBottom >= threshold && !loadingMore.value && hasMore.value) {
    loadMore()
  }
}

// 打开应用详情
async function handleOpenAppDetail(app) {
  await openAppDetail(app)
  showDetailModal.value = true
}

// 跳转到部署页面
function goToDeploy(app) {
  if (deployingAppId.value) return
  
  // 先关闭弹窗（如果打开的话），然后立即跳转
  showDetailModal.value = false
  deployingAppId.value = app.id
  
  // 使用 nextTick 确保弹窗关闭后再跳转，避免闪烁
  router.push(`/appstore/deploy/${app.id}`)
}

// 图片预览
function previewImage(url) {
  previewImageUrl.value = url
}

// 搜索处理
function onSearch() {
  handleSearch(searchQuery.value)
}

async function refreshAppStore() {
  await Promise.allSettled([fetchStatus(), fetchApps({ force: true })])
  await fetchMetaStatus({ force: true })
}

async function initializeAppStore() {
  await Promise.allSettled([fetchStatus(), fetchApps()])
  await fetchMetaStatus()
}

onMounted(() => {
  initializeAppStore()
})
</script>

<style scoped>
.appstore-page {
  box-sizing: border-box;
  height: 100%;
  display: flex;
  flex-direction: column;
  min-height: 0;
  gap: 14px;
  padding-bottom: 22px;
  color: var(--resource-ink);
}

/* 工具栏 */
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin: 0;
}

.store-status-chip {
  display: flex;
  align-items: center;
  gap: 7px;
  height: 34px;
  padding: 0 10px;
  border: 1px solid var(--border-subtle);
  border-radius: 999px;
  background: var(--bg-secondary);
}

.store-status-chip.is-success {
  border-color: var(--color-success-300);
  background: color-mix(in srgb, var(--color-success-500) 8%, var(--bg-secondary));
}

.store-status-chip.is-warning {
  border-color: var(--color-warning-300);
  background: color-mix(in srgb, var(--color-warning-500) 10%, var(--bg-secondary));
}

.store-status-chip.is-error {
  border-color: var(--color-danger-300);
  background: color-mix(in srgb, var(--color-danger-500) 8%, var(--bg-secondary));
}

.store-status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
}

.store-status-chip.is-success {
  color: var(--color-success-600);
}

.store-status-chip.is-warning {
  color: var(--color-warning-700);
}

.store-status-chip.is-error {
  color: var(--color-danger-600);
}

.store-status-text {
  color: var(--text-primary);
  font-size: 0.68rem;
  font-weight: 700;
  white-space: nowrap;
}

.toolbar-left {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  min-width: 0;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 0 0 auto;
}

.refresh-btn {
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

.refresh-btn:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

.refresh-btn svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
}

.refresh-btn.is-spinning svg {
  animation: resource-refresh-spin 0.75s linear infinite;
}

/* 分类标签 */
/* 排序控件 - 自定义下拉 */
.sort-control {
  position: relative;
}

.sort-control.custom-select .select-trigger {
  width: 100%;
  min-width: 0;
  min-height: 36px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  color: var(--text-primary);
  font-size: 0.8125rem;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  justify-content: space-between;
}

.sort-control.custom-select .select-trigger:hover {
  color: var(--resource-ink);
  background: transparent;
}

.sort-control.custom-select .select-trigger svg {
  transition: transform var(--motion-duration-quick) var(--motion-ease-out);
  color: var(--text-tertiary);
}

.sort-control.custom-select .select-trigger svg.is-open {
  transform: rotate(180deg);
}

.select-dropdown {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  min-width: 140px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.15);
  z-index: 100;
  padding: 4px;
}

.select-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-radius: 6px;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  font-size: 0.875rem;
  color: var(--text-secondary);
}

.select-option:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

.select-option.active {
  color: var(--color-primary-600);
  font-weight: 500;
}

.option-check {
  display: flex;
  align-items: center;
  justify-content: center;
}

.category-tabs-wrapper {
  position: relative;
  flex: 1;
  min-width: 0;
  overflow: hidden;
}

/* 加载骨架屏 */
.loading-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 12px;
}

.app-card.skeleton {
  cursor: default;
  animation: none;
}

.app-card.skeleton:hover,
.app-card.skeleton:active {
  transform: none;
  border-color: var(--border-subtle);
  box-shadow: none;
}

.app-skeleton-logo {
  width: 56px;
  height: 56px;
  border-radius: 12px;
  flex-shrink: 0;
}

.app-skeleton-deploy {
  width: 56px;
  height: 28px;
  border-radius: 8px;
}

.app-skeleton-title {
  width: min(42%, 130px);
  height: 15px;
  border-radius: 4px;
}

.app-skeleton-version {
  width: 42px;
  height: 12px;
  border-radius: 4px;
}

.app-skeleton-category {
  width: 38px;
  height: 20px;
  border-radius: 4px;
}

.app-skeleton-description {
  display: flex;
  flex-direction: column;
  gap: 7px;
  padding-top: 3px;
}

.app-skeleton-description span {
  width: 100%;
  height: 10px;
  border-radius: 4px;
}

.app-skeleton-description span.short {
  width: 68%;
}

.app-skeleton-logo,
.app-skeleton-deploy,
.app-skeleton-title,
.app-skeleton-version,
.app-skeleton-category,
.app-skeleton-description span {
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s cubic-bezier(0.4, 0, 0.2, 1) infinite;
}

/* P2: 骨架屏元素交错动画 */
.skeleton-logo,
.skeleton-title,
.skeleton-desc,
.skeleton-meta,
.skeleton-line {
  animation: shimmer 1.5s cubic-bezier(0.4, 0, 0.2, 1) infinite;
}

.loading-grid .skeleton:nth-child(3n + 2) [class^='app-skeleton'] { animation-delay: 100ms; }
.loading-grid .skeleton:nth-child(3n) [class^='app-skeleton'] { animation-delay: 200ms; }

/* 空状态 */
.empty-state-wrapper {
  padding: 80px 20px;
}

/* 应用网格 */
.apps-grid-container {
  flex: 1;
  overflow-y: auto;
  padding: 0;
  scrollbar-gutter: stable;
}

.apps-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  align-content: start;
  gap: 12px;
}

/* 骨架屏也使用相同布局 */
.loading-grid {
  flex: 1;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 12px;
  padding: 0;
  overflow-y: auto;
  scrollbar-gutter: stable;
  min-height: 0;
}

/* 加载更多 */
.load-more {
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 24px 0;
}

.load-more-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 24px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  color: var(--text-secondary);
  font-size: 0.875rem;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.load-more-btn:hover {
  background: var(--bg-secondary);
  border-color: var(--border-default);
  color: var(--text-primary);
}

.loading-spinner {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-secondary);
  font-size: 0.875rem;
}

.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid var(--border-subtle);
  border-top-color: var(--color-primary-500);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.all-loaded {
  color: var(--resource-muted);
  font-size: 0.72rem;
}

.app-card {
  min-width: 0;
  display: flex;
  gap: 12px;
  padding: 13px;
  background: var(--resource-surface);
  border: 1px solid var(--resource-line);
  border-radius: 10px;
  color: var(--resource-ink);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  min-height: 108px;
  /* P1: 列表交错动画 */
  animation: cardSlideUp var(--motion-duration-slow) var(--motion-ease-out) backwards;
}

/* 交错延迟 - 基于索引的动画延迟 */
.app-card:nth-child(1) { animation-delay: 0ms; }
.app-card:nth-child(2) { animation-delay: 30ms; }
.app-card:nth-child(3) { animation-delay: 60ms; }
.app-card:nth-child(4) { animation-delay: 90ms; }
.app-card:nth-child(5) { animation-delay: 120ms; }
.app-card:nth-child(6) { animation-delay: 150ms; }
.app-card:nth-child(7) { animation-delay: 180ms; }
.app-card:nth-child(8) { animation-delay: 210ms; }
.app-card:nth-child(9) { animation-delay: 240ms; }
.app-card:nth-child(10) { animation-delay: 270ms; }
.app-card:nth-child(11) { animation-delay: 300ms; }
.app-card:nth-child(12) { animation-delay: 330ms; }

@keyframes cardSlideUp {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .app-card {
    animation: none;
  }
}

.app-card:hover,
.app-card:focus-within {
  z-index: 2;
  border-color: color-mix(in srgb, var(--resource-accent) 45%, var(--resource-line));
  background: var(--bg-elevated);
  transform: translateY(-1px);
  box-shadow: none;
}

.app-card:active {
  transform: translateY(-1px);
  transition-duration: var(--motion-duration-micro);
}

/* 左侧：Icon + 部署按钮 */
.app-left {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
  width: 52px;
}

/* 部署按钮在 icon 下面 */
.app-left .deploy-btn {
  width: 52px;
  padding: 6px 0;
  font-size: 0.68rem;
  min-width: auto;
  white-space: nowrap;
  overflow: hidden;
}

.app-left .deploy-btn .btn-text {
  flex-shrink: 0;
}

/* 右侧内容区 */
.app-content {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 7px;
}

/* 头部：左侧自适应 + 类型置顶右 */
.app-header {
  display: flex;
  align-items: center;
  gap: 4px;
  min-height: 24px;
  min-width: 0;
}

/* 左侧：名字、版本、下载数 自适应（保留详情页使用） */
.header-left {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: 1;
  min-width: 0;
  overflow: hidden;
}

.app-name {
  color: var(--resource-ink);
  font-size: 0.86rem;
  font-weight: 700;
  margin: 0;
  flex: 1;
  min-width: 60px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.app-category {
  padding: 3px 6px;
  border-radius: 5px;
  background: var(--ops-muted-soft);
  color: var(--resource-muted);
  font-size: 0.64rem;
  flex-shrink: 0;
  margin-left: auto;
}

.app-version {
  color: var(--resource-muted);
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 0.58rem;
  flex-shrink: 0;
  padding: 1px 4px;
  border-radius: 4px;
  background: var(--ops-muted-soft);
}

.deploy-count {
  display: flex;
  align-items: center;
  gap: 1px;
  color: var(--resource-muted);
  font-size: 0.58rem;
  flex-shrink: 0;
}

.deploy-count svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
}

.deploy-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px 16px;
  background: var(--color-primary-500);
  color: var(--text-inverse);
  border: none;
  border-radius: 8px;
  font-size: 0.8125rem;
  font-weight: 700;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  min-width: 72px;
  white-space: nowrap;
  user-select: none;
}

.deploy-btn:hover:not(:disabled) {
  background: var(--color-primary-600);
  transform: translateY(-1px);
  box-shadow: 0 4px 12px var(--color-primary-500-20);
}

/* P2: 按钮 active 态微反馈 */
.deploy-btn:active:not(:disabled) {
  transform: scale(0.98);
  box-shadow: 0 2px 6px var(--color-primary-500-20);
  transition-duration: var(--motion-duration-micro);
}

.deploy-btn:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.deploy-btn.is-loading {
  background: var(--color-primary-400);
}

.btn-spinner {
  width: 14px;
  height: 14px;
  border: 2px solid color-mix(in srgb, var(--text-inverse) 30%, transparent);
  border-top-color: var(--text-inverse);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

/* 详情弹窗 */
.detail-loading {
  padding: 24px;
}

.detail-skeleton {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.skeleton-header {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}

.skeleton-header .skeleton-logo {
  width: 80px;
  height: 80px;
  border-radius: 16px;
}

.skeleton-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.skeleton-title {
  height: 24px;
  width: 40%;
  border-radius: 4px;
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}

.skeleton-meta {
  height: 16px;
  width: 60%;
  border-radius: 4px;
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}

.skeleton-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.skeleton-line {
  height: 14px;
  width: 100%;
  border-radius: 4px;
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}

.skeleton-line.short {
  width: 60%;
}

.app-detail {
  padding: 8px;
}

.detail-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 20px;
  margin-bottom: 24px;
  padding: 16px;
  background: var(--bg-secondary);
  border-radius: 12px;
  border: 1px solid var(--border-subtle);
}

.header-left {
  display: flex;
  gap: 20px;
  align-items: center;
  flex: 1;
}

.detail-header .dynamic-app-icon {
  flex-shrink: 0;
}

.detail-info {
  flex: 1;
  min-width: 0;
}

.info-row {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}

.info-row > * {
  flex-shrink: 0;
}

.info-row h2 {
  font-size: 1.5rem;
  font-weight: 600;
  margin: 0;
  color: var(--text-primary);
}

.category-badge {
  padding: 4px 12px;
  background: var(--color-primary-100);
  color: var(--color-primary-700);
  border-radius: 6px;
  font-size: 0.75rem;
  font-weight: 500;
}

.version {
  font-size: 0.875rem;
  color: var(--text-tertiary);
  font-family: monospace;
  padding: 2px 8px;
  background: var(--bg-tertiary);
  border-radius: 4px;
}

.btn-deploy {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 24px;
  font-size: 1rem;
  font-weight: 600;
  white-space: nowrap;
}

.link-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  color: var(--text-secondary);
  font-size: 0.8125rem;
  text-decoration: none;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.link-btn:hover {
  background: var(--bg-tertiary);
  color: var(--text-primary);
}

.link-btn svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
}

.btn-deploy {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 20px;
  background: var(--color-primary);
  border: none;
  border-radius: 8px;
  color: white;
  font-size: 0.9375rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.btn-deploy:hover {
  background: var(--color-primary-dark);
  transform: translateY(-1px);
  box-shadow: 0 4px 12px var(--shadow-primary);
}

.detail-body {
  border-top: 1px solid var(--border-subtle);
  padding-top: 20px;
}

.detail-description {
  margin-bottom: 24px;
}

.detail-description h4 {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 12px;
}

.detail-description p {
  font-size: 0.9375rem;
  color: var(--text-secondary);
  line-height: 1.7;
  margin: 0;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}

.detail-screenshots h4 {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 12px;
}

.screenshot-list {
  display: flex;
  gap: 12px;
  overflow-x: auto;
  padding-bottom: 8px;
}

.screenshot-list img {
  width: 200px;
  height: 120px;
  object-fit: cover;
  border-radius: 8px;
  border: 1px solid var(--border-subtle);
  cursor: zoom-in;
  transition: transform var(--motion-duration-quick) var(--motion-ease-out);
}

.screenshot-list img:hover {
  transform: scale(1.02);
}

/* 按钮 */
.btn {
  padding: 10px 20px;
  border-radius: 8px;
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  border: none;
}

.btn-secondary {
  background: var(--bg-secondary);
  color: var(--text-primary);
  border: 1px solid var(--border-subtle);
}

.btn-secondary:hover {
  background: var(--bg-tertiary);
}

.btn-primary {
  background: var(--color-primary-500);
  color: white;
}

.btn-primary:hover {
  background: var(--color-primary-600);
}

/* 图片预览 */
.image-preview-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.9);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px;
}

.image-preview-panel {
  position: relative;
  max-width: 100%;
  max-height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  outline: none;
}

.image-preview-panel img {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
  border-radius: 8px;
}

.close-preview {
  position: absolute;
  top: 20px;
  right: 20px;
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.1);
  border: none;
  color: white;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.close-preview:hover {
  background: rgba(255, 255, 255, 0.2);
}

.close-preview:focus-visible {
  outline: 2px solid var(--text-inverse);
  outline-offset: 2px;
}

.close-preview svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
}

/* 响应式 */
@media (max-width: 768px) {
  .appstore-page {
    height: auto;
    min-height: 100%;
    overflow: visible;
  }
  
  .toolbar {
    position: relative;
    align-items: stretch;
  }
  
  .toolbar-left {
    flex-direction: row;
    align-items: stretch;
    flex-wrap: wrap;
    min-width: auto;
  }

  .workbench-toolbar-left > .search-input-wrapper {
    flex: 1 1 100%;
    width: 100%;
    min-width: 0;
    max-width: 100%;
    height: 40px;
  }

  .workbench-toolbar .workbench-toolbar-left > .sort-control {
    flex: 0 0 120px;
    width: 120px;
    min-width: 120px;
    max-width: 120px;
    min-height: 38px;
  }
  
  .category-tabs-wrapper {
    order: 0;
    flex: 1 1 100px;
    width: auto;
    min-width: 0;
    margin: 0 50px 0 0;
  }

  .workbench-toolbar .toolbar-right {
    position: absolute;
    right: 12px;
    bottom: 12px;
    width: auto;
  }

  .store-status-chip {
    display: none;
  }
  
  .apps-grid {
    grid-template-columns: 1fr;
  }
  
  .detail-header {
    flex-direction: column;
    gap: 16px;
  }
  
  .info-row h2 {
    font-size: 1.125rem;
  }
  
  .image-preview-overlay {
    padding: 20px;
  }
}
</style>
