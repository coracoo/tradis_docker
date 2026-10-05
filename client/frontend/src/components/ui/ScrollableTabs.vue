<template>
  <div class="scrollable-tabs">
    <button
      v-show="canScrollLeft"
      type="button"
      class="scrollable-tabs__arrow scrollable-tabs__arrow--left"
      aria-label="向左滚动选项"
      @click="scrollTabs(-SCROLL_STEP)"
    >
      <DynamicIcon name="chevron-left" :size="16" />
    </button>
    <SegmentedTabs
      ref="tabsRef"
      class="scrollable-tabs__track"
      :model-value="modelValue"
      :options="options"
      :aria-label="ariaLabel"
      :compact="compact"
      @update:model-value="value => emit('update:modelValue', value)"
      @change="value => emit('change', value)"
    />
    <button
      v-show="canScrollRight"
      type="button"
      class="scrollable-tabs__arrow scrollable-tabs__arrow--right"
      aria-label="向右滚动选项"
      @click="scrollTabs(SCROLL_STEP)"
    >
      <DynamicIcon name="chevron-right" :size="16" />
    </button>
  </div>
</template>

<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import SegmentedTabs from '@/components/ui/SegmentedTabs.vue'

const props = defineProps({
  modelValue: { type: [String, Number], required: true },
  options: { type: Array, default: () => [] },
  ariaLabel: { type: String, default: '选项切换' },
  compact: { type: Boolean, default: false }
})

const emit = defineEmits(['update:modelValue', 'change'])

const SCROLL_STEP = 200
const SCROLL_DURATION = 220
const tabsRef = ref(null)
const canScrollLeft = ref(false)
const canScrollRight = ref(false)
let scrollEl = null
let resizeObserver = null
let scrollAnimationFrame = null

function checkScroll() {
  const el = scrollEl
  if (!el) return
  canScrollLeft.value = el.scrollLeft > 0
  canScrollRight.value = el.scrollLeft < el.scrollWidth - el.clientWidth - 1
}

function cancelScrollAnimation() {
  if (scrollAnimationFrame !== null) {
    cancelAnimationFrame(scrollAnimationFrame)
    scrollAnimationFrame = null
  }
}

function scrollTabs(delta) {
  if (!scrollEl) return
  cancelScrollAnimation()
  const el = scrollEl
  const maxScroll = Math.max(0, el.scrollWidth - el.clientWidth)
  const target = Math.max(0, Math.min(maxScroll, el.scrollLeft + delta))
  const reduced = window.matchMedia?.('(prefers-reduced-motion: reduce)')?.matches
  if (reduced || target === el.scrollLeft) {
    el.scrollLeft = target
    checkScroll()
    return
  }
  const startedAt = performance.now()
  const startedFrom = el.scrollLeft
  const step = now => {
    const progress = Math.min(1, (now - startedAt) / SCROLL_DURATION)
    const eased = 1 - (1 - progress) ** 3
    el.scrollLeft = startedFrom + (target - startedFrom) * eased
    scrollAnimationFrame = progress < 1 ? requestAnimationFrame(step) : null
  }
  scrollAnimationFrame = requestAnimationFrame(step)
}

function bindScroll() {
  scrollEl = tabsRef.value?.$el
  if (!scrollEl) return
  scrollEl.addEventListener('scroll', checkScroll, { passive: true })
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(checkScroll)
    resizeObserver.observe(scrollEl)
  }
  window.addEventListener('resize', checkScroll)
  checkScroll()
  scrollActiveIntoView()
}

// 选中项变化时滚入可视区，避免激活标签停在滚动区外不可见。
function scrollActiveIntoView() {
  const el = scrollEl
  if (!el) return
  const active = el.querySelector('[aria-selected="true"]')
  if (!active) return
  const left = active.offsetLeft
  const right = left + active.offsetWidth
  const gutter = 34
  if (left - gutter < el.scrollLeft) {
    el.scrollLeft = Math.max(0, left - gutter)
  } else if (right + gutter > el.scrollLeft + el.clientWidth) {
    el.scrollLeft = right + gutter - el.clientWidth
  }
  checkScroll()
}

onMounted(() => nextTick(bindScroll))

watch(() => props.options, () => nextTick(checkScroll), { deep: true })

watch(() => props.modelValue, () => nextTick(scrollActiveIntoView))

onBeforeUnmount(() => {
  cancelScrollAnimation()
  scrollEl?.removeEventListener('scroll', checkScroll)
  resizeObserver?.disconnect()
  window.removeEventListener('resize', checkScroll)
})
</script>

<style scoped>
.scrollable-tabs {
  position: relative;
  display: flex;
  align-items: center;
  width: 100%;
  min-width: 0;
}

.scrollable-tabs__track {
  flex: 1 1 auto;
  min-width: 0;
  /* 两侧常驻留白给箭头，箭头只在可滚动时出现，避免出现时挤压布局 */
  padding-inline: 34px;
}

.scrollable-tabs__arrow {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 1px solid var(--border-subtle);
  border-radius: 50%;
  background: var(--bg-elevated);
  color: var(--text-secondary);
  cursor: pointer;
  box-shadow: 0 1px 4px color-mix(in srgb, var(--text-primary) 8%, transparent);
  transition: background 0.16s ease, color 0.16s ease, border-color 0.16s ease;
}

.scrollable-tabs__arrow:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
  border-color: var(--border-default);
}

.scrollable-tabs__arrow--left {
  left: 0;
}

.scrollable-tabs__arrow--right {
  right: 0;
}

@media (prefers-reduced-motion: reduce) {
  .scrollable-tabs__arrow {
    transition: none;
  }
}
</style>
