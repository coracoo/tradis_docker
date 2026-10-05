const RUNNING_STATES = new Set(['running', '运行中'])
const STOPPED_STATES = new Set(['exited', 'stopped', '已停止'])
const ERROR_STATES = new Set(['dead', 'error', 'failed'])

function normalize(value) {
  return String(value || '').trim().toLowerCase()
}

export function getContainerHealthStatus(container) {
  const explicit = normalize(container?.HealthStatus || container?.healthStatus)
  if (explicit) return explicit

  const status = normalize(container?.Status || container?.statusText)
  if (status.includes('(unhealthy)')) return 'unhealthy'
  if (status.includes('(healthy)')) return 'healthy'
  if (status.includes('(health: starting)') || status.includes('(health:starting)')) return 'starting'
  return ''
}

export function classifyContainerStatus(container) {
  const state = normalize(container?.State || container?.state)
  const health = getContainerHealthStatus(container)

  if (RUNNING_STATES.has(state) && health !== 'unhealthy' && health !== 'starting') {
    return 'running'
  }
  if (STOPPED_STATES.has(state)) return 'stopped'
  return 'unhealthy'
}

export function classifyComposeStatus(project) {
  const containers = project?.containers || []
  if (containers.length === 0) return 'stopped'

  const states = containers.map(classifyContainerStatus)
  if (states.every(state => state === 'running')) return 'running'
  if (states.every(state => state === 'stopped')) return 'stopped'
  return 'unhealthy'
}

export function containerDisplayStatus(container) {
  const state = normalize(container?.State || container?.state)
  const health = getContainerHealthStatus(container)

  if (state === 'running') {
    if (health === 'unhealthy') return 'unhealthy'
    if (health === 'starting') return 'health-starting'
    return 'running'
  }
  if (STOPPED_STATES.has(state)) return 'stopped'
  if (ERROR_STATES.has(state)) return 'error'
  if (state === 'created') return 'created'
  if (state === 'paused') return 'paused'
  if (state === 'restarting') return 'restarting'
  if (state === 'removing') return 'removing'
  return 'unknown'
}

export function composeProjectDisplayStatus(project) {
  const containers = project?.containers || []
  if (containers.length === 0) return 'stopped'

  const states = [...new Set(containers.map(containerDisplayStatus))]
  if (states.length === 1) return states[0]
  if (states.includes('error')) return 'error'
  if (states.includes('unhealthy')) return 'unhealthy'
  return 'partial'
}

// 按 Docker 实际生命周期给出项目状态文案（用于列表/卡片展示），
// 不再把"已创建/重启中/健康检查中"等一律显示成"不健康"。
const PROJECT_STATE_LABELS = {
  running: '运行中',
  paused: '已暂停',
  restarting: '重启中',
  created: '已创建',
  exited: '已停止',
  dead: '故障',
  removing: '移除中'
}
export function composeProjectStateLabel(project) {
  const containers = project?.containers || []
  if (!containers.length) return '已停止'
  const labels = containers.map(c => {
    const state = normalize(c?.State)
    const health = getContainerHealthStatus(c)
    if (state === 'running' && health === 'unhealthy') return '不健康'
    if (state === 'running' && health === 'starting') return '检测中'
    return PROJECT_STATE_LABELS[state] || state || '未知'
  })
  const unique = [...new Set(labels)]
  if (unique.length === 1) return unique[0]
  return unique.includes('运行中') ? '部分运行' : '状态不一'
}

export function formatContainerUptime(container) {
  if (normalize(container?.State || container?.state) !== 'running') return '-'

  const runningTime = String(container?.RunningTime || container?.runningTime || '').trim()
  if (runningTime && runningTime !== '运行中' && runningTime !== '未运行') {
    return runningTime
  }

  const status = String(container?.Status || container?.statusText || '').trim()
  const match = status.match(/^up\s+(.+?)(?:\s+\([^)]*\))?$/i)
  return match?.[1]?.trim() || '运行中'
}

// 毫秒时长 → 中文（"2天3小时"/"3小时20分"/"5分钟"），Compose 与容器详情共用
export function formatElapsed(ms) {
  if (!Number.isFinite(ms) || ms < 0) return '-'
  const seconds = Math.floor(ms / 1000)
  if (seconds < 60) return `${seconds}秒`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}分钟`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) {
    const remainMinutes = minutes % 60
    return remainMinutes > 0 ? `${hours}小时${remainMinutes}分` : `${hours}小时`
  }
  const days = Math.floor(hours / 24)
  const remainHours = hours % 24
  return remainHours > 0 ? `${days}天${remainHours}小时` : `${days}天`
}

// 解析后端 Go time.Duration 字符串（如 "2h3m4s"/"48h0m0s"）为中文时长
export function parseGoDurationCn(text) {
  const raw = String(text || '').trim()
  const match = raw.match(/^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)(?:\.\d+)?s)?$/)
  if (!match) return ''
  const hours = Number(match[1] || 0)
  const minutes = Number(match[2] || 0)
  const seconds = Number(match[3] || 0)
  if (!hours && !minutes && !seconds) return ''
  return formatElapsed((hours * 3600 + minutes * 60 + seconds) * 1000)
}

// 容器运行时长中文版：优先用 Created 时间戳实时计算，其次解析 RunningTime，最后回退 formatContainerUptime
export function formatContainerUptimeCn(container) {
  if (normalize(container?.State || container?.state) !== 'running') return '-'
  const created = Number(container.Created)
  if (Number.isFinite(created) && created > 0) {
    // Created 可能来自 docker 列表（秒）或详情补全后（毫秒），统一为毫秒
    const startedMs = created < 1e12 ? created * 1000 : created
    return formatElapsed(Date.now() - startedMs)
  }
  const parsed = parseGoDurationCn(container?.RunningTime || container?.runningTime)
  return parsed || formatContainerUptime(container)
}
