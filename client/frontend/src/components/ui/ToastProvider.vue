<template>
  <Teleport to="body">
    <TransitionGroup name="toast" tag="div" class="toast-container">
      <div
        v-for="toast in toasts"
        :key="toast.id"
        class="toast-item"
        :class="`toast-${toast.type}`"
        @click="removeToast(toast.id)"
      >
        <div class="toast-icon">
          <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" v-html="getIcon(toast.type)"></svg>
        </div>
        <div class="toast-content">
          <span class="toast-message">{{ toast.message }}</span>
        </div>
        <button class="toast-close" @click.stop="removeToast(toast.id)">
          <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <line x1="18" y1="6" x2="6" y2="18"/>
            <line x1="6" y1="6" x2="18" y2="18"/>
          </svg>
        </button>
      </div>
    </TransitionGroup>
  </Teleport>
</template>

<script setup>
import { computed } from 'vue'
import { useUiStore } from '@/stores/ui.js'

const uiStore = useUiStore()

const toasts = computed(() => uiStore.toasts)

const icons = {
  success: '<path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/>',
  error: '<circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/>',
  warning: '<path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/>',
  info: '<circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/>'
}

function getIcon(type) {
  return icons[type] || icons.info
}

function removeToast(id) {
  uiStore.removeToast(id)
}
</script>

<style scoped>
.toast-container {
  position: fixed;
  top: 20px;
  right: 20px;
  z-index: 9999;
  display: flex;
  flex-direction: column;
  gap: 12px;
  pointer-events: none;
}

.toast-item {
  display: flex;
  align-items: center;
  gap: 12px;
  width: min(450px, calc(100vw - 32px));
  padding: 14px 18px;
  background: var(--bg-elevated);
  border-radius: 12px;
  box-shadow: var(--shadow-lg);
  border-left: 4px solid;
  min-width: min(300px, calc(100vw - 32px));
  max-width: 100%;
  pointer-events: auto;
  cursor: pointer;
}

.toast-success {
  border-left-color: var(--color-success-500);
}

.toast-error {
  border-left-color: var(--color-danger-500);
}

.toast-warning {
  border-left-color: var(--color-warning-500);
}

.toast-info {
  border-left-color: var(--color-primary-500);
}

.toast-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.toast-icon svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.toast-success .toast-icon {
  color: var(--color-success-500);
}

.toast-error .toast-icon {
  color: var(--color-danger-500);
}

.toast-warning .toast-icon {
  color: var(--color-warning-500);
}

.toast-info .toast-icon {
  color: var(--color-primary-500);
}

.toast-content {
  flex: 1;
  min-width: 0;
}

.toast-message {
  display: block;
  font-size: 0.9375rem;
  color: var(--text-primary);
  line-height: 1.4;
  overflow-wrap: anywhere;
}

.toast-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  background: transparent;
  color: var(--text-tertiary);
  cursor: pointer;
  border-radius: 6px;
  transition:
    color var(--motion-duration-quick) var(--motion-ease-out),
    background-color var(--motion-duration-quick) var(--motion-ease-out);
  flex-shrink: 0;
}

.toast-close:hover {
  background: var(--bg-tertiary);
  color: var(--text-primary);
}

.toast-close svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.toast-enter-active,
.toast-leave-active {
  transition:
    opacity var(--motion-duration-fast) var(--motion-ease-out),
    transform var(--motion-duration-fast) var(--motion-ease-out);
}

.toast-move {
  transition: transform var(--motion-duration-fast) var(--motion-ease-out);
}

.toast-enter-from {
  opacity: 0;
  transform: translateY(calc(var(--motion-distance-small) * -1)) scale(var(--motion-scale-enter));
}

.toast-leave-to {
  opacity: 0;
  transform: translateY(calc(var(--motion-distance-micro) * -1)) scale(var(--motion-scale-enter));
}

@media (max-width: 640px) {
  .toast-container {
    top: 12px;
    right: 12px;
    left: 12px;
  }

  .toast-item {
    width: 100%;
    min-width: 0;
    padding: 12px 14px;
  }
}
</style>
