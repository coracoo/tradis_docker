<template>
  <header class="header" role="banner">
    <!-- 左侧：移动端菜单按钮 + 面包屑 -->
    <div class="header-left">
      <!-- 移动端菜单按钮 -->
      <button 
      class="menu-btn" 
      @click="toggleSidebar"
      aria-label="切换侧边栏"
      :aria-expanded="false"
    >
        <DynamicIcon name="menu" :size="20" />
      </button>

      <!-- 面包屑 -->
      <nav class="breadcrumb" aria-label="面包屑导航">
        <span 
        class="breadcrumb-item" 
        @click="goHome"
        @keydown.enter="goHome"
        tabindex="0"
        role="link"
        aria-label="返回首页"
      >首页</span>
        <span v-if="currentRoute.label" class="breadcrumb-separator">
          <DynamicIcon name="chevron-right" :size="14" />
        </span>
        <span 
        v-if="currentRoute.label" 
        class="breadcrumb-item current"
        aria-current="page"
      >{{ currentRoute.label }}</span>
      </nav>
    </div>

    <!-- 中间：搜索触发区域 -->
    <div 
      class="header-center" 
      @click="focusSearch"
      @keydown.enter="focusSearch"
      @keydown.space.prevent="focusSearch"
      tabindex="0"
      role="button"
      aria-label="打开全局搜索 (Ctrl+K 或 Cmd+K)"
    >
      <div class="search-trigger">
        <DynamicIcon name="search" :size="18" class="search-icon" />
        <span class="search-placeholder">全局搜索...</span>
        <div class="search-shortcut-inline">
          <span class="key">⌘</span>
          <span class="key">K</span>
        </div>
      </div>
    </div>

    <!-- 右侧：工具按钮 -->
    <div class="header-right">
      <!-- QQ 交流群 -->
      <a
        class="header-btn qq-group"
        href="https://qun.qq.com/universal-share/share?ac=1&svctype=5&tempid=h5_group_info&busi_data=eyJncm91cENvZGUiOiI3MDAwNjA2NjMifQ%3D%3D"
        target="_blank"
        rel="noopener noreferrer"
        title="加入项目 QQ 群：700060663"
        aria-label="加入项目 QQ 群：700060663"
      >
        <DynamicIcon name="message-circle" :size="16" class="qq-group-icon" />
        <span class="qq-group-text">加入项目 QQ 群</span>
      </a>

      <!-- 主题切换 -->
      <button 
      class="header-btn" 
      @click="toggleTheme" 
      :title="isDark ? '切换亮色模式' : '切换暗色模式'"
      :aria-label="isDark ? '切换到亮色模式' : '切换到暗色模式'"
    >
        <DynamicIcon :name="isDark ? 'sun' : 'moon'" :size="18" />
      </button>

      <!-- 通知 -->
      <button
        class="header-btn"
        @click="showNotifications"
        title="通知"
        aria-label="查看通知"
        :aria-badge="notificationCount > 0 ? notificationCount : null"
      >
        <DynamicIcon name="bell" :size="18" />
        <Transition name="notification-badge">
          <span
            v-if="notificationCount > 0"
            :key="notificationBadgeText"
            class="notification-badge"
          >{{ notificationBadgeText }}</span>
        </Transition>
      </button>

      <!-- 设置 -->
      <button 
      class="header-btn" 
      @click="goToSettings" 
      title="设置"
      aria-label="打开设置"
    >
        <DynamicIcon name="settings" :size="18" />
      </button>

      <!-- 快捷操作 -->
      <button
        class="header-btn quick-action"
        :class="{ 'is-open': quickActionsOpen }"
        @click="showQuickActions"
        title="快捷操作"
        aria-label="打开快捷操作菜单"
        :aria-expanded="quickActionsOpen"
      >
        <span class="quick-action-icon" aria-hidden="true">
          <DynamicIcon name="plus" :size="18" />
        </span>
      </button>
    </div>
  </header>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useThemeStore } from '@/stores/theme.js'
import DynamicIcon from '../ui/DynamicIcon.vue'

const props = defineProps({
  notificationCount: {
    type: Number,
    default: 0
  },
  quickActionsOpen: { type: Boolean, default: false }
})

const emit = defineEmits(['toggle-sidebar', 'show-quick-actions', 'focus-search', 'show-notifications'])

const route = useRoute()
const router = useRouter()
const themeStore = useThemeStore()

const isDark = computed(() => themeStore.isDark)
const notificationBadgeText = computed(() => props.notificationCount > 99 ? '99+' : String(props.notificationCount))

const currentRoute = computed(() => {
  const path = route.path
  const label = [...route.matched].reverse().find(record => record.meta?.title)?.meta.title || ''
  return { path, label }
})

function toggleSidebar() {
  emit('toggle-sidebar')
}

function goHome() {
  router.push('/')
}

function toggleTheme() {
  themeStore.toggleDarkMode()
}

function showNotifications() {
  emit('show-notifications')
}

function showQuickActions() {
  // 显示快捷操作菜单
  emit('show-quick-actions')
}

function goToSettings() {
  router.push('/settings')
}

function focusSearch() {
  emit('focus-search')
}
</script>

<style scoped>
.header {
  height: 64px;
  background: var(--sidebar-bg);
  -webkit-backdrop-filter: blur(14px);
  backdrop-filter: blur(14px);
  border-bottom: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  position: sticky;
  top: 0;
  z-index: 50;
}

/* 左侧 */
.header-left {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-shrink: 0;
}

.menu-btn {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  color: var(--text-secondary);
  background: transparent;
  border: none;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.menu-btn:hover {
  background-color: var(--bg-secondary);
  color: var(--text-primary);
}

@media (min-width: 769px) {
  .menu-btn {
    display: none;
  }
}

/* 面包屑 */
.breadcrumb {
  display: flex;
  align-items: center;
  gap: 8px;
}

.breadcrumb-item {
  font-size: 0.875rem;
  color: var(--text-secondary);
  cursor: pointer;
  transition: color var(--motion-duration-quick) var(--motion-ease-out);
}

.breadcrumb-item:hover,
  .breadcrumb-item:focus-visible {
  color: var(--text-primary);
  outline: none;
  box-shadow: 0 0 0 2px var(--color-primary-500);
  border-radius: 4px;
}

.breadcrumb-item.current {
  color: var(--text-primary);
  font-weight: 500;
  cursor: default;
}

.breadcrumb-separator {
  color: var(--text-tertiary);
  display: flex;
  align-items: center;
}

/* 中间搜索触发区域 */
.header-center {
  flex: 1;
  max-width: 480px;
  margin: 0 24px;
  cursor: pointer;
}

.search-trigger {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 14px;
  height: 40px;
  background: var(--bg-secondary);
  border: 1px solid transparent;
  border-radius: 10px;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.search-trigger:hover,
  .search-trigger:focus-visible {
  background: var(--bg-tertiary);
  border-color: var(--border-subtle);
  outline: none;
  box-shadow: 0 0 0 2px var(--color-primary-500);
}

.search-icon {
  color: var(--text-tertiary);
  flex-shrink: 0;
}

.search-placeholder {
  flex: 1;
  font-size: 0.875rem;
  color: var(--text-tertiary);
}

.search-shortcut-inline {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.search-shortcut-inline .key {
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.625rem;
  padding: 2px 5px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  color: var(--text-tertiary);
}

@media (min-width: 769px) and (max-width: 900px) {
  .header-center {
    flex: 0 0 40px;
    max-width: 40px;
    margin: 0 8px;
  }

  .search-trigger {
    box-sizing: border-box;
    justify-content: center;
    width: 40px;
    padding: 0;
  }

  .search-placeholder,
  .search-shortcut-inline {
    display: none;
  }
}

@media (max-width: 768px) {
  .header-center {
    display: none;
  }
}

/* 右侧工具按钮 */
.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.header-btn {
  position: relative;
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  color: var(--text-secondary);
  background: transparent;
  border: none;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.header-btn:hover {
  background-color: var(--bg-secondary);
  color: var(--text-primary);
}

.quick-action {
  background: var(--color-primary-500);
  color: var(--text-inverse);
}

.quick-action:hover {
  background: var(--color-primary-600);
  color: var(--text-inverse);
}

.quick-action-icon {
  display: grid;
  place-items: center;
  transition: transform var(--motion-duration-panel) var(--motion-ease-out);
}

.quick-action.is-open .quick-action-icon {
  transform: rotate(45deg) scale(var(--motion-scale-enter));
}

@media (prefers-reduced-motion: reduce) {
  .quick-action-icon {
    transition-duration: 1ms;
  }
}

.header-btn.qq-group {
  width: auto;
  height: 36px;
  padding: 0 12px;
  gap: 6px;
  font-size: 0.8125rem;
  font-weight: 500;
  text-decoration: none;
  color: var(--text-inverse);
  background: var(--color-primary-500);
  border: 1px solid var(--color-primary-600);
}

.header-btn.qq-group:hover {
  background: var(--color-primary-600);
  color: var(--text-inverse);
  border-color: var(--color-primary-700);
}

.notification-badge {
  position: absolute;
  top: 5px;
  right: 4px;
  min-width: 17px;
  height: 17px;
  padding: 0 5px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: var(--color-danger-500);
  color: var(--text-inverse);
  border: 2px solid var(--bg-elevated);
  border-radius: 999px;
  font-size: 0.625rem;
  font-weight: 700;
  line-height: 1;
  font-variant-numeric: tabular-nums;
  box-shadow: 0 2px 6px color-mix(in srgb, var(--color-danger-500) 28%, transparent);
  transform: translate(30%, -30%);
}

.notification-badge-enter-active {
  animation: notification-badge-pop var(--motion-duration-fast) var(--motion-ease-out);
}

.notification-badge-leave-active {
  transition:
    opacity var(--motion-duration-quick) var(--motion-ease-out),
    transform var(--motion-duration-quick) var(--motion-ease-out);
}

.notification-badge-leave-to {
  opacity: 0;
  transform: translate(30%, -30%) scale(var(--motion-scale-enter));
}

@keyframes notification-badge-pop {
  0% {
    opacity: 0;
    transform:
      translate(
        calc(30% + var(--motion-distance-medium)),
        calc(-30% - var(--motion-distance-medium))
      )
      scale(var(--motion-scale-enter));
  }
  68% {
    opacity: 1;
    transform: translate(30%, -30%) scale(var(--motion-scale-pop));
  }
  100% {
    opacity: 1;
    transform: translate(30%, -30%) scale(1);
  }
}

@media (max-width: 480px) {
  .header {
    gap: 6px;
    padding: 0 12px;
    overflow: hidden;
  }

  .header-left {
    flex: 1 1 auto;
    min-width: 0;
    gap: 8px;
  }

  .breadcrumb {
    min-width: 0;
    gap: 5px;
  }

  .breadcrumb-item.current {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .header-right {
    gap: 2px;
  }

  .header-btn {
    width: 36px;
    height: 36px;
  }

  .header-btn.qq-group {
    width: 36px;
    height: 32px;
    padding: 0;
  }

  .qq-group-text {
    display: none;
  }

  .header-btn.quick-action {
    width: 40px;
    height: 40px;
  }
}

/* 搜索快捷键 */
</style>
