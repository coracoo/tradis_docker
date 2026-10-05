<template>
  <div class="layout">
    <!-- 侧边栏 -->
    <AppSidebar 
      :collapsed="sidebarCollapsed"
      :hidden="isMobile() && sidebarHidden"
      @toggle-collapse="toggleSidebarCollapse"
      @close="closeSidebar"
    />

    <!-- 主内容区域 -->
    <div class="layout-main" :class="{ 'sidebar-collapsed': sidebarCollapsed }">
      <!-- 顶部栏 -->
      <AppHeader 
        :notification-count="notificationCount"
        :quick-actions-open="showQuickActions"
        @toggle-sidebar="toggleMobileSidebar"
        @show-quick-actions="openQuickActions"
        @focus-search="openSearch"
        @show-notifications="openNotifications"
      />

      <!-- 页面内容 -->
      <main class="layout-content" id="main-content">
        <div class="content-wrapper">
          <router-view v-slot="{ Component }">
            <Suspense timeout="0">
              <component :is="Component" />
              <template #fallback>
                <div class="route-loading-shell" role="status" aria-label="页面加载中">
                  <div class="route-loading-toolbar">
                    <span class="route-loading-line is-wide"></span>
                    <span class="route-loading-line"></span>
                    <span class="route-loading-line is-short"></span>
                  </div>
                  <div class="route-loading-panel">
                    <span v-for="n in 8" :key="n" class="route-loading-row"></span>
                  </div>
                </div>
              </template>
            </Suspense>
          </router-view>
        </div>
      </main>
    </div>

    <!-- 移动端底部导航 -->
    <MobileDock />
    
    <!-- 快捷操作菜单 -->
    <QuickActionsMenu
      v-if="quickActionsLoaded"
      :visible="showQuickActions"
      @close="showQuickActions = false"
    />
    
    <!-- 通知面板 -->
    <NotificationPanel
      v-if="notificationsLoaded"
      :visible="showNotifications"
      @close="showNotifications = false"
      @read="handleNotificationsRead"
      @update-count="updateNotificationCount"
    />
    
    <!-- 全局搜索 -->
    <GlobalSearch
      v-if="searchLoaded"
      :visible="showSearch"
      @close="showSearch = false"
    />

    <ToastProvider />

    <OnboardingTour />

    <component
      :is="currentModalComponent"
      v-if="currentModal && currentModalComponent"
      v-bind="currentModal.props"
      :visible="true"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, defineAsyncComponent, provide } from 'vue'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'
import MobileDock from './MobileDock.vue'
import ToastProvider from '@/components/ui/ToastProvider.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import OnboardingTour from '@/components/onboarding/OnboardingTour.vue'
import { useUiStore } from '@/stores/ui.js'
import { useOnboardingStore } from '@/stores/onboarding.js'
import { createLayoutEnvironment, environmentContextKey } from '@edition/layout-environment'
import * as systemApi from '@/api/system.js'

const QuickActionsMenu = defineAsyncComponent(() => import('./QuickActionsMenu.vue'))
const NotificationPanel = defineAsyncComponent(() => import('./NotificationPanel.vue'))
const GlobalSearch = defineAsyncComponent(() => import('./GlobalSearch.vue'))
const uiStore = useUiStore()
const onboardingStore = useOnboardingStore()
const layoutEnvironment = createLayoutEnvironment()
provide(environmentContextKey, layoutEnvironment.context)
const modalComponents = {
  ConfirmDialog
}
const currentModal = computed(() => uiStore.currentModal)
const currentModalComponent = computed(() => modalComponents[currentModal.value?.component])

// 屏幕宽度检测
const isMobile = () => window.innerWidth <= 768

// 侧边栏状态 - 根据屏幕宽度设置初始值
const sidebarCollapsed = ref(false)
const sidebarHidden = ref(isMobile()) // 移动端默认隐藏，桌面端默认显示

// 弹窗状态
const showQuickActions = ref(false)
const showNotifications = ref(false)
const showSearch = ref(false)
const quickActionsLoaded = ref(false)
const notificationsLoaded = ref(false)
const searchLoaded = ref(false)

// 通知状态
const notificationCount = ref(0)
let notificationTimer = null

// 更新通知数量
function updateNotificationCount(count) {
  notificationCount.value = count
}

// 标记通知已读
function handleNotificationsRead() {
  notificationCount.value = 0
}

async function loadNotificationCount() {
  try {
    const summary = await systemApi.getNotificationSummary()
    notificationCount.value = Number(summary?.unread_count || 0)
  } catch (error) {
    console.error('获取通知数量失败:', error)
  }
}

function startNotificationPolling() {
  stopNotificationPolling()
  notificationTimer = setInterval(loadNotificationCount, 15000)
}

function stopNotificationPolling() {
  if (notificationTimer) {
    clearInterval(notificationTimer)
    notificationTimer = null
  }
}

function openQuickActions() {
  quickActionsLoaded.value = true
  showQuickActions.value = true
}

function openNotifications() {
  notificationsLoaded.value = true
  showNotifications.value = true
}

function openSearch() {
  searchLoaded.value = true
  showSearch.value = true
}



// 初始化响应式状态
function initResponsive() {
  if (isMobile()) {
    sidebarCollapsed.value = false
    sidebarHidden.value = true
  } else {
    sidebarCollapsed.value = false
    sidebarHidden.value = false
  }
}

// 切换侧边栏折叠（桌面端）
function toggleSidebarCollapse() {
  if (isMobile()) {
    sidebarHidden.value = !sidebarHidden.value
  } else {
    sidebarCollapsed.value = !sidebarCollapsed.value
  }
}

// 切换移动端侧边栏
function toggleMobileSidebar() {
  sidebarHidden.value = !sidebarHidden.value
}

// 关闭侧边栏
function closeSidebar() {
  sidebarHidden.value = true
}

// 监听窗口大小变化
function handleResize() {
  initResponsive()
}

// 键盘事件 - Cmd/Ctrl + K 打开搜索
function handleKeydown(e) {
  if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
    e.preventDefault()
    openSearch()
  }
}

// 生命周期
onMounted(() => {
  initResponsive()
  loadNotificationCount()
  startNotificationPolling()
  onboardingStore.hydrate()
  if (layoutEnvironment.load) void layoutEnvironment.load().catch(() => {})
  window.addEventListener('resize', handleResize)
  document.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  stopNotificationPolling()
  window.removeEventListener('resize', handleResize)
  document.removeEventListener('keydown', handleKeydown)
})
</script>

<style scoped>
.layout {
  height: 100vh;
  overflow: hidden;
  background: var(--bg-secondary);
}

/* 主内容区域 */
.layout-main {
  margin-left: 260px;
  height: 100vh;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.layout-main.sidebar-collapsed {
  margin-left: 72px;
}

.layout-main :deep(.header) {
  flex: 0 0 64px;
}

/* 页面内容 */
.layout-content {
  --layout-grid-line: color-mix(in srgb, var(--text-primary, #132033) 3.5%, transparent);
  flex: 1;
  min-height: 0;
  overflow: hidden;
  padding: 24px;
  background-color: var(--bg-secondary, #eef3f7);
  background-image:
    linear-gradient(90deg, var(--layout-grid-line) 1px, transparent 1px),
    linear-gradient(180deg, var(--layout-grid-line) 1px, transparent 1px);
  background-size: 28px 28px;
}

.layout-content:has(.resource-workbench) {
  padding: 0;
}



.content-wrapper {
  width: 100%;
  height: 100%;
  min-height: 0;
}

.route-loading-shell {
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
  height: 100%;
  min-height: 0;
  animation: route-loading-enter var(--motion-duration-fast) var(--motion-ease-out) both;
}

.route-loading-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 40px;
}

.route-loading-line,
.route-loading-row {
  display: block;
  border-radius: 8px;
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: route-loading-shimmer 1.5s infinite;
}

.route-loading-line {
  width: 160px;
  height: 36px;
}

.route-loading-line.is-wide {
  width: min(360px, 42vw);
}

.route-loading-line.is-short {
  width: 96px;
  margin-left: auto;
}

.route-loading-panel {
  flex: 1 1 auto;
  min-height: 0;
  padding: 16px;
  overflow: hidden;
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  background: var(--bg-elevated);
}

.route-loading-row {
  height: 18px;
  margin-bottom: 18px;
}

@keyframes route-loading-shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}

@keyframes route-loading-enter {
  from {
    opacity: 0;
    transform: translateY(var(--motion-distance-micro));
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .route-loading-line,
  .route-loading-row {
    animation-duration: var(--motion-duration-micro);
    animation-iteration-count: 1;
  }
}

@media (min-width: 769px) {
  .content-wrapper :deep(.compose-page),
  .content-wrapper :deep(.containers-page),
  .content-wrapper :deep(.images-page),
  .content-wrapper :deep(.networks-page),
  .content-wrapper :deep(.volumes-page),
  .content-wrapper :deep(.ports-page),
  .content-wrapper :deep(.navigation-page),
  .content-wrapper :deep(.appstore-page),
  .content-wrapper :deep(.scheduled-tasks-page) {
    box-sizing: border-box;
    height: 100%;
    min-height: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    padding-bottom: 0;
  }

  .content-wrapper :deep(.toolbar) {
    flex: 0 0 auto;
  }

  .content-wrapper :deep(.list-view .data-table thead th),
  .content-wrapper :deep(.table-wrapper .data-table thead th) {
    position: sticky;
    top: 0;
    z-index: 5;
    background: var(--bg-elevated);
    box-shadow: 0 1px 0 var(--border-subtle);
  }

  .content-wrapper :deep(.table-wrapper .data-table thead th) {
    background: var(--bg-secondary);
  }

  .content-wrapper :deep(.overview-page),
  .content-wrapper :deep(.settings-page),
  .content-wrapper :deep(.nas-store-page),
  .content-wrapper :deep(.github-apps-page) {
    box-sizing: border-box;
    height: 100%;
    min-height: 0;
    overflow-y: auto;
    scrollbar-gutter: stable;
  }

  .content-wrapper :deep(.ai-agent-page) {
    height: 100%;
  }
}

/* 响应式 */
@media (max-width: 768px) {
  .layout {
    height: auto;
    min-height: 100vh;
    overflow: visible;
  }

  .layout-main {
    margin-left: 0 !important;
    height: auto;
    min-height: 100vh;
    overflow: visible;
    padding-bottom: 64px; /* 为底部导航留出空间 */
  }
  
  .layout-content {
    overflow-y: auto;
    padding: 16px;
  }

  .content-wrapper {
    height: auto;
  }
}
</style>
