export const DEFAULT_NAS_FILTER_CATALOG = Object.freeze({
  brands: ['群晖', '威联通', '绿联', '极空间', '铁威马', '自组'],
  driveBays: [2, 4, 6, 8],
  architectures: ['Intel', 'AMD', 'Realtek', 'ARM'],
  networks: ['10G', '5G', '2.5G', '1000M']
})

function formatRate(value) {
  return Number.isInteger(value) ? String(value) : String(value).replace(/0+$/, '').replace(/\.$/, '')
}

export function normalizeNetworkTier(value) {
  const match = String(value || '').trim().match(/^(\d+(?:\.\d+)?)\s*(G|M)(?:B(?:E|PS)?)?$/i)
  if (!match) return ''
  const amount = Number(match[1])
  if (!Number.isFinite(amount) || amount <= 0) return ''
  return `${formatRate(amount)}${match[2].toUpperCase()}`
}

function networkRateMbps(value, unit) {
  const amount = Number(value)
  if (!Number.isFinite(amount) || amount <= 0) return 0
  return unit.toUpperCase() === 'G' ? amount * 1000 : amount
}

function networkRates(network) {
  const rates = new Set()
  const pattern = /(\d+(?:\.\d+)?)\s*(G|M)(?:B(?:IT)?(?:\/S)?|BE)?/gi
  for (const match of String(network || '').matchAll(pattern)) {
    const mbps = networkRateMbps(match[1], match[2])
    if (mbps > 0) rates.add(mbps)
  }
  return rates
}

export function normalizeNetworkTiers(tiers) {
  const normalized = []
  const seen = new Set()
  for (const raw of Array.isArray(tiers) ? tiers : []) {
    const tier = normalizeNetworkTier(raw)
    if (!tier || seen.has(tier)) continue
    seen.add(tier)
    normalized.push(tier)
  }
  return normalized
}

export function matchingNetworkTiers(network, tiers) {
  const rates = networkRates(network)
  return normalizeNetworkTiers(tiers).filter((tier) => {
    const match = tier.match(/^(\d+(?:\.\d+)?)(G|M)$/)
    return match && rates.has(networkRateMbps(match[1], match[2]))
  })
}

export function cpuArchitecture(cpu) {
  return String(cpu || '').trim().split(/\s+/)[0] || ''
}

function valuesOrFallback(values, fallback) {
  return values.length > 0 ? values : [...fallback]
}

export function deriveNASFilterCatalog(devices, configuredNetworkTiers) {
  const source = Array.isArray(devices) ? devices : []
  const tiers = normalizeNetworkTiers(configuredNetworkTiers)
  const effectiveTiers = tiers.length > 0 ? tiers : [...DEFAULT_NAS_FILTER_CATALOG.networks]

  const brands = [...new Set(source.map(item => String(item?.brand || '').trim()).filter(Boolean))]
    .sort((a, b) => a.localeCompare(b, 'zh-CN'))
  const driveBays = [...new Set(source.map(item => Number(item?.drive_bays)).filter(value => value > 0))]
    .sort((a, b) => a - b)
  const architectures = [...new Set(source.flatMap((item) => {
    const cpus = Array.isArray(item?.variants) && item.variants.length > 0
      ? item.variants.map(variant => variant?.cpu)
      : [item?.cpu]
    return cpus.map(cpuArchitecture).filter(Boolean)
  }))]
    .sort((a, b) => a.localeCompare(b, 'zh-CN'))
  const matchedNetworks = new Set()
  source.forEach((item) => {
    matchingNetworkTiers(item?.network, effectiveTiers).forEach(tier => matchedNetworks.add(tier))
  })
  const networks = effectiveTiers.filter(tier => matchedNetworks.has(tier))

  return {
    brands: valuesOrFallback(brands, DEFAULT_NAS_FILTER_CATALOG.brands),
    driveBays: valuesOrFallback(driveBays, DEFAULT_NAS_FILTER_CATALOG.driveBays),
    architectures: valuesOrFallback(architectures, DEFAULT_NAS_FILTER_CATALOG.architectures),
    networks: valuesOrFallback(networks, effectiveTiers)
  }
}
