import { buildApiUrl, get, post } from '../utils/request.js'

export const getCleanupEvaluation = ({ refresh = false } = {}) => (
  get('/cleanup/evaluation', refresh ? { refresh: 'true' } : {})
)

export const listCleanupTasks = (params = {}) => get('/cleanup/tasks', params)

export const getCleanupTask = id => get(`/cleanup/tasks/${encodeURIComponent(id)}`)

export const startCleanupTask = payload => post('/cleanup/tasks', payload)

export const getCleanupTaskEventsUrl = id => (
  buildApiUrl(`/cleanup/tasks/${encodeURIComponent(id)}/events`)
)

export default {
  getEvaluation: getCleanupEvaluation,
  listTasks: listCleanupTasks,
  getTask: getCleanupTask,
  startTask: startCleanupTask,
  getTaskEventsUrl: getCleanupTaskEventsUrl
}
