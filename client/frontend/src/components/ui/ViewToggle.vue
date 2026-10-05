<template>
  <div class="view-toggle" role="group" aria-label="视图切换">
    <span class="view-toggle-indicator" :data-view="modelValue" aria-hidden="true"></span>
    <button
      class="view-toggle-button"
      :class="{ active: modelValue === 'grid' }"
      data-view-option="grid"
      title="卡片网格"
      aria-label="切换到卡片网格视图"
      :aria-pressed="modelValue === 'grid'"
      @click="$emit('update:modelValue', 'grid')"
    >
      <DynamicIcon name="grid" :size="16" />
    </button>
    <button
      class="view-toggle-button"
      :class="{ active: modelValue === 'table' }"
      data-view-option="table"
      title="表格列表"
      aria-label="切换到表格列表视图"
      :aria-pressed="modelValue === 'table'"
      @click="$emit('update:modelValue', 'table')"
    >
      <DynamicIcon name="table" :size="16" />
    </button>
  </div>
</template>

<script setup>
import DynamicIcon from './DynamicIcon.vue'

defineProps({
  modelValue: {
    type: String,
    default: 'grid',
    validator: (value) => ['grid', 'table'].includes(value)
  }
})

defineEmits(['update:modelValue'])
</script>

<style scoped>
.view-toggle {
  position: relative;
  display: grid;
  grid-template-columns: repeat(2, 32px);
  gap: 2px;
  flex: 0 0 auto;
  padding: 2px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--bg-tertiary);
}

.view-toggle-indicator {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 32px;
  height: 32px;
  border-radius: 6px;
  background: var(--bg-elevated);
  box-shadow: var(--shadow-sm);
  transform: translateX(0);
  transition: transform var(--motion-duration-fast) var(--motion-ease-out);
  pointer-events: none;
}

.view-toggle-indicator[data-view='table'] {
  transform: translateX(34px);
}

.view-toggle-button {
  position: relative;
  z-index: 1;
  width: 32px;
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--text-tertiary);
  cursor: pointer;
  transition: color var(--motion-duration-quick) var(--motion-ease-out);
}

.view-toggle-button:hover {
  color: var(--text-secondary);
}

.view-toggle-button.active {
  color: var(--text-primary);
}

.view-toggle-button:focus-visible {
  outline: 2px solid var(--color-primary-500);
  outline-offset: 1px;
}
</style>
