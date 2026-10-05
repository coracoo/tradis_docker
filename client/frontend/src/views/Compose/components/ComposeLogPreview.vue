<template>
  <LiveLogPreview
    :source="logSource"
    title="Compose 日志"
    :tail="tail"
    :max-lines="maxLines"
    :expandable="expandable"
    @open="$emit('open', project)"
  />
</template>

<script setup>
import { computed } from 'vue'
import LiveLogPreview from '@/components/container/LiveLogPreview.vue'
import { useEnvironmentContext } from '@/composables/useEnvironmentContext.js'

const props = defineProps({
  project: {
    type: Object,
    default: null
  },
  tail: {
    type: Number,
    default: 20
  },
  maxLines: {
    type: Number,
    default: 80
  },
  expandable: {
    type: Boolean,
    default: false
  }
})

defineEmits(['open'])
const { isRemoteEnvironment } = useEnvironmentContext()

function isRunning(container) {
  const state = String(container?.State || container?.state || '').toLowerCase()
  return state === 'running' || state === '运行中'
}

function containerLabel(container) {
  const dockerName = container?.Names?.[0]?.replace(/^\//, '')
  return container?._name || container?.Service || container?.service || dockerName || String(container?.Id || '').slice(0, 12) || 'container'
}

const logSource = computed(() => {
  const containers = props.project?.containers || []
  const running = containers.some(isRunning)

  if (props.project?.type === 'container') {
    const container = containers[0]
    return {
      type: 'container',
      id: container?.Id || container?.id || '',
      label: containerLabel(container),
      running
    }
  }

	const displayName = String(props.project?.name || '').trim()
	const name = isRemoteEnvironment.value
		? String(props.project?.composeProjectName || displayName).trim()
		: displayName
  return {
    type: 'compose',
    id: name,
		label: displayName || name || 'compose',
    running
  }
})
</script>
