// 卷管理 API
import { get, post, del } from '../utils/request.js'

/**
 * 获取卷列表
 * @returns {Promise<Array>} 卷列表
 */
export const listVolumes = (options = {}) => {
  return get('/volumes', {}, options)
}

/**
 * 创建卷
 * @param {Object} data - { name, driver, driverOpts }
 * @returns {Promise}
 */
export const createVolume = (data) => {
  return post('/volumes', data)
}

/**
 * 删除卷
 * @param {string} name - 卷名称
 * @param {Object} options - 删除选项
 * @returns {Promise}
 */
export const removeVolume = (name, options = {}) => {
  const query = options.force ? '?force=true' : ''
  return del(`/volumes/${encodeURIComponent(name)}${query}`)
}

/**
 * 清理未使用的卷
 * @returns {Promise}
 */
export const pruneVolumes = () => {
  return post('/volumes/prune')
}

/**
 * 启动卷文件浏览器
 * @param {string} name - 卷名称
 * @returns {Promise<{sessionId: string, url: string, readOnly: boolean}>} 会话信息
 */
export const browseVolumeStart = (name) => {
  return post(`/volumes/${encodeURIComponent(name)}/browse/start`)
}

/**
 * 关闭卷文件浏览器会话
 * @param {string} sessionId - 浏览会话 ID
 * @returns {Promise}
 */
export const browseVolumeClose = (sessionId) => {
  return post(`/volumes/browse/${encodeURIComponent(sessionId)}/close`)
}

// 默认导出
export default {
  list: listVolumes,
  create: createVolume,
  remove: removeVolume,
  prune: pruneVolumes,
  browseStart: browseVolumeStart,
  browseClose: browseVolumeClose
}
