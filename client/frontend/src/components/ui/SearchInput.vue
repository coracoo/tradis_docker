<template>
  <div 
    class="search-input-wrapper" 
    :class="{ 'is-focused': isFocused, 'is-loading': loading }"
    role="search"
  >
    <div class="search-icon">
      <DynamicIcon name="search" :size="18" />
    </div>
    <input
      ref="inputRef"
      :value="modelValue"
      type="search"
      class="search-input"
      name="tradis-filter-query"
      autocomplete="one-time-code"
      autocapitalize="none"
      spellcheck="false"
      data-1p-ignore
      data-lpignore="true"
      data-form-type="other"
      :placeholder="placeholder"
      :aria-label="placeholder"
      :aria-busy="loading"
      @input="$emit('update:modelValue', $event.target.value)"
      @focus="isFocused = true"
      @blur="isFocused = false"
      @keyup.enter="$emit('search', modelValue)"
    />
    <div v-if="loading" class="loading-spinner">
      <DynamicIcon name="refresh" :size="16" class="animate-spin" />
    </div>
    <button
      v-else-if="modelValue"
      class="clear-btn"
      @click="clear"
      title="清除"
      aria-label="清除搜索内容"
    >
      <DynamicIcon name="x" :size="16" />
    </button>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import DynamicIcon from './DynamicIcon.vue'

defineProps({
  modelValue: {
    type: String,
    default: ''
  },
  placeholder: {
    type: String,
    default: '搜索...'
  },
  loading: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['update:modelValue', 'search', 'clear'])

const inputRef = ref(null)
const isFocused = ref(false)

function clear() {
  emit('update:modelValue', '')
  emit('clear')
  inputRef.value?.focus()
}
</script>

<style scoped>
.search-input-wrapper {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  height: 40px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  transition: all var(--motion-duration-fast) var(--motion-ease-out);
}

.search-input-wrapper:hover {
  border-color: var(--border-default);
}

/* P2: Focus 时的动画效果 */
.search-input-wrapper.is-focused {
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
  transform: translateY(-1px);
}

.search-input-wrapper.is-focused .search-icon {
  color: var(--color-primary-500);
  transform: scale(1.1);
}

.search-icon {
  transition: all var(--motion-duration-fast) var(--motion-ease-out);
}

.search-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-tertiary);
  flex-shrink: 0;
}

.search-icon svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.search-input {
  flex: 1;
  border: none;
  background: transparent;
  color: var(--text-secondary, #64748b);
  font-size: 0.8125rem;
  font-weight: 400;
  outline: none;
  min-width: 0;
}

.search-input::placeholder {
  color: var(--text-tertiary, #94a3b8);
}

.loading-spinner {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-tertiary);
}

.loading-spinner svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.animate-spin {
  animation: spin 1s linear infinite;
}

.clear-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--bg-tertiary);
  border: none;
  color: var(--text-tertiary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  flex-shrink: 0;
  padding: 0;
}

.clear-btn:hover {
  background: var(--border-default);
  color: var(--text-primary);
  transform: scale(1.1);
}

/* P2: 清除按钮 active 态 */
.clear-btn:active {
  transform: scale(0.9);
  transition-duration: var(--motion-duration-micro);
}

.clear-btn svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}
</style>
