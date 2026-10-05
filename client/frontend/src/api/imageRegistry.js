// 镜像仓库 API
import { get, post } from '../utils/request.js'

/**
 * 获取镜像仓库列表
 * @returns {Promise}
 */
export const getRegistries = () => {
  return get('/image-registry')
}

/**
 * 更新镜像仓库配置
 * @param {Object} data - { registries: [...] }
 * @returns {Promise}
 */
export const updateRegistries = (data) => {
  return post('/image-registry', data)
}

// 默认导出
export default {
  get: getRegistries,
  update: updateRegistries
}
