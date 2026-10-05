// 端口管理 API
import { get, post } from '../utils/request.js'

/**
 * 获取端口列表
 * @param {Object} params - 查询参数 { protocol, status, keyword }
 * @returns {Promise}
 */
export const listPorts = (params = {}) => {
  return get('/ports', params)
}

/**
 * 获取端口范围配置
 * @returns {Promise}
 */
export const getPortRange = () => {
  return get('/ports/range')
}

/**
 * 更新端口范围配置
 * @param {Object} data - { start, end }
 * @returns {Promise}
 */
export const updatePortRange = (data) => {
  return post('/ports/range', data)
}

/**
 * 保存端口备注
 * @param {Object} data - { port, protocol, note }
 * @returns {Promise}
 */
export const savePortNote = (data) => {
  return post('/ports/note', data)
}

/**
 * 分配端口
 * @param {Object} data - { count, protocol }
 * @returns {Promise}
 */
export const allocatePort = (data) => {
  return post('/ports/allocate', data)
}

export const releasePort = (data) => {
  return post('/ports/release', data)
}

// 默认导出
export default {
  list: listPorts,
  getRange: getPortRange,
  updateRange: updatePortRange,
  saveNote: savePortNote,
  allocate: allocatePort,
  release: releasePort
}
