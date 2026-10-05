// 导航管理 API
import { get, post, put, del } from '../utils/request.js'

/**
 * 获取应用列表
 * @param {Object} params - 查询参数 { includeDeleted }
 * @returns {Promise}
 */
export const listNavigation = (params = {}) => {
  return get('/navigation', params)
}

/**
 * 添加应用
 * @param {Object} data - { name, category, icon, lanUrl, wanUrl }
 * @returns {Promise}
 */
export const addNavigation = (data) => {
  return post('/navigation', data)
}

/**
 * 更新应用
 * @param {string|number} id - 应用 ID
 * @param {Object} data - 更新数据
 * @returns {Promise}
 */
export const updateNavigation = (id, data) => {
  return put(`/navigation/${id}`, data)
}

/**
 * 删除应用（软删除）
 * @param {string|number} id - 应用 ID
 * @returns {Promise}
 */
export const deleteNavigation = (id) => {
  return del(`/navigation/${id}`)
}

/**
 * 彻底删除应用
 * @param {string|number} id - 应用 ID
 * @returns {Promise}
 */
export const purgeNavigation = (id) => {
  return del(`/navigation/${id}?permanent=true`)
}

/**
 * 恢复应用
 * @param {string|number} id - 应用 ID
 * @returns {Promise}
 */
export const restoreNavigation = (id) => {
  return post(`/navigation/${id}/restore`)
}

/**
 * 上传图标
 * @param {string|number} id - 应用 ID
 * @param {File} file - 图标文件
 * @returns {Promise}
 */
export const uploadIcon = (id, file) => {
  const formData = new FormData()
  formData.append('file', file)
  return post(`/navigation/${id}/icon`, formData)
}

// 默认导出
export default {
  list: listNavigation,
  add: addNavigation,
  update: updateNavigation,
  delete: deleteNavigation,
  purge: purgeNavigation,
  restore: restoreNavigation,
  uploadIcon
}
