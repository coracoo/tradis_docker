<template>
  <div class="multiselect" :class="{ 'is-open': isOpen }">
    <!-- 输入框区域 -->
    <div
      class="multiselect-input"
      role="combobox"
      tabindex="0"
      aria-haspopup="listbox"
      :aria-expanded="String(isOpen)"
      @click="toggleOpen"
      @keydown="handleTriggerKeydown"
    >
      <div class="selected-tags">
        <!-- 摘要模式：不展示逐项标签，选择只体现在下拉复选框中，避免触发框随内容变形 -->
        <span v-if="!showTags" class="summary-text" :class="{ 'is-placeholder': modelValue.length === 0 }">
          {{ modelValue.length ? `已选 ${modelValue.length} 项` : placeholder }}
        </span>
        <template v-else-if="modelValue.length > 0">
          <span v-for="item in modelValue" :key="item" class="tag">
            {{ item }}
            <span class="tag-remove" @click.stop="removeItem(item)">
              <DynamicIcon name="x" :size="12" />
            </span>
          </span>
        </template>
        <span v-else class="placeholder">{{ placeholder }}</span>
      </div>
      <div class="arrow-icon">
        <DynamicIcon name="chevron-down" :size="16" :class="{ 'is-open': isOpen }" />
      </div>
    </div>
    
    <!-- 下拉面板 -->
    <div v-if="isOpen" class="multiselect-dropdown">
      <!-- 搜索框 -->
      <div class="dropdown-search">
        <input
          ref="searchInput"
          v-model="searchQuery"
          type="search"
          class="search-input"
          name="tradis-option-query"
          autocomplete="one-time-code"
          autocapitalize="none"
          spellcheck="false"
          data-1p-ignore
          data-lpignore="true"
          data-form-type="other"
          placeholder="搜索..."
          @click.stop
          @keydown="handleSearchKeydown"
        />
      </div>
      
      <!-- 选项列表 -->
      <div class="options-list" role="listbox" aria-multiselectable="true">
        <div
          v-for="(option, index) in filteredOptions"
          :key="option.value"
          class="option"
          :class="{ 'is-selected': isSelected(option), 'is-active': index === activeIndex }"
          role="option"
          :aria-selected="String(isSelected(option))"
          @click="toggleOption(option)"
          @mouseenter="activeIndex = index"
        >
          <span class="checkbox">
            <DynamicIcon v-if="isSelected(option)" name="check" :size="14" />
          </span>
          <span class="option-text">{{ option.label }}</span>
        </div>
        <div v-if="filteredOptions.length === 0" class="no-options">
          无匹配选项
        </div>
      </div>
      
      <!-- 底部操作 -->
      <div class="dropdown-footer">
        <span class="selected-count">已选 {{ modelValue.length }} 项</span>
        <div class="footer-actions">
          <button class="select-all-btn" :disabled="filteredOptions.length === 0" @click="selectAllShown">全选</button>
          <button class="clear-btn" @click="clearAll">清空</button>
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
    type: Array,
    default: () => []
  },
  options: {
    type: Array,
    default: () => []
  },
  placeholder: {
    type: String,
    default: '请选择...'
  },
  // 关闭标签展示：触发框只显示稳定的“已选 N 项”摘要，选择状态体现在下拉复选框
  showTags: {
    type: Boolean,
    default: true
  }
})

const emit = defineEmits(['update:modelValue'])

const isOpen = ref(false)
const searchQuery = ref('')
const searchInput = ref(null)
const activeIndex = ref(-1)

// 过滤后的选项
const filteredOptions = computed(() => {
  if (!searchQuery.value) return normalizedOptions.value
  const query = searchQuery.value.toLowerCase()
  return normalizedOptions.value.filter(opt => opt.label.toLowerCase().includes(query))
})

// 选项支持纯字符串（label 与 value 相同）或 { value, label } 对象；
// modelValue 始终为值数组（字符串），对象选项只在展示层使用 label。
const normalizedOptions = computed(() => props.options.map(opt => {
  if (opt && typeof opt === 'object') {
    const value = String(opt.value ?? '')
    return { value, label: String(opt.label ?? value) }
  }
  const value = String(opt)
  return { value, label: value }
}))

// 是否选中
const isSelected = (option) => {
  return props.modelValue.includes(option.value)
}

// 切换选项
const toggleOption = (option) => {
  const newValue = [...props.modelValue]
  const index = newValue.indexOf(option.value)
  if (index > -1) {
    newValue.splice(index, 1)
  } else {
    newValue.push(option.value)
  }
  emit('update:modelValue', newValue)
}

// 移除选项
const removeItem = (item) => {
  const newValue = props.modelValue.filter(v => v !== item)
  emit('update:modelValue', newValue)
}

// 清空所有
const clearAll = () => {
  emit('update:modelValue', [])
}

// 全选当前过滤出的选项（与已有选中合并，不影响被搜索过滤掉的选中项）
const selectAllShown = () => {
  emit('update:modelValue', [...new Set([...props.modelValue, ...filteredOptions.value.map(option => option.value)])])
}

// 切换下拉
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
  const count = filteredOptions.value.length
  if (count === 0) {
    activeIndex.value = -1
    return
  }
  activeIndex.value = (activeIndex.value + offset + count) % count
}

const selectActiveOption = () => {
  const option = filteredOptions.value[activeIndex.value]
  if (option !== undefined) {
    toggleOption(option)
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
    selectActiveOption()
  } else if (event.key === 'Escape') {
    event.preventDefault()
    close()
  }
}

// 点击外部关闭
const handleClickOutside = (e) => {
  if (!e.target.closest('.multiselect')) {
    isOpen.value = false
    searchQuery.value = ''
  }
}

watch(isOpen, (val) => {
  if (val) {
    document.addEventListener('click', handleClickOutside)
  } else {
    document.removeEventListener('click', handleClickOutside)
    searchQuery.value = ''
  }
})

watch(filteredOptions, () => {
  activeIndex.value = filteredOptions.value.length > 0 ? 0 : -1
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.multiselect {
  position: relative;
  width: 100%;
}

.multiselect-input {
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

.multiselect-input:hover {
  border-color: var(--border-default);
}

.multiselect-input:focus-visible {
  outline: 2px solid var(--color-primary-300);
  outline-offset: 2px;
}

.option.is-active {
  background: var(--bg-hover);
}

.multiselect.is-open .multiselect-input {
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.selected-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  flex: 1;
}

.tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  background: var(--color-primary-100);
  color: var(--color-primary-700);
  font-size: 0.8125rem;
  border-radius: 6px;
}

.tag-remove {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: 4px;
  cursor: pointer;
  transition: background var(--motion-duration-quick) var(--motion-ease-out);
}

.tag-remove:hover {
  background: var(--color-primary-200);
}

.tag-remove svg {
  stroke: currentColor;
  stroke-width: 2;
}

.placeholder {
  color: var(--text-tertiary);
  font-size: 0.875rem;
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
.multiselect-dropdown {
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
  max-height: 200px;
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

.option.is-selected {
  color: var(--color-primary-600);
}

.checkbox {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border: 2px solid var(--border-default);
  border-radius: 4px;
  transition: all 0.15s ease;
}

.option.is-selected .checkbox {
  background: var(--color-primary-500);
  border-color: var(--color-primary-500);
  color: var(--text-inverse);
}

.checkbox svg {
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.no-options {
  padding: 20px;
  text-align: center;
  color: var(--text-tertiary);
  font-size: 0.875rem;
}

/* 底部 */
.dropdown-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px;
  border-top: 1px solid var(--border-subtle);
  font-size: 0.8125rem;
}

.selected-count {
  color: var(--text-secondary);
}

.footer-actions {
  display: flex;
  gap: 4px;
}

.select-all-btn {
  padding: 4px 10px;
  background: transparent;
  border: none;
  color: var(--color-primary-600);
  font-size: 0.8125rem;
  cursor: pointer;
  border-radius: 4px;
  transition: background var(--motion-duration-quick) var(--motion-ease-out);
}

.select-all-btn:hover:not(:disabled) {
  background: var(--color-primary-100);
}

.select-all-btn:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.summary-text {
  color: var(--text-primary);
  font-size: 0.875rem;
  white-space: nowrap;
}

.summary-text.is-placeholder {
  color: var(--text-tertiary);
}

.clear-btn {
  padding: 4px 10px;
  background: transparent;
  border: none;
  color: var(--color-danger-600);
  font-size: 0.8125rem;
  cursor: pointer;
  border-radius: 4px;
  transition: background var(--motion-duration-quick) var(--motion-ease-out);
}

.clear-btn:hover {
  background: var(--color-danger-100);
}
</style>
