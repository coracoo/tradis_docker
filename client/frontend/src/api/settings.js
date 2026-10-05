// 系统设置 API
import { get, post, put } from '../utils/request.js'
import { deploymentDefaultsPath } from '@edition/deployment-defaults'

/**
 * 获取系统设置
 * @returns {Promise}
 */
export const getSettings = () => {
  return get('/settings')
}

/**
 * 保存系统设置
 * @param {Object} data - 设置数据
 * @returns {Promise}
 */
export const saveSettings = (data) => {
  return post('/settings', data)
}

/**
 * 获取全局设置
 * @returns {Promise}
 */
export const getGlobalSettings = () => {
  return get('/settings/global')
}

/**
 * 保存全局设置
 * @param {Object} data - 全局设置数据
 * @returns {Promise}
 */
export const saveGlobalSettings = (data) => {
  return post('/settings/global', data)
}

export const getKVSetting = (key) => {
  return get(`/settings/kv/${encodeURIComponent(key)}`)
}

export const setKVSetting = (key, value) => {
  return post(`/settings/kv/${encodeURIComponent(key)}`, { value })
}

export const getDeploymentDefaults = () => {
  return get(deploymentDefaultsPath)
}

export const updateDeploymentDefaults = (profile) => {
  return put(deploymentDefaultsPath, profile)
}

// 默认导出
export default {
  get: getSettings,
  save: saveSettings,
  getGlobal: getGlobalSettings,
  saveGlobal: saveGlobalSettings,
  getKVSetting,
  setKVSetting,
  getDeploymentDefaults,
  updateDeploymentDefaults
}
