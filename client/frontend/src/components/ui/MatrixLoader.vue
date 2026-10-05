<template>
  <span class="matrix-loader" :data-variant="variant" role="status" :aria-label="label">
    <i
      v-for="(delay, index) in delays"
      :key="index"
      :class="{ 'is-gap': rounded && corners.has(index) }"
      :style="{ '--matrix-delay': `${delay}ms` }"
      aria-hidden="true"
    ></i>
  </span>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  variant: { type: String, default: 'scan', validator: value => ['scan', 'twinkle', 'orbit', 'pulse'].includes(value) },
  label: { type: String, default: '加载中' },
  rounded: { type: Boolean, default: false }
})

const cycle = 1200
const corners = new Set([0, 3, 12, 15])
const twinkleOrder = [7, 2, 11, 5, 14, 9, 0, 12, 3, 15, 6, 10, 13, 1, 8, 4]
const orbitOrder = [1, 2, 7, 11, 14, 13, 8, 4]
const inner = new Set([5, 6, 9, 10])

const delays = computed(() => Array.from({ length: 16 }, (_, index) => {
  if (props.variant === 'twinkle') return twinkleOrder.indexOf(index) * cycle / 16
  if (props.variant === 'orbit') {
    const position = orbitOrder.indexOf(index)
    return position >= 0 ? position * cycle / 8 : 0
  }
  if (props.variant === 'pulse') return inner.has(index) ? 0 : cycle * 0.16
  return (index % 4) * cycle / 10
}))
</script>

<style scoped>
.matrix-loader {
  display: inline-grid;
  grid-template-columns: repeat(4, 3px);
  grid-auto-rows: 3px;
  gap: 2px;
  flex: 0 0 auto;
}

.matrix-loader i {
  display: block;
  border-radius: 1px;
  background: var(--text-muted);
  animation: matrix-loader-pulse 1200ms ease-in-out infinite;
  animation-delay: var(--matrix-delay);
}

.matrix-loader i.is-gap { visibility: hidden; animation: none; }

@keyframes matrix-loader-pulse {
  0%, 45%, 100% { background-color: var(--text-muted); opacity: 0.45; }
  15% { background-color: var(--color-primary-500); opacity: 1; }
}

@media (prefers-reduced-motion: reduce) {
  .matrix-loader i { animation: none !important; opacity: 0.7; }
}
</style>
