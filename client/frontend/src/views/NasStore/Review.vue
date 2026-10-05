<template>
  <div class="review-page nas-detail" role="main">
    <!-- 加载 -->
    <div v-if="loading" class="loading-state">
      <DynamicIcon name="loading" :size="32" />
      <p>加载中...</p>
    </div>

    <template v-else-if="review.Title">
      <!-- Hero -->
      <div class="review-hero">
        <div v-if="review.CoverImage" class="review-cover">
          <img :src="review.CoverImage" :alt="review.Title" />
        </div>
        <div class="review-hero-content">
          <div v-if="review.Tags && review.Tags.length" class="review-tags">
            <span v-for="tag in review.Tags" :key="tag" class="review-tag">{{ tag }}</span>
          </div>
          <h1 class="review-title">{{ review.Title }}</h1>
          <div class="review-meta">
            <span class="review-author">{{ review.Author }}</span>
            <span class="review-date">{{ review.PublishedAt }}</span>
          </div>
          <p v-if="review.Summary" class="review-summary">{{ review.Summary }}</p>
        </div>
      </div>

      <!-- 评测内容 -->
      <div class="review-body">
        <div class="markdown-content" v-html="renderedContent"></div>
      </div>

      <!-- 相关设备 -->
      <div v-if="relatedDevice" class="related-device">
        <h2 class="section-title">相关设备</h2>
        <div class="device-link-card" @click="router.push('/nas-store')">
          <div class="device-link-info">
            <span class="device-link-brand">{{ relatedDevice.Brand }}</span>
            <span class="device-link-name">{{ relatedDevice.Name }}</span>
          </div>
          <span class="device-link-price">{{ relatedDevice.PriceLabel }}</span>
          <DynamicIcon name="chevron-right" :size="18" />
        </div>
      </div>
    </template>

    <!-- 未找到 -->
    <div v-else class="empty-state">
      <DynamicIcon name="file-text" :size="48" />
      <p>未找到评测内容</p>
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
import { getNASReview, getNASDevices } from '@/api/nas.js'
import { renderSafeMarkdown } from '@/utils/markdown.js'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import './detailShared.css'

const route = useRoute()
const router = useRouter()
const review = ref({})
const loading = ref(true)
const allDevices = ref([])

const renderedContent = computed(() => {
  return renderSafeMarkdown(review.value.Content || '')
})

const relatedDevice = computed(() => {
  if (!review.value.DeviceID) return null
  return allDevices.value.find(d => d.id === review.value.DeviceID || d.ID === review.value.DeviceID) || null
})

onMounted(async () => {
  try {
    const id = route.params.id
    const [reviewData, devicesData] = await Promise.all([
      getNASReview(id).catch(() => null),
      getNASDevices().catch(() => [])
    ])
    if (reviewData) {
      review.value = reviewData
    }
    allDevices.value = Array.isArray(devicesData) ? devicesData : []
  } catch (e) {
    console.error('加载评测失败:', e)
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.review-hero {
  margin-bottom: 2rem;
}

.review-cover {
  width: 100%;
  height: 320px;
  margin-bottom: 1.5rem;
  border-radius: 1rem;
  overflow: hidden;
}

.review-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.review-hero-content {
  max-width: 720px;
}

.review-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
}

.review-tag {
  padding: 0.25rem 0.75rem;
  border-radius: 999px;
  background: var(--color-primary-100);
  color: var(--color-primary-600);
  font-size: 0.75rem;
}

.review-title {
  margin: 0 0 0.75rem;
  font-size: 1.75rem;
  font-weight: 700;
  color: var(--text-primary);
}

.review-meta {
  display: flex;
  gap: 1rem;
  margin-bottom: 0.75rem;
  font-size: 0.875rem;
  color: var(--text-secondary);
}

.review-summary {
  margin: 0;
  padding: 1rem;
  border-left: 3px solid var(--color-primary-400);
  border-radius: 0.5rem;
  background: var(--bg-secondary);
  color: var(--text-secondary);
  font-size: 0.9375rem;
  line-height: 1.7;
}

.review-body {
  margin-bottom: 2rem;
}

.device-link-card {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1rem 1.25rem;
  border: 1px solid var(--border-subtle);
  border-radius: 0.75rem;
  background: var(--bg-elevated);
  cursor: pointer;
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out), box-shadow var(--motion-duration-quick) var(--motion-ease-out);
}

.device-link-card:hover {
  border-color: var(--color-primary-300);
  box-shadow: var(--shadow-md);
}

.device-link-info {
  flex: 1;
}

.device-link-brand {
  display: block;
  font-size: 0.75rem;
  color: var(--text-tertiary);
}

.device-link-name {
  font-weight: 600;
  color: var(--text-primary);
}

.device-link-price {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--color-danger-500);
}
</style>
