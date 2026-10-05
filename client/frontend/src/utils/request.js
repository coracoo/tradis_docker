import { applyEnvironmentHeaders, publishEnvironmentResponse } from '@edition/request-environment'

// API 请求封装
// 支持通过环境变量配置API基础URL
const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || ''
const BASE_URL = API_BASE_URL ? `${API_BASE_URL}/api` : '/api'

// 受保护请求的 401 跳转去重；登录失败由登录页展示，不触发整页导航。
let isRedirectingTo401 = false
const DEFAULT_REQUEST_TIMEOUT_MS = 120_000

export class ApiError extends Error {
  constructor(message, { status = 0, code = '', retryable = false, requestId = '', details = null, payload = null } = {}) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.retryable = retryable
    this.requestId = requestId
    this.details = details
    this.payload = payload
  }
}

export const buildApiUrl = (path) => {
  const normalizedPath = path.startsWith('/') ? path : `/${path}`
  return `${BASE_URL}${normalizedPath}`
}

/**
 * 发送 HTTP 请求
 * @param {string} url - 请求路径
 * @param {object} options - 请求选项
 * @returns {Promise} - 返回响应数据
 */
async function request(url, options = {}) {
  const fullUrl = url.startsWith('http') ? url : `${BASE_URL}${url}`
  
  const config = {
    method: 'GET',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
    },
    ...options,
  }

  const requestedTimeout = Number(config.timeout)
  const timeoutMs = Number.isFinite(requestedTimeout) ? requestedTimeout : DEFAULT_REQUEST_TIMEOUT_MS
  delete config.timeout

  const upstreamSignal = options.signal
  const controller = new AbortController()
  let timedOut = false
  let timeoutId = null
  const abortFromUpstream = () => controller.abort(upstreamSignal?.reason)
  if (upstreamSignal?.aborted) {
    abortFromUpstream()
  } else if (upstreamSignal) {
    upstreamSignal.addEventListener('abort', abortFromUpstream, { once: true })
  }
  if (timeoutMs > 0) {
    timeoutId = setTimeout(() => {
      timedOut = true
      controller.abort()
    }, timeoutMs)
  }
  config.signal = controller.signal

  // 添加 token（如果有）
  const token = localStorage.getItem('token')
  if (token) {
    config.headers['Authorization'] = `Bearer ${token}`
  }

  config.headers = applyEnvironmentHeaders(url, config.method, config.headers)

  const isFormData = config.body instanceof FormData
  if (isFormData) {
    delete config.headers['Content-Type']
  }
  
  // 处理请求体
  if (config.body && typeof config.body === 'object' && !isFormData) {
    config.body = JSON.stringify(config.body)
  }
  
  try {
    const response = await fetch(fullUrl, config)

    publishEnvironmentResponse(url, config.method, config.headers, response)
    
    // 处理 blob 响应
    if (config.responseType === 'blob' && response.ok) {
      return response.blob()
    }
    
    // 解析 JSON 响应
    const text = await response.text()
    // console.log('[Request] 原始响应:', text.substring(0, 200))
    let data = null
    try {
      data = text ? JSON.parse(text) : null
    } catch (e) {
      console.error('[Request] JSON 解析失败:', e)
      data = text
    }
    
    if (!response.ok) {
			const message = (typeof data?.error === 'string' && data.error) || data?.message || `请求失败: ${response.status}`
			const apiError = new ApiError(message, {
				status: response.status,
				code: data?.error_code || data?.code || '',
				retryable: Boolean(data?.retryable),
				requestId: data?.request_id || data?.requestId || '',
				details: data?.details ?? null,
				payload: data
			})
			if (response.status === 401 && url.split('?', 1)[0] !== '/auth/login') {
				localStorage.removeItem('loggedIn')
				localStorage.removeItem('token')
				if (!isRedirectingTo401 && window.location.pathname !== '/login') {
					isRedirectingTo401 = true
					window.location.href = '/login'
				}
			}
			throw apiError
    }
    
    return data
  } catch (error) {
    console.error('Request error:', error)
    if (timedOut) {
      throw new Error('请求超时，请稍后重试')
    }
    throw error
  } finally {
    if (timeoutId !== null) clearTimeout(timeoutId)
    upstreamSignal?.removeEventListener?.('abort', abortFromUpstream)
  }
}

// HTTP 方法封装
export const get = (url, params = {}, options = {}) => {
  const queryString = new URLSearchParams(params).toString()
  const fullUrl = queryString ? `${url}?${queryString}` : url
  return request(fullUrl, { method: 'GET', ...options })
}

export const post = (url, data = {}, options = {}) => {
  return request(url, { method: 'POST', body: data, ...options })
}

export const put = (url, data = {}, options = {}) => {
  return request(url, { method: 'PUT', body: data, ...options })
}

export const del = (url, options = {}) => {
  return request(url, { method: 'DELETE', ...options })
}

// 导出默认请求函数
export default request

// API 对象
export const api = {
  system: {
    info: () => get('/system/info'),
    stats: () => get('/system/stats'),
    events: () => get('/system/events'),
    diskUsage: () => get('/system/disk-usage'),
    notificationSummary: () => get('/system/notifications/summary'),
  },
  auth: {
    login: (data) => post('/auth/login', data),
    changePassword: (data) => post('/auth/change-password', data),
  },
  images: {
    list: () => get('/images'),
    remove: (id, repoTag = '') => {
      const query = repoTag ? `?repoTag=${encodeURIComponent(repoTag)}` : ''
      return del(`/images/${encodeURIComponent(id)}${query}`)
    },
    pull: (data) => post('/images/pull', data),
    tag: (data) => post('/images/tag', data),
    getExportUrl: (id) => {
      const baseUrl = import.meta.env.VITE_API_BASE_URL || ''
      return `${baseUrl}/api/images/export/${encodeURIComponent(id)}`
    },
    exportImage: async (id) => {
      const baseUrl = import.meta.env.VITE_API_BASE_URL || ''
      const url = `${baseUrl}/api/images/export/${encodeURIComponent(id)}`
      const response = await fetch(url, {
        credentials: 'include'  // 使用 Cookie 认证
      })
      if (!response.ok) {
        throw new Error(`导出失败: ${response.status}`)
      }
      return response.blob()
    },
    prune: () => post('/images/prune'),
    checkUpdates: () => get('/images/updates'),
    getUpdateStatus: () => get('/images/updates/status'),
    applyUpdates: () => post('/images/updates/apply'),
    getProxy: () => get('/images/proxy'),
    updateProxy: (data) => post('/images/proxy', data),
  },
  containers: {
    list: () => get('/containers'),
  }
}
