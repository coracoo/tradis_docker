<template>
  <div class="network-card" :class="{ 'network-card-system': isSystem, 'is-selected': selected }" @click="$emit('select')">
    <!-- 名称 -->
    <div class="card-header">
      <h3 class="network-name" :title="network.Name">{{ network.Name }}</h3>
    </div>

    <!-- 状态 -->
    <div class="status-row">
      <span class="status-badge" :class="isSystem ? 'system' : 'unused'">
        {{ isSystem ? '系统' : '自定义' }}
      </span>
      <span class="container-count" :class="{ 'has-containers': containerCount > 0 }" :title="containerNames">
        {{ containerCount }} 个容器
      </span>
    </div>

    <!-- 信息列表 -->
    <div class="network-info">
      <div class="info-row">
        <span class="info-label">类型</span>
        <span class="info-value">{{ network.Driver || 'bridge' }}</span>
      </div>
      <div class="info-row">
        <span class="info-label">子网</span>
        <span class="info-value">{{ subnet || '-' }}</span>
      </div>
      <div class="info-row">
        <span class="info-label">网关</span>
        <span class="info-value">{{ gateway || '-' }}</span>
      </div>

    </div>

    <!-- 操作按钮 -->
    <div class="card-actions" data-remote-write>
      <button 
        v-if="!isSystem"
        class="action-btn info"
        @click.stop="$emit('edit')"
      >
        <DynamicIcon name="edit" :size="14" />
      </button>
      <button 
        v-if="!isSystem"
        class="action-btn danger"
        :disabled="containerCount > 0"
        :title="containerCount > 0 ? '有容器使用，无法删除' : '删除'"
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
  network: {
    type: Object,
    required: true
  },
  selected: {
    type: Boolean,
    default: false
  }
})

defineEmits(['select', 'edit', 'remove'])

// 系统网络
const systemNetworks = ['bridge', 'host', 'none']
const isSystem = computed(() => {
  return systemNetworks.includes(props.network.Name)
})

// IPAM 配置
const ipamConfig = computed(() => {
  if (!props.network.IPAM || !props.network.IPAM.Config) return null
  return props.network.IPAM.Config.find(config => !(config?.Subnet || '').includes(':')) || props.network.IPAM.Config[0]
})

const subnet = computed(() => ipamConfig.value?.Subnet || '')
const gateway = computed(() => ipamConfig.value?.Gateway || '')

// 容器数量
const containerCount = computed(() => {
  if (!props.network.Containers) return 0
  return Object.keys(props.network.Containers).length
})

// 容器名称列表（用于悬浮提示）
const containerNames = computed(() => {
  if (containerCount.value === 0) return '无容器使用此网络'
  const names = Object.values(props.network.Containers)
    .map(c => c.Name)
    .filter(Boolean)
  return names.join('\n')
})
</script>

<style scoped>
.network-card {
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

.network-card:hover {
  border-color: var(--border-medium);
  transform: none;
  box-shadow: inset 0 0 0 1px var(--border-subtle);
}

.network-card:active {
  transform: none;
  transition-duration: 0.1s;
}

.network-card-system {
  background: linear-gradient(135deg, var(--bg-elevated) 0%, var(--color-warning-500-10) 100%);
}

/* 头部 */
.card-header {
  margin-bottom: 6px;
}

.network-name {
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

.status-badge.system {
  background: var(--color-warning-100);
  color: var(--color-warning-700);
}

.status-badge.unused {
  background: var(--bg-tertiary);
  color: var(--text-tertiary);
}

/* 信息列表 */
.network-info {
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
  max-width: 120px;
}

.container-count {
  font-size: 0.75rem;
  color: var(--text-tertiary);
  margin-left: auto;
}

.container-count.has-containers {
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

/* 查看/编辑 - 浅蓝色 */
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
