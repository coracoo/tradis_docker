<template>
  <div class="resource-workbench" :data-remote-readonly="showRemoteContext ? 'true' : 'false'">
    <section v-if="$slots.rail" class="resource-workbench__rail">
      <slot name="rail" />
    </section>
    <section v-if="$slots.toolbar" class="resource-workbench__toolbar">
      <slot name="toolbar" />
    </section>
    <div
      v-if="showRemoteContext"
      class="resource-workbench__remote-context"
      :class="remoteContextClass"
      data-test="remote-context"
      role="status"
    >
      <span class="remote-context-dot" aria-hidden="true"></span>
      <strong>{{ environment.name }}</strong>
      <span>{{ remoteContextLabel }}</span>
    </div>
    <div class="resource-workbench__body">
      <slot />
    </div>
  </div>
</template>

<script setup>
import { useEditionResourceWorkbenchContext } from '@edition/resource-workbench-context'

const props = defineProps({
  remoteContext: { type: Boolean, default: false }
})

const {
  environment,
  showRemoteContext,
  isRemoteReadOnly,
  remoteContextClass,
  remoteContextLabel
} = useEditionResourceWorkbenchContext(() => props.remoteContext)
</script>

<style scoped>
.resource-workbench {
  --ops-bg: var(--bg-secondary);
  --ops-surface: var(--bg-elevated);
  --ops-panel: var(--bg-primary);
  --ops-ink: var(--text-primary);
  --ops-muted: var(--text-secondary);
  --ops-line: var(--border-subtle);
  --ops-strong-line: var(--border-default);
  --ops-green: var(--color-success-500);
  --ops-green-strong: var(--color-success-700);
  --ops-cyan: var(--color-info-600);
  --ops-blue: var(--color-primary-600);
  --ops-amber: var(--color-warning-600);
  --ops-red: var(--color-danger-600);
  --ops-green-soft: var(--color-success-100);
  --ops-green-chip: var(--color-success-200);
  --ops-blue-soft: var(--color-primary-100);
  --ops-cyan-soft: var(--color-info-100);
  --ops-amber-soft: var(--color-warning-100);
  --ops-muted-soft: var(--bg-tertiary);
  --ops-row-hover: color-mix(in srgb, var(--color-success-500) 5%, var(--bg-elevated));
  --ops-row-active: var(--ops-row-hover);
  --ops-row-focus: var(--ops-row-hover);
  --resource-surface: color-mix(in srgb, var(--bg-elevated) 88%, transparent);
  --resource-panel: color-mix(in srgb, var(--bg-primary) 92%, transparent);
  --resource-line: var(--border-subtle);
  --resource-line-strong: var(--border-default);
  --resource-ink: var(--text-primary);
  --resource-muted: var(--text-secondary);
  --resource-accent: var(--color-success-500);
  --resource-accent-strong: var(--color-success-700);
  --resource-accent-soft: var(--color-success-100);
  --resource-row-hover: var(--ops-row-hover);
  --resource-toolbar-search-width: 300px;
  --resource-toolbar-sort-width: 170px;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: 14px;
  width: 100%;
  height: 100%;
  min-height: 0;
  padding: 22px 24px 0;
  overflow: hidden;
  color: var(--resource-ink);
  background: transparent;
  font-family: var(--font-sans, Inter, ui-sans-serif, system-ui, sans-serif);
}

.resource-workbench__rail,
.resource-workbench__toolbar,
.resource-workbench__remote-context,
.resource-workbench__body {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
}

.resource-workbench__remote-context {
  display: flex;
  align-items: center;
  gap: 7px;
  flex: 0 0 auto;
  min-height: 30px;
  padding: 6px 10px;
  border: 1px solid color-mix(in srgb, var(--ops-cyan) 24%, var(--resource-line));
  border-radius: 8px;
  background: color-mix(in srgb, var(--ops-cyan-soft) 58%, var(--resource-surface));
  color: var(--resource-muted);
  font-size: 0.7rem;
}

.resource-workbench__remote-context strong {
  color: var(--resource-ink);
  font-size: 0.72rem;
  font-weight: 700;
}

.remote-context-dot {
  flex: 0 0 7px;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--ops-green-strong);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--ops-green) 14%, transparent);
}

.resource-workbench__remote-context.is-stale,
.resource-workbench__remote-context.is-offline {
  border-color: color-mix(in srgb, var(--ops-amber) 30%, var(--resource-line));
  background: color-mix(in srgb, var(--ops-amber-soft) 56%, var(--resource-surface));
}

.resource-workbench__remote-context.is-stale .remote-context-dot,
.resource-workbench__remote-context.is-offline .remote-context-dot {
  background: var(--ops-amber);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--ops-amber) 14%, transparent);
}

.resource-workbench[data-remote-readonly='true'] :deep([data-remote-write]) {
  display: none !important;
}

.resource-workbench__toolbar {
  container-type: inline-size;
}

.resource-workbench__rail,
.resource-workbench__toolbar {
  flex: 0 0 auto;
}

.resource-workbench__body {
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
  scrollbar-gutter: stable;
}

.resource-workbench__rail :deep(.resource-rail),
.resource-workbench__rail :deep(.compose-rail) {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 8px;
  width: 100%;
  max-width: none;
  margin: 0;
}

.resource-workbench__rail :deep(.rail-card) {
  box-sizing: border-box;
  display: grid;
  grid-template-columns: 34px minmax(0, 1fr);
  grid-template-areas:
    "icon copy"
    "icon note";
  align-items: center;
  gap: 2px 10px;
  min-height: 64px;
  padding: 10px 12px;
  border: 1px solid var(--resource-line);
  border-radius: 10px;
  background: var(--resource-surface);
  box-shadow: 0 12px 34px color-mix(in srgb, var(--text-primary) 5%, transparent);
  backdrop-filter: none;
}

.resource-workbench__rail :deep(.rail-icon) {
  grid-area: icon;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 34px;
  width: 34px;
  height: 34px;
  border-radius: 9px;
}

.resource-workbench__rail :deep(.rail-copy) {
  grid-area: copy;
  display: flex;
  flex-direction: row;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}

.resource-workbench__rail :deep(.rail-copy strong) {
  flex: 0 0 auto;
  font-size: 1.18rem;
  line-height: 1;
  font-weight: 800;
}

.resource-workbench__rail :deep(.rail-copy span),
.resource-workbench__rail :deep(.rail-card small) {
  overflow: hidden;
  color: var(--resource-muted);
  font-size: 0.7rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.resource-workbench__rail :deep(.rail-card small) {
  grid-area: note;
}

.resource-workbench__toolbar :deep(.workbench-toolbar) {
  box-sizing: border-box;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
  min-height: 66px;
  max-width: none;
  margin: 0;
  padding: 12px;
  border: 1px solid var(--resource-line);
  border-radius: 12px;
  background: var(--resource-surface);
  box-shadow: 0 12px 34px color-mix(in srgb, var(--text-primary) 5%, transparent);
  backdrop-filter: none;
}

.resource-workbench__toolbar :deep(.workbench-toolbar-left) {
  display: flex;
  align-items: center;
  gap: 12px;
  flex: 1 1 auto;
  min-width: 0;
}

.resource-workbench__toolbar :deep(.workbench-toolbar-left > .search-input-wrapper) {
  box-sizing: border-box;
  flex: 0 0 var(--resource-toolbar-search-width);
  width: var(--resource-toolbar-search-width);
  min-width: var(--resource-toolbar-search-width);
  max-width: var(--resource-toolbar-search-width);
}

.resource-workbench__toolbar :deep(.workbench-toolbar-left > .sort-control) {
  box-sizing: border-box;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex: 0 0 var(--resource-toolbar-sort-width);
  width: var(--resource-toolbar-sort-width);
  min-width: var(--resource-toolbar-sort-width);
  max-width: var(--resource-toolbar-sort-width);
  min-height: 38px;
  padding: 0 8px 0 10px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--bg-elevated);
  color: var(--text-secondary);
}

.resource-workbench__toolbar :deep(.workbench-toolbar-left > .sort-control select) {
  flex: 1;
  width: 0;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--text-primary);
  font-family: var(--font-sans, Inter, ui-sans-serif, system-ui, sans-serif);
  font-size: 0.8125rem;
  font-weight: 400;
  cursor: pointer;
}

.resource-workbench__toolbar :deep(.workbench-toolbar-left > .sort-control select option) {
  background: var(--bg-elevated);
  color: var(--text-primary);
}

.resource-workbench__toolbar :deep(.workbench-toolbar-left > .sort-control .sort-direction-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 28px;
  width: 28px;
  height: 28px;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  cursor: pointer;
}

.resource-workbench__toolbar :deep(.workbench-toolbar-left > .sort-control .sort-direction-btn:hover:not(:disabled)) {
  background: var(--color-primary-100);
  color: var(--color-primary-600);
}

.resource-workbench__toolbar :deep(.toolbar-right) {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  flex: 0 0 auto;
  min-width: 0;
}

.resource-workbench__toolbar :deep(.secondary-btn) {
  box-sizing: border-box;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  flex: 0 0 auto;
  min-height: 38px;
  padding: 0 14px;
  border: 1px solid var(--border-default);
  border-radius: 9px;
  background: var(--bg-elevated);
  color: var(--text-primary);
  font-family: var(--font-sans, Inter, ui-sans-serif, system-ui, sans-serif);
  font-size: 0.875rem;
  font-weight: 500;
  line-height: 1;
  white-space: nowrap;
  cursor: pointer;
  transition: background-color 0.16s ease, border-color 0.16s ease, color 0.16s ease;
}

.resource-workbench__toolbar :deep(.secondary-btn:hover:not(:disabled)) {
  border-color: var(--color-primary-300);
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

.resource-workbench__toolbar :deep(.secondary-btn:focus-visible) {
  outline: 3px solid color-mix(in srgb, var(--color-primary-500) 24%, transparent);
  outline-offset: 2px;
}

.resource-workbench__toolbar :deep(.secondary-btn:disabled) {
  cursor: not-allowed;
  opacity: 0.55;
}

.resource-workbench__toolbar :deep(.primary-btn) {
  box-sizing: border-box;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  flex: 0 0 auto;
  min-height: 38px;
  padding: 0 14px;
  border: 1px solid var(--color-primary-700);
  border-radius: 10px;
  background: linear-gradient(135deg, var(--color-primary-500), var(--color-primary-600));
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--text-inverse) 16%, transparent);
  color: var(--text-inverse);
  font-family: var(--font-sans, Inter, ui-sans-serif, system-ui, sans-serif);
  font-size: 0.875rem;
  font-weight: 600;
  line-height: 1;
  white-space: nowrap;
  cursor: pointer;
  transition: transform 0.16s ease, border-color 0.16s ease, background 0.16s ease, box-shadow 0.16s ease;
}

.resource-workbench__toolbar :deep(.primary-btn:hover:not(:disabled)) {
  transform: translateY(-1px);
  border-color: color-mix(in srgb, var(--color-primary-700) 82%, black);
  background: linear-gradient(135deg, var(--color-primary-600), var(--color-primary-700));
  box-shadow:
    inset 0 0 0 1px color-mix(in srgb, var(--text-inverse) 20%, transparent),
    0 5px 14px color-mix(in srgb, var(--color-primary-500) 28%, transparent);
}

.resource-workbench__toolbar :deep(.primary-btn:focus-visible) {
  outline: 3px solid color-mix(in srgb, var(--color-primary-500) 28%, transparent);
  outline-offset: 2px;
}

.resource-workbench__toolbar :deep(.primary-btn:disabled) {
  cursor: not-allowed;
  opacity: 0.55;
}

@media (max-width: 900px) {
  .resource-workbench {
    height: auto;
    min-height: 100%;
    padding: 16px 16px 0;
    overflow: visible;
  }

  .resource-workbench__rail :deep(.resource-rail),
  .resource-workbench__rail :deep(.compose-rail) {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .resource-workbench__toolbar :deep(.workbench-toolbar) {
    align-items: stretch;
    flex-direction: column;
  }

  .resource-workbench__toolbar :deep(.workbench-toolbar-left) {
    align-items: stretch;
    flex-wrap: wrap;
    width: 100%;
  }

  .resource-workbench__toolbar :deep(.workbench-toolbar-left > .search-input-wrapper) {
    flex: 1 1 260px;
    width: auto;
    min-width: 0;
    max-width: none;
  }
}

@media (max-width: 1280px) {
  .resource-workbench__toolbar :deep(.workbench-toolbar) {
    align-items: stretch;
    flex-direction: column;
  }

  .resource-workbench__toolbar :deep(.workbench-toolbar-left) {
    flex: 0 0 auto;
    width: 100%;
  }

  .resource-workbench__toolbar :deep(.toolbar-right) {
    justify-content: flex-end;
    width: 100%;
  }
}

</style>
