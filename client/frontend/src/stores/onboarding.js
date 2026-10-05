import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import settingsApi from '@/api/settings.js'
import { tourSteps } from '@/components/onboarding/tourSteps.js'

const KV_KEY = 'onboarding_tour'
const SCHEMA_VERSION = 1

function emptyPersisted() {
  return {
    version: SCHEMA_VERSION,
    dismissed: false,
    completed: false,
    completedSteps: [],
    stepIndex: 0
  }
}

function normalizePersisted(input) {
  const value = input && typeof input === 'object' ? input : {}
  const completedSteps = Array.isArray(value.completedSteps)
    ? value.completedSteps.filter(item => typeof item === 'string')
    : []
  return {
    version: SCHEMA_VERSION,
    dismissed: Boolean(value.dismissed),
    completed: Boolean(value.completed),
    completedSteps,
    stepIndex: Number(value.stepIndex) || 0
  }
}

export const useOnboardingStore = defineStore('onboarding', () => {
  const active = ref(false)
  const stepIndex = ref(0)
  const persisted = ref(emptyPersisted())
  const openDockerSettings = ref(false)
  const anchorRect = ref(null)
  const hydrated = ref(false)

  const totalSteps = tourSteps.length
  const currentStep = computed(() => tourSteps[stepIndex.value] || null)
  const isLastStep = computed(() => stepIndex.value >= totalSteps - 1)
  const showEntryBanner = computed(
    () => hydrated.value && !persisted.value.dismissed && !persisted.value.completed
  )
  const completedStepSet = computed(() => new Set(persisted.value.completedSteps))

  function clampStep(index) {
    return Math.min(Math.max(index, 0), totalSteps - 1)
  }

  function markCompleted(id) {
    if (!id) return
    if (completedStepSet.value.has(id)) return
    persisted.value = {
      ...persisted.value,
      completedSteps: [...persisted.value.completedSteps, id]
    }
  }

  async function syncPersist() {
    try {
      await settingsApi.setKVSetting(KV_KEY, JSON.stringify({
        ...persisted.value,
        stepIndex: stepIndex.value,
        version: SCHEMA_VERSION
      }))
    } catch {
      // 剪贴板/网络失败不应阻塞巡游。
    }
  }

  async function hydrate() {
    if (hydrated.value) return
    try {
      const res = await settingsApi.getKVSetting(KV_KEY)
      const raw = res?.value
      if (raw) {
        const parsed = normalizePersisted(JSON.parse(raw))
        persisted.value = parsed
        stepIndex.value = clampStep(parsed.stepIndex)
      }
    } catch {
      // 首次使用尚无记录，保持默认值。
    }
    hydrated.value = true
  }

  function start({ resume = false } = {}) {
    active.value = true
    stepIndex.value = resume ? clampStep(persisted.value.stepIndex) : 0
    openDockerSettings.value = false
    anchorRect.value = null
    void refreshProgress()
  }

  function next() {
    const step = currentStep.value
    if (step) markCompleted(step.id)
    if (isLastStep.value) {
      finish()
      return
    }
    stepIndex.value = clampStep(stepIndex.value + 1)
    openDockerSettings.value = false
    anchorRect.value = null
    void syncPersist()
  }

  function prev() {
    stepIndex.value = clampStep(stepIndex.value - 1)
    openDockerSettings.value = false
    anchorRect.value = null
  }

  function skip() {
    active.value = false
    persisted.value = {
      ...persisted.value,
      dismissed: true,
      stepIndex: stepIndex.value
    }
    openDockerSettings.value = false
    anchorRect.value = null
    void syncPersist()
  }

  function finish() {
    if (currentStep.value) markCompleted(currentStep.value.id)
    active.value = false
    persisted.value = {
      ...persisted.value,
      completed: true,
      dismissed: true,
      stepIndex: 0
    }
    openDockerSettings.value = false
    anchorRect.value = null
    void syncPersist()
  }

  function reset() {
    persisted.value = emptyPersisted()
    stepIndex.value = 0
    hydrated.value = true
    void syncPersist()
  }

  async function refreshProgress() {
    const results = await Promise.allSettled(
      tourSteps.map(step => (typeof step.detect === 'function' ? step.detect() : false))
    )
    const next = new Set(persisted.value.completedSteps)
    results.forEach((result, index) => {
      if (result.status === 'fulfilled' && result.value) next.add(tourSteps[index].id)
    })
    persisted.value = { ...persisted.value, completedSteps: [...next] }
  }

  function setAnchorRect(rect) {
    anchorRect.value = rect
  }

  return {
    active,
    stepIndex,
    persisted,
    openDockerSettings,
    anchorRect,
    hydrated,
    totalSteps,
    currentStep,
    isLastStep,
    showEntryBanner,
    completedStepSet,
    hydrate,
    start,
    next,
    prev,
    skip,
    finish,
    reset,
    refreshProgress,
    setAnchorRect
  }
})
