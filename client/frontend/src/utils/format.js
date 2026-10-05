/**
 * 格式化字节大小
 * @param {number} bytes - 字节数
 * @param {number} decimals - 小数位数
 * @returns {string} - 格式化后的字符串
 */
export function formatBytes(bytes, decimals = 2) {
  if (bytes === 0 || bytes === undefined || bytes === null) return '—'

  const k = 1024
  const dm = decimals < 0 ? 0 : decimals
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB']

  const i = Math.floor(Math.log(bytes) / Math.log(k))

  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i]
}

/**
 * 紧凑格式化字节大小：只保留整数，如 22G、1T、900M
 * @param {number} bytes - 字节数
 * @returns {string} - 格式化后的字符串
 */
export function formatBytesCompact(bytes) {
  if (bytes === 0 || bytes === undefined || bytes === null) return '—'

  const k = 1024
  const sizes = ['B', 'K', 'M', 'G', 'T', 'P']

  const i = Math.floor(Math.log(bytes) / Math.log(k))

  return Math.round(bytes / Math.pow(k, i)) + sizes[i]
}

/**
 * 格式化大小（formatBytes 的别名）
 * @param {number} bytes - 字节数
 * @param {number} decimals - 小数位数
 * @returns {string} - 格式化后的字符串
 */
export function formatSize(bytes, decimals = 2) {
  return formatBytes(bytes, decimals)
}

/**
 * 格式化时间
 * @param {string|number|Date} time - 时间字符串、Unix 时间戳或 Date 对象
 * @returns {string} - 格式化后的时间 yyyy-mm-dd hh:mm
 */
export function formatTime(time) {
  if (!time) return '—'
  if (time instanceof Date) {
    return Number.isNaN(time.getTime()) ? '—' : formatInChina(time)
  }
  if (typeof time === 'number' && Number.isFinite(time)) {
    const milliseconds = Math.abs(time) < 1e12 ? time * 1000 : time
    const date = new Date(milliseconds)
    return Number.isNaN(date.getTime()) ? '—' : formatInChina(date)
  }
  const raw = String(time).trim()
  if (!raw) return '—'
  if (/^\d{10}$|^\d{13}$/.test(raw)) {
    const value = Number(raw)
    const date = new Date(raw.length === 10 ? value * 1000 : value)
    if (!Number.isNaN(date.getTime())) return formatInChina(date)
  }
  // 带时区偏移的 ISO/RFC3339：解析成确定瞬间，按中国时间展示。
  if (raw.includes('T') || raw.endsWith('Z') || /[+-]\d{2}:?\d{2}$/.test(raw)) {
    const d = new Date(raw)
    if (!Number.isNaN(d.getTime())) return formatInChina(d)
  }
  // naive 字符串：按项目约定视为中国时间墙上时钟，直接截取，避免被浏览器时区二次偏移。
  const s = raw.replace('T', ' ')
  return s.length >= 16 ? `${s.slice(0, 10)} ${s.slice(11, 16)}` : s.slice(0, 16)
}

function formatInChina(date) {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23'
  }).formatToParts(date)
  const pick = type => parts.find(part => part.type === type)?.value || '00'
  return `${pick('year')}-${pick('month')}-${pick('day')} ${pick('hour')}:${pick('minute')}`
}

/**
 * 格式化时间（双行显示，兼容旧调用）
 * @param {string|Date} time - 时间字符串或 Date 对象
 * @returns {object} - { date: '日期', time: '时间' }
 */
export function formatTimeTwoLines(time) {
  const formatted = formatTime(time)
  if (formatted === '—') return { date: '—', time: '' }
  const [date, timePart] = formatted.split(' ')
  return { date, time: timePart }
}

/**
 * 格式化百分比
 * @param {number} value - 数值
 * @param {number} decimals - 小数位数
 * @returns {string} - 格式化后的百分比
 */
export function formatPercent(value, decimals = 1) {
  if (value === undefined || value === null) return '—'
  return parseFloat(value).toFixed(decimals) + '%'
}

/**
 * Shortens Docker Hub image references for display without changing the
 * canonical value used by Docker APIs.
 */
export function formatDockerImageReference(reference) {
  const value = String(reference || '').trim()
  if (!value) return value

  let display = value
  if (display.startsWith('docker.io/')) {
    display = display.slice('docker.io/'.length)
  } else if (display.startsWith('index.docker.io/')) {
    display = display.slice('index.docker.io/'.length)
  }
  if (display.startsWith('library/')) display = display.slice('library/'.length)
  return display
}

/**
 * 格式化日期时间（formatTime 的别名）
 * @param {string|Date} time - 时间字符串或 Date 对象
 * @returns {string} - 格式化后的日期时间
 */
export function formatDateTime(time) {
  return formatTime(time)
}
