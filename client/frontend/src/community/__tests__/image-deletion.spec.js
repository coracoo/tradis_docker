import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { del } from '@/utils/request.js'
import { removeImage } from '@/api/images.js'

vi.mock('@/utils/request.js', () => ({
  get: vi.fn(), post: vi.fn(), del: vi.fn(), buildApiUrl: path => `/api${path}`
}))

const describeCommunity = import.meta.env.VITE_TRADIS_EDITION === 'community' ? describe : describe.skip

describeCommunity('community image deletion', () => {
  beforeEach(() => {
    localStorage.clear()
    del.mockReset().mockResolvedValue({})
  })
  afterEach(() => localStorage.clear())

  it('stays local despite a stored or explicitly requested remote environment', async () => {
    localStorage.setItem('tradis_current_environment', 'remote-stale')
    const a = removeImage('image', '', { environmentId: 'remote-one' })
    const b = removeImage('image', '', { environmentId: 'remote-two' })
    await Promise.all([a, b])
    expect(a).toBe(b)
    expect(del).toHaveBeenCalledTimes(1)
    expect(del).toHaveBeenCalledWith('/images/image', {
      headers: {}
    })
  })
})
