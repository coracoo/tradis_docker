// 定时任务系统 API
import { get, post, put, del } from '../utils/request.js'

/**
 * 获取定时任务列表
 * @returns {Promise<Array>}
 */
export const listScheduledTasks = () => {
  return get('/scheduled-tasks')
}

/**
 * 创建定时任务
 * @param {Object} data - { name, task_type, cron_expr, target, payload_json, enabled }
 * @returns {Promise}
 */
export const createScheduledTask = (data) => {
  return post('/scheduled-tasks', data)
}

/**
 * 更新定时任务
 * @param {number} id - 任务 ID
 * @param {Object} data - 更新数据
 * @returns {Promise}
 */
export const updateScheduledTask = (id, data) => {
  return put(`/scheduled-tasks/${id}`, data)
}

/**
 * 删除定时任务
 * @param {number} id - 任务 ID
 * @returns {Promise}
 */
export const deleteScheduledTask = (id) => {
  return del(`/scheduled-tasks/${id}`)
}

/**
 * 立即执行一次定时任务（手动触发）
 * @param {number} id - 任务 ID
 * @returns {Promise}
 */
export const runScheduledTaskNow = (id) => {
  return post(`/scheduled-tasks/${id}/run`)
}

/**
 * 查询某个任务的执行历史
 * @param {number} id - 任务 ID
 * @param {Object} [params] - { limit }
 * @returns {Promise}
 */
export const listScheduledTaskRuns = (id, params = {}) => {
  return get(`/scheduled-tasks/${id}/runs`, params)
}

/**
 * 预览 cron 表达式的下次触发时间（实时校验用）
 * @param {string} expr - cron 表达式
 * @returns {Promise}
 */
export const previewCronExpr = (expr) => {
  return get('/scheduled-tasks/cron-preview', { expr })
}

export default {
  list: listScheduledTasks,
  create: createScheduledTask,
  update: updateScheduledTask,
  remove: deleteScheduledTask,
  runNow: runScheduledTaskNow,
  runs: listScheduledTaskRuns,
  previewCron: previewCronExpr
}
