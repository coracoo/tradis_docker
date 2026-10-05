import { get } from '../utils/request.js'

export const getTutorialManifest = (params = {}, options = {}) => get('/tutorials/manifest', params, options)
export const getTutorialArticle = (slug, params = {}, options = {}) => get(`/tutorials/${encodeURIComponent(slug)}`, params, options)
