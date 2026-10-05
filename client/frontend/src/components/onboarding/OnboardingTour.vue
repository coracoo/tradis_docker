<template>
  <div v-if="onboarding.active" class="onboarding-tour" aria-live="polite">
    <div class="tour-scrim" @click="onScrimClick" />
    <div
      v-if="spotlightStyle"
      class="tour-spotlight"
      :style="spotlightStyle"
      aria-hidden="true"
    />
    <div
      v-if="popoverStyle && currentStep"
      class="tour-popover"
      :class="`is-${popoverStyle.placement}`"
      :style="popoverStyle"
      role="dialog"
      aria-modal="false"
      aria-labelledby="tour-step-title"
    >
      <aside class="tour-rail" aria-hidden="true">
        <span
          v-for="(step, index) in steps"
          :key="step.id"
          class="tour-rail-node"
          :class="{
            'is-done': onboarding.completedStepSet.has(step.id),
            'is-current': index === onboarding.stepIndex
          }"
        >{{ index + 1 }}</span>
      </aside>
      <div class="tour-content">
        <p class="tour-eyebrow">第 {{ onboarding.stepIndex + 1 }} / {{ onboarding.totalSteps }} 步</p>
        <h3 id="tour-step-title" class="tour-title">{{ currentStep.title }}</h3>
        <p class="tour-text">{{ currentStep.body }}</p>
        <div class="tour-actions">
          <button type="button" class="tour-btn ghost" @click="onboarding.skip()">跳过引导</button>
          <button v-if="onboarding.stepIndex > 0" type="button" class="tour-btn" @click="onboarding.prev()">上一步</button>
          <button type="button" class="tour-btn primary" @click="advance">
            {{ onboarding.isLastStep ? '完成' : '下一步' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useOnboardingStore } from '@/stores/onboarding.js'
import { tourSteps } from './tourSteps.js'
import { computePopoverPlacement, computeSpotlightStyle, waitForSelector } from './anchorGeometry.js'

const POPOVER_SIZE = { width: 340, height: 220 }

const onboarding = useOnboardingStore()
const route = useRoute()
const router = useRouter()

const steps = tourSteps
const currentStep = computed(() => onboarding.currentStep)
const viewport = ref(readViewport())

function readViewport() {
  if (typeof window === 'undefined') return { width: 1280, height: 800 }
  return { width: window.innerWidth, height: window.innerHeight }
}

const spotlightStyle = computed(() => {
  const style = computeSpotlightStyle(onboarding.anchorRect)
  if (!style) return null
  return {
    top: `${style.top}px`,
    left: `${style.left}px`,
    width: `${style.width}px`,
    height: `${style.height}px`
  }
})

const popoverStyle = computed(() => {
  const placement = computePopoverPlacement(onboarding.anchorRect, viewport.value, POPOVER_SIZE)
  return {
    placement: placement.placement,
    top: `${placement.top}px`,
    left: `${placement.left}px`
  }
})

let anchorEl = null
let resizeObserver = null

function readRect() {
  if (!anchorEl) return
  const rect = anchorEl.getBoundingClientRect()
  onboarding.setAnchorRect({
    top: rect.top,
    left: rect.left,
    right: rect.right,
    bottom: rect.bottom,
    width: rect.width,
    height: rect.height
  })
}

function onScroll() { readRect() }
function onResize() {
  viewport.value = readViewport()
  readRect()
}

function attachObservers() {
  if (!anchorEl || typeof window === 'undefined') return
  resizeObserver = new ResizeObserver(readRect)
  resizeObserver.observe(anchorEl)
  window.addEventListener('scroll', onScroll, true)
  window.addEventListener('resize', onResize)
}

function teardownAnchor() {
  if (resizeObserver) { resizeObserver.disconnect(); resizeObserver = null }
  if (typeof window !== 'undefined') {
    window.removeEventListener('scroll', onScroll, true)
    window.removeEventListener('resize', onResize)
  }
  anchorEl = null
}

async function runStep(step) {
  teardownAnchor()
  onboarding.setAnchorRect(null)
  if (!step) return
  if (step.openDockerSettings) onboarding.openDockerSettings = true
  else onboarding.openDockerSettings = false

  if (step.route && route.path !== step.route) {
    try { await router.push(step.route) } catch { /* navigation aborted */ }
  }
  const el = await waitForSelector(step.anchor, { timeout: 3000 })
  if (!el || !onboarding.active) {
    onboarding.setAnchorRect(null)
    return
  }
  anchorEl = el
  readRect()
  attachObservers()
}

watch(
  () => (onboarding.active ? onboarding.stepIndex : -1),
  async () => {
    if (!onboarding.active) {
      teardownAnchor()
      onboarding.setAnchorRect(null)
      return
    }
    await runStep(currentStep.value)
  },
  { immediate: true }
)

function onScrimClick() {
  // 吸收点击，避免误触背后页面；不关闭巡游。
}

function onKeydown(event) {
  if (!onboarding.active) return
  if (event.key === 'Escape') onboarding.skip()
  else if (event.key === 'ArrowRight') advance()
  else if (event.key === 'ArrowLeft' && onboarding.stepIndex > 0) onboarding.prev()
}

function advance() {
  if (onboarding.isLastStep) onboarding.finish()
  else onboarding.next()
}

if (typeof window !== 'undefined') {
  window.addEventListener('keydown', onKeydown)
}

onBeforeUnmount(() => {
  if (typeof window !== 'undefined') window.removeEventListener('keydown', onKeydown)
  teardownAnchor()
})
</script>

<style scoped>
.onboarding-tour {
  position: fixed;
  inset: 0;
  z-index: 9999;
  color: var(--text-primary);
  font-family: var(--font-sans, Inter, ui-sans-serif, system-ui, sans-serif);
}

.tour-scrim {
  position: absolute;
  inset: 0;
  background: transparent;
  cursor: default;
}

.tour-spotlight {
  position: absolute;
  background: transparent;
  border: 2px solid var(--color-primary-500, var(--color-primary));
  border-radius: 10px;
  box-shadow: 0 0 0 9999px color-mix(in srgb, #0b1220 55%, transparent);
  pointer-events: none;
}

.tour-popover {
  position: absolute;
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr);
  gap: 14px;
  width: 340px;
  padding: 16px 18px 14px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 14px;
  box-shadow: var(--shadow-xl, 0 18px 48px rgba(0, 0, 0, 0.18));
}

.tour-rail {
  display: grid;
  grid-template-rows: repeat(auto-fit, minmax(0, 1fr));
  gap: 8px;
  align-content: start;
  padding-top: 4px;
}

.tour-rail-node {
  display: inline-grid;
  place-items: center;
  width: 26px;
  height: 26px;
  border-radius: 999px;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  font-size: 0.72rem;
  font-weight: 700;
  border: 1px solid transparent;
}

.tour-rail-node.is-current {
  background: color-mix(in srgb, var(--color-primary-500, var(--color-primary)) 14%, transparent);
  color: var(--color-primary-700, var(--color-primary));
  border-color: var(--color-primary-400, var(--color-primary));
}

.tour-rail-node.is-done {
  background: var(--color-success-100, var(--bg-tertiary));
  color: var(--color-success-700, var(--text-primary));
}

.tour-content {
  min-width: 0;
}

.tour-eyebrow {
  margin: 0 0 4px;
  color: var(--text-tertiary);
  font-size: 0.68rem;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.tour-title {
  margin: 0 0 6px;
  color: var(--text-primary);
  font-size: 1rem;
  font-weight: 650;
}

.tour-text {
  margin: 0 0 14px;
  color: var(--text-secondary);
  font-size: 0.8125rem;
  line-height: 1.55;
}

.tour-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.tour-btn {
  min-height: 34px;
  padding: 0 14px;
  border: 1px solid var(--border-default);
  border-radius: 9px;
  background: var(--bg-elevated);
  color: var(--text-primary);
  font-family: inherit;
  font-size: 0.8125rem;
  font-weight: 500;
  cursor: pointer;
  transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}

.tour-btn:hover {
  border-color: var(--color-primary-400, var(--color-primary));
  color: var(--color-primary-700, var(--color-primary));
}

.tour-btn.ghost {
  border-color: transparent;
  color: var(--text-tertiary);
  margin-right: auto;
}

.tour-btn.ghost:hover {
  color: var(--text-secondary);
}

.tour-btn.primary {
  border-color: var(--color-primary-600, var(--color-primary));
  background: linear-gradient(135deg, var(--color-primary-500, var(--color-primary)), var(--color-primary-700, var(--color-primary)));
  color: var(--text-inverse, #fff);
}

.tour-btn.primary:hover {
  color: var(--text-inverse, #fff);
  filter: brightness(1.04);
}

@media (prefers-reduced-motion: no-preference) {
  .tour-spotlight {
    transition: top 0.18s ease, left 0.18s ease, width 0.18s ease, height 0.18s ease;
  }
  .tour-btn {
    transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease, filter 0.15s ease;
  }
}
</style>
