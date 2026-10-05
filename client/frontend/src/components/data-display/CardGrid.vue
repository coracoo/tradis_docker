<template>
  <div class="card-grid-wrapper">
    <!-- 排序控制栏 -->
    <div v-if="sortable && sortFields.length" class="sort-controls">
      <span class="sort-label">排序:</span>
      <button
        v-for="field in sortFields"
        :key="field.key"
        class="sort-btn"
        :class="{ active: sortState.prop === field.key }"
        @click="handleSort(field.key)"
      >
        {{ field.label }}
        <span class="sort-icon">
          <IconEpArrowUp v-if="getSortIcon(field.key) === 'asc'" />
          <IconEpArrowDown v-else-if="getSortIcon(field.key) === 'desc'" />
          <span v-else class="sort-placeholder">↕</span>
        </span>
      </button>
    </div>
    
    <div class="card-grid" :class="{ 'card-grid-loading': loading, 'motion-reveal': !loading }">
      <template v-if="loading">
        <div v-for="n in skeletonCount" :key="n" class="card-skeleton">
          <div class="skeleton-header">
            <div class="skeleton-icon"></div>
            <div class="skeleton-title"></div>
          </div>
          <div class="skeleton-content">
            <div class="skeleton-line"></div>
            <div class="skeleton-line short"></div>
          </div>
          <div class="skeleton-footer">
            <div class="skeleton-action"></div>
          </div>
        </div>
      </template>
      <template v-else-if="items.length === 0">
        <div class="empty-wrapper">
          <slot name="empty">
            <EmptyState v-bind="emptyProps" />
          </slot>
        </div>
      </template>
      <template v-else>
        <slot :items="items" />
      </template>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import { useSort } from '@/composables/useSort.js'

const props = defineProps({
  items: {
    type: Array,
    default: () => []
  },
  loading: {
    type: Boolean,
    default: false
  },
  skeletonCount: {
    type: Number,
    default: 8
  },
  emptyProps: {
    type: Object,
    default: () => ({
      title: '暂无数据',
      description: '当前列表为空'
    })
  },
  sortable: {
    type: Boolean,
    default: false
  },
  sortFields: {
    type: Array,
    default: () => []
    // 格式: [{ key: 'name', label: '名称' }, { key: 'created', label: '创建时间' }]
  },
  storageKey: {
    type: String,
    default: ''
  }
})

// 集成 useSort
const { sortState, handleSort, sortData, getSortIcon } = useSort(
  props.storageKey || 'cardgrid_sort',
  { prop: '', order: '' }
)

// 对外暴露排序后的数据和方法
defineExpose({
  sortState,
  sortData
})
</script>

<style scoped>
.card-grid-wrapper {
  width: 100%;
}

/* 排序控制栏 */
.sort-controls {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
  padding: 8px 12px;
  background: var(--bg-secondary);
  border-radius: 8px;
}

.sort-label {
  font-size: 0.875rem;
  color: var(--text-secondary);
}

.sort-btn {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 12px;
  font-size: 0.875rem;
  color: var(--text-secondary);
  background: transparent;
  border: 1px solid transparent;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
}

.sort-btn:hover {
  background: var(--bg-tertiary);
}

.sort-btn.active {
  color: var(--color-primary-600);
  background: var(--color-primary-50);
  border-color: var(--color-primary-200);
}

.sort-icon {
  display: inline-flex;
  align-items: center;
  font-size: 12px;
}

.sort-placeholder {
  font-size: 12px;
  color: var(--text-tertiary);
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 26px;
  padding: 12px 0;
}

/* 等级2：列表进入动画 */
@keyframes slideInUp {
  from {
    opacity: 0;
    transform: translateY(12px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.card-grid > * {
  animation: slideInUp 0.35s cubic-bezier(0.4, 0, 0.2, 1) backwards;
}

/* 错开动画效果 */
.card-grid > *:nth-child(1) { animation-delay: 0ms; }
.card-grid > *:nth-child(2) { animation-delay: 40ms; }
.card-grid > *:nth-child(3) { animation-delay: 80ms; }
.card-grid > *:nth-child(4) { animation-delay: 120ms; }
.card-grid > *:nth-child(5) { animation-delay: 160ms; }
.card-grid > *:nth-child(6) { animation-delay: 200ms; }
.card-grid > *:nth-child(7) { animation-delay: 240ms; }
.card-grid > *:nth-child(8) { animation-delay: 280ms; }

/* 响应式布局 - 间距大时减少列数 */
@media (max-width: 1920px) {
  .card-grid {
    grid-template-columns: repeat(5, 1fr);
  }
}

@media (max-width: 1600px) {
  .card-grid {
    grid-template-columns: repeat(4, 1fr);
  }
}

@media (max-width: 1400px) {
  .card-grid {
    grid-template-columns: repeat(3, 1fr);
  }
}

@media (max-width: 1200px) {
  .card-grid {
    grid-template-columns: repeat(3, 1fr);
  }
}

@media (max-width: 900px) {
  .card-grid {
    grid-template-columns: repeat(3, 1fr);
    gap: 16px;
  }
}

@media (max-width: 640px) {
  .card-grid {
    grid-template-columns: repeat(2, 1fr);
    gap: 12px;
  }
  
  .sort-controls {
    flex-wrap: wrap;
    gap: 8px;
  }
  
  .sort-btn {
    padding: 4px 10px;
    font-size: 0.8125rem;
  }
}

/* 骨架屏 */
.card-skeleton {
  aspect-ratio: 1;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.skeleton-header {
  display: flex;
  align-items: center;
  gap: 10px;
}

.skeleton-icon {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}

.skeleton-title {
  flex: 1;
  height: 16px;
  border-radius: 4px;
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}

.skeleton-content {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.skeleton-line {
  height: 12px;
  border-radius: 4px;
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}

.skeleton-line.short {
  width: 60%;
}

.skeleton-footer {
  margin-top: auto;
  padding-top: 8px;
  border-top: 1px solid var(--border-subtle);
}

.skeleton-action {
  height: 28px;
  border-radius: 6px;
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}

/* 空状态 */
.empty-wrapper {
  grid-column: 1 / -1;
  display: flex;
  justify-content: center;
  padding: 48px 0;
}

/* 加载状态 */
.card-grid-loading {
  pointer-events: none;
}
</style>
