<template>
  <nav class="pagination" role="navigation" aria-label="分页导航">
    <!-- 左侧：总条数 -->
    <div v-if="showTotal" class="page-total" aria-live="polite">
      共 {{ total }} 条
    </div>

    <!-- 右侧：每页条数 + 页码 + 跳转 -->
    <div class="pagination-right">
      <!-- 每页条数 -->
      <div v-if="showSizeChanger" class="page-size">
        <span class="size-label">每页</span>
        <select 
          :value="pageSize" 
          class="size-select" 
          @change="handleSizeChange"
          aria-label="每页显示条数"
        >
          <option v-for="size in pageSizeOptions" :key="size" :value="size">
            {{ size }}条
          </option>
        </select>
      </div>

      <!-- 页码 -->
      <div class="page-list" role="tablist" aria-label="页码选择">
      <button
        class="page-btn"
        :disabled="current <= 1"
        @click="handlePrev"
        aria-label="上一页"
      >
        <DynamicIcon name="chevron-left" :size="16" />
      </button>

      <template v-for="(page, index) in displayPages" :key="index">
        <button
          v-if="page !== '...'"
          class="page-btn"
          :class="{ 'page-btn-active': page === current }"
          :aria-current="page === current ? 'page' : null"
          :aria-label="`第 ${page} 页`"
          role="tab"
          @click="handlePageChange(page)"
        >
          {{ page }}
        </button>
        <span v-else class="page-ellipsis" aria-hidden="true">...</span>
      </template>

      <button
        class="page-btn"
        :disabled="current >= totalPages"
        @click="handleNext"
        aria-label="下一页"
      >
        <DynamicIcon name="chevron-right" :size="16" />
      </button>
    </div>

      <!-- 跳转 -->
      <div v-if="showQuickJumper" class="page-jumper">
        <span class="jumper-label">跳至</span>
        <input
          v-model.number="jumpPage"
          type="number"
          class="jumper-input"
          min="1"
          :max="totalPages"
          @keyup.enter="handleJump"
          aria-label="跳转到指定页码"
        />
        <span class="jumper-label">页</span>
      </div>
    </div>
  </nav>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import DynamicIcon from './DynamicIcon.vue'

const props = defineProps({
  current: {
    type: Number,
    default: 1
  },
  pageSize: {
    type: Number,
    default: 20
  },
  total: {
    type: Number,
    default: 0
  },
  pageSizeOptions: {
    type: Array,
    default: () => [10, 20, 50, 100]
  },
  showSizeChanger: {
    type: Boolean,
    default: true
  },
  showQuickJumper: {
    type: Boolean,
    default: false
  },
  showTotal: {
    type: Boolean,
    default: true
  }
})

const emit = defineEmits(['update:current', 'update:pageSize', 'change'])

const PAGE_SIZE_STORAGE_KEY = 'tradis_page_size'

// 启动时把上次选择的每页条数同步给父组件，使所有列表的"每页分栏"跨页面/刷新保留。
onMounted(() => {
  const stored = Number.parseInt(localStorage.getItem(PAGE_SIZE_STORAGE_KEY) || '', 10)
  if (Number.isFinite(stored) && stored > 0 && stored !== props.pageSize) {
    emit('update:pageSize', stored)
    emit('change', { current: props.current, pageSize: stored })
  }
})

const jumpPage = ref('')

// 总页数
const totalPages = computed(() => Math.ceil(props.total / props.pageSize) || 1)

// 显示的页码
const displayPages = computed(() => {
  const pages = []
  const current = props.current
  const total = totalPages.value
  
  if (total <= 7) {
    for (let i = 1; i <= total; i++) {
      pages.push(i)
    }
  } else {
    if (current <= 3) {
      pages.push(1, 2, 3, 4, '...', total)
    } else if (current >= total - 2) {
      pages.push(1, '...', total - 3, total - 2, total - 1, total)
    } else {
      pages.push(1, '...', current - 1, current, current + 1, '...', total)
    }
  }
  
  return pages
})

function handlePageChange(page) {
  if (page < 1 || page > totalPages.value) return
  emit('update:current', page)
  emit('change', { current: page, pageSize: props.pageSize })
}

function handlePrev() {
  if (props.current > 1) {
    handlePageChange(props.current - 1)
  }
}

function handleNext() {
  if (props.current < totalPages.value) {
    handlePageChange(props.current + 1)
  }
}

function handleSizeChange(e) {
  const size = parseInt(e.target.value)
  localStorage.setItem(PAGE_SIZE_STORAGE_KEY, String(size))
  emit('update:pageSize', size)
  emit('update:current', 1)
  emit('change', { current: 1, pageSize: size })
}

function handleJump() {
  const page = parseInt(jumpPage.value)
  if (page && page >= 1 && page <= totalPages.value) {
    handlePageChange(page)
    jumpPage.value = ''
  }
}

// 监听变化
watch(() => props.current, (val) => {
  jumpPage.value = val
})
</script>

<style scoped>
.pagination {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  width: 100%;
}

.pagination-right {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}

/* 总条数 */
.page-total {
  font-size: 0.875rem;
  color: var(--text-secondary);
}

/* 每页条数 */
.page-size {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 0.875rem;
  color: var(--text-secondary);
}

.size-select {
  height: 32px;
  padding: 0 8px;
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  background: var(--bg-elevated);
  color: var(--text-primary);
  font-size: 0.875rem;
  cursor: pointer;
}

.size-select:focus {
  outline: none;
  border-color: var(--color-primary-500);
}

/* 页码 */
.page-list {
  display: flex;
  align-items: center;
  gap: 4px;
}

.page-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 32px;
  height: 32px;
  padding: 0 8px;
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  background: var(--bg-elevated);
  color: var(--text-secondary);
  font-size: 0.875rem;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.page-btn:hover:not(:disabled) {
  border-color: var(--color-primary-500);
  color: var(--color-primary-500);
}

.page-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.page-btn-active {
  background: var(--color-primary-500);
  border-color: var(--color-primary-500);
  color: var(--text-inverse);
}

.page-btn-active:hover {
  background: var(--color-primary-600);
  color: var(--text-inverse);
}

.page-btn svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.page-ellipsis {
  padding: 0 10px;
  color: var(--text-tertiary);
}

/* 跳转 */
.page-jumper {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 0.875rem;
  color: var(--text-secondary);
}

.jumper-input {
  width: 48px;
  height: 32px;
  padding: 0 8px;
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  background: var(--bg-elevated);
  color: var(--text-primary);
  font-size: 0.875rem;
  text-align: center;
}

.jumper-input:focus {
  outline: none;
  border-color: var(--color-primary-500);
}


</style>
