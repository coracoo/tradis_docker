import { computed, onBeforeUnmount, onMounted, ref, unref } from 'vue'
import settingsApi from '@/api/settings.js'

const COLUMN_SETTINGS_KEY = 'resource_table_columns'
const WIDTH_SETTINGS_KEY = 'resource_column_widths'
const DEFAULT_WIDTH = 140
const MIN_WIDTH = 54
const GRID_GAP = 8
const TABLE_CHROME_WIDTH = 44

let persistenceQueue = Promise.resolve()

function parseSettingMap(response) {
  try {
    const raw = typeof response?.value === 'string' ? response.value : ''
    const parsed = raw ? JSON.parse(raw) : {}
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {}
  } catch {
    return {}
  }
}

async function readSettingMap(key) {
  try {
    return parseSettingMap(await settingsApi.getKVSetting(key))
  } catch {
    return {}
  }
}

function updateSettingMap(key, update) {
  const task = persistenceQueue
    .catch(() => {})
    .then(async () => {
      const current = await readSettingMap(key)
      const next = update(current)
      await settingsApi.setKVSetting(key, JSON.stringify(next))
    })
  persistenceQueue = task
  return task
}

function normalizedWidth(value, fallback, minimum) {
  const number = Number(value)
  if (Number.isFinite(number) && number >= minimum) return Math.round(number)
  return Math.max(minimum, Math.round(Number(fallback) || DEFAULT_WIDTH))
}

export function useResourceTableColumns(columnsSource, tableKeySource) {
  const order = ref([])
  const hidden = ref(new Set())
  const widths = ref({})
  const explicitWidths = ref(false)
  let removeResizeListeners = null

  const sourceColumns = computed(() => unref(columnsSource) || [])
  const tableKey = computed(() => String(unref(tableKeySource) || '').trim())

  const orderedColumns = computed(() => {
    const columns = sourceColumns.value
    const byKey = new Map(columns.map(column => [column.key, column]))
    const sourceKeys = columns.map(column => column.key)
    const saved = [...new Set(order.value.filter(key => byKey.has(key)))]
    const merged = [...saved]

    for (const key of sourceKeys) {
      if (merged.includes(key)) continue
      const sourceIndex = sourceKeys.indexOf(key)
      const hasPreviousColumn = sourceKeys.slice(0, sourceIndex).some(previousKey => merged.includes(previousKey))
      if (!hasPreviousColumn) {
        merged.unshift(key)
        continue
      }
      const nextKey = sourceKeys.slice(sourceIndex + 1).find(candidate => merged.includes(candidate))
      if (nextKey) {
        merged.splice(merged.indexOf(nextKey), 0, key)
      } else {
        merged.push(key)
      }
    }

    return merged.map(key => byKey.get(key))
  })

  const visibleColumns = computed(() => {
    const visible = orderedColumns.value.filter(column => !hidden.value.has(column.key))
    return visible.length > 0 ? visible : orderedColumns.value.slice(0, 1)
  })

  const tableStyle = computed(() => {
    if (!tableKey.value || visibleColumns.value.length === 0) return {}
    const tracks = visibleColumns.value.map((column) => {
      const minimum = Number(column.minWidth) || MIN_WIDTH
      const width = normalizedWidth(widths.value[column.key], column.width, minimum)
      return column.flex && !explicitWidths.value ? `minmax(${width}px, 1fr)` : `${width}px`
    })
    const total = visibleColumns.value.reduce((sum, column) => {
      const minimum = Number(column.minWidth) || MIN_WIDTH
      return sum + normalizedWidth(widths.value[column.key], column.width, minimum)
    }, 0) + Math.max(0, visibleColumns.value.length - 1) * GRID_GAP + TABLE_CHROME_WIDTH

    return {
      '--resource-grid-template': tracks.join(' '),
      '--resource-table-min-width': `${total}px`
    }
  })

  onMounted(async () => {
    if (!tableKey.value) return
    const [columnSettings, widthSettings] = await Promise.all([
      readSettingMap(COLUMN_SETTINGS_KEY),
      readSettingMap(WIDTH_SETTINGS_KEY)
    ])
    const savedColumns = columnSettings[tableKey.value]
    if (savedColumns && typeof savedColumns === 'object') {
      order.value = Array.isArray(savedColumns.order) ? savedColumns.order.map(String) : []
      hidden.value = new Set(Array.isArray(savedColumns.hidden) ? savedColumns.hidden.map(String) : [])
      const validKeys = sourceColumns.value.map(column => column.key)
      if (validKeys.length > 0 && validKeys.every(key => hidden.value.has(key))) {
        hidden.value.delete(validKeys[0])
      }
    }

    const savedWidths = widthSettings[tableKey.value]
    if (Array.isArray(savedWidths)) {
      const keys = sourceColumns.value.map(column => column.key)
      const savedOrder = Array.isArray(savedColumns?.order)
        ? savedColumns.order.map(String).filter(key => keys.includes(key))
        : []
      let legacyKeys = keys
      if (savedOrder.length === savedWidths.length && savedOrder.length < keys.length) {
        const previousKeys = new Set(savedOrder)
        legacyKeys = keys.filter(key => previousKeys.has(key))
      } else if (keys[0] === 'check' && savedWidths.length === keys.length - 1) {
        legacyKeys = keys.slice(1)
      }
      widths.value = Object.fromEntries(legacyKeys.map((key, index) => [key, savedWidths[index]]))
      explicitWidths.value = savedWidths.length === legacyKeys.length && legacyKeys.length > 0
    } else if (savedWidths && typeof savedWidths === 'object') {
      widths.value = { ...savedWidths }
      explicitWidths.value = Object.keys(savedWidths).length > 0
    }
  })

  onBeforeUnmount(() => removeResizeListeners?.())

  function persistColumns() {
    if (!tableKey.value) return Promise.resolve()
    const entry = {
      order: orderedColumns.value.map(column => column.key),
      hidden: orderedColumns.value
        .filter(column => hidden.value.has(column.key))
        .map(column => column.key)
    }
    return updateSettingMap(COLUMN_SETTINGS_KEY, current => ({
      ...current,
      [tableKey.value]: entry
    }))
  }

  function persistWidths() {
    if (!tableKey.value) return Promise.resolve()
    return updateSettingMap(WIDTH_SETTINGS_KEY, current => ({
      ...current,
      [tableKey.value]: { ...widths.value }
    }))
  }

  function persistInBackground(promise) {
    promise.catch((error) => {
      console.warn('Failed to persist resource table preferences:', error)
    })
  }

  function toggleColumn(key) {
    const next = new Set(hidden.value)
    if (next.has(key)) {
      next.delete(key)
    } else {
      if (visibleColumns.value.length <= 1) return
      next.add(key)
    }
    hidden.value = next
    persistInBackground(persistColumns())
  }

  function moveColumnTo(key, targetKey, placement = 'before') {
    if (!key || !targetKey || key === targetKey) return
    const keys = orderedColumns.value.map(column => column.key)
    const from = keys.indexOf(key)
    if (from < 0 || !keys.includes(targetKey)) return
    keys.splice(from, 1)
    const target = keys.indexOf(targetKey)
    const insertion = placement === 'after' ? target + 1 : target
    keys.splice(insertion, 0, key)
    order.value = keys
    persistInBackground(persistColumns())
  }

  function resetColumns() {
    order.value = []
    hidden.value = new Set()
    widths.value = {}
    explicitWidths.value = false
    if (!tableKey.value) return
    persistInBackground(updateSettingMap(COLUMN_SETTINGS_KEY, (current) => {
      const next = { ...current }
      delete next[tableKey.value]
      return next
    }))
    persistInBackground(updateSettingMap(WIDTH_SETTINGS_KEY, (current) => {
      const next = { ...current }
      delete next[tableKey.value]
      return next
    }))
  }

  function startColumnResize(index, event, headerElement = null) {
    const columns = visibleColumns.value
    if (index < 0 || index >= columns.length - 1) return
    removeResizeListeners?.()

    const header = headerElement || event.currentTarget?.closest?.('.table-head')
    const cells = header ? Array.from(header.children) : []
    const measured = cells.length === columns.length
      ? Object.fromEntries(cells.map((cell, cellIndex) => {
          const measuredColumn = columns[cellIndex]
          const minimum = Number(measuredColumn.minWidth) || MIN_WIDTH
          return [
            measuredColumn.key,
            normalizedWidth(
              cell.getBoundingClientRect().width,
              widths.value[measuredColumn.key] ?? measuredColumn.width,
              minimum
            )
          ]
        }))
      : {}
    const column = columns[index]
    const minimum = Number(column.minWidth) || MIN_WIDTH
    const startX = event.clientX
    const startWidth = normalizedWidth(measured[column.key], widths.value[column.key] ?? column.width, minimum)
    widths.value = { ...widths.value, ...measured }
    explicitWidths.value = true
    event.preventDefault?.()

    const handleMove = (moveEvent) => {
      widths.value = {
        ...widths.value,
        [column.key]: Math.max(minimum, Math.round(startWidth + moveEvent.clientX - startX))
      }
    }

    const handleUp = () => {
      removeResizeListeners?.()
      persistInBackground(persistWidths())
    }

    const pointerEvents = String(event.type || '').startsWith('pointer')
    const moveEvent = pointerEvents ? 'pointermove' : 'mousemove'
    const upEvent = pointerEvents ? 'pointerup' : 'mouseup'
    const cancelEvent = pointerEvents ? 'pointercancel' : null
    removeResizeListeners = () => {
      window.removeEventListener(moveEvent, handleMove)
      window.removeEventListener(upEvent, handleUp)
      if (cancelEvent) window.removeEventListener(cancelEvent, handleUp)
      removeResizeListeners = null
    }
    window.addEventListener(moveEvent, handleMove)
    window.addEventListener(upEvent, handleUp)
    if (cancelEvent) window.addEventListener(cancelEvent, handleUp)
  }

  return {
    orderedColumns,
    visibleColumns,
    hiddenColumns: hidden,
    tableStyle,
    toggleColumn,
    moveColumnTo,
    resetColumns,
    startColumnResize
  }
}
