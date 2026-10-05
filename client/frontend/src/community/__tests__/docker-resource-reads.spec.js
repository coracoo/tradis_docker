import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const api = vi.hoisted(() => ({
  images: { list: vi.fn() }, containers: { list: vi.fn() },
  volumes: { list: vi.fn() }, networks: { list: vi.fn() },
  compose: { list: vi.fn(), listContainerRemarks: vi.fn() }
}))
vi.mock('@edition/api', () => api)
import { useDockerResourcesStore } from '@/stores/dockerResources.js'

const describeCommunity = import.meta.env.VITE_TRADIS_EDITION === 'community' ? describe : describe.skip

describeCommunity('community Docker display reads', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
    vi.resetAllMocks()
    api.images.list.mockResolvedValue([])
    api.containers.list.mockResolvedValue([])
    api.compose.list.mockResolvedValue([])
    api.compose.listContainerRemarks.mockResolvedValue({})
    api.volumes.list.mockResolvedValue({ Volumes: [] })
    api.networks.list.mockResolvedValue([])
  })

  it('shares one local container read without sending stored remote identity', async () => {
    localStorage.setItem('tradis_current_environment', 'remote-stale')
    const store = useDockerResourcesStore()
    await Promise.all([store.loadImages(), store.loadComposeProjects(), store.loadVolumes(), store.loadNetworks()])
    expect(api.containers.list).toHaveBeenCalledTimes(1)
    expect(api.containers.list).toHaveBeenCalledWith({ all: true }, { headers: {} })
    expect(api.images.list).toHaveBeenCalledWith({ headers: {} })
    expect(api.compose.list).toHaveBeenCalledWith({}, { headers: {} })
    expect(api.volumes.list).toHaveBeenCalledWith({ headers: {} })
    expect(api.networks.list).toHaveBeenCalledWith({ brief: true }, { headers: {} })
    expect(store.resources.containers.environmentId).toBe('local')
  })

  it('expires shared display data after resource invalidation', async () => {
    const store = useDockerResourcesStore()
    await store.loadImages()
    store.invalidate('images')
    await store.loadComposeProjects()
    expect(api.containers.list).toHaveBeenCalledTimes(2)
  })
})
