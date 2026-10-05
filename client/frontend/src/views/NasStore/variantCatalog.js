import { cpuArchitecture } from './filterCatalog.js'

export function normalizedNASVariants(device) {
  const variants = Array.isArray(device?.variants) && device.variants.length > 0
    ? device.variants
    : [{
        id: 'default',
        label: '默认规格',
        cpu: device?.cpu || '',
        ram: device?.ram || '',
        price_cny: device?.price_cny || 0,
        price_label: device?.price_label || '',
        jd_search_url: device?.jd_search_url || device?.purchase_url || '',
        tb_search_url: device?.tb_search_url || '',
        official_url: device?.official_url || '',
        default: true,
        sort_order: 10
      }]

  return variants
    .map((variant, index) => ({
      ...variant,
      id: String(variant?.id || `variant_${index + 1}`),
      label: String(variant?.label || [variant?.cpu, variant?.ram].filter(Boolean).join(' / ') || '默认规格'),
      sort_order: Number(variant?.sort_order) || (index + 1) * 10
    }))
    .sort((a, b) => a.sort_order - b.sort_order || a.id.localeCompare(b.id))
}

export function selectedNASVariant(device, variantID = '') {
  const variants = normalizedNASVariants(device)
  return variants.find(variant => variant.id === variantID) ||
    variants.find(variant => variant.default) ||
    variants[0] || null
}

function variantMatchesFilters(variant, filters = {}) {
  const architectures = Array.isArray(filters.architectures) ? filters.architectures : []
  if (architectures.length > 0 && !architectures.includes(cpuArchitecture(variant?.cpu))) {
    return false
  }
  const min = filters.minPrice === '' || filters.minPrice == null ? 0 : Number(filters.minPrice)
  const max = filters.maxPrice === '' || filters.maxPrice == null ? Infinity : Number(filters.maxPrice)
  const price = Number(variant?.price_cny) || 0
  return price >= min && price <= max
}

export function selectVariantForFilters(device, filters = {}) {
  const hasVariantFilter = (Array.isArray(filters.architectures) && filters.architectures.length > 0) ||
    (filters.minPrice !== '' && filters.minPrice != null && Number(filters.minPrice) > 0) ||
    (filters.maxPrice !== '' && filters.maxPrice != null && Number.isFinite(Number(filters.maxPrice)))
  if (!hasVariantFilter) return selectedNASVariant(device)
  return normalizedNASVariants(device).find(variant => variantMatchesFilters(variant, filters)) || null
}

export function deviceMatchesVariantFilters(device, filters = {}) {
  return selectVariantForFilters(device, filters) !== null
}

export function deviceWithVariant(device, variantID = '') {
  const variant = selectedNASVariant(device, variantID)
  if (!variant) return device
  return {
    ...device,
    cpu: variant.cpu || '',
    ram: variant.ram || '',
    price_cny: variant.price_cny || 0,
    price_label: variant.price_label || '',
    jd_search_url: variant.jd_search_url || '',
    tb_search_url: variant.tb_search_url || '',
    official_url: variant.official_url || '',
    purchase_url: variant.purchase_url || variant.jd_search_url || variant.tb_search_url || variant.official_url || device?.purchase_url || '',
    selected_variant_id: variant.id
  }
}
