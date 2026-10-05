// 容器管理 API
import { buildApiUrl, get, post, del } from '../utils/request.js'

/**
 * 获取容器列表
 * @param {Object} params - 查询参数 { all: true }
 * @param {Object} [options] - 额外请求选项，如 { signal } 用于取消请求
 * @returns {Promise<Array>}
 */
export const listContainers = (params = { all: true }, options = {}) => {
  return get('/containers', params, options)
}

/**
 * 获取容器详情
 * @param {string} id - 容器 ID
 * @param {Object} [options] - 额外请求选项，如 { signal } 用于取消请求
 * @returns {Promise}
 */
export const getContainer = (id, options = {}) => {
  return get(`/containers/${encodeURIComponent(id)}`, {}, options)
}

/**
 * 创建容器
 * @param {Object} data
 * @returns {Promise}
 */
export const createContainer = (data) => {
  return post('/containers/create', data)
}

/**
 * 启动容器
 * @param {string} id - 容器 ID
 * @returns {Promise}
 */
export const startContainer = (id) => {
  return post(`/containers/${encodeURIComponent(id)}/start`)
}

/**
 * 停止容器
 * @param {string} id - 容器 ID
 * @returns {Promise}
 */
export const stopContainer = (id) => {
  return post(`/containers/${encodeURIComponent(id)}/stop`)
}

/**
 * 强制停止容器（立即发送 SIGKILL，不做优雅停止，不修改重启策略）
 * @param {string} id - 容器 ID
 * @returns {Promise}
 */
export const forceStopContainer = (id) => {
  return post(`/containers/${encodeURIComponent(id)}/kill`)
}

/**
 * 重启容器
 * @param {string} id - 容器 ID
 * @returns {Promise}
 */
export const restartContainer = (id) => {
  return post(`/containers/${encodeURIComponent(id)}/restart`)
}

/**
 * 删除容器
 * @param {string} id - 容器 ID
 * @param {Object} params - { force: boolean, v: boolean }
 * @returns {Promise}
 */
export const removeContainer = (id, params = {}) => {
  const query = new URLSearchParams(params).toString()
  const queryStr = query ? `?${query}` : ''
  return del(`/containers/${encodeURIComponent(id)}${queryStr}`)
}

/**
 * 获取容器日志 URL
 * @param {string} id - 容器 ID
 * @param {Object} params - { tail, since, timestamps }
 * @returns {string}
 */
export const getContainerLogsUrl = (id, params = {}) => {
  const query = new URLSearchParams(params).toString()
  const queryStr = query ? `&${query}` : ''
  return `/api/containers/${encodeURIComponent(id)}/logs?stream=true${queryStr}`
}

/**
 * 获取容器状态
 * @param {string} id - 容器 ID
 * @returns {Promise}
 */
export const getContainerStats = (id) => {
  return get(`/containers/${encodeURIComponent(id)}/stats`)
}

/**
 * 清理已停止的容器
 * @returns {Promise}
 */
export const pruneContainers = () => {
  return post('/containers/prune')
}

/**
 * 使用容器当前镜像引用重新创建容器，并通过 SSE 返回操作日志。
 * pull=false 时直接使用本地标签，适用于镜像已单独更新的场景。
 */
export const recreateContainerStream = (id, options = {}) => {
  const query = new URLSearchParams({
    pull: String(options.pull === true)
  })
  const url = buildApiUrl(`/containers/${encodeURIComponent(id)}/update/events?${query.toString()}`)
  const eventSource = new EventSource(url)
  let settled = false

  const close = () => {
    eventSource.close()
  }

  const handleLog = (event) => {
    if (settled) return
    const line = String(event?.data || '').trim()
    if (!line) return

    options.onLog?.(line)

    if (line.startsWith('error:')) {
      settled = true
      close()
      options.onError?.(new Error(line.slice(6).trim() || '重置容器失败'))
      return
    }

    if (line.startsWith('success:')) {
      settled = true
      close()
      options.onComplete?.(line.slice(8).trim())
    }
  }

  eventSource.onmessage = handleLog
  eventSource.addEventListener('log', handleLog)
  eventSource.onerror = () => {
    if (settled) return
    settled = true
    close()
    options.onError?.(new Error('容器重置连接中断'))
  }

  return close
}

/**
 * 在后台拉取镜像并重建容器。
 */
export const startContainerUpdateTask = (id, options = {}) => {
  const query = new URLSearchParams({
    pull: String(options.pull !== false)
  })
  return post(`/containers/${encodeURIComponent(id)}/update/tasks?${query.toString()}`)
}

export const getContainerUpdateTask = (taskId) => {
  return get(`/containers/tasks/${encodeURIComponent(taskId)}`)
}

export const getContainerUpdateTaskEventsUrl = (taskId) => {
  return buildApiUrl(`/containers/tasks/${encodeURIComponent(taskId)}/events`)
}

// 默认导出
export default {
  list: listContainers,
  get: getContainer,
  create: createContainer,
  start: startContainer,
  stop: stopContainer,
  forceStop: forceStopContainer,
  restart: restartContainer,
  remove: removeContainer,
  prune: pruneContainers,
  recreateStream: recreateContainerStream,
  startUpdateTask: startContainerUpdateTask,
  getUpdateTask: getContainerUpdateTask,
  getUpdateTaskEventsUrl: getContainerUpdateTaskEventsUrl,
  getLogsUrl: getContainerLogsUrl,
  getStats: getContainerStats
}
