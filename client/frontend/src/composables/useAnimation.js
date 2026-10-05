// 动画 Composable
import { computed, onScopeDispose, ref, watch } from 'vue'

// 用户开启减少动态效果时跳过动画
const prefersReducedMotion = () =>
  window.matchMedia?.('(prefers-reduced-motion: reduce)')?.matches === true

/**
 * 数字滚动动画
 * @param {Ref<number>} valueRef - 数值 ref
 * @param {Object} options - 配置
 */
export function useCountUp(valueRef, options = {}) {
  const { duration = 1000, decimals = 0 } = options
  const displayValue = ref(0)
  const isAnimating = ref(false)

  let animationId = null

  const animate = (target) => {
    if (animationId) cancelAnimationFrame(animationId)

    if (prefersReducedMotion()) {
      displayValue.value = target
      isAnimating.value = false
      return
    }

    isAnimating.value = true
    const start = displayValue.value
    const diff = target - start
    const startTime = performance.now()

    const step = (currentTime) => {
      const elapsed = currentTime - startTime
      const progress = Math.min(elapsed / duration, 1)

      // easeOutQuart
      const easeProgress = 1 - Math.pow(1 - progress, 4)

      displayValue.value = start + diff * easeProgress

      if (progress < 1) {
        animationId = requestAnimationFrame(step)
      } else {
        isAnimating.value = false
        displayValue.value = target
      }
    }

    animationId = requestAnimationFrame(step)
  }

  // 监听值变化
  watch(valueRef, (newVal) => {
    animate(newVal)
  }, { immediate: true })

  onScopeDispose(() => {
    if (animationId) cancelAnimationFrame(animationId)
  })

  const formattedValue = computed(() => {
    return displayValue.value.toFixed(decimals)
  })

  return {
    displayValue,
    formattedValue,
    isAnimating
  }
}
