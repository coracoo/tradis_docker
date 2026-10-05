// 系统信息 API
import { get, post, put, del } from '../utils/request.js'

let systemStatsRequest = null

/**
 * 获取系统信息
 * @returns {Promise}
 */
export const getSystemInfo = () => {
  return get('/system/info')
}

/**
 * 获取系统统计
 * @returns {Promise}
 */
export const getSystemStats = () => {
  if (systemStatsRequest) {
    return systemStatsRequest
  }

  const request = get('/system/stats').finally(() => {
    if (systemStatsRequest === request) {
      systemStatsRequest = null
    }
  })
  systemStatsRequest = request
  return request
}

/**
 * 获取 Docker 磁盘使用情况
 * @returns {Promise}
 */
export const getDiskUsage = () => {
  return get('/system/disk-usage')
}

/**
 * 获取系统事件
 * @returns {Promise}
 */
export const getSystemEvents = () => {
  return get('/system/events')
}

export const exportDiagnostics = () => {
  return get('/system/diagnostics/export', {}, { responseType: 'blob' })
}

export const getCDNStatus = () => {
  return get('/system/cdn/status')
}

export const refreshCDNSpeedTest = () => {
  return post('/system/cdn/retest')
}

/**
 * 添加系统通知
 * @param {Object} data - { type, message }
 * @returns {Promise}
 */
export const addNotification = (data) => {
  return post('/system/notifications', data)
}

/**
 * 获取系统通知
 * @param {Object} params - 查询参数
 * @returns {Promise}
 */
export const getNotifications = (params = {}) => {
  return get('/system/notifications', params)
}

export const getNotificationSummary = () => {
  return get('/system/notifications/summary')
}

/**
 * 删除通知
 * @param {string|number} id - 通知 ID
 * @returns {Promise}
 */
export const deleteNotification = (id) => {
  return del(`/system/notifications/${id}`)
}

/**
 * 清空所有通知
 * @returns {Promise}
 */
export const clearAllNotifications = () => {
  return del('/system/notifications')
}

/**
 * 标记所有通知为已读
 * @returns {Promise}
 */
export const markNotificationsRead = () => {
  return post('/system/notifications/read')
}

/**
 * 重建导航
 * @returns {Promise}
 */
export const rebuildNavigation = () => {
  return post('/system/navigation/rebuild')
}

/**
 * 重建卷备份容器
 * @returns {Promise}
 */
export const rebuildVolumeBackup = () => {
  return post('/system/volume-backup/rebuild')
}

export const getNotificationChannels = () => get('/notification-channels')
export const createNotificationChannel = (data) => post('/notification-channels', data)
export const updateNotificationChannel = (id, data) => put(`/notification-channels/${encodeURIComponent(id)}`, data)
export const deleteNotificationChannel = (id) => del(`/notification-channels/${encodeURIComponent(id)}`)
export const testNotificationChannel = (id) => post(`/notification-channels/${encodeURIComponent(id)}/test`)
export const getNotificationDeliveries = (id, params = {}) => get(`/notification-channels/${encodeURIComponent(id)}/deliveries`, params)

// 默认导出
export default {
  getInfo: getSystemInfo,
  getStats: getSystemStats,
  getDiskUsage,
  getEvents: getSystemEvents,
  exportDiagnostics,
  getCDNStatus,
  refreshCDNSpeedTest,
  addNotification,
  getNotifications,
  getNotificationSummary,
  deleteNotification,
  clearAllNotifications,
  markNotificationsRead,
  rebuildNavigation,
  rebuildVolumeBackup,
  getNotificationChannels,
  createNotificationChannel,
  updateNotificationChannel,
  deleteNotificationChannel,
  testNotificationChannel,
  getNotificationDeliveries
}
