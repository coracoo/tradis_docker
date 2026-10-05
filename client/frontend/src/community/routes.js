import { sharedChildRoutes } from '../edition/sharedRoutes.js'

export const editionChildRoutes = sharedChildRoutes
export const COMMUNITY_ROUTE_NAMES = editionChildRoutes.map(route => route.name)
export const COMMUNITY_ROUTE_PATHS = editionChildRoutes.map(route => route.path).filter(Boolean)
export const editionStandaloneRoutes = []
