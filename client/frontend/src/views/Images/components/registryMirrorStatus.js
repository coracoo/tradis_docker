export const defaultHubMirrors = [
  { label: '毫秒镜像（1ms，免费）', url: 'https://docker.1ms.run', website: 'https://1ms.run/' },
  { label: '1panel', url: 'https://docker.1panel.live' },
  { label: 'agsv', url: 'https://docker.agsv.top' },
  { label: '耗子面板', url: 'https://hub.rat.dev' },
  { label: '轩辕镜像', url: 'https://docker.xuanyuan.me' },
  { label: 'DockerProxy', url: 'https://dockerproxy.net' },
  { label: 'DaoCloud', url: 'https://docker.m.daocloud.io' }
]

export const defaultGhcrMirrors = [
  { label: 'GHCR 官方', url: 'https://ghcr.io' },
  { label: '毫秒镜像（1ms，免费）', url: 'https://ghcr.1ms.run', website: 'https://1ms.run/' }
]

export const probeLabels = {
  reachable: '可达', auth_required: '可达 · 认证握手', rate_limited: '限流',
  unexpected_response: '响应异常', timeout: '超时', unreachable: '连接失败', cancelled: '已取消',
  network_restricted: '检测受限（DNS / 代理）', dns_failed: 'DNS 解析失败'
}

const cacheKey = 'tradis_registry_probe_cache'
export const ghcrCandidatesKey = 'tradis_ghcr_probe_candidates'

export function normalizeMirrorURL(raw) {
  try {
    const url = new URL(String(raw).trim())
    if (url.protocol !== 'https:' || url.username || url.password || url.search || url.hash ||
      !['', '/', '/v2', '/v2/'].includes(url.pathname)) return ''
    return url.origin
  } catch { return '' }
}

export function validProbeResult(result, url) {
  return result?.url === url && normalizeMirrorURL(url) === url &&
    Object.hasOwn(probeLabels, result.status) && Number.isFinite(Date.parse(result.checkedAt)) &&
    Number.isFinite(result.latencyMs) && result.latencyMs >= 0
}

export function readProbeCache() {
  try {
    const raw = localStorage.getItem(cacheKey) || '{}'
    if (raw.length > 65536) return {}
    const parsed = JSON.parse(raw)
    return Object.fromEntries(Object.entries(parsed).slice(-64).filter(([url, result]) => validProbeResult(result, url)))
  } catch { return {} }
}

export function writeProbeCache(results) {
  try {
    const entries = Object.entries(results).filter(([url, result]) => validProbeResult(result, url)).slice(-64)
    localStorage.setItem(cacheKey, JSON.stringify(Object.fromEntries(entries)))
  } catch { /* Storage can be disabled; checks remain usable. */ }
}

export function isProbeStale(result, now = Date.now()) {
  const checked = Date.parse(result?.checkedAt)
  return !Number.isFinite(checked) || checked > now + 60000 || now - checked > 5 * 60 * 1000
}

export function readGhcrCandidates() {
  try {
    const raw = localStorage.getItem(ghcrCandidatesKey) || '[]'
    if (raw.length > 16384) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return [...new Set(parsed.filter(url => typeof url === 'string').map(normalizeMirrorURL).filter(Boolean))].slice(0, 20)
  } catch { return [] }
}
