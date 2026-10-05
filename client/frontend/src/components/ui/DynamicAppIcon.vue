<template>
  <div 
    class="dynamic-app-icon"
    :class="[sizeClass, { 'has-image': showImage && imageLoaded }]"
    :style="containerStyle"
  >
    <!-- 外部图标 - 尝试加载，优先使用缓存 -->
    <img
      v-if="showImage"
      :src="src"
      :alt="name"
      class="app-image"
      loading="lazy"
      decoding="async"
      @load="onImageLoad"
      @error="onImageError"
    />
    
    <!-- 生成的图标卡片 - 无外部图标或加载失败时显示 -->
    <div 
      v-else
      class="generated-icon"
      :style="generatedStyle"
    >
      <DynamicIcon :name="iconName" :size="iconSize" class="icon-white" />
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import DynamicIcon from './DynamicIcon.vue'

const props = defineProps({
  // 应用名称
  name: {
    type: String,
    required: true
  },
  // 外部图标 URL
  src: {
    type: String,
    default: ''
  },
  // 尺寸: sm(24px), md(32px), lg(48px), xl(56px), 2xl(80px)
  size: {
    type: String,
    default: 'lg',
    validator: (v) => ['sm', 'md', 'lg', 'xl', '2xl', '3xl'].includes(v)
  },
  // 是否强制使用动态图标（忽略外部图标）
  forceGenerated: {
    type: Boolean,
    default: false
  },
  // 圆角精度
  rounded: {
    type: String,
    default: '12px'
  }
})

// 图片加载状态
const imageLoaded = ref(false)
const imageError = ref(false)

// 是否显示外部图片
const showImage = computed(() => {
  if (props.forceGenerated) return false
  if (!props.src) return false
  if (imageError.value) return false
  return true
})

// 监听 src 变化，重置状态
watch(() => props.src, () => {
  imageLoaded.value = false
  imageError.value = false
})

// 处理图片加载成功
function onImageLoad() {
  imageLoaded.value = true
}

// 处理图片加载失败
function onImageError() {
  imageError.value = true
}

// 基于名称生成哈希值
const nameHash = computed(() => {
  let hash = 0
  const name = props.name.toLowerCase().trim()
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash)
  }
  return Math.abs(hash)
})

// 颜色预设
const colors = [
  { bg: '#3b82f6', name: 'blue' },      // blue-500
  { bg: '#10b981', name: 'emerald' },   // emerald-500
  { bg: '#f59e0b', name: 'amber' },     // amber-500
  { bg: '#ef4444', name: 'red' },       // red-500
  { bg: '#8b5cf6', name: 'violet' },    // violet-500
  { bg: '#06b6d4', name: 'cyan' },      // cyan-500
  { bg: '#ec4899', name: 'pink' },      // pink-500
  { bg: '#f97316', name: 'orange' },    // orange-500
  { bg: '#84cc16', name: 'lime' },      // lime-500
  { bg: '#14b8a6', name: 'teal' },      // teal-500
  { bg: '#6366f1', name: 'indigo' },    // indigo-500
  { bg: '#a855f7', name: 'purple' },    // purple-500
  { bg: '#22c55e', name: 'green' },     // green-500
  { bg: '#0ea5e9', name: 'sky' },       // sky-500
  { bg: '#e11d48', name: 'rose' },      // rose-500
  { bg: '#d946ef', name: 'fuchsia' },   // fuchsia-500
]

// 根据名称选择图标
const iconName = computed(() => {
  const name = props.name.toLowerCase()
  const icons = [
    // Docker相关
    { keywords: ['docker', 'container'], icon: 'docker' },
    // 数据库
    { keywords: ['database', 'db', 'mysql', 'postgres', 'redis', 'mongo', 'mariadb', 'sqlite'], icon: 'database' },
    // 网络/代理
    { keywords: ['proxy', 'vpn', 'nginx', 'traefik', 'caddy'], icon: 'network' },
    // 存储/文件
    { keywords: ['storage', 'file', 'nas', 'cloud', 'sync', 'backup'], icon: 'folder' },
    // 媒体
    { keywords: ['media', 'video', 'audio', 'music', 'photo', 'image', 'plex', 'jellyfin'], icon: 'image' },
    // 开发
    { keywords: ['code', 'git', 'dev', 'api', 'web'], icon: 'code' },
    // 监控
    { keywords: ['monitor', 'prometheus', 'grafana', 'uptime', 'watch'], icon: 'activity' },
    // 安全
    { keywords: ['security', 'vault', 'ssl', 'cert'], icon: 'shield' },
    // AI/机器学习
    { keywords: ['ai', 'ml', 'openai', 'ollama', 'chatgpt'], icon: 'bot' },
    // 通讯
    { keywords: ['chat', 'message', 'mail', 'email', 'notification'], icon: 'message' },
    // 文档/笔记
    { keywords: ['wiki', 'doc', 'note', 'book', 'blog'], icon: 'book' },
    // 下载
    { keywords: ['download', 'torrent', 'qbittorrent', 'transmission'], icon: 'download' },
    // 自动化
    { keywords: ['automation', 'schedule', 'cron', 'task'], icon: 'zap' },
    // 网站
    { keywords: ['website', 'blog', 'cms', 'wordpress'], icon: 'globe' },
    // 终端
    { keywords: ['terminal', 'ssh', 'console', 'shell'], icon: 'terminal' },
  ]
  
  for (const item of icons) {
    if (item.keywords.some(k => name.includes(k))) {
      return item.icon
    }
  }
  
  // 默认图标 - 所有容器默认使用 Docker 鲸鱼图标
  return 'docker'
})

// 根据哈希选择颜色
const generatedColor = computed(() => {
  return colors[nameHash.value % colors.length]
})

const sizeClass = computed(() => {
  const classMap = {
    sm: 'size-sm',
    md: 'size-md',
    lg: 'size-lg',
    xl: 'size-xl',
    '2xl': 'size-2xl',
    '3xl': 'size-3xl'
  }
  return classMap[props.size] || 'size-lg'
})

// 根据尺寸返回图标大小
const iconSize = computed(() => {
  const sizes = {
    sm: 14,
    md: 18,
    lg: 24,
    xl: 28,
    '2xl': 32,
    '3xl': 48
  }
  return sizes[props.size] || 24
})

// 容器样式
const containerStyle = computed(() => ({
  borderRadius: props.rounded
}))

// 生成图标样式
const generatedStyle = computed(() => ({
  backgroundColor: generatedColor.value.bg
}))
</script>

<style scoped>
.dynamic-app-icon {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  flex-shrink: 0;
  background: var(--bg-tertiary);
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

/* 尺寸变体 */
.dynamic-app-icon.size-sm {
  width: 24px;
  height: 24px;
}

.dynamic-app-icon.size-md {
  width: 32px;
  height: 32px;
}

.dynamic-app-icon.size-lg {
  width: 48px;
  height: 48px;
}

.dynamic-app-icon.size-xl {
  width: 56px;
  height: 56px;
}

.dynamic-app-icon.size-2xl {
  width: 64px;
  height: 64px;
}

.dynamic-app-icon.size-3xl {
  width: 80px;
  height: 80px;
}

/* 外部图片 - 统一白底+圆角，与生成图标一致 */
.app-image {
  width: 100%;
  height: 100%;
  object-fit: contain;
  /* 强制白底，解决透明图标风格不统一问题 */
  background: white;
  /* 统一圆角，与容器一致 */
  border-radius: inherit;
  /* 缓冲区避免贴边 */
  padding: 8px;
}

/* 根据尺寸调整缓冲区 */
.dynamic-app-icon.size-sm .app-image {
  padding: 2px;
}

.dynamic-app-icon.size-md .app-image {
  padding: 3px;
}

.dynamic-app-icon.size-lg .app-image {
  padding: 5px;
}

.dynamic-app-icon.size-xl .app-image {
  padding: 6px;
}

.dynamic-app-icon.size-2xl .app-image {
  padding: 8px;
}

.dynamic-app-icon.size-3xl .app-image {
  padding: 10px;
}

/* 生成的图标卡片 */
.generated-icon {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  user-select: none;
}

.icon-white {
  color: white !important;
  opacity: 0.95;
}

/* 悬停效果 */
.dynamic-app-icon:hover {
  transform: scale(1.05);
}

.dynamic-app-icon:hover .generated-icon {
  filter: brightness(1.1);
}
</style>
