<template>
  <div 
    class="image-card"
    :class="{ 
      'is-selected': selected, 
      'is-hovered': hovered,
      'no-repo': !displayName || displayName === '<none>' || displayName === '未命名镜像',
      'has-update': image.hasUpdate
    }"
    @click="$emit('click', image)"
    @mouseenter="hovered = true"
    @mouseleave="hovered = false"
  >
    <!-- 镜像名称 -->
    <div class="card-header">
      <h3 class="image-name" :title="displayName">{{ displayName }}</h3>
    </div>

    <!-- 状态和标签 -->
    <div class="status-row">
      <span class="status-badge" :class="statusInfo.class">
        {{ statusInfo.label }}
      </span>
      <span v-if="image.hasUpdate" class="update-badge">可更新</span>
      <span class="tag-separator">|</span>
      <div class="image-tag-wrapper">
        <span class="image-tag-inline">{{ primaryTag }}</span>
        <span v-if="tagCount > 1" class="tag-count" :title="allTagsTitle">+{{ tagCount - 1 }}</span>
      </div>
    </div>

    <!-- 镜像信息 -->
    <div class="image-info">
      <div class="info-row">
        <span class="info-label">ID</span>
        <span class="info-value">{{ shortId }}</span>
      </div>
      <div class="info-row">
        <span class="info-label">大小</span>
        <span class="info-value">{{ formattedSize }}</span>
      </div>
      <div class="info-row">
        <span class="info-label">创建</span>
        <span class="info-value">{{ formattedDate }}</span>
      </div>
    </div>

    <!-- 操作按钮 -->
    <div class="card-actions" data-remote-write>
      <button 
        class="action-btn"
        :class="{ 'is-running': isInUse }"
        title="运行"
        @click.stop="$emit('run', image)"
      >
        <DynamicIcon name="play" :size="14" />
      </button>
      <button 
        v-if="image.hasUpdate"
        class="action-btn update"
        title="更新"
        @click.stop="$emit('update', image)"
      >
        <DynamicIcon name="cloud-download" :size="14" />
      </button>
      <button 
        class="action-btn"
        title="导出"
        @click.stop="$emit('export', image)"
      >
        <DynamicIcon name="download" :size="14" />
      </button>
      <button 
        class="action-btn"
        title="修改标签"
        @click.stop="$emit('tag', image)"
      >
        <DynamicIcon name="tag" :size="14" />
      </button>
      <button 
        class="action-btn danger"
        :disabled="isInUse"
        title="删除"
        @click.stop="$emit('remove', image)"
      >
        <DynamicIcon name="trash" :size="14" />
      </button>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { formatBytes } from '@/utils/format.js'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'

const props = defineProps({
  image: {
    type: Object,
    required: true
  },
  selected: {
    type: Boolean,
    default: false
  },
  inUseIds: {
    type: Set,
    default: () => new Set()
  }
})

defineEmits(['click', 'run', 'export', 'tag', 'remove', 'update'])

const hovered = ref(false)

function parseImageRef(ref) {
  const value = String(ref || '')
  const lastSlash = value.lastIndexOf('/')
  const lastColon = value.lastIndexOf(':')
  if (lastColon > lastSlash) {
    return {
      name: value.slice(0, lastColon) || '<none>',
      tag: value.slice(lastColon + 1) || 'latest'
    }
  }
  return {
    name: value || '<none>',
    tag: 'latest'
  }
}

// 显示名称
const displayName = computed(() => {
  if (props.image.RepoTags && props.image.RepoTags.length > 0) {
    return parseImageRef(props.image.RepoTags[0]).name
  }
  return '<none>'
})

// 主标签（第一个）
const primaryTag = computed(() => {
  if (!props.image.RepoTags || props.image.RepoTags.length === 0) return 'latest'
  return parseImageRef(props.image.RepoTags[0]).tag
})

// 标签总数
const tagCount = computed(() => {
  return props.image.RepoTags?.length || 0
})

// 所有标签的悬浮提示
const allTagsTitle = computed(() => {
  if (!props.image.RepoTags || props.image.RepoTags.length === 0) return ''
  return props.image.RepoTags.join('\n')
})

// 简化 ID
const shortId = computed(() => {
  const id = props.image.Id
  if (!id) return '-'
  return id.replace('sha256:', '').slice(0, 12)
})

// 格式化大小
const formattedSize = computed(() => {
  return formatBytes(props.image.Size || 0)
})

// 格式化日期
const formattedDate = computed(() => {
  if (!props.image.Created) return '-'
  const date = new Date(props.image.Created * 1000)
  const now = new Date()
  const diffMs = now - date
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))
  
  if (diffDays === 0) return '今天'
  if (diffDays === 1) return '昨天'
  if (diffDays < 7) return `${diffDays}天前`
  if (diffDays < 30) return `${Math.floor(diffDays / 7)}周前`
  if (diffDays < 365) return `${Math.floor(diffDays / 30)}个月前`
  return `${Math.floor(diffDays / 365)}年前`
})

// 是否在使用中
const isInUse = computed(() => {
  return props.inUseIds.has(props.image.Id)
})

// 状态信息
const statusInfo = computed(() => {
  if (isInUse.value) {
    return { 
      class: 'in-use', 
      label: '使用中'
    }
  }
  
  // 检查是否是悬空镜像（无标签或无命名）
  const hasValidTag = props.image.RepoTags && props.image.RepoTags.length > 0 && 
                      props.image.RepoTags[0] !== '<none>:<none>'
  if (!hasValidTag) {
    return { 
      class: 'dangling', 
      label: '未标记'
    }
  }
  
  return { 
    class: 'unused', 
    label: '未使用'
  }
})
</script>

<style scoped>
.image-card {
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

.image-card:hover {
  border-color: var(--border-medium);
  transform: none;
  box-shadow: inset 0 0 0 1px var(--border-subtle);
}

.image-card:active {
  transform: translateY(0) scale(0.99);
  transition-duration: 0.1s;
}

.image-card.is-selected {
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.image-card.no-repo {
  background: linear-gradient(135deg, var(--bg-elevated) 0%, rgba(245, 158, 11, 0.05) 100%);
  border-color: var(--color-warning-200);
}

/* 有更新的镜像 - 主题色渐变 */
.image-card.has-update {
  background: linear-gradient(135deg, var(--bg-elevated) 0%, var(--color-primary-500-5) 100%);
}

.image-card.has-update:hover {
  box-shadow: inset 0 0 0 1px var(--border-subtle);
}

/* 头部 */
.card-header {
  margin-bottom: 6px;
}

.image-name {
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

.tag-separator {
  color: var(--border-medium);
  font-size: 0.75rem;
  margin: 0 2px;
}

.image-tag-wrapper {
  display: flex;
  align-items: center;
  gap: 4px;
}

.image-tag-inline {
  font-size: 0.75rem;
  color: var(--text-tertiary);
  font-family: 'JetBrains Mono', monospace;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100px;
}

.tag-count {
  font-size: 0.6875rem;
  padding: 2px 6px;
  background: var(--color-primary-100);
  color: var(--color-primary-700);
  border-radius: 4px;
  font-weight: 500;
  cursor: help;
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

.status-badge.dangling {
  background: var(--color-warning-100);
  color: var(--color-warning-700);
}

.update-badge {
  display: inline-block;
  padding: 3px 8px;
  border-radius: 6px;
  font-size: 0.6875rem;
  font-weight: 500;
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

/* 镜像信息 */
.image-info {
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
}

/* 操作按钮 */
.card-actions {
  display: flex;
  gap: 6px;
  padding-top: 8px;
  padding-bottom: 2px;
  margin-top: auto;
  border-top: 1px solid var(--border-subtle);
  flex-wrap: wrap;
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

.action-btn:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

/* 运行 - 绿色 */
.action-btn.is-running {
  background: var(--color-success-100);
  color: var(--color-success-600);
}

.action-btn.is-running svg {
  stroke: var(--color-success-600);
}

.action-btn.is-running:hover {
  background: var(--color-success-200);
  color: var(--color-success-700);
}

.action-btn.is-running:hover svg {
  stroke: var(--color-success-700);
}

/* 更新 - 紫色 */
.action-btn.update {
  background: var(--color-accent-100);
  color: var(--color-accent-600);
}

.action-btn.update svg {
  stroke: var(--color-accent-600);
}

.action-btn.update:hover {
  background: var(--color-accent-200);
  color: var(--color-accent-700);
}

.action-btn.update:hover svg {
  stroke: var(--color-accent-700);
}

/* 导出/标签 - 浅蓝色 */
.action-btn.info {
  background: var(--color-primary-100);
  color: var(--color-primary-600);
}

.action-btn.info svg {
  stroke: #2563eb;
}

.action-btn.info:hover {
  background: var(--color-primary-200);
  color: var(--color-primary-800);
}

.action-btn.info:hover svg {
  stroke: #1d4ed8;
}

/* 终端 - 浅黑色 */
.action-btn.terminal {
  background: var(--bg-tertiary);
  color: var(--text-secondary);
}

.action-btn.terminal:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
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

/* 查看/编辑/日志 - 浅蓝色 */
.action-btn.info:hover {
  background: var(--color-primary-100);
  color: var(--color-primary-600);
}

/* 终端 - 浅黑色 */
.action-btn.terminal:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
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
