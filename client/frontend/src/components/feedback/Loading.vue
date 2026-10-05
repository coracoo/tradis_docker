<template>
  <div 
    v-if="visible" 
    class="loading-overlay" 
    :class="{ 'loading-fullscreen': fullscreen }"
    role="status"
    aria-live="polite"
    aria-busy="true"
  >
    <div class="loading-content">
      <div class="loading-spinner" aria-hidden="true">
        <DynamicIcon name="refresh" :size="40" class="loading-icon" />
      </div>
      <p v-if="text" class="loading-text">{{ text }}</p>
    </div>
  </div>
</template>

<script setup>
import DynamicIcon from '../ui/DynamicIcon.vue'

defineProps({
  visible: {
    type: Boolean,
    default: false
  },
  text: {
    type: String,
    default: '加载中...'
  },
  fullscreen: {
    type: Boolean,
    default: true
  }
})
</script>

<style scoped>
.loading-overlay {
  position: fixed;
  inset: 0;
  z-index: 9998;
  background: var(--overlay-bg);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
}

.loading-fullscreen {
  position: fixed;
}

.loading-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  padding: 32px 48px;
  background: var(--bg-elevated);
  border-radius: 16px;
  box-shadow: var(--shadow-xl);
}

.loading-spinner {
  color: var(--color-primary-500);
  animation: spin 1s linear infinite;
}

.loading-icon {
  color: inherit;
}

.loading-text {
  font-size: 0.9375rem;
  color: var(--text-secondary);
  margin: 0;
}
</style>
