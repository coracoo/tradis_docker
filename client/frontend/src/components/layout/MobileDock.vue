<template>
  <nav class="mobile-dock" role="navigation" aria-label="移动端底部导航">
    <div class="dock-items">
      <button
        v-for="item in dockItems"
        :key="item.path"
        class="dock-item"
        :class="{ 'dock-item-active': isActive(item.path) }"
        :aria-label="item.label"
        :aria-current="isActive(item.path) ? 'page' : null"
        @click="navigate(item.path)"
      >
        <div class="dock-icon">
          <DynamicIcon :name="item.icon" :size="22" />
        </div>
        <span class="dock-label">{{ item.label }}</span>
      </button>
    </div>
  </nav>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { sidebarCatalog } from '@edition/sidebar-catalog'
import DynamicIcon from '../ui/DynamicIcon.vue'

const route = useRoute()
const router = useRouter()
const primaryDockPaths = new Set(['/', '/containers', '/compose', '/appstore'])
const moreRoutePrefixes = sidebarCatalog.flatMap(section => section.items.map(item => item.path))
  .filter(path => !primaryDockPaths.has(path))

// Dock 项目 - 统一显示容器+Compose，无项目页面
const dockItems = computed(() => [
  { path: '/', label: '概览', icon: 'dashboard' },
  { path: '/containers', label: '容器', icon: 'container' },
  { path: '/compose', label: 'Compose', icon: 'compose' },
  { path: '/appstore', label: '商店', icon: 'store' },
  { path: '/more', label: '更多', icon: 'more-vertical' }
])

function isActive(path) {
  if (path === '/') {
    return route.path === '/'
  }
  if (path === '/more') {
    return [...moreRoutePrefixes, '/settings'].some(prefix => route.path.startsWith(prefix))
  }
  return route.path.startsWith(path)
}

function navigate(path) {
  if (path === '/more') {
    // 可以展开更多菜单
    return
  }
  router.push(path)
}
</script>

<style scoped>
.mobile-dock {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  height: 64px;
  background: var(--bg-elevated);
  border-top: 1px solid var(--border-subtle);
  z-index: 100;
  display: none;
}

.dock-items {
  display: flex;
  align-items: center;
  justify-content: space-around;
  height: 100%;
  padding: 0 8px;
  padding-bottom: env(safe-area-inset-bottom, 0);
}

.dock-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 8px 12px;
  border-radius: 12px;
  color: var(--text-tertiary);
  background: transparent;
  border: none;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  flex: 1;
  max-width: 80px;
}

.dock-item:hover {
  color: var(--text-secondary);
}

.dock-item-active {
  color: var(--color-primary-600);
}

.dock-icon {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.dock-label {
  font-size: 0.625rem;
  font-weight: 500;
}

/* 只在移动端显示 */
@media (max-width: 768px) {
  .mobile-dock {
    display: block;
  }
}
</style>
