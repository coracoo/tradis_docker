// AI 模型选择：localStorage 缓存读写、候选列表构建
// 缓存契约：localStorage[ai_models_cache_v1] = { baseUrl, models, fetchedAt }

export const AI_MODELS_CACHE_KEY = 'ai_models_cache_v1'

function clearInvalidModelsCache(storage) {
  try {
    storage.removeItem(AI_MODELS_CACHE_KEY)
  } catch {
    // Storage may be unavailable; callers still fall back to an empty cache.
  }
}

// 归一化模型列表：去空白、去空项、按出现顺序去重
export function normalizeModelList(list) {
  if (!Array.isArray(list)) return []
  const seen = new Set()
  const result = []
  for (const item of list) {
    const id = String(item ?? '').trim()
    if (!id || seen.has(id)) continue
    seen.add(id)
    result.push(id)
  }
  return result
}

// 读取模型列表缓存；缓存缺失、损坏或结构不合法时返回 null
export function readAiModelsCache(storage = localStorage) {
  try {
    const raw = storage.getItem(AI_MODELS_CACHE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed) || !Array.isArray(parsed.models)) {
      clearInvalidModelsCache(storage)
      return null
    }
    return {
      baseUrl: String(parsed.baseUrl || '').trim(),
      models: normalizeModelList(parsed.models),
      fetchedAt: Number.isFinite(Number(parsed.fetchedAt)) ? Number(parsed.fetchedAt) : 0
    }
  } catch {
    clearInvalidModelsCache(storage)
    return null
  }
}

// 写入模型列表缓存，返回归一化后的缓存对象
export function writeAiModelsCache({ baseUrl, models, fetchedAt } = {}, storage = localStorage) {
  const entry = {
    baseUrl: String(baseUrl || '').trim(),
    models: normalizeModelList(models),
    fetchedAt: Number.isFinite(Number(fetchedAt)) ? Number(fetchedAt) : Date.now()
  }
  try {
    storage.setItem(AI_MODELS_CACHE_KEY, JSON.stringify(entry))
  } catch (e) {
    console.error('保存 AI 模型缓存失败:', e)
  }
  return entry
}

// 构建模型字段下拉候选：缓存模型按原顺序排列，当前值（已保存或手输的自定义模型）不在缓存时追加在后，保证可回显
export function buildModelSelectOptions(models, currentValues = []) {
  return normalizeModelList([...normalizeModelList(models), ...normalizeModelList(currentValues)])
}

// 当前表单 Base URL 与缓存来源不一致时需要提示重新拉取
export function isCacheBaseUrlMismatch(cache, currentBaseUrl) {
  if (!cache) return false
  return cache.baseUrl !== String(currentBaseUrl || '').trim()
}
