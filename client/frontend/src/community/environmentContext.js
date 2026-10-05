import { computed } from 'vue'

export const environmentContextKey = Symbol('tradis-community-local-context')

const localEnvironment = Object.freeze({
  id: 'local',
  name: '本机',
  connectionMode: 'local',
  status: 'online',
  accessMode: 'manage',
  capabilitiesJson: '{}'
})

const localContext = Object.freeze({
  environmentId: computed(() => 'local'),
  environment: computed(() => localEnvironment),
  isRemoteEnvironment: computed(() => false),
  isRemoteManaged: computed(() => false),
  capabilities: computed(() => []),
  supportsCapability: () => false,
  canManageCapability: () => false
})

export function environmentCapabilities() {
  return []
}

export function remoteCapabilityAllowed() {
  return false
}

export function createEnvironmentContext() {
  return localContext
}

export function useEnvironmentContext() {
  return localContext
}
