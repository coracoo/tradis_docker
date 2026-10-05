import { computed, provide } from 'vue'

const localEnvironment = Object.freeze({ name: '本机' })

export function useEditionResourceWorkbenchContext() {
  const environmentId = computed(() => 'local')
  const isRemoteReadOnly = computed(() => false)
  const isStale = computed(() => false)

  provide('remoteResourceContext', {
    environmentId,
    readOnly: isRemoteReadOnly,
    stale: isStale
  })

  return {
    environment: computed(() => localEnvironment),
    showRemoteContext: computed(() => false),
    isRemoteReadOnly,
    remoteContextClass: computed(() => ({})),
    remoteContextLabel: computed(() => '')
  }
}
