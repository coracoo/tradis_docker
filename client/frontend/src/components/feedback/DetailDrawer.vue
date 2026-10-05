<template>
  <Teleport to="body">
    <Transition name="drawer">
      <div
        v-if="visible"
        class="drawer-overlay"
        :style="{ zIndex: overlayZIndex }"
        @click.self="handleOverlayClick"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="title ? titleId : null"
        :aria-label="title ? null : ariaLabel"
      >
        <div ref="panelRef" class="drawer-panel" :style="{ width: panelWidth }" tabindex="-1">
        <!-- 头部 -->
        <div class="drawer-header">
          <slot name="header-custom">
            <div class="header-content">
              <div class="icon-wrapper" v-if="icon">
                <slot name="icon">
                  <DynamicIcon :name="icon" :size="22" />
                </slot>
              </div>
              <div class="header-info">
                <h3 :id="titleId" class="drawer-title">{{ title }}</h3>
                <p v-if="subtitle" class="drawer-subtitle">{{ subtitle }}</p>
              </div>
            </div>
          </slot>
          <div class="header-actions">
            <slot name="header-actions" />
            <button class="close-btn" @click="handleClose" aria-label="关闭抽屉">
              <DynamicIcon name="x" :size="18" />
            </button>
          </div>
        </div>

        <!-- 内容区 -->
        <div class="drawer-body">
          <slot />
        </div>

        <!-- 底部 -->
        <div v-if="$slots.footer" class="drawer-footer">
          <slot name="footer" />
        </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup>
import { computed, ref } from 'vue'
import DynamicIcon from '../ui/DynamicIcon.vue'
import { useOverlayController } from '@/composables/useOverlayController.js'

const props = defineProps({
  visible: {
    type: Boolean,
    default: false
  },
  title: {
    type: String,
    default: ''
  },
  ariaLabel: {
    type: String,
    default: '详情抽屉'
  },
  subtitle: {
    type: String,
    default: ''
  },
  icon: {
    type: String,
    default: ''
  },
  width: {
    type: [String, Number],
    default: 600
  },
  closeOnOverlay: {
    type: Boolean,
    default: true
  },
  closeOnEsc: {
    type: Boolean,
    default: true
  }
})

const emit = defineEmits(['update:visible', 'close'])
const panelRef = ref(null)

const panelWidth = computed(() => {
  if (typeof props.width === 'number') {
    return `${props.width}px`
  }
  return props.width
})

function handleClose(reason = 'button') {
  emit('update:visible', false)
  emit('close', reason)
}

function handleOverlayClick() {
  if (props.closeOnOverlay) handleClose('overlay')
}

const { overlayId, overlayZIndex } = useOverlayController({
  visible: computed(() => props.visible),
  containerRef: panelRef,
  closeOnEscape: computed(() => props.closeOnEsc),
  onClose: () => handleClose('escape')
})

const titleId = `${overlayId}-title`
</script>

<style scoped>
.drawer-overlay {
  position: fixed;
  inset: 0;
  background: var(--overlay-bg);
  backdrop-filter: blur(2px);
  display: flex;
  justify-content: flex-end;
}

.drawer-panel {
  background: var(--bg-elevated);
  border-left: 1px solid var(--border-subtle);
  height: 100%;
  display: flex;
  flex-direction: column;
  box-shadow: var(--shadow-xl);
  max-width: 90vw;
  border-top-left-radius: 12px;
  border-bottom-left-radius: 12px;
  outline: none;
}

/* 头部 */
.drawer-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  min-height: 64px;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
}

.header-content {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  flex: 1;
  min-width: 0;
}

.icon-wrapper {
  width: 38px;
  height: 38px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--color-primary-100);
  border-radius: 10px;
  color: var(--color-primary-700);
  flex-shrink: 0;
}

.header-info {
  flex: 1;
  min-width: 0;
}

.drawer-title {
  font-size: 1rem;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
  line-height: 1.4;
  word-break: break-word;
}

.drawer-subtitle {
  font-size: 0.875rem;
  color: var(--text-secondary);
  margin: 4px 0 0;
}

.close-btn {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: transparent;
  border: 1px solid transparent;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  flex-shrink: 0;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

.close-btn:hover {
  background: var(--color-danger-100);
  border-color: var(--color-danger-200);
  color: var(--color-danger-600);
}

.close-btn:focus-visible,
.drawer-footer :deep(button:focus-visible) {
  outline: 2px solid var(--color-primary-500);
  outline-offset: 2px;
}

/* 内容区 */
.drawer-body {
  flex: 1;
  overflow-y: auto;
  padding: 20px;
  background: var(--bg-primary);
}

/* 底部 */
.drawer-footer {
  min-height: 58px;
  padding: 12px 20px;
  border-top: 1px solid var(--border-subtle);
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  flex-shrink: 0;
}

.drawer-footer :deep(button) {
  min-height: 38px;
  padding: 0 16px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border-radius: 8px;
  font: inherit;
  font-size: 0.875rem;
  font-weight: 500;
}

.drawer-enter-active,
.drawer-leave-active {
  transition: opacity var(--motion-duration-fast) var(--motion-ease-out);
}

.drawer-enter-from,
.drawer-leave-to {
  opacity: 0;
}

.drawer-enter-from .drawer-panel,
.drawer-leave-to .drawer-panel {
  transform: translateX(var(--motion-distance-medium));
  opacity: 0;
}

.drawer-enter-active .drawer-panel,
.drawer-leave-active .drawer-panel {
  transition:
    opacity var(--motion-duration-panel) var(--motion-ease-out),
    transform var(--motion-duration-panel) var(--motion-ease-out);
}

.drawer-enter-to .drawer-panel,
.drawer-leave-from .drawer-panel {
  transform: translateX(0);
  opacity: 1;
}
</style>
