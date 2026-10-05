const ansiPattern = /\u001b\[[0-?]*[ -/]*[@-~]/g
const transportTimestampPattern = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?)\s*/
const applicationTimestampPattern = /^(\d{4}[/-]\d{1,2}[/-]\d{1,2}[ T]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?|\d{2}:\d{2}:\d{2}(?:[.,]\d+)?)\s*/
const sourcePrefixPattern = /^(\[[^\]\r\n]{1,24}\]\s*)/
// Compose SSE 给每行加了 "service | " 前缀（服务名不含空格/竖线）。
// 把它拆进 source 字段，否则后面的传输时间戳正则（锚定在行首）匹配不到，
// 时间列会退化为"收到时刻"且日志内容里残留原时间戳，造成时间重复。
const servicePrefixPattern = /^([^\s|]{1,64})\s+\|\s+/

const logTonePatterns = [
  ['error', /\b(?:error|err|fatal|panic|exception|failed|failure)\b|失败|错误|异常/i],
  ['warning', /\b(?:warn|warning|deprecated|retry|retrying)\b|警告|重试/i],
  ['success', /\b(?:success|successful|ready|started|healthy|completed)\b|成功|完成|就绪/i],
  ['debug', /\b(?:debug|trace)\b/i],
  ['info', /\b(?:info|notice|listening)\b|信息/i]
]

export function stripAnsi(value) {
  return String(value || '').replace(ansiPattern, '')
}

export function classifyLogLine(value) {
  const line = stripAnsi(value)
  return logTonePatterns.find(([, pattern]) => pattern.test(line))?.[0] || 'default'
}

function padDatePart(value) {
  return String(value).padStart(2, '0')
}

export function formatLogTimestamp(value) {
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return [
    date.getFullYear(),
    padDatePart(date.getMonth() + 1),
    padDatePart(date.getDate())
  ].join('-') + ` ${padDatePart(date.getHours())}:${padDatePart(date.getMinutes())}:${padDatePart(date.getSeconds())}`
}

export function formatLogTime(value) {
  return String(value || '').match(/(?:^|\s)(\d{2}:\d{2}:\d{2})$/)?.[1] || ''
}

function parseTimestamp(value, receivedAt) {
  const numeric = String(value).match(/^(\d{4})[/-](\d{1,2})[/-](\d{1,2})[ T](\d{2}):(\d{2}):(\d{2})(?:[.,]\d+)?$/)
  if (numeric) {
    return new Date(
      Number(numeric[1]),
      Number(numeric[2]) - 1,
      Number(numeric[3]),
      Number(numeric[4]),
      Number(numeric[5]),
      Number(numeric[6])
    )
  }
  const timeOnly = String(value).match(/^(\d{2}):(\d{2}):(\d{2})(?:[.,]\d+)?$/)
  if (timeOnly) {
    const date = new Date(receivedAt)
    date.setHours(Number(timeOnly[1]), Number(timeOnly[2]), Number(timeOnly[3]), 0)
    return date
  }
  return new Date(value)
}

function extractLeadingApplicationTimestamp(value, receivedAt) {
  const sourceMatch = value.match(sourcePrefixPattern)
  const source = sourceMatch?.[1] || ''
  const rest = value.slice(source.length)
  const timestampMatch = rest.match(applicationTimestampPattern)
  if (!timestampMatch) return null

  return {
    date: parseTimestamp(timestampMatch[1], receivedAt),
    text: `${source.trimEnd()}${source ? ' ' : ''}${rest.slice(timestampMatch[0].length).trimStart()}`.trim()
  }
}

export function parseLogEntry(value, receivedAt = new Date()) {
  let text = stripAnsi(value).replace(/\r/g, '').trim()
  let source = ''
  let timestampDate = null

  const serviceMatch = text.match(servicePrefixPattern)
  if (serviceMatch) {
    source = serviceMatch[1].trim()
    text = text.slice(serviceMatch[0].length).trimStart()
  }

  const transportMatch = text.match(transportTimestampPattern)
  if (transportMatch) {
    timestampDate = parseTimestamp(transportMatch[1], receivedAt)
    text = text.slice(transportMatch[0].length).trimStart()

    const duplicateTimestamp = extractLeadingApplicationTimestamp(text, receivedAt)
    if (duplicateTimestamp) text = duplicateTimestamp.text
  } else {
    const applicationTimestamp = extractLeadingApplicationTimestamp(text, receivedAt)
    if (applicationTimestamp) {
      timestampDate = applicationTimestamp.date
      text = applicationTimestamp.text
    }
  }

  if (!timestampDate || Number.isNaN(timestampDate.getTime())) {
    timestampDate = new Date(receivedAt)
  }

  return {
    source,
    timestamp: formatLogTimestamp(timestampDate),
    text,
    tone: classifyLogLine(text)
  }
}

function escapeRegExp(value) {
  return String(value).replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// 为 Compose 日志里不同服务生成稳定的辨识色（字符串散列 → 色相），
// 同一个服务在任何组件/时刻都得到同一个 hue，方便肉眼区分来源。
export function sourceHue(name) {
  const source = String(name || '').trim()
  if (!source) return 0
  let hash = 0
  for (let i = 0; i < source.length; i += 1) {
    hash = ((hash << 5) - hash + source.charCodeAt(i)) | 0
  }
  return Math.abs(hash) % 360
}

export function sourceColors(name) {
  const source = String(name || '').trim()
  if (!source) return null
  const hue = sourceHue(source)
  return {
    tint: `hsl(${hue} 62% 55% / 0.08)`,
    label: `hsl(${hue} 70% 46%)`
  }
}

function escapeHtml(value) {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function renderSegment(text, type, query) {
  let html = escapeHtml(text)
  const cleanQuery = String(query || '').trim()
  if (cleanQuery) {
    const escapedQuery = escapeRegExp(escapeHtml(cleanQuery))
    html = html.replace(new RegExp(`(${escapedQuery})`, 'gi'), '<mark class="highlight">$1</mark>')
  }
  return type ? `<span class="log-token-${type}">${html}</span>` : html
}

export function renderLogLineHtml(value, query = '') {
  const line = stripAnsi(value).replace(/\r/g, '')
  const segments = []
  let cursor = 0

  const source = line.match(/^\[[^\]\r\n]{1,24}\]/)
  if (source) {
    segments.push({ type: 'source', text: source[0] })
    cursor = source[0].length
  }

  const timestampPattern = /\b(?:\d{4}[/-]\d{1,2}[/-]\d{1,2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?|\d{2}:\d{2}:\d{2})\b/g
  timestampPattern.lastIndex = cursor
  let match = timestampPattern.exec(line)

  while (match) {
    if (match.index > cursor) {
      segments.push({ type: '', text: line.slice(cursor, match.index) })
    }
    segments.push({ type: 'time', text: match[0] })
    cursor = match.index + match[0].length
    match = timestampPattern.exec(line)
  }

  if (cursor < line.length) {
    segments.push({ type: '', text: line.slice(cursor) })
  }

  return segments.map(segment => renderSegment(segment.text, segment.type, query)).join('')
}
