<template>
  <div class="topic-page nas-detail" role="main">
    <!-- Banner -->
    <div v-if="topic.BannerImage" class="topic-hero" :style="{ backgroundImage: `url(${topic.BannerImage})` }">
      <div class="topic-hero-overlay">
        <h1 class="topic-hero-title">{{ topic.Title }}</h1>
        <p v-if="topic.Description" class="topic-hero-desc">{{ topic.Description }}</p>
        <p v-if="topic.StartTime" class="topic-hero-time">
          {{ topic.StartTime }} ~ {{ topic.EndTime }}
        </p>
      </div>
    </div>
    <div v-else class="topic-header">
      <h1>{{ topic.Title }}</h1>
      <p v-if="topic.Description">{{ topic.Description }}</p>
    </div>

    <!-- 加载状态 -->
    <div v-if="loading" class="loading-state">
      <DynamicIcon name="loading" :size="32" />
      <p>加载中...</p>
    </div>

    <!-- 内容 -->
    <div v-else-if="topic.Content" class="topic-body">
      <div class="markdown-content" v-html="renderedContent"></div>
    </div>

    <!-- 推荐设备 -->
    <div v-if="topicDevices.length > 0" class="topic-devices">
      <h2 class="section-title">推荐设备</h2>
      <div class="devices-grid">
        <div
          v-for="device in topicDevices"
          :key="device.id"
          class="device-card"
          @click="router.push(`/nas-store?device=${device.id}`)"
        >
          <div class="device-brand">{{ device.Brand }}</div>
          <div class="device-name">{{ device.Name }}</div>
          <div class="device-price">{{ device.PriceLabel }}</div>
          <div class="device-specs">
            <span>{{ device.DriveBays }} 盘位</span>
            <span>{{ device.CPU }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 返回 -->
    <button class="back-btn" @click="router.push('/nas-store')">
      ← 返回导购
    </button>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getNASTopic, getNASDevices } from '@/api/nas.js'
import { renderSafeMarkdown } from '@/utils/markdown.js'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import './detailShared.css'

const route = useRoute()
const router = useRouter()
const topic = ref({})
const loading = ref(true)
const allDevices = ref([])

const renderedContent = computed(() => {
  return renderSafeMarkdown(topic.value.Content || '')
})

const topicDevices = computed(() => {
  if (!topic.value.Devices || topic.value.Devices.length === 0) return []
  const ids = new Set(topic.value.Devices)
  return allDevices.value.filter(d => ids.has(d.id) || ids.has(d.ID))
})

onMounted(async () => {
  try {
    const slug = route.params.slug
    const [topicData, devicesData] = await Promise.all([
      getNASTopic(slug).catch(() => null),
      getNASDevices().catch(() => [])
    ])
    if (topicData) {
      topic.value = topicData
    }
    allDevices.value = Array.isArray(devicesData) ? devicesData : []
  } catch (e) {
    console.error('加载专题失败:', e)
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.topic-hero {
  height: 280px;
  margin-bottom: 2rem;
  border-radius: 1rem;
  overflow: hidden;
  background-size: cover;
  background-position: center;
}

.topic-hero-overlay {
  height: 100%;
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
  padding: 2rem;
  background: linear-gradient(
    to top,
    color-mix(in srgb, var(--color-gray-950) 70%, transparent),
    color-mix(in srgb, var(--color-gray-950) 20%, transparent)
  );
}

/* 图片遮罩上的文字固定走浅色,与 PromoBanner 约定一致 */
.topic-hero-title {
  margin: 0 0 0.5rem;
  font-size: 1.75rem;
  font-weight: 700;
  color: var(--color-gray-50);
}

.topic-hero-desc {
  margin: 0 0 0.25rem;
  color: color-mix(in srgb, var(--color-gray-50) 85%, transparent);
}

.topic-hero-time {
  margin: 0;
  font-size: 0.875rem;
  color: color-mix(in srgb, var(--color-gray-50) 60%, transparent);
}

.topic-header {
  margin-bottom: 2rem;
}

.topic-header h1 {
  margin: 0 0 0.5rem;
  font-size: 1.75rem;
  font-weight: 700;
  color: var(--text-primary);
}

.topic-header p {
  margin: 0;
  color: var(--text-secondary);
}

.topic-body {
  margin-bottom: 2rem;
}

.devices-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 1rem;
  margin-bottom: 2rem;
}

.device-card {
  padding: 1rem;
  border: 1px solid var(--border-subtle);
  border-radius: 0.75rem;
  background: var(--bg-elevated);
  cursor: pointer;
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out), transform var(--motion-duration-quick) var(--motion-ease-out), box-shadow var(--motion-duration-quick) var(--motion-ease-out);
}

.device-card:hover {
  border-color: var(--color-primary-300);
  box-shadow: var(--shadow-md);
  transform: translateY(-2px);
}

.device-brand {
  font-size: 0.75rem;
  color: var(--text-tertiary);
}

.device-name {
  margin: 0.25rem 0;
  font-weight: 600;
  color: var(--text-primary);
}

.device-price {
  margin-bottom: 0.5rem;
  font-weight: 600;
  color: var(--color-danger-500);
}

.device-specs {
  display: flex;
  gap: 0.75rem;
  font-size: 0.75rem;
  color: var(--text-secondary);
}
</style>
