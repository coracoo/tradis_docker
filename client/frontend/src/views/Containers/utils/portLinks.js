// 端口跳转链接：公开端口 → http://<当前访问主机>:<PublicPort>。
// 用页面自身 hostname 作为内网地址，用户从哪个内网地址访问面板就从哪跳。
// Docker 对同一端口会分别返回 IPv4/IPv6 两条记录，按 PublicPort:PrivatePort 去重。
export function buildPortLinks (ports) {
  if (!ports || ports.length === 0) return []
  const host = window.location.hostname || '127.0.0.1'
  const seen = new Set()
  const links = []
  for (const port of ports) {
    if (!port || !port.PublicPort || String(port.Type || 'tcp').toLowerCase() !== 'tcp') continue
    const key = `${port.PublicPort}:${port.PrivatePort}`
    if (seen.has(key)) continue
    seen.add(key)
    links.push({ label: key, url: `http://${host}:${port.PublicPort}` })
  }
  return links
}
