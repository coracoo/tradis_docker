<template>
  <Teleport to="body">
    <!-- 遮罩 -->
    <Transition name="overlay-fade">
      <div
        v-if="visible"
        class="quick-actions-overlay"
        @click="close"
      ></div>
    </Transition>
    
    <!-- 菜单 -->
    <Transition name="menu">
      <div v-if="visible" class="quick-actions-menu" :style="menuStyle">
        <div class="menu-header">
          <span class="menu-title">快捷操作</span>
          <button class="menu-close" @click="close">
            <DynamicIcon name="x" :size="16" />
          </button>
        </div>
        
        <div class="menu-section">
          <div class="section-title">创建</div>
          <div class="menu-items">
            <button class="menu-item" @click="handleAction('compose')">
              <div class="item-icon secondary">
                <DynamicIcon name="compose" :size="18" />
              </div>
              <div class="item-info">
                <span class="item-label">创建 Compose</span>
                <span class="item-desc">编写或粘贴 YAML</span>
              </div>
            </button>
            
            <button class="menu-item" @click="handleAction('appstore')">
              <div class="item-icon accent">
                <DynamicIcon name="store" :size="18" />
              </div>
              <div class="item-info">
                <span class="item-label">部署应用</span>
                <span class="item-desc">从应用商店安装</span>
              </div>
            </button>

            <button v-if="isFullEdition" class="menu-item" @click="handleAction('remote')">
              <div class="item-icon primary">
                <DynamicIcon name="server" :size="18" />
              </div>
              <div class="item-info">
                <span class="item-label">添加远程设备</span>
                <span class="item-desc">生成设备登录凭据</span>
              </div>
            </button>
          </div>
        </div>
        
        <div class="menu-divider"></div>
        
        <div class="menu-section">
          <div class="section-title">导航</div>
          <div class="menu-items">
            <button class="menu-item" @click="handleAction('containers')">
              <div class="item-icon">
                <DynamicIcon name="container" :size="18" />
              </div>
              <span class="item-label">容器列表</span>
            </button>
            
            <button class="menu-item" @click="handleAction('images')">
              <div class="item-icon">
                <DynamicIcon name="image" :size="18" />
              </div>
              <span class="item-label">镜像管理</span>
            </button>
            
            <button class="menu-item" @click="handleAction('volumes')">
              <div class="item-icon">
                <DynamicIcon name="database" :size="18" />
              </div>
              <span class="item-label">数据卷</span>
            </button>
            
            <button class="menu-item" @click="handleAction('navigation')">
              <div class="item-icon">
                <DynamicIcon name="navigation" :size="18" />
              </div>
              <span class="item-label">应用导航</span>
            </button>
          </div>
        </div>
        
        <div class="menu-divider"></div>
        
        <!-- 账号操作 -->
        <div class="menu-section">
          <div class="section-title">账号</div>
          <div class="menu-items">
            <button class="menu-item logout-item" @click="handleLogout">
              <div class="item-icon danger">
                <DynamicIcon name="logout" :size="18" />
              </div>
              <span class="item-label">退出登录</span>
            </button>
          </div>
        </div>
        
        <div class="menu-divider"></div>
        
        <div class="menu-footer">
          <div class="keyboard-hint">
            <span class="key">Esc</span>
            <span>关闭</span>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import DynamicIcon from '../ui/DynamicIcon.vue'
import * as authApi from '@/api/auth.js'
import { useUiStore } from '@/stores/ui.js'

const props = defineProps({
  visible: {
    type: Boolean,
    default: false
  },
  triggerRef: {
    type: Object,
    default: null
  }
})

const emit = defineEmits(['close', 'action'])

const router = useRouter()
const uiStore = useUiStore()
const isFullEdition = __TRADIS_EDITION__ === 'full'

// 菜单位置 - 默认在右上角
const menuStyle = computed(() => {
  if (props.triggerRef) {
    const rect = props.triggerRef.getBoundingClientRect()
    return {
      position: 'fixed',
      top: `${rect.bottom + 8}px`,
      right: `${window.innerWidth - rect.right}px`
    }
  }
  return {
    position: 'fixed',
    top: '72px',
    right: '20px'
  }
})

// 处理操作
function handleAction(action) {
  if (action === 'remote') {
    if (isFullEdition) router.push({ path: '/settings', query: { section: 'remoteNodes' } })
    close()
    return
  }
  const routes = {
    compose: { path: '/compose', query: { action: 'create' } },
    appstore: '/appstore',
    containers: '/containers',
    images: '/images',
    volumes: '/volumes',
    navigation: '/navigation'
  }
  
  const routeConfig = routes[action]
  if (routeConfig) {
    if (typeof routeConfig === 'string') {
      router.push(routeConfig)
    } else {
      router.push(routeConfig)
    }
  }
  
  emit('action', action)
  close()
}

// 处理退出登录
async function handleLogout() {
  const confirmed = await uiStore.confirm({
    type: 'warning',
    title: '退出登录',
    message: '确定要退出登录吗？',
    confirmText: '退出'
  })
  if (!confirmed) {
    return
  }
  
  try {
    // 调用退出登录 API（如果有的话）
    await authApi.logout?.()
  } catch (e) {
    // 忽略错误，继续清除本地状态
  }
  
  // 清除本地存储的登录状态
  localStorage.removeItem('token')
  localStorage.removeItem('loggedIn')
  
  // 跳转到登录页
  router.push('/login')
  
  emit('action', 'logout')
  close()
}

function close() {
  emit('close')
}

// 键盘事件
function handleKeydown(e) {
  if (e.key === 'Escape' && props.visible) {
    close()
  }
}

onMounted(() => {
  document.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
})
</script>

<style scoped>
.quick-actions-overlay {
  position: fixed;
  inset: 0;
  z-index: 199;
}

.quick-actions-menu {
  width: 280px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  box-shadow: 0 20px 40px var(--shadow-lg);
  z-index: 200;
  overflow: hidden;
  transform-origin: top right;
}

.menu-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--border-subtle);
}

.menu-title {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--text-primary);
}

.menu-close {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  color: var(--text-tertiary);
  background: transparent;
  border: none;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.menu-close:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

.menu-section {
  padding: 8px;
}

.section-title {
  padding: 8px 12px 4px;
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-tertiary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.menu-items {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.menu-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 8px;
  background: transparent;
  border: none;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  text-align: left;
  user-select: none;
}

.menu-item:hover {
  background: var(--bg-secondary);
  transform: translateX(2px);
}

/* P2: 菜单项 active 态 */
.menu-item:active {
  transform: scale(0.98);
  background: var(--border-subtle);
  transition-duration: var(--motion-duration-micro);
}

/* 退出登录项特殊样式 */
.logout-item:hover {
  background: var(--color-danger-50);
}

.logout-item:hover .item-label {
  color: var(--color-danger-600);
}

.item-icon {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  flex-shrink: 0;
}

.item-icon.primary {
  background: var(--color-primary-100);
  color: var(--color-primary-600);
}

.item-icon.secondary {
  background: var(--color-secondary-100);
  color: var(--color-secondary-600);
}

.item-icon.accent {
  background: var(--color-accent-100);
  color: var(--color-accent-600);
}

.item-icon.danger {
  background: var(--color-danger-100);
  color: var(--color-danger-600);
}

.item-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.item-label {
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--text-primary);
}

.item-desc {
  font-size: 0.75rem;
  color: var(--text-tertiary);
}

.menu-divider {
  height: 1px;
  background: var(--border-subtle);
  margin: 0 8px;
}

.menu-footer {
  padding: 10px 16px;
  background: var(--bg-secondary);
}

.keyboard-hint {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 0.75rem;
  color: var(--text-tertiary);
}

.key {
  font-family: 'JetBrains Mono', monospace;
  padding: 2px 6px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  font-size: 0.625rem;
}

.menu-enter-active,
.menu-leave-active {
  transition:
    opacity var(--motion-duration-fast) var(--motion-ease-out),
    transform var(--motion-duration-fast) var(--motion-ease-out);
}

.menu-enter-from,
.menu-leave-to {
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
</style>
