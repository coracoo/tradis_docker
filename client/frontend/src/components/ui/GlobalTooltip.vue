<template>
  <Teleport to="body">
    <div
      v-if="text"
      :id="tooltipId"
      ref="tooltipRef"
      class="global-tooltip"
      :class="{ 'is-visible': visible }"
      :style="tooltipStyle"
      role="tooltip"
    >
      {{ text }}
    </div>
  </Teleport>
</template>

<script setup>
import { nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'

const tooltipId = 'tradis-global-tooltip'
const tooltipRef = ref(null)
const text = ref('')
const visible = ref(false)
const tooltipStyle = reactive({
  left: '0px',
  top: '0px',
  transformOrigin: '50% 100%'
})

const nativeTitles = new WeakMap()
const managedDescriptions = new WeakSet()
let activeTarget = null
let hoveredTarget = null
let focusedTarget = null
let lastInputMode = 'keyboard'
let hideTimer = 0
let showEpoch = 0
let titleObserver = null

function tooltipTarget(node) {
  if (!(node instanceof Element)) return null
  if (activeTarget?.isConnected && activeTarget.contains(node)) return activeTarget
  const target = node.closest('[title], [data-tooltip]')
  if (!target || target.closest('[data-tooltip-disabled]')) return null
  return target
}

function tooltipText(target) {
  return (
    target.getAttribute('data-tooltip') ||
    target.getAttribute('title') ||
    nativeTitles.get(target) ||
    ''
  ).trim()
}

function captureTarget(target) {
  const title = target.getAttribute('title')
  if (title !== null) {
    nativeTitles.set(target, title)
    target.removeAttribute('title')
  }

  const current = target.getAttribute('aria-describedby')?.split(/\s+/).filter(Boolean) || []
  if (!current.includes(tooltipId)) {
    target.setAttribute('aria-describedby', [...current, tooltipId].join(' '))
    managedDescriptions.add(target)
  }
}

function restoreTarget(target) {
  if (!target) return
  titleObserver?.disconnect()
  titleObserver = null
  const title = nativeTitles.get(target)
  if (title !== undefined && target.isConnected) {
    target.setAttribute('title', title)
  }
  nativeTitles.delete(target)

  if (target.isConnected && managedDescriptions.has(target)) {
    const descriptions = target.getAttribute('aria-describedby')?.split(/\s+/).filter(Boolean) || []
    const remaining = descriptions.filter(id => id !== tooltipId)
    if (remaining.length) target.setAttribute('aria-describedby', remaining.join(' '))
    else target.removeAttribute('aria-describedby')
  }
  managedDescriptions.delete(target)
}

function observeTargetTitle(target) {
  titleObserver?.disconnect()
  titleObserver = new MutationObserver(mutations => {
    if (activeTarget !== target) return
    if (!target.isConnected) {
      hideTooltip()
      return
    }
    if (!mutations.some(mutation => mutation.type === 'attributes' && mutation.target === target)) return
    const updatedTitle = target.getAttribute('title')
    if (updatedTitle === null) return
    nativeTitles.set(target, updatedTitle)
    target.removeAttribute('title')
    text.value = tooltipText(target)
    positionTooltip(++showEpoch)
  })
  titleObserver.observe(document.body, {
    attributes: true,
    attributeFilter: ['title'],
    childList: true,
    subtree: true
  })
}

async function positionTooltip(epoch) {
  await nextTick()
  if (epoch !== showEpoch || !activeTarget || !tooltipRef.value) return

  const targetRect = activeTarget.getBoundingClientRect()
  const tooltipRect = tooltipRef.value.getBoundingClientRect()
  const gap = 8
  const viewportPadding = 12
  const openAbove = targetRect.top >= tooltipRect.height + gap + viewportPadding
  const idealLeft = targetRect.left + (targetRect.width - tooltipRect.width) / 2
  const maxLeft = Math.max(viewportPadding, window.innerWidth - tooltipRect.width - viewportPadding)

  tooltipStyle.left = `${Math.min(Math.max(idealLeft, viewportPadding), maxLeft)}px`
  tooltipStyle.top = `${openAbove
    ? targetRect.top - tooltipRect.height - gap
    : targetRect.bottom + gap}px`
  tooltipStyle.transformOrigin = openAbove ? '50% 100%' : '50% 0%'

  requestAnimationFrame(() => {
    if (epoch === showEpoch) visible.value = true
  })
}

function showTooltip(target) {
  const nextText = tooltipText(target)
  if (!nextText) return

  window.clearTimeout(hideTimer)
  if (activeTarget && activeTarget !== target) restoreTarget(activeTarget)

  activeTarget = target
  captureTarget(target)
  observeTargetTitle(target)
  text.value = nextText
  visible.value = false
  const epoch = ++showEpoch
  positionTooltip(epoch)
}

function hideTooltip() {
  if (!activeTarget) return
  visible.value = false
  ++showEpoch
  window.clearTimeout(hideTimer)
  hideTimer = window.setTimeout(() => {
    restoreTarget(activeTarget)
    activeTarget = null
    text.value = ''
  }, 55)
}

function handlePointerOver(event) {
  lastInputMode = event.pointerType === 'touch' ? 'touch' : 'pointer'
  if (lastInputMode === 'touch') return
  const target = tooltipTarget(event.target)
  if (!target) return
  hoveredTarget = target
  if (activeTarget !== target || !visible.value) showTooltip(target)
}

function handlePointerOut(event) {
  if (!hoveredTarget || !hoveredTarget.contains(event.target)) return
  if (event.relatedTarget instanceof Node && hoveredTarget.contains(event.relatedTarget)) return
  const leavingTarget = hoveredTarget
  hoveredTarget = null
  if (focusedTarget) {
    if (focusedTarget !== activeTarget) showTooltip(focusedTarget)
    return
  }
  if (activeTarget === leavingTarget) hideTooltip()
}

function handleFocusIn(event) {
  if (lastInputMode === 'touch') return
  const target = tooltipTarget(event.target)
  if (!target) return
  focusedTarget = target
  if (activeTarget !== target || !visible.value) showTooltip(target)
}

function handlePointerDown(event) {
  lastInputMode = event.pointerType === 'touch' ? 'touch' : 'pointer'
}

function handleKeyDown() {
  lastInputMode = 'keyboard'
}

function handleFocusOut(event) {
  if (!focusedTarget || !focusedTarget.contains(event.target)) return
  if (event.relatedTarget instanceof Node && focusedTarget.contains(event.relatedTarget)) return
  const leavingTarget = focusedTarget
  focusedTarget = null
  if (hoveredTarget) {
    if (hoveredTarget !== activeTarget) showTooltip(hoveredTarget)
    return
  }
  if (activeTarget === leavingTarget) hideTooltip()
}

function handleViewportChange() {
  if (!activeTarget || !activeTarget.isConnected) {
    hideTooltip()
    return
  }
  positionTooltip(showEpoch)
}

onMounted(() => {
  document.addEventListener('pointerover', handlePointerOver)
  document.addEventListener('pointerout', handlePointerOut)
  document.addEventListener('pointerdown', handlePointerDown)
  document.addEventListener('keydown', handleKeyDown)
  document.addEventListener('focusin', handleFocusIn)
  document.addEventListener('focusout', handleFocusOut)
  window.addEventListener('resize', handleViewportChange)
  window.addEventListener('scroll', handleViewportChange, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerover', handlePointerOver)
  document.removeEventListener('pointerout', handlePointerOut)
  document.removeEventListener('pointerdown', handlePointerDown)
  document.removeEventListener('keydown', handleKeyDown)
  document.removeEventListener('focusin', handleFocusIn)
  document.removeEventListener('focusout', handleFocusOut)
  window.removeEventListener('resize', handleViewportChange)
  window.removeEventListener('scroll', handleViewportChange, true)
  window.clearTimeout(hideTimer)
  restoreTarget(activeTarget)
})
</script>

<style scoped>
.global-tooltip {
  position: fixed;
  z-index: 10000;
  width: max-content;
  max-width: min(360px, calc(100vw - 24px));
  padding: 8px 11px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--bg-elevated);
  box-shadow: var(--shadow-lg);
  color: var(--text-primary);
  font-size: 0.75rem;
  line-height: 1.55;
  opacity: 0;
  overflow-wrap: anywhere;
  pointer-events: none;
  transform: scale(var(--motion-tooltip-scale));
  transition:
    opacity var(--motion-tooltip-out) var(--motion-ease-out),
    transform var(--motion-tooltip-out) var(--motion-ease-out),
    visibility 0s linear var(--motion-tooltip-out);
  visibility: hidden;
  white-space: pre-wrap;
}

.global-tooltip.is-visible {
  opacity: 1;
  transform: scale(1);
  transition:
    opacity var(--motion-tooltip-in) var(--motion-ease-out) var(--motion-tooltip-delay),
    transform var(--motion-tooltip-in) var(--motion-ease-out) var(--motion-tooltip-delay),
    visibility 0s linear var(--motion-tooltip-delay);
  visibility: visible;
}
</style>
