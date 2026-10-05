<template>
  <Teleport to="body">
    <Transition name="overlay-fade">
      <div v-if="visible" class="notification-overlay" @click="close"></div>
    </Transition>
    
    <Transition name="panel">
      <div v-if="visible" class="notification-panel">
        <!-- Header -->
        <div class="panel-header">
          <div class="header-left">
            <span class="header-title">Notifications</span>
            <span v-if="unreadCount > 0" class="header-badge">{{ unreadBadgeText }}</span>
          </div>
          <div class="header-actions">
            <button 
              v-if="notifications.length > 0"
              class="header-btn danger"
              @click="clearAll"
            >
              Clear
            </button>
            <button class="header-close" @click="close">
              <DynamicIcon name="x" :size="14" />
            </button>
          </div>
        </div>

        <!-- Content -->
        <div class="panel-body">
          <div v-if="notifications.length > 0" class="category-filter" role="tablist" aria-label="通知分类">
            <button
              v-for="option in categoryOptions"
              :key="option.value"
              class="category-filter-btn"
              :class="{ active: selectedCategory === option.value }"
              type="button"
              role="tab"
              :aria-selected="selectedCategory === option.value"
              @click="selectedCategory = option.value"
            >
              {{ option.label }}
            </button>
          </div>

          <!-- Empty -->
          <div v-if="visibleNotifications.length === 0 && !loading" class="empty-state">
            <DynamicIcon name="bell" :size="20" />
            <span>{{ notifications.length === 0 ? 'No notifications' : '当前分类暂无通知' }}</span>
          </div>

          <!-- List -->
          <div v-else class="notification-list">
            <div
              v-for="item in visibleNotifications"
              :key="item.id"
              class="notification-item"
              :class="{ unread: !item.read }"
            >
              <!-- Row 1: Status + Date -->
              <div class="item-row">
                <div class="item-status" :class="item.type">
                  <span class="status-dot"></span>
                  <span class="status-label">{{ getStatusLabel(item.type) }}</span>
                  <span class="category-badge">{{ getCategoryLabel(item.category) }}</span>
                </div>
                <span class="item-date">{{ formatTime(item.created_at) }}</span>
              </div>

              <!-- Row 2: Title + Detail -->
              <div
                class="item-content"
                role="button"
                tabindex="0"
                title="点击复制完整通知明细"
                @click="copyNotification(item)"
                @keydown.enter.prevent="copyNotification(item)"
                @keydown.space.prevent="copyNotification(item)"
              >
                <div class="item-main">
                  <span v-if="item.title" class="item-title">{{ item.title }}</span>
                  <span class="item-detail">{{ item.message }}</span>
                </div>
              </div>
              <button
                v-if="item.link"
                type="button"
                class="item-link"
                title="打开相关页面"
                @click.stop="openNotificationLink(item)"
              >
                <DynamicIcon name="external-link" :size="13" />
              </button>
            </div>

            <!-- Load more -->
            <div v-if="hasMore" class="load-more">
              <button v-if="!loading" class="load-btn" @click="loadMore">
                Load more
              </button>
              <span v-else class="loading-text">Loading...</span>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import DynamicIcon from '../ui/DynamicIcon.vue'
import * as systemApi from '@/api/system.js'
import { formatTime as formatDateTime } from '@/utils/format.js'
import { copyToClipboard } from '@/utils/helpers.js'
import { useUiStore } from '@/stores/ui.js'

const props = defineProps({ visible: { type: Boolean, default: false } })
const emit = defineEmits(['close', 'read', 'update-count'])
const uiStore = useUiStore()

const router = useRouter()
const notifications = ref([])
const loading = ref(false)
const hasMore = ref(true)
const beforeId = ref(0)
const selectedCategory = ref('all')
const latestSummary = ref({
  latestId: 0,
  totalCount: 0,
  unreadCount: 0
})
let refreshTimer = null

const unreadCount = computed(() => notifications.value.filter(n => !n.read).length)
const unreadBadgeText = computed(() => unreadCount.value > 99 ? '99+' : String(unreadCount.value))
const categoryOptions = [
  { value: 'all', label: '全部' },
  { value: 'deploy_task', label: '部署' },
  { value: 'git_task', label: 'Git' },
  { value: 'navigation_task', label: '导航' },
  { value: 'volume_backup_task', label: '卷备份' },
  { value: 'app_protection_task', label: '应用保护' },
  { value: 'system', label: '系统' }
]
const visibleNotifications = computed(() => {
  if (selectedCategory.value === 'all') return notifications.value
  return notifications.value.filter(item => item.category === selectedCategory.value)
})

function getStatusLabel(type) {
  const labels = { success: 'Success', error: 'Error', warning: 'Warning', info: 'Info', system: 'System' }
  return labels[type] || type
}

function getCategoryLabel(category) {
  const target = categoryOptions.find(item => item.value === category)
  return target?.label || '系统'
}

function normalizeNotificationCategory(category) {
  const value = String(category || '').trim()
  return categoryOptions.some(item => item.value === value && value !== 'all') ? value : ''
}

function inferNotificationCategory(message = '') {
  const text = String(message || '')
  if (text.includes('应用部署') || (text.includes('Compose 项目') && /部署|构建|更新|启动|停止|重启/.test(text))) {
    return 'deploy_task'
  }
  if (text.includes('GitHub 应用') || text.includes(' Git ') || text.includes('Git 导入') || text.includes('Git 同步')) {
    return 'git_task'
  }
  if (text.includes('AI 导航识别')) return 'navigation_task'
  if (text.includes('应用备份') || text.includes('应用恢复')) return 'app_protection_task'
  if (text.includes('卷备份')) return 'volume_backup_task'
  return 'system'
}

function normalizeNotification(item) {
  const category = normalizeNotificationCategory(item?.category) || inferNotificationCategory(item?.message)
  return { ...item, category }
}

function formatTime(time) {
  return formatDateTime(time)
}

function updateCount() {
  emit('update-count', unreadCount.value)
}

function syncSummary(summary) {
  latestSummary.value = {
    latestId: Number(summary?.latest_id || 0),
    totalCount: Number(summary?.total_count || 0),
    unreadCount: Number(summary?.unread_count || 0)
  }
  emit('update-count', latestSummary.value.unreadCount)
}

async function loadNotifications(reset = false) {
  if (loading.value) return
  loading.value = true
  
  if (reset) {
    beforeId.value = 0
    notifications.value = []
  }
  
  try {
    const params = { pageSize: 20 }
    if (beforeId.value > 0) params.before_id = beforeId.value
    const res = await systemApi.getNotifications(params)
    const list = res.notifications || res.list || res || []
    const existingIds = new Set(notifications.value.map(item => String(item.id)))
    const uniqueItems = list
      .map(normalizeNotification)
      .filter(item => {
        const id = String(item.id)
        if (existingIds.has(id)) return false
        existingIds.add(id)
        return true
      })
    notifications.value.push(...uniqueItems)
    updateCount()
    hasMore.value = list.length === 20
    const lastID = Number(list[list.length - 1]?.id || 0)
    if (lastID > 0) beforeId.value = lastID
    if (notifications.value.length > 0) {
      latestSummary.value.latestId = Number(notifications.value[0]?.id || latestSummary.value.latestId || 0)
      latestSummary.value.totalCount = Math.max(latestSummary.value.totalCount, notifications.value.length)
    }
    if (props.visible) {
      await markAllRead()
    }
  } catch (e) {
    console.error('Load failed:', e)
  } finally {
    loading.value = false
  }
}

async function refreshNotificationsIfNeeded(force = false) {
  try {
    const summary = await systemApi.getNotificationSummary()
    const nextLatestId = Number(summary?.latest_id || 0)
    const nextTotalCount = Number(summary?.total_count || 0)
    const changed = nextLatestId !== latestSummary.value.latestId || nextTotalCount !== latestSummary.value.totalCount
    syncSummary(summary)
    if (force || notifications.value.length === 0 || changed) {
      await loadNotifications(true)
    }
  } catch (e) {
    console.error('Refresh summary failed:', e)
    if (force && notifications.value.length === 0) {
      await loadNotifications(true)
    }
  }
}

async function markAllRead() {
  try {
    await systemApi.markNotificationsRead()
    notifications.value.forEach(n => n.read = true)
    updateCount()
    latestSummary.value.unreadCount = 0
    emit('update-count', 0)
    emit('read')
  } catch (e) {
    console.error('Auto mark notifications read failed:', e)
  }
}

async function clearAll() {
  const confirmed = await uiStore.confirm({
    type: 'danger',
    title: 'Clear notifications',
    message: 'Clear all notifications?',
    confirmText: 'Clear'
  })
  if (!confirmed) return
  try {
    await systemApi.clearAllNotifications()
    notifications.value = []
    beforeId.value = 0
    hasMore.value = false
    updateCount()
    latestSummary.value = { latestId: 0, totalCount: 0, unreadCount: 0 }
    emit('update-count', 0)
  } catch (e) {
    console.error('Clear all failed:', e)
  }
}

async function copyNotification(item) {
  const ok = await copyToClipboard(item?.message || '')
  if (ok) uiStore.toastSuccess('已复制通知明细')
  else uiStore.toastError('复制失败，请手动复制')
}

function openNotificationLink(item) {
  if (!item?.link) return
  router.push(item.link)
  close()
}

function loadMore() {
  loadNotifications()
}

function close() {
  emit('close')
}

function startRefreshTimer() {
  stopRefreshTimer()
  refreshTimer = setInterval(() => {
    if (props.visible) {
      refreshNotificationsIfNeeded()
    }
  }, 15000)
}

function stopRefreshTimer() {
  if (refreshTimer) {
    clearInterval(refreshTimer)
    refreshTimer = null
  }
}

function handleEscapeKeydown(e) {
  if (e.key === 'Escape' && props.visible) {
    close()
  }
}

function handleNotificationEvent(event) {
  const detail = event?.detail || {}
  const dbId = Number(detail.dbId || 0)
  if (!dbId) return
  const exists = notifications.value.some(item => Number(item.id) === dbId)
  if (!exists) {
    notifications.value.unshift({
      id: dbId,
      type: detail.type || 'info',
      category: normalizeNotificationCategory(detail.category) || inferNotificationCategory(detail.message),
      title: detail.title || '',
      message: detail.message || '',
      created_at: detail.createdAt || new Date().toISOString(),
      read: !!detail.read
    })
  }
  latestSummary.value.latestId = Math.max(latestSummary.value.latestId, dbId)
  latestSummary.value.totalCount += exists ? 0 : 1
  latestSummary.value.unreadCount += detail.read ? 0 : (exists ? 0 : 1)
  emit('update-count', latestSummary.value.unreadCount)
  if (props.visible) {
    markAllRead()
  }
}

watch(
  () => props.visible,
  (val) => {
    if (val) {
      refreshNotificationsIfNeeded(true)
      markAllRead()
      startRefreshTimer()
    } else {
      stopRefreshTimer()
    }
  },
  { immediate: true }
)

onMounted(() => {
  document.addEventListener('keydown', handleEscapeKeydown)
  window.addEventListener('dockpier-notification', handleNotificationEvent)
})
onUnmounted(() => {
  stopRefreshTimer()
  document.removeEventListener('keydown', handleEscapeKeydown)
  window.removeEventListener('dockpier-notification', handleNotificationEvent)
})
</script>

<style scoped>
.notification-overlay {
  position: fixed;
  inset: 0;
  z-index: 199;
}

.notification-panel {
  position: fixed;
  top: 76px;
  right: 12px;
  width: 400px;
  max-width: calc(100vw - 24px);
  max-height: calc(100vh - 88px);
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  box-shadow: 0 20px 40px -8px rgba(0, 0, 0, 0.12);
  z-index: 200;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* Header */
.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
  border-bottom: 1px solid var(--border-subtle);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.header-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  letter-spacing: -0.01em;
}

.header-badge {
  min-width: 18px;
  height: 18px;
  padding: 0 6px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: var(--color-primary-100);
  color: var(--color-primary-700);
  border: 1px solid var(--color-primary-200);
  border-radius: 999px;
  font-size: 10px;
  font-weight: 700;
  line-height: 1;
  font-variant-numeric: tabular-nums;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.header-btn {
  padding: 4px 10px;
  font-size: 11px;
  font-weight: 500;
  color: var(--text-secondary);
  background: transparent;
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.header-btn:hover:not(:disabled) {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

.header-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.header-btn.danger {
  color: var(--color-danger-600);
  border-color: var(--color-danger-200);
}

.header-btn.danger:hover {
  background: var(--color-danger-50);
  border-color: var(--color-danger-300);
}

.header-close {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-tertiary);
  background: transparent;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.header-close:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

/* Body */
.panel-body {
  flex: 1;
  overflow-y: auto;
  padding: 6px;
}

.category-filter {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 2px 2px 8px;
}

.category-filter-btn {
  flex: 1 0 auto;
  height: 26px;
  padding: 0 9px;
  border-radius: 7px;
  border: 1px solid var(--border-subtle);
  background: var(--bg-primary);
  color: var(--text-secondary);
  font-size: 11px;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}

.category-filter-btn:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

.category-filter-btn.active {
  border-color: var(--color-primary-300);
  background: var(--color-primary-50);
  color: var(--color-primary-700);
}

/* Empty */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 48px 20px;
  color: var(--text-tertiary);
}

.empty-state span {
  font-size: 12px;
}

/* List */
.notification-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

/* Item */
.notification-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 10px 10px 12px;
  background: var(--bg-primary);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  position: relative;
  cursor: pointer;
}

.notification-item:hover {
  border-color: var(--border-default);
  background: var(--bg-secondary);
  transform: translateX(2px);
}

/* P2: 通知项 active 态 */
.notification-item:active {
  transform: scale(0.99);
  background: var(--border-subtle);
  transition-duration: var(--motion-duration-micro);
}

.notification-item.unread {
  background: var(--color-primary-50);
  border-color: var(--color-primary-200);
}

.notification-item.unread:hover {
  border-color: var(--color-primary-300);
}

/* Row 1: Status + Date */
.item-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}

.item-status {
  display: flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
}

.status-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
}

.status-label {
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.03em;
}

.item-status.success .status-dot { background: var(--color-success-500); }
.item-status.success .status-label { color: var(--color-success-600); }

.item-status.error .status-dot { background: var(--color-danger-500); }
.item-status.error .status-label { color: var(--color-danger-600); }

.item-status.warning .status-dot { background: var(--color-warning-500); }
.item-status.warning .status-label { color: var(--color-warning-600); }

.item-status.info .status-dot { background: var(--color-primary-500); }
.item-status.info .status-label { color: var(--color-primary-600); }

.item-status.system .status-dot { background: var(--color-secondary-500); }
.item-status.system .status-label { color: var(--color-secondary-600); }

.category-badge {
  max-width: 58px;
  padding: 2px 6px;
  border-radius: 999px;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  font-size: 10px;
  line-height: 1.2;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.item-date {
  font-size: 10px;
  color: var(--text-tertiary);
  font-family: 'JetBrains Mono', monospace;
}

/* Row 2: Content */
.item-content {
  flex: 1;
  min-width: 0;
  cursor: pointer;
  padding-right: 28px;
}

.item-content:focus-visible {
  outline: 2px solid var(--color-primary-500);
  outline-offset: 2px;
  border-radius: 4px;
}

.item-content:hover .item-title {
  color: var(--color-primary-600);
}

.item-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.item-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-primary);
  line-height: 1.4;
}

.notification-item.unread .item-title {
  font-weight: 700;
}

.item-detail {
  font-size: 12px;
  color: var(--text-primary);
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.item-link {
  position: absolute;
  right: 9px;
  bottom: 9px;
  width: 26px;
  height: 26px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  color: var(--text-tertiary);
  background: var(--bg-elevated);
  cursor: pointer;
  transition: color 0.15s ease, border-color 0.15s ease, background-color 0.15s ease;
}

.item-link:hover,
.item-link:focus-visible {
  outline: none;
  border-color: var(--color-primary-300);
  color: var(--color-primary-700);
  background: var(--color-primary-50);
}

/* Load more */
.load-more {
  padding: 8px 0;
  text-align: center;
}

.load-btn {
  padding: 6px 16px;
  font-size: 11px;
  font-weight: 500;
  color: var(--text-secondary);
  background: transparent;
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.load-btn:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

.loading-text {
  font-size: 11px;
  color: var(--text-tertiary);
}

.panel-enter-active,
.panel-leave-active {
  transition:
    opacity var(--motion-duration-panel) var(--motion-ease-out),
    transform var(--motion-duration-panel) var(--motion-ease-out);
}

.panel-enter-from,
.panel-leave-to {
  opacity: 0;
  transform: translateY(calc(var(--motion-distance-medium) * -1)) scale(var(--motion-scale-enter));
}

.overlay-fade-enter-active,
.overlay-fade-leave-active {
  transition: opacity var(--motion-duration-quick) var(--motion-ease-out);
}

.overlay-fade-enter-from,
.overlay-fade-leave-to {
  opacity: 0;
}

.notification-item {
  animation: itemSlideIn var(--motion-duration-fast) var(--motion-ease-out) backwards;
}

.notification-item:nth-child(1) { animation-delay: 0ms; }
.notification-item:nth-child(2) { animation-delay: 30ms; }
.notification-item:nth-child(3) { animation-delay: 60ms; }
.notification-item:nth-child(4) { animation-delay: 90ms; }
.notification-item:nth-child(5) { animation-delay: 120ms; }
.notification-item:nth-child(6) { animation-delay: 150ms; }
.notification-item:nth-child(7) { animation-delay: 180ms; }
.notification-item:nth-child(8) { animation-delay: 210ms; }
.notification-item:nth-child(9) { animation-delay: 240ms; }
.notification-item:nth-child(10) { animation-delay: 270ms; }

@keyframes itemSlideIn {
  from {
    opacity: 0;
    transform: translateX(calc(var(--motion-distance-small) * -1));
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .notification-item {
    animation-delay: 0ms !important;
  }
}

/* Mobile */
@media (max-width: 768px) {
  .notification-panel {
    position: fixed;
    top: auto;
    bottom: 0;
    left: 0;
    right: 0;
    width: 100%;
    max-height: 85vh;
    border-radius: 16px 16px 0 0;
  }
  
}
</style>
