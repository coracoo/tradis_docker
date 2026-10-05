<template>
  <aside
    class="sidebar"
    :class="{
      'sidebar-collapsed': collapsed,
      'sidebar-hidden': hidden
    }"
    role="navigation"
    aria-label="主导航"
  >
    <!-- Logo 区域 -->
    <div class="sidebar-header">
      <div
        v-if="!collapsed"
        class="logo"
        @click="goHome"
        @keydown.enter="goHome"
        @keydown.space.prevent="goHome"
        tabindex="0"
        role="button"
        aria-label="返回首页"
      >
        <div class="logo-icon">
          <img src="/tradis-logo.png" alt="" aria-hidden="true" draggable="false" />
        </div>
        <span class="logo-text">TRADIS</span>
      </div>

      <button
        v-else
        type="button"
        class="sidebar-expand-btn"
        title="展开"
        aria-label="展开侧边栏"
        :aria-expanded="false"
        @click="toggleCollapse"
      >
        <DynamicIcon name="chevron-right" :size="14" />
      </button>
    </div>

    <button
      v-if="!collapsed"
      type="button"
      class="collapse-btn"
      @click="toggleCollapse"
      title="收起"
      aria-label="收起侧边栏"
      :aria-expanded="true"
    >
      <DynamicIcon name="chevron-left" :size="14" />
    </button>

    <!-- 导航菜单 -->
    <nav class="sidebar-nav" aria-label="侧边导航菜单">
      <div v-for="section in sidebarCatalog" :key="section.key" class="nav-section">
        <div class="nav-section-title" v-show="!collapsed">{{ section.title }}</div>
        <ul class="nav-list">
          <li
            v-for="item in section.items"
            :key="item.path"
            class="nav-item"
            :class="item.remoteRestricted ? navItemClass(item.path) : { 'nav-item-active': isActive(item.path) }"
            @click="navigate(item.path)"
            @keydown.enter="navigate(item.path)"
            @keydown.space.prevent="navigate(item.path)"
            tabindex="0"
            role="link"
            :aria-current="isActive(item.path) ? 'page' : null"
            :aria-label="item.ariaLabel"
            :aria-disabled="item.remoteRestricted ? isRemoteNavigationDisabled(item.path) : undefined"
            :title="item.remoteRestricted ? remoteNavigationReason(item.path) || undefined : undefined"
          >
            <div class="nav-icon"><DynamicIcon :name="item.icon" :size="20" /></div>
            <span v-show="!collapsed" class="nav-label">{{ item.label }}</span>
          </li>
        </ul>
      </div>
    </nav>

    <!-- 底部状态 -->
    <div class="sidebar-footer" v-show="!collapsed">
      <div class="footer-status">
        <div class="footer-status-row">
          <div
            class="status-item"
            :title="appStoreTooltip"
            :aria-label="appStoreTooltip"
          >
            <span :class="['status-dot', `is-${appStoreIndicatorTone}`]" />
            <span class="status-text">商城 {{ appStoreIndicatorLabel }}</span>
          </div>
        <span v-if="isFullEdition && showNewVersion" class="update-badge">有新版</span>
        </div>
        <button v-if="isFullEdition" class="version-info" type="button" title="查看版本与更新内容" @click="updateDrawerVisible = true">
          <span class="version-cell">
            <span class="version-label">本地</span>
            <span class="version-value">{{ localVersionText }}</span>
          </span>
          <span class="version-cell">
            <span class="version-label">服务端</span>
            <span class="version-value">{{ serverVersionText }}</span>
          </span>
        </button>
        <span v-else class="edition-label">Community 本地版</span>
      </div>
    </div>
  </aside>

  <UpdateDrawer
    v-if="isFullEdition"
    v-model:visible="updateDrawerVisible"
    :status="versionStatus"
    :refreshing="versionRefreshing"
    @refresh="refreshVersionStatus"
  />

  <!-- 移动端遮罩 -->
  <div
    v-if="!hidden"
    class="sidebar-overlay"
    @click="closeSidebar"
    aria-hidden="true"
  ></div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import appStoreApi from '@/api/appstore.js'
import { useEnvironmentContext } from '@/composables/useEnvironmentContext.js'
import { useUiStore } from '@/stores/ui.js'
import { sidebarCatalog } from '@edition/sidebar-catalog'
import { UpdateDrawer, loadSidebarVersionStatus, checkSidebarVersionStatus } from '@edition/sidebar-update'
import { remoteTargetNavigationReason } from '@edition/remote-navigation'
import DynamicIcon from '../ui/DynamicIcon.vue'

const props = defineProps({
  collapsed: {
    type: Boolean,
    default: false
  },
  hidden: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['toggle-collapse', 'close'])

const route = useRoute()
const router = useRouter()
const uiStore = useUiStore()
const isFullEdition = __TRADIS_EDITION__ === 'full'
const { isRemoteEnvironment } = useEnvironmentContext()
// 商城连接状态
const appStoreStatus = ref(appStoreApi.normalizeAppStoreStatus())
let pingTimer = null

// 版本信息
const localVersion = ref('')
const serverVersion = ref('')
const hasNewVersion = ref(false)
const versionStatus = ref({})
const updateDrawerVisible = ref(false)
const versionRefreshing = ref(false)
let versionTimer = null

const localVersionText = computed(() => {
  const v = String(localVersion.value || '').trim()
  if (!v) return 'v0.9.7' // x-release-please-version
  return v.startsWith('v') ? v : `v${v}`
})

const serverVersionText = computed(() => {
  const v = (serverVersion.value || '').trim()
  if (!v) return '—'
  return v.startsWith('v') ? v : `v${v}`
})

const showNewVersion = computed(() => {
  const local = String(localVersion.value || '').trim()
  const server = String(serverVersion.value || '').trim()
  if (!local || !server) return false
  return hasNewVersion.value
})

const appStoreIndicatorTone = computed(() => {
  if (appStoreStatus.value?.connected && appStoreStatus.value?.cdnConfigured) return 'success'
  if (appStoreStatus.value?.connected) return 'warning'
  return 'error'
})

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
  if (appStoreStatus.value?.summary) {
    parts.push(`详情：${appStoreStatus.value.summary}`)
  }
  return parts.join('\n')
})

// 判断是否激活
function isActive(path) {
  const currentPath = route.path
  if (path === '/') {
    return currentPath === '/'
  }
  if (path === '/containers') {
    return currentPath.startsWith('/containers')
  }
  if (path === '/compose') {
    return currentPath.startsWith('/compose')
  }
  return currentPath === path || currentPath.startsWith(path + '/')
}

function remoteNavigationReason(path) {
	return remoteTargetNavigationReason(path, isRemoteEnvironment.value)
}

function isRemoteNavigationDisabled(path) {
	return remoteNavigationReason(path) !== ''
}

function navItemClass(path) {
	return {
		'nav-item-active': isActive(path),
		'nav-item-disabled': isRemoteNavigationDisabled(path)
	}
}

// 导航
function navigate(path) {
	const reason = remoteNavigationReason(path)
	if (reason) {
		uiStore.toastInfo(reason)
		return
	}
  router.push(path)
  if (props.hidden === false) {
    emit('close')
  }
}

// 返回首页
function goHome() {
  router.push('/')
}

// 切换折叠
function toggleCollapse() {
  emit('toggle-collapse')
}

// 关闭侧边栏（移动端）
function closeSidebar() {
  emit('close')
}

// 检测商城连接
const pingAppStore = async () => {
  try {
    appStoreStatus.value = await appStoreApi.getStatus()
  } catch (e) {
    appStoreStatus.value = appStoreApi.normalizeAppStoreStatus({
      state: 'offline',
      summary: '无法获取应用商店状态'
    })
  }
}

// 加载版本信息
const loadVersionStatusFromDB = async () => {
  if (!isFullEdition) return
  try {
    const status = await loadSidebarVersionStatus()
    localVersion.value = status && status.localVersion ? String(status.localVersion) : ''
    serverVersion.value = status && status.serverVersion ? String(status.serverVersion) : ''
    hasNewVersion.value = !!(status && status.hasNewVersion)
    versionStatus.value = status || {}
  } catch (e) {
    localVersion.value = ''
    serverVersion.value = ''
    hasNewVersion.value = false
    versionStatus.value = {}
  }
}

const refreshVersionStatus = async () => {
  if (!isFullEdition) return
  if (versionRefreshing.value) return
  versionRefreshing.value = true
  try {
    const status = await checkSidebarVersionStatus()
    localVersion.value = status?.localVersion ? String(status.localVersion) : ''
    serverVersion.value = status?.serverVersion ? String(status.serverVersion) : ''
    hasNewVersion.value = !!status?.hasNewVersion
    versionStatus.value = status || {}
  } finally {
    versionRefreshing.value = false
  }
}

// 生命周期
onMounted(() => {
  pingAppStore()
  pingTimer = setInterval(pingAppStore, 15000)
  if (isFullEdition) {
    loadVersionStatusFromDB()
    versionTimer = setInterval(loadVersionStatusFromDB, 60000)
  }
})

onUnmounted(() => {
  if (pingTimer) clearInterval(pingTimer)
  if (versionTimer) clearInterval(versionTimer)
})
</script>

<style scoped>
.sidebar {
  position: fixed;
  left: 0;
  top: 0;
  bottom: 0;
  width: 260px;
  background: var(--sidebar-bg);
  -webkit-backdrop-filter: blur(14px);
  backdrop-filter: blur(14px);
  border-right: 1px solid var(--border-subtle);
  color: var(--sidebar-ink);
  font-family: var(--font-sans);
  display: flex;
  flex-direction: column;
  z-index: 100;
  transition:
    width var(--motion-duration-fast) var(--motion-ease-out),
    transform var(--motion-duration-fast) var(--motion-ease-out);
}

.sidebar-collapsed {
  width: 72px;
}

.sidebar-hidden {
  transform: translateX(-100%);
}

/* Logo 区域 */
.sidebar-header {
  height: 72px;
  display: flex;
  align-items: center;
  justify-content: flex-start;
  padding: 0 16px;
}

.sidebar-collapsed .sidebar-header {
  justify-content: center;
  padding-inline: 0;
}

.logo {
  display: flex;
  align-items: center;
  gap: 12px;
  cursor: pointer;
  padding: 4px 6px;
  border-radius: 10px;
  transition: background-color var(--motion-duration-quick) var(--motion-ease-out);
}

.logo:hover,
  .logo:focus-visible {
  background-color: var(--bg-secondary);
  outline: none;
  box-shadow: 0 0 0 2px var(--color-primary-500);
}

.logo-icon {
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.logo-icon img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.logo-text {
  font-size: 1.15rem;
  font-weight: 800;
  line-height: 1.1;
  color: var(--sidebar-ink);
  letter-spacing: 0;
}

.sidebar-expand-btn {
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 1px solid var(--border-default);
  border-radius: 8px;
  color: var(--sidebar-muted);
  background: var(--bg-elevated);
  box-shadow: 0 2px 8px color-mix(in srgb, var(--sidebar-ink) 10%, transparent);
  cursor: pointer;
  transition:
    border-color 0.16s ease,
    background-color 0.16s ease,
    color 0.16s ease,
    transform 0.16s ease;
}

.sidebar-expand-btn:hover {
  border-color: var(--color-primary-300);
  background: var(--color-primary-50);
  color: var(--sidebar-ink);
}

.sidebar-expand-btn:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.sidebar-expand-btn:active {
  transform: scale(0.94);
}

.collapse-btn {
  position: absolute;
  top: 72px;
  right: -11px;
  z-index: 4;
  width: 22px;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--border-default);
  border-radius: 0 7px 7px 0;
  color: var(--sidebar-muted);
  background: var(--bg-elevated);
  box-shadow: 0 2px 8px color-mix(in srgb, var(--sidebar-ink) 12%, transparent);
  cursor: pointer;
  transition:
    transform 0.16s ease,
    border-color 0.16s ease,
    background-color 0.16s ease,
    color 0.16s ease,
    box-shadow 0.16s ease;
  flex-shrink: 0;
}

.collapse-btn:hover {
  border-color: var(--color-primary-300);
  background-color: var(--color-primary-50);
  color: var(--sidebar-ink);
  transform: translateX(2px);
}

.collapse-btn:focus-visible {
  outline: none;
  border-color: var(--color-primary-400);
  box-shadow:
    0 2px 8px color-mix(in srgb, var(--sidebar-ink) 12%, transparent),
    0 0 0 3px var(--focus-ring);
}

.collapse-btn:active {
  transform: translateX(1px) scale(0.96);
}

/* 导航菜单 */
.sidebar-nav {
  flex: 1;
  overflow-y: auto;
  padding: 12px 0;
}

.nav-section {
  margin-bottom: 10px;
}

.nav-section-title {
  padding: 12px 20px 7px;
  font-size: 0.6875rem;
  font-weight: 700;
  color: var(--sidebar-muted);
  text-transform: uppercase;
  letter-spacing: 0;
}

.nav-list {
  list-style: none;
  padding: 0 12px;
  margin: 0;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 40px;
  padding: 0 10px;
  border: 1px solid transparent;
  border-radius: 9px;
  color: var(--sidebar-ink);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  margin-bottom: 4px;
  position: relative;
  user-select: none;
}

.nav-item:hover,
  .nav-item:focus-visible {
  border-color: var(--border-subtle);
  background-color: var(--bg-elevated);
  color: var(--sidebar-ink);
  transform: none;
  outline: none;
  box-shadow: none;
}

/* P2: 导航项 active 态微反馈 */
.nav-item:active:not(.nav-item-active) {
  transform: scale(0.98);
  background-color: var(--border-subtle);
}

.nav-item-active {
  border-color: var(--border-subtle);
  background: var(--bg-elevated);
  color: var(--sidebar-ink);
  box-shadow: inset 3px 0 0 var(--color-primary-500);
}

.nav-item-active:hover,
  .nav-item-active:focus-visible {
  border-color: var(--border-subtle);
  background: var(--bg-elevated);
  color: var(--sidebar-ink);
  transform: none;
  outline: none;
  box-shadow:
    inset 3px 0 0 var(--color-primary-500),
    0 0 0 2px var(--focus-ring);
}

.nav-item-active:active {
  transform: scale(0.98);
  box-shadow: inset 3px 0 0 var(--color-primary-500);
}

.nav-item-disabled,
.nav-item-disabled:hover,
.nav-item-disabled:focus-visible,
.nav-item-disabled:active {
	border-color: transparent;
	background: transparent;
	color: var(--sidebar-muted);
	cursor: not-allowed;
	opacity: 0.5;
	transform: none;
	box-shadow: none;
}

.nav-icon {
  width: 20px;
  height: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.nav-label {
  font-size: 0.875rem;
  font-weight: 500;
  line-height: 1.35;
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 底部状态 */
.sidebar-footer {
  flex: 0 0 auto;
  padding: 8px 14px 14px;
  border-top: 0;
  background: transparent;
}

.footer-status {
  padding: 10px;
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  background: var(--bg-elevated);
  font-size: 0.75rem;
  color: var(--sidebar-muted);
}

.footer-status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
  margin-bottom: 8px;
}

.edition-label {
  display: block;
  margin-top: 8px;
  color: var(--text-tertiary);
  font-size: 0.65rem;
}

.status-item {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--color-danger-500);
  flex-shrink: 0;
}

.status-dot.is-success {
  background: var(--color-success-500);
  box-shadow: 0 0 0 4px var(--ring-success);
}

.status-dot.is-warning {
  background: var(--color-warning-500);
  box-shadow: 0 0 0 4px var(--ring-warning);
}

.status-dot.is-error {
  background: var(--color-danger-500);
  box-shadow: 0 0 0 4px var(--ring-danger);
}

.status-text {
  overflow: hidden;
  color: var(--sidebar-muted);
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.version-info {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  width: 100%;
  padding: 7px 8px;
  margin: 0;
  border: 0;
  border-radius: 8px;
  color: inherit;
  background: var(--bg-primary);
  text-align: left;
  cursor: pointer;
  transition: background-color var(--motion-duration-quick) var(--motion-ease-out), transform var(--motion-duration-quick) var(--motion-ease-out);
}

.version-info:hover,
.version-info:focus-visible {
  background: var(--bg-tertiary);
  outline: none;
}

.version-info:active {
  transform: scale(0.98);
}

.version-cell {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.version-cell + .version-cell {
  padding-left: 8px;
  border-left: 1px solid var(--border-subtle);
}

.version-label {
  color: var(--sidebar-subtle);
  font-size: 0.625rem;
}

.version-value {
  overflow: hidden;
  color: var(--sidebar-muted);
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.6875rem;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.update-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 6px;
  background: linear-gradient(135deg, var(--color-accent-500), var(--color-accent-600));
  color: var(--text-inverse);
  font-size: 0.625rem;
  font-weight: 600;
  border-radius: 4px;
  white-space: nowrap;
}

/* 移动端遮罩 */
.sidebar-overlay {
  display: none;
}

/* 响应式 */
@media (max-width: 768px) {
  .sidebar {
    transform: translateX(-100%);
    transition: transform var(--motion-duration-panel) var(--motion-ease-out);
  }

  .sidebar:not(.sidebar-hidden) {
    transform: translateX(0);
  }

  .collapse-btn {
    display: none;
  }

  .sidebar-expand-btn {
    display: none;
  }

  .sidebar-overlay {
    display: block;
    position: fixed;
    inset: 0;
    background: var(--overlay-bg);
    z-index: 99;
    animation: overlayFadeIn 0.25s ease forwards;
  }

  /* P0: 遮罩层淡入动画 */
  @keyframes overlayFadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }
}

@media (prefers-reduced-motion: reduce) {
  .sidebar,
  .collapse-btn,
  .sidebar-expand-btn,
  .nav-item {
    transition: none;
  }
}
</style>
