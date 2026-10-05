import { reactive, shallowRef } from 'vue'

function inputDefault(input, initialValues) {
  if (Object.prototype.hasOwnProperty.call(initialValues || {}, input.id)) {
    return initialValues[input.id]
  }
  return input.default_value ?? ''
}

function normalizeLineValue(raw) {
  const value = String(raw ?? '').trim()
  if (value.length >= 2) {
    const quote = value[0]
    if ((quote === '"' || quote === "'") && value.at(-1) === quote) {
      if (quote === '"') {
        try {
          return JSON.parse(value)
        } catch {
          // Keep accepting existing dotenv quoting even when it is not JSON-compatible.
        }
      }
      return value.slice(1, -1)
    }
  }
  return value
}

function formatLineValue(raw) {
  const value = String(raw ?? '')
  if (!/[\s#"'\\\r\n]/.test(value)) return value
  return `"${value.replaceAll('\\', '\\\\').replaceAll('"', '\\"').replaceAll('\r', '\\r').replaceAll('\n', '\\n')}"`
}

function dotenvKeyPattern(key) {
  const escaped = String(key || '').replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return new RegExp(`^(\\s*(?:export\\s+)?${escaped}\\s*=\\s*).*$`)
}

function serviceForInput(input, manifest) {
  if (input?.scope === 'project') return 'Global'
  const binding = (manifest?.bindings || []).find(item => item.input_id === input?.id && item.service)
  return binding?.service || 'Global'
}

export function normalizeManifestGroup(group) {
  return String(group || '').trim().toLowerCase() === 'advanced' ? 'advanced' : 'basic'
}

export function manifestInputLevel(manifest = {}, inputID = '') {
  const input = (manifest.inputs || []).find(item => item.id === inputID)
  if (!input) return null
  return normalizeManifestGroup(input.presentation?.group)
}

export function manifestInputsToSchema(manifest = {}) {
  return (manifest.inputs || [])
    .filter(input => !['port', 'bind', 'device'].includes(input.kind))
    .map(input => ({
      _uid: input.id,
      inputId: input.id,
      name: input.key,
      label: input.presentation?.label || input.key,
      description: input.presentation?.description || '',
      default: input.default_value ?? '',
      category: normalizeManifestGroup(input.presentation?.group),
      serviceName: serviceForInput(input, manifest),
      paramType: input.kind === 'secret' ? 'secret' : 'env',
      kind: input.kind || 'env',
      required: !!input.required,
      envFile: input.file || '',
      sensitive: !!input.sensitive
    }))
}

export function createTemplateManifestForm(initialManifest = {}, initialValues = {}) {
  const manifest = shallowRef(initialManifest || {})
  const valuesByInputID = reactive({})
  const mappingOverlays = reactive([])

  function replaceManifest(nextManifest = {}, nextInitialValues = {}, options = {}) {
    const previous = { ...valuesByInputID }
    const next = {}
    for (const input of nextManifest.inputs || []) {
      next[input.id] = options.preserveValues && Object.prototype.hasOwnProperty.call(previous, input.id)
        ? previous[input.id]
        : inputDefault(input, nextInitialValues)
    }
    for (const key of Object.keys(valuesByInputID)) delete valuesByInputID[key]
    Object.assign(valuesByInputID, next)
    mappingOverlays.splice(0, mappingOverlays.length)
    manifest.value = nextManifest || {}
  }

  function setValue(inputID, value) {
    if (!Object.prototype.hasOwnProperty.call(valuesByInputID, inputID)) {
      throw new Error(`unknown template input: ${inputID}`)
    }
    valuesByInputID[inputID] = value
  }

  function valueOf(inputID) {
    return valuesByInputID[inputID]
  }

  function bindingPreviews(inputID) {
    return (manifest.value.bindings || [])
      .filter(binding => binding.input_id === inputID)
      .map(binding => ({ ...binding, value: valueOf(inputID) }))
  }

  function mappingByID(id) {
    return (manifest.value.mappings || []).find(mapping => mapping.id === id)
  }

  function normalizedOverlay(mapping, patch = {}) {
    return {
      id: mapping.id,
      kind: patch.kind ?? mapping.kind,
      service: patch.service ?? mapping.service,
      source: patch.source ?? mapping.source ?? '',
      target: patch.target ?? mapping.target ?? '',
      protocol: patch.protocol ?? mapping.protocol ?? '',
      mode: patch.mode ?? mapping.mode ?? ''
    }
  }

  function updateMapping(id, patch = {}) {
    const base = mappingByID(id) || mappingOverlays.find(item => item.id === id)
    if (!base) throw new Error(`unknown template mapping: ${id}`)
    const overlay = normalizedOverlay(base, patch)
    const index = mappingOverlays.findIndex(item => item.id === id)
    if (index >= 0) mappingOverlays.splice(index, 1, overlay)
    else mappingOverlays.push(overlay)
    if (Object.prototype.hasOwnProperty.call(valuesByInputID, id)) {
      valuesByInputID[id] = overlay.source
    }
  }

  function addMapping(mapping) {
    if (!mapping?.id || mappingByID(mapping.id) || mappingOverlays.some(item => item.id === mapping.id)) {
      throw new Error('mapping id must be unique')
    }
    if (!['port', 'bind', 'device', 'environment'].includes(mapping.kind)) {
      throw new Error(`unsupported mapping kind: ${mapping.kind}`)
    }
    mappingOverlays.push(normalizedOverlay(mapping))
  }

  function removeAddedMapping(id) {
    if (mappingByID(id)) return false
    const index = mappingOverlays.findIndex(item => item.id === id)
    if (index < 0) return false
    mappingOverlays.splice(index, 1)
    return true
  }

  function projectInputs() {
    return (manifest.value.inputs || []).filter(input => input.scope === 'project')
  }

  function serviceNames() {
    const names = []
    const seen = new Set()
    const add = (name) => {
      if (!name || seen.has(name)) return
      seen.add(name)
      names.push(name)
    }
    ;(manifest.value.services || []).forEach(add)
    for (const binding of manifest.value.bindings || []) {
      add(binding.service)
    }
    for (const mapping of manifest.value.mappings || []) {
      add(mapping.service)
    }
    return names
  }

  function renderProjectDotenv(sourceText = '') {
    const normalized = String(sourceText || '').replaceAll('\r\n', '\n')
    const lines = normalized === '' ? [] : normalized.replace(/\n$/, '').split('\n')

    for (const input of projectInputs()) {
      const pattern = dotenvKeyPattern(input.key)
      let matchedIndex = -1
      for (let index = 0; index < lines.length; index += 1) {
        const trimmed = lines[index].trim()
        if (!trimmed || trimmed.startsWith('#')) continue
        if (pattern.test(lines[index])) matchedIndex = index
      }
      const value = formatLineValue(valuesByInputID[input.id] ?? '')
      if (matchedIndex >= 0) {
        lines[matchedIndex] = lines[matchedIndex].replace(pattern, (_match, prefix) => `${prefix}${value}`)
      } else {
        lines.push(`${input.key}=${value}`)
      }
    }

    return lines.length > 0 ? `${lines.join('\n')}\n` : ''
  }

  function applyProjectDotenv(content) {
    const byKey = new Map()
    for (const input of projectInputs()) {
      if (!byKey.has(input.key)) byKey.set(input.key, [])
      byKey.get(input.key).push(input)
    }
    const parsed = new Map()
    const errors = []
    String(content || '').replaceAll('\r\n', '\n').split('\n').forEach((raw, index) => {
      const line = raw.trim()
      if (!line || line.startsWith('#')) return
      const separator = line.indexOf('=')
      if (separator <= 0) {
        errors.push(`第 ${index + 1} 行不是有效的 KEY=value`)
        return
      }
      const key = line.slice(0, separator).trim().replace(/^export\s+/, '')
      if (parsed.has(key)) {
        errors.push(`第 ${index + 1} 行重复定义 ${key}`)
        return
      }
      const matches = byKey.get(key) || []
      if (matches.length !== 1) {
        errors.push(matches.length === 0 ? `第 ${index + 1} 行包含未知变量 ${key}` : `第 ${index + 1} 行变量 ${key} 归属不明确`)
        return
      }
      parsed.set(key, normalizeLineValue(line.slice(separator + 1)))
    })
    if (errors.length > 0) return { ok: false, errors }
    for (const input of projectInputs()) {
      valuesByInputID[input.id] = ''
    }
    for (const [key, value] of parsed) {
      valuesByInputID[byKey.get(key)[0].id] = value
    }
    return { ok: true, errors: [] }
  }

  replaceManifest(initialManifest, initialValues)

  return {
    manifest,
    valuesByInputID,
    mappingOverlays,
    replaceManifest,
    setValue,
    valueOf,
    bindingPreviews,
    updateMapping,
    addMapping,
    removeAddedMapping,
    serviceNames,
    renderProjectDotenv,
    applyProjectDotenv
  }
}
