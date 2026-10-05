<template>
  <div class="volume-card" :class="{ 'volume-card-inuse': isInUse, 'is-selected': selected }" @click="$emit('select')">
    <!-- 名称 -->
    <div class="card-header">
      <h3 class="volume-name" :title="volume.Name">{{ volume.Name }}</h3>
    </div>

    <!-- 状态 -->
    <div class="status-row">
      <span class="status-badge" :class="isInUse ? 'in-use' : 'unused'">
        {{ isInUse ? '使用中' : '未使用' }}
      </span>
    </div>

    <!-- 信息列表 -->
    <div class="volume-info">
      <div class="info-row">
        <span class="info-label">驱动</span>
        <span class="info-value">{{ volume.Driver || 'local' }}</span>
      </div>
      <div class="info-row">
        <span class="info-label">容器</span>
        <span class="info-value" :class="{ 'has-containers': containerCount > 0 }">
          {{ containerCount }} 个
        </span>
      </div>
      <div class="info-row">
        <span class="info-label">挂载</span>
        <span class="info-value" :title="volume.Mountpoint">{{ shortMountpoint }}</span>
      </div>
    </div>

    <!-- 操作按钮 -->
    <div class="card-actions" data-remote-write>
      <button class="action-btn info" @click.stop="$emit('browse')">
        <DynamicIcon name="file-text" :size="14" />
      </button>
      <button 
        class="action-btn danger"
        :disabled="isInUse"
        :title="isInUse ? '使用中，无法删除' : '删除'"
        @click.stop="$emit('remove')"
      >
        <DynamicIcon name="trash-2" :size="14" />
      </button>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'

const props = defineProps({
  volume: {
    type: Object,
    required: true
  },
  selected: {
    type: Boolean,
    default: false
  }
})

defineEmits(['select', 'browse', 'remove'])

// 容器数量
const containerCount = computed(() => {
  if (!props.volume.Containers) return 0
  return Object.keys(props.volume.Containers).length
})

// 是否在使用中
const isInUse = computed(() => {
  return containerCount.value > 0
})

// 短挂载点
const shortMountpoint = computed(() => {
  const path = props.volume.Mountpoint || ''
  if (path.length > 30) {
    return '...' + path.slice(-27)
  }
  return path || '-'
})
</script>

<style scoped>
.volume-card {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  cursor: pointer;
  position: relative;
  overflow: hidden;
  min-height: 236px;
  aspect-ratio: auto;
}

.volume-card:hover {
  border-color: var(--border-medium);
  transform: none;
  box-shadow: inset 0 0 0 1px var(--border-subtle);
}

.volume-card:active {
  transform: translateY(0) scale(0.99);
  transition-duration: 0.1s;
}

/* 使用中的卷 - 主题色渐变 */
.volume-card-inuse {
  background: linear-gradient(135deg, var(--bg-elevated) 0%, var(--color-primary-500-5) 100%);
}

.volume-card-inuse:hover {
  box-shadow: inset 0 0 0 1px var(--border-subtle);
}

/* 头部 */
.card-header {
  margin-bottom: 6px;
}

.volume-name {
  font-size: 0.9375rem;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 状态行 */
.status-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  flex-wrap: wrap;
}

.status-badge {
  display: inline-block;
  padding: 3px 8px;
  border-radius: 6px;
  font-size: 0.6875rem;
  font-weight: 500;
}

.status-badge.in-use {
  background: var(--color-success-100);
  color: var(--color-success-700);
}

.status-badge.unused {
  background: var(--bg-tertiary);
  color: var(--text-tertiary);
}

/* 信息列表 */
.volume-info {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 8px;
  flex: 1;
}

.info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.info-label {
  font-size: 0.6875rem;
  color: var(--text-tertiary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.info-value {
  font-size: 0.8125rem;
  color: var(--text-secondary);
}

.info-value.mono {
  font-family: 'JetBrains Mono', monospace;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 140px;
}

.info-value.has-containers {
  color: var(--color-success-600);
  font-weight: 500;
}

/* 操作按钮 */
.card-actions {
  display: flex;
  gap: 6px;
  padding-top: 8px;
  padding-bottom: 2px;
  margin-top: auto;
  border-top: 1px solid var(--border-subtle);
  flex-shrink: 0;
}

.action-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  background: var(--bg-tertiary);
  border: none;
  border-radius: 8px;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

/* 浏览 - 浅蓝色 */
.action-btn.info {
  background: var(--color-primary-100);
  color: var(--color-primary-600);
}

.action-btn.info svg {
  stroke: var(--color-primary-600);
}

.action-btn.info:hover {
  background: var(--color-primary-200);
  color: var(--color-primary-800);
}

.action-btn.info:hover svg {
  stroke: var(--color-primary-700);
}

/* 删除 - 粉红色 */
.action-btn.danger {
  background: var(--color-danger-100);
  color: var(--color-danger-600);
}

.action-btn.danger svg {
  stroke: var(--color-danger-600);
}

.action-btn.danger:hover:not(:disabled) {
  background: var(--color-danger-200);
  color: var(--color-danger-700);
}

.action-btn.danger:hover:not(:disabled) svg {
  stroke: var(--color-danger-700);
}

.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.action-btn svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}
</style>
