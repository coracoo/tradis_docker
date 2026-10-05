<template>
  <div
    ref="rootRef"
    class="segmented-tabs"
    :class="{ 'is-ready': ready, 'is-compact': compact }"
    role="tablist"
    :aria-label="ariaLabel"
  >
    <span class="segmented-tabs__pill" :style="pillStyle" aria-hidden="true"></span>
    <button
      v-for="option in options"
      :key="option.value"
      ref="tabRefs"
      type="button"
      class="segmented-tabs__tab"
      role="tab"
      :aria-selected="modelValue === option.value"
      :aria-controls="panelId || undefined"
      :tabindex="modelValue === option.value ? 0 : -1"
      :disabled="option.disabled"
      @click="select(option.value)"
      @keydown="handleKeydown(option.value, $event)"
    >
      <span>{{ option.label }}</span>
      <small v-if="option.count !== undefined">{{ option.count }}</small>
    </button>
  </div>
</template>

<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps({
  modelValue: { type: [String, Number], required: true },
  options: { type: Array, default: () => [] },
  ariaLabel: { type: String, default: '选项切换' },
  panelId: { type: String, default: '' },
  compact: { type: Boolean, default: false }
})

const emit = defineEmits(['update:modelValue', 'change'])
const rootRef = ref(null)
const tabRefs = ref([])
const pillStyle = ref({ width: '0px', transform: 'translateX(0px)' })
const ready = ref(false)
let resizeObserver = null

function activeIndex() {
  return props.options.findIndex(option => option.value === props.modelValue)
}

async function syncPill(animate = true) {
  await nextTick()
  const index = Math.max(0, activeIndex())
  const tab = tabRefs.value[index]
  const root = rootRef.value
  if (!tab || !root) return
  pillStyle.value = {
    width: `${tab.offsetWidth}px`,
    transform: `translateX(${tab.offsetLeft}px)`
  }
  if (!animate) requestAnimationFrame(() => { ready.value = true })
}

function select(value) {
  const option = props.options.find(item => item.value === value)
  if (!option || option.disabled || value === props.modelValue) return
  emit('update:modelValue', value)
  emit('change', value)
}

function handleKeydown(value, event) {
  const enabled = props.options.filter(option => !option.disabled)
  const index = enabled.findIndex(option => option.value === value)
  if (index < 0) return
  let nextIndex = index
  if (event.key === 'ArrowRight' || event.key === 'ArrowDown') nextIndex = (index + 1) % enabled.length
  else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') nextIndex = (index - 1 + enabled.length) % enabled.length
  else if (event.key === 'Home') nextIndex = 0
  else if (event.key === 'End') nextIndex = enabled.length - 1
  else return
  event.preventDefault()
  select(enabled[nextIndex].value)
  tabRefs.value[props.options.findIndex(option => option.value === enabled[nextIndex].value)]?.focus()
}

watch(() => [props.modelValue, props.options], () => syncPill(), { deep: true })

onMounted(() => {
  syncPill(false)
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => syncPill(false))
    resizeObserver.observe(rootRef.value)
  } else {
    window.addEventListener('resize', syncPill)
  }
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  window.removeEventListener('resize', syncPill)
})
</script>

<style scoped>
.segmented-tabs {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: 3px;
  min-width: 0;
  padding: 3px;
  overflow-x: auto;
  border: 1px solid var(--border-subtle);
  border-radius: 9px;
  background: var(--bg-secondary);
  scrollbar-width: none;
}

.segmented-tabs::-webkit-scrollbar { display: none; }

.segmented-tabs__pill {
  position: absolute;
  top: 3px;
  left: 0;
  height: 30px;
  border-radius: 7px;
  background: var(--bg-elevated);
  box-shadow: var(--shadow-sm);
  pointer-events: none;
  z-index: 0;
}

.segmented-tabs.is-ready .segmented-tabs__pill {
  transition:
    transform var(--motion-duration-panel) var(--motion-ease-out),
    width var(--motion-duration-panel) var(--motion-ease-out);
}

.segmented-tabs__tab {
  position: relative;
  z-index: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 30px;
  padding: 0 12px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--text-secondary);
  font: inherit;
  font-size: 0.8125rem;
  font-weight: 500;
  white-space: nowrap;
  cursor: pointer;
  transition: color var(--motion-duration-fast) var(--motion-ease-out);
}

.segmented-tabs__tab:hover,
.segmented-tabs__tab[aria-selected='true'] { color: var(--text-primary); }
.segmented-tabs__tab[aria-selected='true'] { font-weight: 600; }
.segmented-tabs__tab:disabled { opacity: 0.45; cursor: not-allowed; }
.segmented-tabs__tab:focus-visible { outline: 2px solid var(--color-primary-500); outline-offset: -2px; }
.segmented-tabs__tab small { color: var(--text-tertiary); font-size: 0.6875rem; }

.segmented-tabs.is-compact .segmented-tabs__tab { min-height: 28px; padding-inline: 10px; font-size: 0.78rem; }
.segmented-tabs.is-compact .segmented-tabs__pill { height: 28px; }

@media (prefers-reduced-motion: reduce) {
  .segmented-tabs__pill,
  .segmented-tabs__tab { transition: none !important; }
}
</style>
