<template>
  <div class="empty-state" :class="{ 'empty-state-compact': compact }">
    <div class="empty-icon" :style="iconStyle">
      <DynamicIcon :name="iconName" :size="iconSize" class="empty-icon-svg" />
    </div>
    <h3 v-if="title" class="empty-title">{{ title }}</h3>
    <p v-if="description" class="empty-description">{{ description }}</p>
    <div v-if="$slots.action || actionText" class="empty-action">
      <slot name="action">
        <button v-if="actionText" class="action-btn" @click="$emit('action')">
          {{ actionText }}
        </button>
      </slot>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import DynamicIcon from './DynamicIcon.vue'

const props = defineProps({
  title: {
    type: String,
    default: '暂无数据'
  },
  description: {
    type: String,
    default: ''
  },
  actionText: {
    type: String,
    default: ''
  },
  icon: {
    type: String,
    default: ''
  },
  iconSize: {
    type: Number,
    default: 64
  },
  compact: {
    type: Boolean,
    default: false
  }
})

defineEmits(['action'])

// 预设图标映射（将预设名称映射到 DynamicIcon 支持的名称）
const presetIconMap = {
  'box': 'box',
  'search': 'search',
  'file': 'file',
  'image': 'image',
  'container': 'container',
  'network': 'network',
  'volume': 'volume',
  'error': 'error',
  'offline': 'wifi'
}

const iconName = computed(() => {
  // 如果提供了图标名称，优先使用预设映射或直接使用
  if (props.icon) {
    return presetIconMap[props.icon] || props.icon
  }
  // 默认使用 box 图标
  return 'box'
})

const iconStyle = computed(() => ({
  width: `${props.iconSize}px`,
  height: `${props.iconSize}px`
}))
</script>

<style scoped>
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 48px 24px;
  text-align: center;
}

.empty-state-compact {
  padding: 24px;
}

.empty-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-tertiary);
  opacity: 0.5;
  margin-bottom: 16px;
}

.empty-icon-svg {
  color: inherit;
}

.empty-title {
  font-size: 1rem;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 8px;
}

.empty-description {
  font-size: 0.875rem;
  color: var(--text-secondary);
  margin: 0 0 20px;
  max-width: 400px;
  line-height: 1.5;
}

.empty-action {
  margin-top: 4px;
}

.action-btn {
  padding: 8px 16px;
  background: var(--color-primary-500);
  color: var(--text-inverse);
  border: none;
  border-radius: 8px;
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.action-btn:hover {
  background: var(--color-primary-600);
  transform: translateY(-1px);
}
</style>
