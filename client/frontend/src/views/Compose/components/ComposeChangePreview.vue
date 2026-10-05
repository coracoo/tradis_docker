<template>
  <div v-if="preview?.hasChanges" class="change-preview">
    <div class="change-summary" aria-label="配置变更摘要">
      <strong>{{ summary.total }} 项变更</strong>
      <span v-if="summary.added" class="summary-added">新增 {{ summary.added }}</span>
      <span v-if="summary.modified" class="summary-modified">修改 {{ summary.modified }}</span>
      <span v-if="summary.removed" class="summary-removed">删除 {{ summary.removed }}</span>
    </div>

    <div class="change-groups" role="list">
      <section
        v-for="group in groups"
        :key="group.key"
        class="change-group"
        :data-group="group.key"
        role="listitem"
      >
        <header class="change-group-header">
          <span>{{ group.label || group.key }}</span>
          <small>{{ group.changes.length }}</small>
        </header>

        <div class="change-list">
          <div
            v-for="change in group.changes"
            :key="`${change.path}:${change.kind}`"
            class="change-row"
            :class="`is-${change.kind}`"
            :data-field="change.field"
          >
            <span class="change-rail" aria-hidden="true"></span>
            <div class="change-identity">
              <strong>{{ change.field }}</strong>
              <small v-if="change.path && change.path !== change.field">{{ change.path }}</small>
            </div>
            <div v-if="hasValues(change)" class="change-values">
              <code v-if="change.before" class="change-before">{{ change.before }}</code>
              <span v-if="change.before && change.after" aria-hidden="true">→</span>
              <code v-if="change.after" class="change-after">{{ change.after }}</code>
            </div>
            <span class="change-kind">{{ kindLabel(change.kind) }}</span>
          </div>
        </div>
      </section>
    </div>
  </div>

  <div v-else class="change-preview-empty">
    配置未发生变化
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  preview: {
    type: Object,
    default: () => ({})
  }
})

const summary = computed(() => props.preview?.summary || { total: 0, added: 0, modified: 0, removed: 0 })
const groups = computed(() => Array.isArray(props.preview?.groups) ? props.preview.groups : [])

function hasValues (change) {
  return Boolean(change?.before || change?.after)
}

function kindLabel (kind) {
  return ({ added: '已新增', modified: '已修改', removed: '已删除' })[kind] || '已变更'
}
</script>

<style scoped>
.change-preview {
  min-width: 0;
  color: var(--text-primary);
}

.change-summary {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px 16px;
  min-height: 38px;
  padding: 0 2px 12px;
  color: var(--text-secondary);
  font-size: 0.8125rem;
}

.change-summary strong {
  color: var(--text-primary);
  font-size: 0.875rem;
  font-weight: 600;
}

.summary-added { color: var(--color-success-600); }
.summary-modified { color: var(--color-warning-600); }
.summary-removed { color: var(--color-danger-600); }

.change-groups {
  border-block: 1px solid var(--border-subtle);
}

.change-group + .change-group {
  border-top: 1px solid var(--border-subtle);
}

.change-group-header {
  min-height: 38px;
  padding: 0 12px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  background: color-mix(in srgb, var(--bg-secondary) 74%, var(--bg-elevated));
  color: var(--text-secondary);
  font-size: 0.8125rem;
  font-weight: 600;
}

.change-group-header small {
  color: var(--text-tertiary);
  font: 500 0.75rem/1 var(--font-mono, ui-monospace, monospace);
}

.change-row {
  position: relative;
  min-height: 52px;
  display: grid;
  grid-template-columns: minmax(150px, 0.85fr) minmax(220px, 1.5fr) auto;
  align-items: center;
  gap: 14px;
  padding: 8px 12px 8px 16px;
  background: var(--bg-elevated);
}

.change-row + .change-row {
  border-top: 1px solid var(--border-subtle);
}

.change-rail {
  position: absolute;
  left: 0;
  top: 10px;
  bottom: 10px;
  width: 3px;
  background: var(--color-warning-500);
}

.change-row.is-added .change-rail { background: var(--color-success-500); }
.change-row.is-removed .change-rail { background: var(--color-danger-500); }

.change-identity {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.change-identity strong {
  overflow: hidden;
  color: var(--text-primary);
  font: 600 0.8125rem/1.35 var(--font-mono, ui-monospace, monospace);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.change-identity small {
  overflow: hidden;
  color: var(--text-tertiary);
  font-size: 0.75rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.change-values {
  min-width: 0;
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  color: var(--text-tertiary);
}

.change-values code {
  overflow: hidden;
  padding: 5px 7px;
  border: 1px solid var(--border-subtle);
  border-radius: 5px;
  background: var(--bg-primary);
  color: var(--text-secondary);
  font: 500 0.75rem/1.35 var(--font-mono, ui-monospace, monospace);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.change-after {
  border-color: color-mix(in srgb, var(--color-primary-500) 32%, var(--border-subtle)) !important;
  color: var(--text-primary) !important;
}

.change-kind {
  justify-self: end;
  color: var(--text-secondary);
  font-size: 0.75rem;
  white-space: nowrap;
}

.is-added .change-kind { color: var(--color-success-600); }
.is-modified .change-kind { color: var(--color-warning-600); }
.is-removed .change-kind { color: var(--color-danger-600); }

.change-preview-empty {
  min-height: 120px;
  display: grid;
  place-items: center;
  border-block: 1px solid var(--border-subtle);
  color: var(--text-tertiary);
  font-size: 0.875rem;
}

@media (max-width: 720px) {
  .change-row {
    grid-template-columns: minmax(0, 1fr) auto;
  }

  .change-values {
    grid-column: 1 / -1;
    grid-row: 2;
  }
}
</style>
