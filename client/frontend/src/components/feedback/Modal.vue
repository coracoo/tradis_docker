<template>
  <Teleport to="body">
    <Transition name="modal">
      <div 
        v-if="visible" 
        class="modal-overlay" 
        :style="{ zIndex: overlayZIndex }"
        @click.self="handleOverlayClick"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="title ? titleId : null"
        :aria-label="title ? null : ariaLabel"
        :aria-describedby="$slots.default ? descriptionId : null"
      >
        <div
          ref="panelRef"
          class="modal-panel"
          :style="{ width: width, maxWidth: maxWidth }"
          tabindex="-1"
        >
          <div v-if="title || showClose" class="modal-header">
            <h3 v-if="title" :id="titleId" class="modal-title">{{ title }}</h3>
            <button
              v-if="showClose"
              class="modal-close"
              @click="handleClose"
              aria-label="关闭对话框"
            >
              <DynamicIcon name="x" :size="20" />
            </button>
          </div>
          <div :id="descriptionId" class="modal-body">
            <slot />
          </div>
          <div v-if="$slots.footer" class="modal-footer">
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
    default: '对话框'
  },
  width: {
    type: String,
    default: '500px'
  },
  maxWidth: {
    type: String,
    default: '90vw'
  },
  showClose: {
    type: Boolean,
    default: true
  },
  closeOnOverlay: {
    type: Boolean,
    default: false
  },
  closeOnEsc: {
    type: Boolean,
    default: true
  }
})

const emit = defineEmits(['update:visible', 'close'])
const panelRef = ref(null)
const closeOnEscape = computed(() => props.closeOnEsc)

function handleClose(reason = 'button') {
  emit('update:visible', false)
  emit('close', reason)
}

function handleOverlayClick() {
  if (props.closeOnOverlay) {
    handleClose('overlay')
  }
}

const { overlayId, overlayZIndex } = useOverlayController({
  visible: computed(() => props.visible),
  containerRef: panelRef,
  closeOnEscape,
  onClose: () => handleClose('escape')
})

const titleId = `${overlayId}-title`
const descriptionId = `${overlayId}-description`
</script>

<style scoped>
.modal-overlay {
  position: fixed;
  inset: 0;
  background: var(--overlay-bg);
  backdrop-filter: blur(2px);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
}

.modal-panel {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  box-shadow: var(--shadow-xl);
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  outline: none;
  color: var(--text-primary);
  font-family: var(--font-sans, Inter, ui-sans-serif, system-ui, sans-serif);
}

.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-height: 56px;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border-subtle);
}

.modal-title {
  font-size: 1rem;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
}

.modal-close {
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

.modal-close:hover {
  background: var(--color-danger-100);
  border-color: var(--color-danger-200);
  color: var(--color-danger-600);
}

.modal-close:focus-visible,
.modal-footer :deep(button:focus-visible) {
  outline: 2px solid var(--color-primary-500);
  outline-offset: 2px;
}

.modal-body {
  flex: 1;
  overflow-y: auto;
  padding: 20px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  min-height: 58px;
  padding: 12px 20px;
  border-top: 1px solid var(--border-subtle);
  align-items: center;
}

.modal-footer :deep(button) {
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
  cursor: pointer;
  transition: background-color var(--motion-duration-quick) var(--motion-ease-out), border-color var(--motion-duration-quick) var(--motion-ease-out), color var(--motion-duration-quick) var(--motion-ease-out), opacity var(--motion-duration-quick) var(--motion-ease-out);
}

.modal-footer :deep(button:disabled) {
  opacity: 0.6;
  cursor: not-allowed;
}

.modal-enter-active,
.modal-leave-active {
  transition: opacity var(--motion-duration-fast) var(--motion-ease-out);
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-from .modal-panel,
.modal-leave-to .modal-panel {
  transform: translateY(var(--motion-distance-medium)) scale(var(--motion-scale-enter));
}

.modal-enter-active .modal-panel,
.modal-leave-active .modal-panel {
  transition: transform var(--motion-duration-panel) var(--motion-ease-out);
}
</style>
