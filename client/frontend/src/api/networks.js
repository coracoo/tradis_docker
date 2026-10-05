// 网络管理 API
import { get, post, put, del } from '../utils/request.js'

/**
 * 获取网络列表
 * @returns {Promise<Array>} 网络列表
 */
export const listNetworks = (params = {}, options = {}) => {
  return get('/networks', params, options)
}

/**
 * 获取网络详情
 * @param {string} id - 网络 ID
 * @returns {Promise}
 */
export const getNetwork = (id) => {
  return get(`/networks/${encodeURIComponent(id)}`)
}

/**
 * 创建网络
 * @param {Object} data - { name, driver, subnet, gateway, ... }
 * @returns {Promise}
 */
export const createNetwork = (data) => {
  return post('/networks', data)
}

/**
 * 更新网络
 * @param {string} id - 网络 ID
 * @param {Object} data - 更新数据
 * @returns {Promise}
 */
export const updateNetwork = (id, data) => {
  return put(`/networks/${encodeURIComponent(id)}`, data)
}

/**
 * 删除网络
 * @param {string} id - 网络 ID
 * @returns {Promise}
 */
export const removeNetwork = (id) => {
  return del(`/networks/${encodeURIComponent(id)}`)
}

/**
 * 清理未使用的网络
 * @returns {Promise}
 */
export const pruneNetworks = () => {
  return post('/networks/prune')
}

// 默认导出
export default {
  list: listNetworks,
  get: getNetwork,
  create: createNetwork,
  update: updateNetwork,
  remove: removeNetwork,
  prune: pruneNetworks
}
