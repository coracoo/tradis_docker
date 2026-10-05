// 应用商店 API
import { get, post, buildApiUrl } from '../utils/request.js'
import { resolveEnvironmentId } from '@edition/current-environment'

const APPSTORE_MODE_LABELS = {
  'cdn+origin': 'CDN + 源站',
  cdn: '仅 CDN',
  origin: '仅源站',
  cache: '仅缓存',
  unconfigured: '未配置'
}

const APPSTORE_STATE_PRESETS = {
  ready: {
    level: 'success',
    label: '可用',
    connected: true,
    summary: '应用商店连接正常'
  },
  degraded: {
    level: 'warning',
    label: '降级',
    connected: true,
    summary: '应用商店处于降级模式'
  },
  cached: {
    level: 'warning',
    label: '仅缓存',
    connected: true,
    summary: '应用商店当前仅可使用本地缓存'
  },
  offline: {
    level: 'error',
    label: '不可用',
    connected: false,
    summary: '应用商店未配置或不可访问'
  }
}

export const normalizeAppStoreStatus = (raw = {}) => {
  const state = typeof raw.state === 'string' && APPSTORE_STATE_PRESETS[raw.state]
    ? raw.state
    : (raw.connected ? 'ready' : 'offline')
  const preset = APPSTORE_STATE_PRESETS[state]
  const mode = typeof raw.mode === 'string' && raw.mode
    ? raw.mode
    : 'unconfigured'
  const cache = raw && typeof raw.cache === 'object' && raw.cache ? raw.cache : {}

  return {
    state,
    level: typeof raw.level === 'string' && raw.level ? raw.level : preset.level,
    label: typeof raw.label === 'string' && raw.label ? raw.label : preset.label,
    connected: typeof raw.connected === 'boolean' ? raw.connected : preset.connected,
    summary: typeof raw.summary === 'string' && raw.summary ? raw.summary : preset.summary,
    mode,
    modeLabel: APPSTORE_MODE_LABELS[mode] || mode,
    readConfigured: !!raw.readConfigured,
    writeConfigured: !!raw.writeConfigured,
    cdnConfigured: !!raw.cdnConfigured,
    originConfigured: !!raw.originConfigured,
    cache: {
      hasListCache: !!cache.hasListCache,
      listAppCount: Number(cache.listAppCount || 0),
      listFetchedAt: cache.listFetchedAt || '',
      cachedDetailFileCount: Number(cache.cachedDetailFileCount || 0)
    }
  }
}

/**
 * 获取应用列表
 * @param {Object} params - 查询参数 { category, keyword, page, pageSize }
 * @returns {Promise}
 */
export const getApps = (params = {}, options = {}) => {
  return get('/appstore/apps', params, options)
}

export const getAppsMeta = (params = {}, options = {}) => {
  return get('/appstore/meta', params, options)
}

export const getAppsMetaStatus = (params = {}, options = {}) => {
  return get('/appstore/meta/status', params, options)
}

/**
 * 获取应用详情
 * @param {string|number} id - 应用 ID
 * @returns {Promise}
 */
export const getAppDetail = (id) => {
  return get(`/appstore/apps/${id}`)
}

/**
 * 获取应用变量
 * @param {string|number} id - 应用 ID
 * @returns {Promise}
 */
export const getAppVars = (id) => {
  return get(`/appstore/apps/${id}/vars`)
}

export const parseAppVars = (data) => {
  return post('/appstore/parse-vars', data)
}

/**
 * 部署应用
 * @param {Object} data - { projectId, ...vars }
 * @returns {Promise}
 */
export const deployApp = (id, data) => {
  return post(`/appstore/deploy/${id}`, data)
}

export const preflightDeployApp = (id, data) => {
  return post(`/appstore/deploy/${id}/preflight`, data)
}

/**
 * 提交部署次数统计
 * @param {string|number} id - 应用 ID
 * @returns {Promise}
 */
/**
 * 获取应用部署状态
 * @param {string|number} id - 应用 ID
 * @returns {Promise}
 */
export const getAppStatus = (id) => {
  return get(`/appstore/status/${id}`)
}

export const getStatus = () => {
  return get('/appstore/status').then(normalizeAppStoreStatus)
}

/**
 * 获取应用商店部署任务列表
 * @param {Object} params - { statuses, limit }
 * @returns {Promise<Array>}
 */
export const listDeployTasks = (params = {}) => {
  const environmentId = resolveEnvironmentId(params.environmentId)
  return get('/appstore/tasks', { ...params, environmentId })
}

/**
 * 获取应用商店部署任务详情
 * @param {string} id
 * @returns {Promise}
 */
export const getDeployTask = (id, environmentId = '') => {
  return get(`/appstore/tasks/${encodeURIComponent(id)}`, { environmentId: resolveEnvironmentId(environmentId) })
}

/**
 * 获取应用商店部署任务 SSE URL
 * @param {string} id
 * @returns {string}
 */
export const getDeployTaskEventsUrl = (id, environmentId = '') => {
  const targetEnvironmentId = resolveEnvironmentId(environmentId)
  return buildApiUrl(`/appstore/tasks/${encodeURIComponent(id)}/events?environmentId=${encodeURIComponent(targetEnvironmentId)}`)
}

/**
 * 取消应用商店部署任务
 * @param {string} id
 * @returns {Promise}
 */
export const cancelDeployTask = (id, environmentId = '') => {
  const query = `?environmentId=${encodeURIComponent(resolveEnvironmentId(environmentId))}`
  return post(`/appstore/tasks/${encodeURIComponent(id)}/cancel${query}`)
}

// 默认导出
export default {
  list: getApps,
  get: getAppDetail,
  getVars: getAppVars,
  parseVars: parseAppVars,
  deploy: deployApp,
  preflightDeploy: preflightDeployApp,
  getStatus,
  normalizeAppStoreStatus,
  getAppStatus,
  listDeployTasks,
  getDeployTask,
  getDeployTaskEventsUrl,
  cancelDeployTask
}
