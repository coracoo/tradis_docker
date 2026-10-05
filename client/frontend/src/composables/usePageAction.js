import { nextTick, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

export function usePageAction(actions, dependencies = {}) {
  const route = dependencies.route || useRoute()
  const router = dependencies.router || useRouter()

  if (!route || !router) return () => {}

  return watch(
    () => route.query?.action,
    async action => {
      const handler = actions[action]
      if (!action || typeof handler !== 'function') return

      const query = { ...route.query }
      delete query.action
      await router.replace({ query })
      await nextTick()
      await handler()
    },
    { immediate: true, flush: 'post' }
  )
}
