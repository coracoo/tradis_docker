// 认证 API
import { get, post } from '../utils/request.js'

/**
 * 用户登录
 * @param {Object} data - { username, password }
 * @returns {Promise}
 */
export const login = (data) => {
  return post('/auth/login', data)
}

/**
 * 获取当前用户信息
 * @returns {Promise}
 */
export const getMe = () => {
  return get('/auth/me')
}

/**
 * 修改密码
 * @param {Object} data - { oldPassword, newPassword }
 * @returns {Promise}
 */
export const changePassword = (data) => {
  return post('/auth/change-password', data)
}

/**
 * 退出登录
 * @returns {Promise}
 */
export const logout = () => {
  return post('/auth/logout')
}

// 默认导出
export default {
  login,
  getMe,
  changePassword,
  logout
}
