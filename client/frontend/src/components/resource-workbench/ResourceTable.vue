<template>
  <div class="resource-table-frame" @scroll.passive="$emit('scroll', $event)">
    <div v-if="loading" class="resource-table__loading" role="status" aria-label="加载中">
      <span v-for="index in skeletonRows" :key="index" />
    </div>
    <div v-else-if="rows.length === 0" class="resource-table__empty">
      <slot name="empty" />
    </div>
    <div
      v-else
      class="resource-table motion-reveal"
      role="table"
      :aria-label="ariaLabel"
      :style="resolvedTableStyle"
    >
      <div
        class="resource-table__head table-head"
        role="row"
        title="拖动列名调整顺序，右键管理显示列"
        @contextmenu.prevent="openColumnMenu"
      >
        <div
          v-for="(column, columnIndex) in renderedColumns"
          :key="column.key"
          class="resource-table__heading"
          :class="[
            column.headerClass,
            {
              'is-header-dragging': draggedColumnKey === column.key,
              'is-drop-before': dropTargetKey === column.key && dropPlacement === 'before',
              'is-drop-after': dropTargetKey === column.key && dropPlacement === 'after'
            }
          ]"
          :data-column-key="column.key"
          role="columnheader"
          @dragover="handleHeaderDragOver(column.key, $event)"
          @drop="dropHeaderColumn(column.key, $event)"
        >
          <div
            class="resource-table__heading-drag"
            :draggable="Boolean(tableKey)"
            @dragstart="startHeaderDrag(column.key, $event)"
            @dragend="finishHeaderDrag"
          >
            <button
              v-if="column.sortable"
              type="button"
              class="resource-table__sort-button sortable"
              @click="handleHeaderSort(column.key, $event)"
            >
              <slot :name="`header-${column.key}`" :column="column">
                {{ column.label }}
              </slot>
            </button>
            <span
              v-else
              class="resource-table__heading-content"
            >
              <slot :name="`header-${column.key}`" :column="column">
                {{ column.label }}
              </slot>
            </span>
          </div>
          <span
            v-if="columnIndex < renderedColumns.length - 1"
            class="resource-table__resize-handle"
            role="separator"
            aria-orientation="vertical"
            :aria-label="`调整${column.label}列宽`"
            @click.stop
            @pointerdown.stop.prevent="handleColumnResizeStart(columnIndex, $event)"
          ></span>
        </div>
      </div>

      <div
        v-for="(row, rowIndex) in rows"
        :key="resolveRowKey(row, rowIndex)"
        class="resource-table__row resource-row"
        :class="[
          resolveRowClass(row),
          {
            selected: interactive && resolveRowKey(row, rowIndex) === selectedKey,
            'is-interactive': interactive,
            'is-static': !interactive
          }
        ]"
        :tabindex="interactive ? 0 : undefined"
        role="row"
        @click="handleRowSelect(row)"
        @keydown.enter="handleRowSelect(row)"
        @keydown.space="handleSpaceSelect(row, $event)"
      >
        <div
          v-for="column in renderedColumns"
          :key="column.key"
          class="resource-table__cell"
          :class="column.cellClass"
          role="cell"
        >
          <slot
            :name="`cell-${column.key}`"
            :row="row"
            :column="column"
            :value="row[column.key]"
            :row-index="rowIndex"
          >
            {{ row[column.key] ?? '-' }}
          </slot>
        </div>
      </div>
    </div>

    <Teleport to="body">
      <div
        v-if="columnMenuOpen"
        ref="columnMenuRef"
        class="resource-column-menu"
        :style="columnMenuStyle"
        role="menu"
        aria-label="列表列设置"
        @click.stop
        @contextmenu.prevent
      >
        <div class="resource-column-menu__header">
          <div>
            <strong>显示列</strong>
            <span>取消勾选可隐藏对应列</span>
          </div>
          <button type="button" title="恢复默认列" @click="resetColumnSettings">
            <DynamicIcon name="refresh" :size="14" />
          </button>
        </div>
        <div class="resource-column-menu__list">
          <div
            v-for="column in orderedColumns"
            :key="column.key"
            class="resource-column-menu__row"
            :data-column-key="column.key"
          >
            <AnimatedCheckbox
              :model-value="!hiddenColumns.has(column.key)"
              :disabled="!hiddenColumns.has(column.key) && renderedColumns.length <= 1"
              :label="column.label"
              @update:model-value="toggleColumn(column.key)"
            />
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, toRef } from 'vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import AnimatedCheckbox from '@/components/ui/AnimatedCheckbox.vue'
import { useResourceTableColumns } from '@/composables/useResourceTableColumns.js'

const props = defineProps({
  columns: {
    type: Array,
    default: () => []
  },
  rows: {
    type: Array,
    default: () => []
  },
  rowKey: {
    type: [String, Function],
    default: 'id'
  },
  selectedKey: {
    type: [String, Number],
    default: ''
  },
  rowClass: {
    type: Function,
    default: null
  },
  tableStyle: {
    type: Object,
    default: () => ({})
  },
  loading: {
    type: Boolean,
    default: false
  },
  skeletonRows: {
    type: Number,
    default: 6
  },
  ariaLabel: {
    type: String,
    default: '资源列表'
  },
  interactive: {
    type: Boolean,
    default: true
  },
  tableKey: {
    type: String,
    default: ''
  }
})

const emit = defineEmits(['select', 'sort', 'column-resize-start', 'scroll'])

const {
  orderedColumns,
  visibleColumns,
  hiddenColumns,
  tableStyle: managedTableStyle,
  toggleColumn,
  moveColumnTo,
  resetColumns,
  startColumnResize
} = useResourceTableColumns(toRef(props, 'columns'), toRef(props, 'tableKey'))

const renderedColumns = computed(() => props.tableKey ? visibleColumns.value : props.columns)
const resolvedTableStyle = computed(() => props.tableKey
  ? { ...props.tableStyle, ...managedTableStyle.value }
  : props.tableStyle)
const columnMenuOpen = ref(false)
const columnMenuRef = ref(null)
const columnMenuPosition = ref({ left: 0, top: 0 })
const draggedColumnKey = ref('')
const dropTargetKey = ref('')
const dropPlacement = ref('before')
let suppressSortUntil = 0
const columnMenuStyle = computed(() => ({
  left: `${columnMenuPosition.value.left}px`,
  top: `${columnMenuPosition.value.top}px`
}))

onMounted(() => {
  document.addEventListener('pointerdown', handleDocumentPointerDown)
  document.addEventListener('keydown', handleDocumentKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', handleDocumentPointerDown)
  document.removeEventListener('keydown', handleDocumentKeydown)
})

function handleColumnResizeStart(index, event) {
  const header = event.currentTarget?.closest?.('.table-head')
  emit('column-resize-start', index, event, header)
  if (props.tableKey) startColumnResize(index, event, header)
}

async function openColumnMenu(event) {
  if (!props.tableKey || props.columns.length === 0) return
  columnMenuPosition.value = { left: event.clientX, top: event.clientY }
  columnMenuOpen.value = true
  await nextTick()
  const rect = columnMenuRef.value?.getBoundingClientRect()
  if (!rect) return
  columnMenuPosition.value = {
    left: Math.max(8, Math.min(event.clientX, window.innerWidth - rect.width - 8)),
    top: Math.max(8, Math.min(event.clientY, window.innerHeight - rect.height - 8))
  }
}

function handleDocumentPointerDown(event) {
  if (columnMenuOpen.value && !columnMenuRef.value?.contains(event.target)) {
    columnMenuOpen.value = false
  }
}

function handleDocumentKeydown(event) {
  if (event.key === 'Escape') columnMenuOpen.value = false
}

function resetColumnSettings() {
  resetColumns()
}

function startHeaderDrag(key, event) {
  if (!props.tableKey) {
    event.preventDefault()
    return
  }
  draggedColumnKey.value = key
  dropTargetKey.value = ''
  event.dataTransfer?.setData('text/plain', key)
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
}

function handleHeaderDragOver(targetKey, event) {
  if (!draggedColumnKey.value || draggedColumnKey.value === targetKey) return
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  const rect = event.currentTarget?.getBoundingClientRect?.()
  dropTargetKey.value = targetKey
  dropPlacement.value = rect && rect.width > 0 && event.clientX >= rect.left + rect.width / 2
    ? 'after'
    : 'before'
}

function dropHeaderColumn(targetKey, event) {
  if (!draggedColumnKey.value || draggedColumnKey.value === targetKey) return
  event.preventDefault()
  moveColumnTo(draggedColumnKey.value, targetKey, dropPlacement.value)
  suppressSortUntil = Date.now() + 180
  finishHeaderDrag()
}

function finishHeaderDrag() {
  draggedColumnKey.value = ''
  dropTargetKey.value = ''
  dropPlacement.value = 'before'
}

function handleHeaderSort(key, event) {
  if (Date.now() < suppressSortUntil) {
    event.preventDefault()
    return
  }
  emit('sort', key)
}

function resolveRowKey(row, index) {
  if (typeof props.rowKey === 'function') return props.rowKey(row, index)
  return row?.[props.rowKey] ?? index
}

function resolveRowClass(row) {
  return props.rowClass ? props.rowClass(row) : ''
}

function handleRowSelect(row) {
  if (props.interactive) emit('select', row)
}

function handleSpaceSelect(row, event) {
  if (!props.interactive) return
  event.preventDefault()
  emit('select', row)
}
</script>

<style scoped>
.resource-table-frame {
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow: auto;
  scrollbar-gutter: stable;
}

.resource-table {
  --resource-table-head-height: 42px;
  --resource-table-body-size: 0.8125rem;
  --resource-table-name-size: 0.8125rem;
  --resource-table-meta-size: 0.6875rem;
  box-sizing: border-box;
  display: grid;
  width: 100%;
  min-width: max(100%, var(--resource-table-min-width, 100%));
  padding: 0 12px 10px;
  color: var(--resource-ink, var(--text-primary));
  font-family: var(--font-sans, Inter, ui-sans-serif, system-ui, sans-serif);
  font-size: var(--resource-table-body-size);
}

.resource-table__head,
.resource-table__row {
  box-sizing: border-box;
  display: grid;
  grid-template-columns: var(--resource-grid-template, minmax(180px, 1fr));
  align-items: center;
  gap: 8px;
  width: 100%;
}

.resource-table__head {
  position: sticky;
  top: 0;
  z-index: 3;
  min-height: var(--resource-table-head-height);
  padding: 0 10px;
  border-bottom: 1px solid var(--resource-line, var(--border-subtle));
  background: color-mix(in srgb, var(--bg-secondary) 92%, transparent);
}

.resource-table__heading {
  position: relative;
  z-index: 1;
  min-width: 0;
  min-height: 100%;
  display: flex;
  align-items: center;
  padding: 0;
  color: var(--resource-muted, var(--text-secondary));
  font-size: var(--resource-table-meta-size);
  font-weight: 700;
  letter-spacing: 0.02em;
  text-align: left;
  text-transform: uppercase;
  white-space: nowrap;
}

.resource-table__heading-drag {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  min-height: 100%;
  display: flex;
  align-items: center;
}

.resource-table__heading-drag[draggable="true"] {
  cursor: grab;
  user-select: none;
}

.resource-table__heading-drag[draggable="true"]:active {
  cursor: grabbing;
}

.resource-table__heading.is-header-dragging {
  opacity: 0.45;
}

.resource-table__heading.is-drop-before::before,
.resource-table__heading.is-drop-after::after {
  content: "";
  position: absolute;
  z-index: 11;
  top: 7px;
  bottom: 7px;
  width: 2px;
  border-radius: 999px;
  background: var(--color-primary-500);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-primary-500) 16%, transparent);
  pointer-events: none;
}

.resource-table__heading.is-drop-before::before {
  left: -5px;
}

.resource-table__heading.is-drop-after::after {
  right: -5px;
}

.resource-table__sort-button,
.resource-table__heading-content {
  box-sizing: border-box;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  width: 100%;
  min-width: 0;
  min-height: 100%;
  padding: 0;
  overflow: hidden;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  letter-spacing: inherit;
  text-align: left;
  text-transform: inherit;
  white-space: nowrap;
}

.resource-table__sort-button.sortable {
  cursor: inherit;
  user-select: none;
}

.resource-table__row.is-interactive {
  cursor: pointer;
}

.resource-table__sort-button.sortable:hover {
  color: var(--resource-ink, var(--text-primary));
}

.resource-table__resize-handle {
  position: absolute;
  z-index: 8;
  top: 0;
  right: -9px;
  bottom: 0;
  width: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: col-resize;
  touch-action: none;
}

.resource-table__resize-handle::after {
  content: "";
  width: 1px;
  height: 22px;
  border-radius: 999px;
  background: var(--resource-line-strong, var(--border-default));
  opacity: 0.55;
  transition: width 0.14s ease, opacity 0.14s ease, background 0.14s ease;
}

.resource-table__resize-handle:hover::after,
.resource-table__resize-handle:active::after {
  width: 2px;
  background: var(--color-primary-500);
  opacity: 1;
}

.resource-table__row {
  min-height: 68px;
  padding: 0 10px;
  border: 0;
  border-bottom: 1px solid var(--resource-line, var(--border-subtle));
  background: transparent;
  color: var(--resource-ink, var(--text-primary));
  cursor: default;
  outline: none;
  transform-origin: center;
  transition:
    transform var(--motion-duration-quick) var(--motion-ease-out),
    background var(--motion-duration-quick) var(--motion-ease-out),
    box-shadow var(--motion-duration-quick) var(--motion-ease-out);
}

.resource-table__row.is-static {
  cursor: default;
}

.resource-table__row:hover,
.resource-table__row.selected {
  background: var(--resource-row-hover, var(--bg-hover));
}

.resource-table__row.is-interactive:active {
  transform: scale(0.992);
}

.resource-table__row.selected {
  box-shadow: inset 4px 0 0 var(--resource-accent, var(--color-success-500));
  animation: resource-table-select var(--motion-duration-fast) var(--motion-ease-out);
}

.resource-table__row:focus-visible {
  background: var(--resource-row-hover, var(--bg-hover));
  outline: 2px solid color-mix(in srgb, var(--resource-accent, var(--color-success-500)) 36%, transparent);
  outline-offset: -2px;
}

.resource-table__cell {
  min-width: 0;
  min-height: 68px;
  display: flex;
  align-items: center;
  overflow: hidden;
  font-family: var(--font-sans, Inter, ui-sans-serif, system-ui, sans-serif);
  font-size: var(--resource-table-body-size);
  font-weight: 400;
  line-height: 1.5;
}

.resource-table :deep(.resource-name-button) {
  box-sizing: border-box;
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  min-width: 0;
  padding: 0;
  overflow: hidden;
  border: 0;
  background: transparent;
  color: var(--resource-ink, var(--text-primary));
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.resource-table :deep(.resource-kind) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 34px;
  height: 34px;
  border-radius: 10px;
  color: var(--color-success-700);
  background: var(--color-success-100);
}

.resource-table :deep(.resource-kind.image) {
  color: var(--color-primary-700);
  background: var(--color-primary-100);
}

.resource-table :deep(.resource-kind.volume) {
  color: var(--color-warning-700);
  background: var(--color-warning-100);
}

.resource-table :deep(.resource-kind.network) {
  color: var(--color-info-700);
  background: var(--color-info-100);
}

.resource-table :deep(.resource-name-copy) {
  display: grid;
  flex: 1 1 auto;
  gap: 2px;
  min-width: 0;
  padding-block: 2px;
}

.resource-table :deep(.resource-name-copy strong) {
  overflow: hidden;
  color: var(--resource-ink, var(--text-primary));
  font-size: var(--resource-table-name-size);
  font-weight: 650;
  line-height: 1.25;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.resource-table :deep(.resource-name-copy small) {
  overflow: hidden;
  color: var(--text-secondary);
  font-family: var(--font-sans, Inter, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif);
  font-size: var(--resource-table-meta-size);
  font-weight: 400;
  line-height: 1.5;
  letter-spacing: 0;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.resource-table :deep(.cell-ellipsis) {
  display: block;
  min-width: 0;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.resource-table :deep(.cell-mono) {
  font-family: var(--font-mono, 'JetBrains Mono', 'Fira Code', ui-monospace, monospace);
  font-size: 0.75rem;
  font-weight: 400;
  line-height: 1.5;
}

.resource-table :deep(.cell-actions) {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 6px;
  min-width: 0;
}

.resource-table__loading {
  display: grid;
  gap: 1px;
  padding: 0 12px;
}

.resource-table__loading span {
  height: 68px;
  border-bottom: 1px solid var(--resource-line, var(--border-subtle));
  background: linear-gradient(90deg, transparent 15%, var(--bg-tertiary) 45%, transparent 75%);
  background-size: 220% 100%;
  animation: resource-table-loading 1.4s linear infinite;
}

.resource-table__empty {
  min-height: 320px;
  display: grid;
  place-items: center;
  padding: 32px;
}

.resource-column-menu {
  position: fixed;
  z-index: 4200;
  width: 276px;
  max-height: min(520px, calc(100vh - 16px));
  overflow: hidden;
  border: 1px solid var(--border-default);
  border-radius: 8px;
  background: var(--bg-elevated);
  color: var(--text-primary);
  box-shadow: var(--shadow-xl);
  font-family: var(--font-sans, Inter, ui-sans-serif, system-ui, sans-serif);
}

.resource-column-menu__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px;
  border-bottom: 1px solid var(--border-subtle);
}

.resource-column-menu__header > div {
  display: grid;
  gap: 2px;
  min-width: 0;
}

.resource-column-menu__header strong {
  font-size: 0.8125rem;
  line-height: 1.2;
}

.resource-column-menu__header span {
  color: var(--text-tertiary);
  font-size: 0.6875rem;
}

.resource-column-menu__header button {
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  padding: 0;
  border: 1px solid transparent;
  border-radius: 6px;
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
}

.resource-column-menu__header button:hover {
  border-color: var(--border-subtle);
  background: var(--bg-hover);
  color: var(--color-primary-600);
}

.resource-column-menu__list {
  max-height: 420px;
  overflow-y: auto;
  padding: 6px;
}

.resource-column-menu__row {
  display: flex;
  align-items: center;
  min-height: 36px;
  padding: 3px 8px;
  border-radius: 6px;
}

.resource-column-menu__row:hover {
  background: var(--bg-hover);
}

.resource-column-menu__row label {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  color: var(--text-primary);
  font-size: 0.78125rem;
  cursor: pointer;
}

.resource-column-menu__row label span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.resource-column-menu__row input {
  width: 14px;
  height: 14px;
  accent-color: var(--color-primary-500);
}

@keyframes resource-table-select {
  0% { transform: scale(1); }
  44% { transform: scale(0.988); }
  100% { transform: scale(1); }
}

@keyframes resource-table-loading {
  from { background-position: 220% 0; }
  to { background-position: -120% 0; }
}

@media (prefers-reduced-motion: reduce) {
  .resource-table__row,
  .resource-table__row.selected,
  .resource-table__loading span {
    animation: none;
    transition: none;
  }
}
</style>
