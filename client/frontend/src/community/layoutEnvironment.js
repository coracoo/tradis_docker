import { createEnvironmentContext, environmentContextKey } from '@/composables/useEnvironmentContext.js'

export { environmentContextKey }

export function createLayoutEnvironment() {
  return { context: createEnvironmentContext(), load: null }
}
