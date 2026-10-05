import { beforeEach, describe, expect, it, vi } from 'vitest'
import { get, put } from '@/utils/request.js'
import { getDeploymentDefaults, updateDeploymentDefaults } from '@/api/settings.js'

vi.mock('@/utils/request.js', () => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn()
}))

const describeCommunity = import.meta.env.VITE_TRADIS_EDITION === 'community' ? describe : describe.skip

describeCommunity('community deployment defaults API', () => {
  beforeEach(() => {
    get.mockReset()
    put.mockReset()
  })

  it('uses the local-only deployment defaults endpoint', async () => {
    const profile = {
      puid: 1001,
      pgid: 1002,
      timezone: 'Asia/Tokyo',
      networkPolicy: { allowHost: false },
      libraryPaths: { media: '/volume1/media' }
    }
    get.mockResolvedValue({ profile })
    put.mockResolvedValue({ profile })

    await expect(getDeploymentDefaults()).resolves.toEqual({ profile })
    await expect(updateDeploymentDefaults(profile)).resolves.toEqual({ profile })

    expect(get).toHaveBeenCalledWith('/settings/deployment-defaults')
    expect(put).toHaveBeenCalledWith('/settings/deployment-defaults', profile)
  })
})
