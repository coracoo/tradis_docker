function formatProgressTime(rawTime) {
  const value = String(rawTime || '').trim()
  const matched = value.match(/(?:T|\s|^)(\d{2}:\d{2}:\d{2})(?:\.|Z|[+-]|$)/)
  if (matched) return matched[1]

  return new Date().toLocaleTimeString('zh-CN', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  })
}

export function upsertPullProgressDetail(details, payload, eventTime) {
  const id = String(payload?.id || '').trim()
  if (!id) return Array.isArray(details) ? details : []

  const current = Array.isArray(details) ? details : []
  const existing = current.find(detail => detail.id === id)
  const updated = {
    id,
    status: String(payload?.status || existing?.status || ''),
    time: formatProgressTime(eventTime)
  }

  return [updated, ...current.filter(detail => detail.id !== id)].slice(0, 30)
}
