// 镜像管理 API
import { get, post, del, buildApiUrl } from '../utils/request.js'
import { resolveEnvironmentId } from '@edition/current-environment'
import { applyEnvironmentHeaders } from '@edition/request-environment'

let removalQueue = Promise.resolve()
const pendingRemovals = new Map()

/**
 * 获取镜像列表
 * @returns {Promise<Array>} 镜像列表
 */
export const listImages = (options = {}) => {
  return get('/images', {}, options)
}

/**
 * 删除镜像
 * @param {string} id - 镜像 ID
 * @param {string} [repoTag] - 镜像标签（可选）
 * @param {Object} [options] - { force, environmentId }
 * @returns {Promise}
 */
export const removeImage = (id, repoTag = '', options = {}) => {
  const environmentId = resolveEnvironmentId(options.environmentId)
  const key = JSON.stringify([environmentId, id, repoTag, Boolean(options.force)])
  if (pendingRemovals.has(key)) return pendingRemovals.get(key)
  const encodedId = encodeURIComponent(id)
  const params = new URLSearchParams()
  if (repoTag) params.set('repoTag', repoTag)
  if (options.force) params.set('force', 'true')
  const query = params.toString() ? `?${params.toString()}` : ''
  const path = `/images/${encodedId}${query}`
  const headers = applyEnvironmentHeaders(path, 'DELETE', {}, environmentId)
  // Capture the target before waiting: switching devices must not redirect a queued deletion.
  const removal = removalQueue.then(() => del(path, { headers }))
    .finally(() => pendingRemovals.delete(key))
  pendingRemovals.set(key, removal)
  removalQueue = removal.catch(() => {})
  return removal
}

/**
 * 获取镜像详情
 * @param {string} id - 镜像 ID
 * @returns {Promise}
 */
export const getImageDetail = (id) => {
  return get(`/images/inspect/${encodeURIComponent(id)}`)
}

/**
 * 获取镜像构建历史
 * @param {string} id - 镜像 ID
 * @returns {Promise}
 */
export const getImageHistory = (id) => {
  return get(`/images/history/${encodeURIComponent(id)}`)
}

/**
 * 拉取镜像
 * @param {Object} data - { name, registry }
 * @returns {Promise}
 */
export const pullImage = (data) => {
  return post('/images/pull', data)
}

/**
 * 创建后台镜像拉取任务
 * @param {Object} data - { name, registry, cleanupOldImage }
 * @returns {Promise<{taskId: string}>}
 */
export const startPullTask = (data) => {
  return post('/images/pull/tasks', data)
}

/**
 * 获取镜像拉取任务列表
 * @param {Object} params - { statuses, limit }
 * @returns {Promise<Array>}
 */
export const listPullTasks = (params = {}) => {
  return get('/images/pull/tasks', params)
}

/**
 * 获取镜像拉取任务详情
 * @param {string} taskId
 * @returns {Promise}
 */
export const getPullTask = (taskId) => {
  return get(`/images/pull/tasks/${encodeURIComponent(taskId)}`)
}

/**
 * 获取后台镜像拉取任务 SSE URL
 * @param {string} taskId
 * @returns {string}
 */
export const getPullTaskEventsUrl = (taskId) => {
  return buildApiUrl(`/images/pull/tasks/${encodeURIComponent(taskId)}/events`)
}

/**
 * 获取拉取镜像进度 URL
 * @param {string} name - 镜像名称
 * @param {string} registry - 镜像仓库
 * @returns {string} SSE URL
 */
export const getPullProgressUrl = (name, registry) => {
  const params = new URLSearchParams()
  if (name) params.append('name', name)
  if (registry) params.append('registry', registry)
  return buildApiUrl(`/images/pull/progress?${params.toString()}`)
}

/**
 * 修改镜像标签
 * @param {Object} data - { id, repo, tag }
 * @returns {Promise}
 */
export const tagImage = (data) => {
  return post('/images/tag', data)
}

/**
 * 构建导出镜像的下载 URL
 * 大文件通过浏览器流式下载，认证统一使用登录时下发的 HttpOnly Cookie。
 * @param {string} id - 镜像 ID
 * @returns {string}
 */
export const getExportUrl = (id) => {
  const baseUrl = import.meta.env.VITE_API_BASE_URL || ''
  return `${baseUrl}/api/images/export/${encodeURIComponent(id)}`
}

/**
 * 导入镜像
 * @param {FormData} formData - 包含镜像文件的 FormData
 * @returns {Promise}
 */
export const importImage = (formData) => {
  return post('/images/import', formData, {
    timeout: 600000 // 10分钟超时
  })
}

/**
 * 清理未使用的镜像
 * @returns {Promise}
 */
export const pruneImages = () => {
  return post('/images/prune')
}

/**
 * 检查镜像更新
 * @param {Object} params - 查询参数
 * @returns {Promise}
 */
export const checkImageUpdates = (params = {}) => {
  return get('/images/updates', params)
}

/**
 * 获取镜像更新状态
 * @returns {Promise}
 */
export const getUpdateStatus = () => {
  return get('/images/updates/status')
}

/**
 * 清除镜像更新标记
 * @param {Object} data - { repoTag }
 * @returns {Promise}
 */
export const clearUpdate = (data) => {
  return post('/images/updates/clear', data)
}

/**
 * 应用所有镜像更新
 * @returns {Promise}
 */
export const applyUpdates = () => {
  return post('/images/updates/apply', {}, { timeout: 600000 })
}

/**
 * 获取 Docker 代理配置
 * @returns {Promise}
 */
export const getProxy = () => {
  return get('/images/proxy')
}

/**
 * 获取镜像注册表列表
 * @returns {Promise}
 */
export const getRegistries = () => {
  return get('/image-registry')
}

/**
 * 更新镜像注册表配置
 * @param {Object} data - { registries: {...} }
 * @returns {Promise}
 */
export const updateRegistries = (data) => {
  return post('/image-registry', data)
}

/**
 * 获取代理历史
 * @returns {Promise}
 */
export const getProxyHistory = () => {
  return get('/images/proxy/history')
}

/**
 * 更新 Docker 代理配置
 * @param {Object} data
 * @returns {Promise}
 */
export const updateProxy = (data) => {
  return post('/images/proxy', data)
}

export const getBuildCache = (params = {}) => {
  return get('/images/build-cache', params)
}

export const pruneBuildCache = (data = {}) => {
  return post('/images/build-cache/prune', data)
}

// 默认导出
export default {
  list: listImages,
  remove: removeImage,
  detail: getImageDetail,
  history: getImageHistory,
  pull: pullImage,
  startPullTask,
  listPullTasks,
  getPullTask,
  getPullTaskEventsUrl,
  getPullProgressUrl,
  tag: tagImage,
  getExportUrl,
  import: importImage,
  prune: pruneImages,
  checkUpdates: checkImageUpdates,
  getUpdateStatus,
  clearUpdate,
  applyUpdates,
  getProxy,
  getProxyHistory,
  updateProxy,
  getRegistries,
  updateRegistries,
  getBuildCache,
  pruneBuildCache
}
