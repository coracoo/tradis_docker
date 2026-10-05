import { afterEach, describe, expect, it, vi } from 'vitest'
import { getDeployTaskEventsUrl } from '@/api/appstore.js'
import { getComposeTaskEventsUrl } from '@/api/compose.js'
import { get } from '@/utils/request.js'
import { COMMUNITY_ROUTE_NAMES, COMMUNITY_ROUTE_PATHS } from '../routes.js'
import { sharedChildRoutes } from '@/edition/sharedRoutes.js'
import { editionChildRoutes as communityRoutes } from '../routes.js'
import { settingSections } from '@/views/Settings/settingsSections.js'
import { GLOBAL_SEARCH_ACTIONS, GLOBAL_SEARCH_PAGES } from '@/components/layout/globalSearchCatalog.js'
import { sidebarCatalog } from '../sidebarCatalog.js'

const describeCommunity = import.meta.env.VITE_TRADIS_EDITION === 'community'
  ? describe
  : describe.skip

describe('community route manifest', () => {
  it('reuses the exact route definitions for every shared page', () => {
    const sharedByName = new Map(sharedChildRoutes.map(route => [route.name, route]))
    const communityByName = new Map(communityRoutes.map(route => [route.name, route]))
    const sharedNames = [
      'Overview', 'Images', 'Networks', 'Volumes', 'Containers', 'Compose',
      'Navigation', 'AppStore', 'AppDeploy', 'Ports', 'Cleanup', 'Settings',
      'ScheduledTasks', 'Tutorials', 'TutorialDetail', 'NasStore', 'NasTopic', 'NasReview'
    ]

    for (const name of sharedNames) {
      expect(communityByName.get(name), `${name} must exist in Community`).toBeDefined()
      expect(communityByName.get(name)).toBe(sharedByName.get(name))
    }
  })

  it('only exposes local management routes', () => {
    expect(COMMUNITY_ROUTE_NAMES).toEqual(expect.arrayContaining([
      'Overview',
      'Images',
      'Networks',
      'Volumes',
      'Containers',
      'Compose',
      'Navigation',
      'AppStore',
      'AppDeploy',
      'Ports',
      'Cleanup',
      'Settings',
      'ScheduledTasks', 'Tutorials', 'TutorialDetail', 'NasStore', 'NasTopic', 'NasReview'
    ]))
    expect(COMMUNITY_ROUTE_PATHS).toEqual(expect.arrayContaining([
      '/images',
      '/compose',
      '/appstore',
      '/settings', '/tutorials', '/nas-store', '/nas-store/topic/:slug', '/nas-store/review/:id'
    ]))
  })

  it('does not retain commercial or remote entry points', () => {
    expect(COMMUNITY_ROUTE_NAMES).not.toEqual(expect.arrayContaining([
      'AIAgent',
      'GitHubApps',
      'Protection',
    ]))
    expect(COMMUNITY_ROUTE_PATHS).not.toEqual(expect.arrayContaining([
      '/ai-agent',
      '/github-apps',
      '/protection',
    ]))
  })

})

describeCommunity('community-only catalogs', () => {
  it('uses the shared settings catalog without Full-only sections', () => {
    const sectionKeys = settingSections.map(section => section.key)
    expect(sectionKeys).not.toEqual(expect.arrayContaining([
      'license', 'remoteNodes', 'github'
    ]))
    expect(sectionKeys).toEqual(expect.arrayContaining([
      'appearance', 'security', 'nasDefaults', 'notifications', 'backup', 'ai'
    ]))
  })

  it('uses the shared global search catalog without Full-only destinations', () => {
    const paths = GLOBAL_SEARCH_PAGES.map(item => item.route.path)
    const actionPaths = GLOBAL_SEARCH_ACTIONS.map(item => item.route.path)
    expect(paths).not.toEqual(expect.arrayContaining([
      '/ai-agent', '/github-apps', '/protection'
    ]))
    expect(actionPaths).not.toEqual(expect.arrayContaining([
      '/ai-agent', '/github-apps', '/protection'
    ]))
    expect(paths).toEqual(expect.arrayContaining([
      '/', '/containers', '/compose', '/appstore', '/settings', '/tutorials', '/nas-store'
    ]))
  })

  it('uses the shared sidebar renderer with only local destinations', () => {
    const paths = sidebarCatalog.flatMap(section => section.items.map(item => item.path))
    expect(paths).toEqual(expect.arrayContaining([
      '/', '/navigation', '/appstore', '/compose', '/containers', '/images', '/volumes', '/networks', '/tutorials', '/nas-store'
    ]))
    expect(paths).not.toEqual(expect.arrayContaining([
      '/ai-agent', '/github-apps', '/protection'
    ]))
  })
})

describeCommunity('community request boundary', () => {
  afterEach(() => {
    localStorage.clear()
    vi.unstubAllGlobals()
  })

  it('never turns a stale remote selection into a request header or response event', async () => {
    localStorage.setItem('tradis_current_environment', 'remote-office')
    const listener = vi.fn()
    window.addEventListener('tradis:remote-response', listener)
    const fetchMock = vi.fn().mockResolvedValue(new Response('[]', {
      status: 200,
      headers: { 'X-Tradis-Remote': 'true' }
    }))
    vi.stubGlobal('fetch', fetchMock)

    await get('/containers')

    expect(fetchMock.mock.calls[0][1].headers).not.toHaveProperty('X-TRADIS-Environment')
    expect(listener).not.toHaveBeenCalled()
    window.removeEventListener('tradis:remote-response', listener)
  })

  it('keeps task event URLs on the local control plane', () => {
    localStorage.setItem('tradis_current_environment', 'remote-office')

    expect(getComposeTaskEventsUrl('compose-task-1')).toContain('environmentId=local')
    expect(getDeployTaskEventsUrl('appstore-task-1')).toContain('environmentId=local')
    expect(getDeployTaskEventsUrl('appstore-task-1', 'remote-office')).toContain('environmentId=local')
  })
})
