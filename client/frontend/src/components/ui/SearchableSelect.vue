<template>
  <div class="searchable-select" :class="{ 'is-open': isOpen }">
    <!-- 触发框：显示当前值，点击展开下拉 -->
    <div
      class="select-trigger"
      role="combobox"
      tabindex="0"
      aria-haspopup="listbox"
      :aria-expanded="String(isOpen)"
      @click="toggleOpen"
      @keydown="handleTriggerKeydown"
    >
      <span class="trigger-text" :class="{ 'is-placeholder': !modelValue }">
        {{ modelValue || placeholder }}
      </span>
      <span class="arrow-icon">
        <DynamicIcon name="chevron-down" :size="16" :class="{ 'is-open': isOpen }" />
      </span>
    </div>

    <!-- 下拉面板 -->
    <div v-if="isOpen" class="select-dropdown">
      <!-- 搜索框：过滤候选；无精确匹配时可直接使用输入值 -->
      <div class="dropdown-search">
        <input
          ref="searchInput"
          v-model="searchQuery"
          type="search"
          class="search-input"
          name="tradis-select-query"
          autocomplete="one-time-code"
          autocapitalize="none"
          spellcheck="false"
          data-1p-ignore
          data-lpignore="true"
          data-form-type="other"
          :placeholder="searchPlaceholder"
          @click.stop
          @keydown="handleSearchKeydown"
        />
      </div>

      <!-- 选项列表 -->
      <div class="options-list" role="listbox">
        <div
          v-if="clearable"
          class="option clear-option"
          role="option"
          :aria-selected="String(modelValue === '')"
          @click="selectValue('')"
        >
          <span class="option-text">{{ clearLabel }}</span>
        </div>
        <div
          v-for="(option, index) in keyboardOptions"
          :key="option.kind + ':' + option.value"
          class="option"
          :class="{ 'is-selected': option.value === modelValue, 'is-active': index === activeIndex, 'custom-option': option.kind === 'custom' }"
          role="option"
          :aria-selected="String(option.value === modelValue)"
          @click="selectValue(option.value)"
          @mouseenter="activeIndex = index"
        >
          <span class="option-text">{{ option.label }}</span>
        </div>
        <div v-if="keyboardOptions.length === 0" class="no-options">
          无匹配选项
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import DynamicIcon from './DynamicIcon.vue'

const props = defineProps({
  modelValue: {
    type: String,
    default: ''
  },
  options: {
    type: Array,
    default: () => []
  },
  placeholder: {
    type: String,
    default: '请选择...'
  },
  searchPlaceholder: {
    type: String,
    default: '搜索或输入自定义值...'
  },
  // 允许清空（值置空），用于「留空时使用主模型」这类可选字段
  clearable: {
    type: Boolean,
    default: false
  },
  clearLabel: {
    type: String,
    default: '清空'
  }
})

const emit = defineEmits(['update:modelValue', 'select'])

const isOpen = ref(false)
const searchQuery = ref('')
const searchInput = ref(null)
const activeIndex = ref(-1)

const normalizedOptions = computed(() => {
  const seen = new Set()
  const result = []
  for (const item of props.options) {
    const value = String(item ?? '').trim()
    if (!value || seen.has(value)) continue
    seen.add(value)
    result.push(value)
  }
  return result
})

// 过滤后的候选
const filteredOptions = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  if (!query) return normalizedOptions.value
  return normalizedOptions.value.filter(value => value.toLowerCase().includes(query))
})

// 输入值与任何候选都不完全一致时，提供「使用输入值」选项，保留手动输入能力
const customValue = computed(() => {
  const query = searchQuery.value.trim()
  if (!query) return ''
  const exact = normalizedOptions.value.some(value => value.toLowerCase() === query.toLowerCase())
  return exact ? '' : query
})

// 键盘导航覆盖「自定义值 + 过滤候选」；清空项只用鼠标，避免误清空
const keyboardOptions = computed(() => {
  const list = []
  if (customValue.value) {
    list.push({ kind: 'custom', value: customValue.value, label: `使用「${customValue.value}」` })
  }
  for (const value of filteredOptions.value) {
    list.push({ kind: 'option', value, label: value })
  }
  return list
})

const selectValue = (value) => {
  emit('update:modelValue', value)
  emit('select', value)
  isOpen.value = false
}

const toggleOpen = () => {
  isOpen.value = !isOpen.value
  if (isOpen.value) {
    activeIndex.value = -1
    nextTick(() => {
      searchInput.value?.focus()
    })
  }
}

const close = () => {
  isOpen.value = false
}

const moveActiveOption = (offset) => {
  const count = keyboardOptions.value.length
  if (count === 0) {
    activeIndex.value = -1
    return
  }
  activeIndex.value = (activeIndex.value + offset + count) % count
}

const selectActiveOption = () => {
  const option = keyboardOptions.value[activeIndex.value]
  if (option !== undefined) {
    selectValue(option.value)
  }
}

const handleTriggerKeydown = (event) => {
  if (['Enter', ' ', 'ArrowDown'].includes(event.key)) {
    event.preventDefault()
    if (!isOpen.value) toggleOpen()
  } else if (event.key === 'Escape') {
    close()
  }
}

const handleSearchKeydown = (event) => {
  if (event.key === 'ArrowDown') {
    event.preventDefault()
    moveActiveOption(1)
  } else if (event.key === 'ArrowUp') {
    event.preventDefault()
    moveActiveOption(-1)
  } else if (event.key === 'Enter') {
    event.preventDefault()
    // 未显式选择时回车使用第一个候选；没有候选但有输入时使用自定义值
    if (activeIndex.value >= 0) {
      selectActiveOption()
    } else if (keyboardOptions.value.length > 0) {
      selectValue(keyboardOptions.value[0].value)
    }
  } else if (event.key === 'Escape') {
    event.preventDefault()
    close()
  }
}

// 点击外部关闭
const handleClickOutside = (e) => {
  if (!e.target.closest('.searchable-select')) {
    isOpen.value = false
  }
}

watch(isOpen, (val) => {
  if (val) {
    document.addEventListener('click', handleClickOutside)
  } else {
    document.removeEventListener('click', handleClickOutside)
    searchQuery.value = ''
    activeIndex.value = -1
  }
})

watch(keyboardOptions, () => {
  activeIndex.value = keyboardOptions.value.length > 0 ? 0 : -1
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.searchable-select {
  position: relative;
  width: 100%;
}

.select-trigger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 40px;
  padding: 6px 10px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--bg-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.select-trigger:hover {
  border-color: var(--border-default);
}

.select-trigger:focus-visible {
  outline: 2px solid var(--color-primary-300);
  outline-offset: 2px;
}

.searchable-select.is-open .select-trigger {
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.trigger-text {
  flex: 1;
  color: var(--text-primary);
  font-size: 0.875rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.trigger-text.is-placeholder {
  color: var(--text-tertiary);
}

.arrow-icon {
  color: var(--text-secondary);
  transition: transform var(--motion-duration-quick) var(--motion-ease-out);
}

.arrow-icon svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
  transition: transform var(--motion-duration-quick) var(--motion-ease-out);
}

.arrow-icon svg.is-open {
  transform: rotate(180deg);
}

/* 下拉面板 */
.select-dropdown {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  right: 0;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  box-shadow: var(--shadow-lg);
  z-index: 100;
  max-height: 300px;
  display: flex;
  flex-direction: column;
}

.dropdown-search {
  padding: 10px;
  border-bottom: 1px solid var(--border-subtle);
}

.search-input {
  width: 100%;
  height: 32px;
  padding: 0 10px;
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  background: var(--bg-secondary);
  color: var(--text-primary);
  font-size: 0.875rem;
}

.search-input:focus {
  outline: none;
  border-color: var(--color-primary-500);
}

.options-list {
  flex: 1;
  overflow-y: auto;
  padding: 6px;
  max-height: 220px;
}

.option {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
  font-size: 0.875rem;
  color: var(--text-primary);
}

.option:hover {
  background: var(--bg-secondary);
}

.option.is-active {
  background: var(--bg-hover);
}

.option.is-selected {
  color: var(--color-primary-600);
}

.custom-option .option-text {
  color: var(--color-primary-600);
}

.clear-option .option-text {
  color: var(--text-tertiary);
}

.no-options {
  padding: 20px;
  text-align: center;
  color: var(--text-tertiary);
  font-size: 0.875rem;
}
</style>
