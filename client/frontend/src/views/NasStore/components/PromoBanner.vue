<template>
  <div v-if="banners.length > 0" class="promo-banner-wrapper">
    <div
      class="promo-banner-track"
      :style="trackStyle"
      @mouseenter="pause"
      @mouseleave="resume"
      @transitionend="onTransitionEnd"
    >
      <div
        v-for="(banner, index) in loopedBanners"
        :key="`${banner.id}-${index}`"
        class="promo-banner-slide"
        :class="{ 'is-clickable': Boolean(banner.LinkURL) }"
        :style="{ backgroundColor: banner.BgColor || backgroundForIndex(index) }"
        :role="banner.LinkURL ? 'link' : undefined"
        :tabindex="banner.LinkURL ? 0 : undefined"
        @click="handleClick(banner)"
        @keydown.enter="handleClick(banner)"
      >
        <div v-if="banner.ImageURL" class="banner-bg">
          <img :src="banner.ImageURL" :alt="banner.Title" />
        </div>
        <div class="banner-content">
          <h3 class="banner-title">{{ banner.Title }}</h3>
          <p v-if="banner.Subtitle" class="banner-subtitle">{{ banner.Subtitle }}</p>
          <span v-if="banner.LinkURL" class="banner-link">查看详情 →</span>
        </div>
        <div v-if="!banner.ImageURL" class="banner-hardware" aria-hidden="true">
          <div class="hardware-topline">
            <span class="hardware-mark"></span>
            <span class="hardware-mark"></span>
            <span class="hardware-mark is-active"></span>
          </div>
          <div class="hardware-bays">
            <span v-for="bay in 4" :key="bay" class="hardware-bay">
              <i></i>
            </span>
          </div>
        </div>
      </div>
    </div>
    <button
      v-if="banners.length > 1"
      type="button"
      class="banner-arrow banner-arrow-left"
      aria-label="上一张"
      @click.stop="prev"
    >
      <DynamicIcon name="chevron-left" :size="18" />
    </button>
    <button
      v-if="banners.length > 1"
      type="button"
      class="banner-arrow banner-arrow-right"
      aria-label="下一张"
      @click.stop="next"
    >
      <DynamicIcon name="chevron-right" :size="18" />
    </button>

    <div v-if="banners.length > 1" class="banner-dots">
      <button
        v-for="(banner, idx) in banners"
        :key="banner.id"
        type="button"
        class="banner-dot"
        :class="{ active: idx === dotIndex }"
        :aria-label="`切换到第 ${idx + 1} 张`"
        @click.stop="goTo(idx)"
      ></button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'

const BACKGROUND_COLORS = [
  'var(--color-gray-900)',
  'var(--color-primary-900)',
  'var(--color-secondary-900)',
  'var(--color-success-900)',
  'var(--color-gray-800)'
]

const props = defineProps({
  banners: {
    type: Array,
    default: () => []
  }
})

const router = useRouter()
const currentIndex = ref(0)
const isPaused = ref(false)
const noTransition = ref(false)
let intervalId = null

const N = computed(() => Math.min(props.banners.length, 5))

const loopedBanners = computed(() => {
  const list = props.banners.slice(0, 5)
  if (list.length <= 1) return list
  return [...list, ...list]
})

const dotIndex = computed(() => currentIndex.value % N.value)

const trackStyle = computed(() => ({
  transform: `translateX(-${currentIndex.value * 100}%)`,
  transition: noTransition.value ? 'none' : 'transform var(--motion-duration-panel) var(--motion-ease-out)'
}))

function backgroundForIndex(index) {
  return BACKGROUND_COLORS[index % BACKGROUND_COLORS.length]
}

function prev() {
  if (N.value <= 1) return
  if (currentIndex.value === 0) {
    // jump to last real slide first (no animation), then animate back one
    noTransition.value = true
    currentIndex.value = N.value
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        noTransition.value = false
        currentIndex.value = N.value - 1
      })
    })
  } else {
    noTransition.value = false
    currentIndex.value--
  }
}

function next() {
  if (N.value <= 1) return
  noTransition.value = false
  currentIndex.value++
}

function onTransitionEnd() {
  if (currentIndex.value >= N.value) {
    noTransition.value = true
    currentIndex.value = 0
  }
}

function goTo(idx) {
  noTransition.value = false
  currentIndex.value = idx
}

function pause() { isPaused.value = true }
function resume() { isPaused.value = false }

function handleClick(banner) {
  if (banner.LinkURL) {
    if (banner.LinkURL.startsWith('/')) {
      router.push(banner.LinkURL)
    } else if (banner.LinkURL.startsWith('http')) {
      window.open(banner.LinkURL, '_blank')
    }
  }
}

function startAutoRotate() {
  stopAutoRotate()
  intervalId = setInterval(() => {
    if (!isPaused.value) next()
  }, 4000)
}

function stopAutoRotate() {
  if (intervalId) {
    clearInterval(intervalId)
    intervalId = null
  }
}

// 标签页隐藏时停掉轮播计时器，恢复可见时重新启动；与 reduced-motion 门控独立叠加
function onVisibilityChange() {
  if (document.hidden) stopAutoRotate()
  else startAutoRotate()
}

onMounted(() => {
  if (N.value <= 1) return
  // prefers-reduced-motion 下关闭自动轮播，手动切换（箭头/圆点）仍可用
  const reduced = window.matchMedia?.('(prefers-reduced-motion: reduce)')?.matches
  if (reduced) return
  startAutoRotate()
  document.addEventListener('visibilitychange', onVisibilityChange)
})

onUnmounted(() => {
  stopAutoRotate()
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<style scoped>
.promo-banner-wrapper {
  position: relative;
  overflow: hidden;
  border: 1px solid var(--resource-line);
  border-radius: 10px;
  margin-bottom: 14px;
}

.promo-banner-track {
  display: flex;
  will-change: transform;
}

.promo-banner-slide {
  flex: 0 0 100%;
  min-height: 136px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 2rem;
  position: relative;
  overflow: hidden;
  padding: 1.1rem 4.25rem;
}

.promo-banner-slide.is-clickable {
  cursor: pointer;
}

.promo-banner-slide:focus-visible {
  outline: 2px solid var(--color-gray-50);
  outline-offset: -4px;
}

.banner-bg {
  position: absolute;
  inset: 0;
  opacity: 0.28;
}

.banner-bg img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.banner-content {
  position: relative;
  z-index: 1;
  max-width: 620px;
}

.banner-title {
  font-size: 1.25rem;
  font-weight: 700;
  color: var(--color-gray-50);
  margin: 0 0 0.35rem;
}

.banner-subtitle {
  font-size: 0.8125rem;
  color: color-mix(in srgb, var(--color-gray-50) 82%, transparent);
  margin: 0 0 0.8rem;
}

.banner-link {
  display: inline-block;
  padding: 0.4rem 0.7rem;
  border: 1px solid color-mix(in srgb, var(--color-gray-50) 30%, transparent);
  border-radius: 7px;
  background: color-mix(in srgb, var(--color-gray-50) 12%, transparent);
  color: var(--color-gray-50);
  font-size: 0.75rem;
  font-weight: 650;
  transition: background var(--motion-duration-quick);
}

.banner-link:hover {
  background: color-mix(in srgb, var(--color-gray-50) 20%, transparent);
}

.banner-hardware {
  position: relative;
  z-index: 1;
  width: 218px;
  flex: 0 0 218px;
  padding: 12px;
  border: 1px solid color-mix(in srgb, var(--color-gray-50) 28%, transparent);
  border-radius: 8px;
  background: color-mix(in srgb, var(--color-gray-50) 9%, transparent);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--text-primary) 12%, transparent);
}

.hardware-topline {
  display: flex;
  justify-content: flex-end;
  gap: 5px;
  margin-bottom: 9px;
}

.hardware-mark {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: color-mix(in srgb, var(--color-gray-50) 38%, transparent);
}

.hardware-mark.is-active {
  background: var(--color-success-400);
}

.hardware-bays {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 7px;
}

.hardware-bay {
  position: relative;
  height: 58px;
  border: 1px solid color-mix(in srgb, var(--color-gray-50) 26%, transparent);
  border-radius: 4px;
  background: color-mix(in srgb, var(--text-primary) 34%, transparent);
}

.hardware-bay::before {
  content: '';
  position: absolute;
  top: 6px;
  left: 50%;
  width: 15px;
  height: 2px;
  transform: translateX(-50%);
  border-radius: 1px;
  background: color-mix(in srgb, var(--color-gray-50) 34%, transparent);
}

.hardware-bay i {
  position: absolute;
  right: 5px;
  bottom: 5px;
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: var(--color-success-400);
}

.banner-dots {
  position: absolute;
  bottom: 0.75rem;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  gap: 0.5rem;
  z-index: 2;
}

.banner-dot {
  width: 8px;
  height: 8px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: color-mix(in srgb, var(--color-gray-50) 40%, transparent);
  cursor: pointer;
  transition: all var(--motion-duration-fast) var(--motion-ease-out);
}

.banner-dot.active {
  background: var(--color-gray-50);
  width: 20px;
  border-radius: 4px;
}

.banner-arrow {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  width: 36px;
  height: 36px;
  border-radius: 50%;
  border: 1px solid color-mix(in srgb, var(--color-gray-50) 22%, transparent);
  background: color-mix(in srgb, var(--color-gray-50) 12%, transparent);
  color: var(--color-gray-50);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 3;
  transition: opacity var(--motion-duration-quick) var(--motion-ease-out);
  opacity: 0;
}

.promo-banner-wrapper:hover .banner-arrow,
.banner-arrow:focus-visible {
  opacity: 1;
}

.banner-arrow:hover {
  background: color-mix(in srgb, var(--color-gray-50) 22%, transparent);
}

.banner-arrow-left {
  left: 0.75rem;
}

.banner-arrow-right {
  right: 0.75rem;
}

@media (max-width: 640px) {
  .promo-banner-slide {
    min-height: 120px;
    padding: 1.15rem 2.8rem;
  }

  .banner-title {
    font-size: 1.05rem;
  }

  .banner-hardware {
    display: none;
  }

  .banner-arrow {
    width: 30px;
    height: 30px;
    opacity: 0.85;
  }
}
</style>
