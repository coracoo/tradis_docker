// Compose 管理 API
import { get, post, put, del, buildApiUrl } from '../utils/request.js'
import { currentEnvironmentId } from '@edition/current-environment'

/**
 * 获取 Compose 项目列表
 * @param {Object} params - 查询参数 { refresh?: '1', force?: '1' }
 * @returns {Promise<Array>} 项目列表
 */
export const listCompose = (params = {}, options = {}) => {
  return get('/compose/list', params, options)
}

export const updateComposeMetadata = (name, data) => {
  return put(`/compose/projects/${encodeURIComponent(name)}/metadata`, data)
}

export const listContainerRemarks = (options = {}) => {
  return get('/compose/container-remarks', {}, options)
}

export const updateContainerRemark = (name, data) => {
  return put(`/compose/containers/${encodeURIComponent(name)}/remark`, data)
}

/**
 * 部署 Compose 项目
 * @param {Object} data - { name, path, yaml, env }
 * @returns {Promise}
 */
export const deployCompose = (data) => {
  return post('/compose/deploy', data)
}

/**
 * 部署 Compose 项目（带任务ID返回）
 * @param {Object} data - { name, compose, dotenv, env, autoStart }
 * @returns {Promise}
 */
export const deployComposeTask = (data) => {
  return post('/compose/deploy', data)
}

export const preflightComposeDeployment = (data) => post('/compose/deploy/preflight', data)

/**
 * 从公开 GitHub 仓库下载 Compose 项目，不自动部署
 * @param {Object} data - { repoUrl, name, branch, acceleratorUrl, overwrite }
 */
export const importComposeFromGit = (data) => {
  return post('/compose/import-git', data)
}

export const syncComposeFromGit = (name) => {
  return post(`/compose/${encodeURIComponent(name)}/sync-git`)
}

/**
 * 获取部署任务列表
 * @param {Object} params - 查询参数
 * @returns {Promise}
 */
export const listComposeTasks = (params = {}) => {
  return get('/compose/tasks', params)
}

/**
 * 取消一个正在运行的任务（部署等）
 * @param {string} id - 任务 ID
 * @returns {Promise}
 */
export const cancelComposeTask = (id) => {
  return post(`/compose/tasks/${encodeURIComponent(id)}/cancel`)
}

/**
 * 获取部署任务详情
 * @param {string} id - 任务 ID
 * @returns {Promise}
 */
export const getComposeTask = (id) => {
  return get(`/compose/tasks/${id}`)
}

/**
 * 获取 Compose 任务 SSE URL
 * @param {string} id - 任务 ID
 * @returns {string}
 */
export const getComposeTaskEventsUrl = (id) => {
  const environmentId = currentEnvironmentId()
  return buildApiUrl(`/compose/tasks/${encodeURIComponent(id)}/events?environmentId=${encodeURIComponent(environmentId)}`)
}

const composeActionQuery = (params = {}) => {
  const query = new URLSearchParams()
  if (params.forcePull || params.pull) {
    query.append('pull', 'true')
  }
  if (params.rebuild || params.noCache || params.forceRebuild) {
    query.append('rebuild', 'true')
  }
  const queryString = query.toString()
  return queryString ? `?${queryString}` : ''
}

/**
 * 启动 Compose 项目
 * @param {string} name - 项目名称
 * @returns {Promise}
 */
export const startCompose = (name) => {
  return post(`/compose/${encodeURIComponent(name)}/start`)
}

export const startComposeTask = (name) => {
  return post(`/compose/${encodeURIComponent(name)}/start/tasks`)
}

/**
 * 停止 Compose 项目
 * @param {string} name - 项目名称
 * @returns {Promise}
 */
export const stopCompose = (name) => {
  return post(`/compose/${encodeURIComponent(name)}/stop`)
}

export const stopComposeTask = (name) => {
  return post(`/compose/${encodeURIComponent(name)}/stop/tasks`)
}

/**
 * 强制停止 Compose 项目（对项目容器逐个发送 SIGKILL，走后台任务）
 * @param {string} name - 项目名称
 * @returns {Promise} 任务响应 { taskId, project, type }
 */
export const forceStopCompose = (name) => {
  return post(`/compose/${encodeURIComponent(name)}/kill/tasks`)
}

/**
 * 重启 Compose 项目
 * @param {string} name - 项目名称
 * @returns {Promise}
 */
export const restartCompose = (name) => {
  return post(`/compose/${encodeURIComponent(name)}/restart`)
}

export const restartComposeTask = (name) => {
  return post(`/compose/${encodeURIComponent(name)}/restart/tasks`)
}

/**
 * 构建 Compose 项目
 * @param {string} name - 项目名称
 * @param {Object} params - 参数 { pull: boolean }
 * @returns {Promise}
 */
export const buildCompose = (name, params = {}) => {
  const query = composeActionQuery(params)
  return post(`/compose/${encodeURIComponent(name)}/build${query}`)
}

export const buildComposeTask = (name, params = {}) => {
  const query = composeActionQuery(params)
  return post(`/compose/${encodeURIComponent(name)}/build/tasks${query}`)
}

export const updateComposeTask = (name, params = {}) => {
  const query = composeActionQuery(params)
  return post(`/compose/${encodeURIComponent(name)}/update/tasks${query}`)
}

/**
 * 更新 Compose 项目 SSE 流（拉取最新镜像并 create，不自动启动）
 * 使用和老版本一样的 /update/events 端点
 * @param {string} name - 项目名称
 * @param {Object} params - 参数 { forcePull: boolean, onLog: function, onError: function }
 * @returns {Function} 停止监听的函数
 */
export const updateComposeStream = (name, params = {}) => {
  const url = buildApiUrl(`/compose/${encodeURIComponent(name)}/update/events`)

  // 构建 query 参数
  const queryParams = new URLSearchParams()
  if (params.forcePull || params.pull) {
    queryParams.append('pull', 'true')
  }
  if (params.rebuild || params.noCache || params.forceRebuild) {
    queryParams.append('rebuild', 'true')
  }

  // SSE 通过 Cookie 认证，不再在 URL 中传递 token
  const queryString = queryParams.toString()
  const urlWithParams = queryString ? `${url}?${queryString}` : url

  const eventSource = new EventSource(urlWithParams)
  let hasReceivedMessage = false
  let hasError = false
  
  eventSource.onopen = () => {
    console.log('[updateComposeStream] SSE 连接已打开')
  }
  
  eventSource.onmessage = (event) => {
    hasReceivedMessage = true
    if (params.onLog && event.data) {
      params.onLog(event.data)
    }
  }
  
  // 后端使用 'log' 事件类型
  eventSource.addEventListener('log', (event) => {
    hasReceivedMessage = true
    if (params.onLog && event.data) {
      params.onLog(event.data)
    }
  })
  
  eventSource.onerror = () => {
    // 如果已经收到过消息，说明是正常完成后关闭，不是错误
    if (!hasReceivedMessage && params.onError) {
      params.onError(new Error('SSE 连接失败'))
    }
    // 正常完成时不触发 onError 回调
  }
  
  return () => {
    eventSource.close()
  }
}

/**
 * 删除 Compose 项目
 * @param {string} name - 项目名称
 * @returns {Promise}
 */
export const removeCompose = (name) => {
  return del(`/compose/remove/${encodeURIComponent(name)}`)
}

export const removeComposeTask = (name) => {
  return post(`/compose/${encodeURIComponent(name)}/remove/tasks`)
}

export const getComposeDestroyPreview = (name) => {
  return get(`/compose/${encodeURIComponent(name)}/destroy-preview`)
}

export const destroyComposeTask = (name, fingerprint, selected) => {
  return post(`/compose/${encodeURIComponent(name)}/destroy/tasks`, { fingerprint, selected })
}

/**
 * 清理 Compose 项目 (down)
 * @param {string} name - 项目名称
 * @returns {Promise}
 */
export const downCompose = (name) => {
  return del(`/compose/${encodeURIComponent(name)}/down`)
}

export const downComposeTask = (name) => {
  return post(`/compose/${encodeURIComponent(name)}/down/tasks`)
}

/**
 * 获取 Compose 项目状态
 * @param {string} name - 项目名称
 * @returns {Promise}
 */
export const getComposeStatus = (name) => {
  return get(`/compose/${encodeURIComponent(name)}/status`)
}

/**
 * 获取 Compose YAML 配置
 * @param {string} name - 项目名称
 * @returns {Promise}
 */
export const getComposeYaml = (name) => {
  return get(`/compose/${encodeURIComponent(name)}/yaml`)
}

/**
 * 保存 Compose YAML 配置
 * @param {string} name - 项目名称
 * @param {string} content - YAML 内容
 * @returns {Promise}
 */
export const saveComposeYaml = (name, content) => {
  return post(`/compose/${encodeURIComponent(name)}/yaml`, { content })
}

/**
 * 获取 Compose 环境变量
 * @param {string} name - 项目名称
 * @returns {Promise}
 */
export const getComposeEnv = (name) => {
  return get(`/compose/${encodeURIComponent(name)}/env`)
}

/**
 * 保存 Compose 环境变量
 * @param {string} name - 项目名称
 * @param {string} content - 环境变量内容
 * @returns {Promise}
 */
export const saveComposeEnv = (name, content) => {
  return post(`/compose/${encodeURIComponent(name)}/env`, { content })
}

export const previewComposeConfig = (name, data) => {
  return post(`/compose/${encodeURIComponent(name)}/config/preview`, data)
}

export const applyComposeConfig = (name, data) => {
  return post(`/compose/${encodeURIComponent(name)}/config/apply`, data)
}

export const listComposeHistory = (name) => {
  return get(`/compose/${encodeURIComponent(name)}/config/history`)
}

export const previewComposeHistory = (name, historyId) => {
  return post(`/compose/${encodeURIComponent(name)}/config/history/${encodeURIComponent(historyId)}/preview`)
}

export const restoreComposeHistory = (name, historyId, data) => {
  return post(`/compose/${encodeURIComponent(name)}/config/history/${encodeURIComponent(historyId)}/restore`, data)
}

// 默认导出
export default {
  list: listCompose,
  updateMetadata: updateComposeMetadata,
  listContainerRemarks,
  updateContainerRemark,
  deploy: deployCompose,
  deployTask: deployComposeTask,
  preflightDeployment: preflightComposeDeployment,
  importGit: importComposeFromGit,
  syncGit: syncComposeFromGit,
  listTasks: listComposeTasks,
  cancelTask: cancelComposeTask,
  getTask: getComposeTask,
  getTaskEventsUrl: getComposeTaskEventsUrl,
  start: startCompose,
  startTask: startComposeTask,
  stop: stopCompose,
  stopTask: stopComposeTask,
  forceStop: forceStopCompose,
  restart: restartCompose,
  restartTask: restartComposeTask,
  build: buildCompose,
  buildTask: buildComposeTask,
  updateTask: updateComposeTask,
  updateStream: updateComposeStream,
  remove: removeCompose,
  removeTask: removeComposeTask,
  destroyPreview: getComposeDestroyPreview,
  destroyTask: destroyComposeTask,
  down: downCompose,
  downTask: downComposeTask,
  getStatus: getComposeStatus,
  getYaml: getComposeYaml,
  saveYaml: saveComposeYaml,
  getEnv: getComposeEnv,
  saveEnv: saveComposeEnv,
  previewConfig: previewComposeConfig,
  applyConfig: applyComposeConfig,
  listHistory: listComposeHistory,
  previewHistory: previewComposeHistory,
  restoreHistory: restoreComposeHistory
}
