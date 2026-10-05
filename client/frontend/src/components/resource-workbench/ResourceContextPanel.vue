<template>
  <section class="resource-context-panel-shell">
    <Transition name="resource-context-content" mode="out-in">
      <div
        :key="String(motionKey)"
        class="resource-context-panel-content"
        :data-motion-key="String(motionKey)"
      >
        <slot />
      </div>
    </Transition>
  </section>
</template>

<script setup>
defineProps({
  motionKey: {
    type: [String, Number],
    default: 'overview'
  }
})
</script>

<style scoped>
.resource-context-panel-shell {
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  width: 100%;
  min-width: 0;
  min-height: 0;
  padding: 16px;
  overflow: auto;
  scrollbar-gutter: stable;
  border: 1px solid var(--resource-line, var(--border-subtle));
  border-radius: 14px;
  background: var(--resource-panel, var(--bg-primary));
  box-shadow: 0 18px 48px color-mix(in srgb, var(--text-primary) 7%, transparent);
}

.resource-context-panel-content {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 16px;
  width: 100%;
  min-width: 0;
  min-height: 0;
}

.resource-context-content-enter-active {
  transition:
    opacity var(--motion-duration-fast) var(--motion-ease-out),
    transform var(--motion-duration-fast) var(--motion-ease-out);
}

.resource-context-content-leave-active {
  transition:
    opacity var(--motion-duration-micro) var(--motion-ease-out),
    transform var(--motion-duration-micro) var(--motion-ease-out);
}

.resource-context-content-enter-from {
  opacity: 0;
  transform: translateY(var(--motion-distance-medium));
}

.resource-context-content-leave-to {
  opacity: 0;
  transform: translateY(calc(var(--motion-distance-micro) * -1));
}

.resource-context-panel-shell :deep(.context-header) {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.resource-context-panel-shell :deep(.context-header > div) {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.resource-context-panel-shell :deep(.context-header strong),
.resource-context-panel-shell :deep(.context-header span) {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.resource-context-panel-shell :deep(.context-header strong) {
  color: var(--resource-ink);
  font-size: 0.98rem;
  font-weight: 800;
}

.resource-context-panel-shell :deep(.context-header span) {
  color: var(--resource-muted);
  font-size: 0.75rem;
}

.resource-context-panel-shell :deep(.resource-kind) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 34px;
  height: 34px;
  border-radius: 10px;
  color: var(--resource-accent-strong);
  background: var(--resource-accent-soft);
}

.resource-context-panel-shell :deep(.context-actions) {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(96px, 1fr));
  gap: 8px;
}

.resource-context-panel-shell :deep(.context-actions:has(> :nth-child(4))) {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.resource-context-panel-shell :deep(.detail-action) {
  min-height: 36px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-width: 0;
  padding: 0 10px;
  border: 1px solid var(--resource-line);
  border-radius: 9px;
  background: var(--resource-surface);
  color: var(--resource-ink);
  font-size: 0.76rem;
  font-weight: 700;
  cursor: pointer;
  transition: border-color 0.16s ease, background 0.16s ease, color 0.16s ease, transform 0.16s ease;
}

.resource-context-panel-shell :deep(.detail-action.info) {
  border-color: var(--color-info-200);
  background: var(--color-info-50);
  color: var(--color-info-700);
}

.resource-context-panel-shell :deep(.detail-action.success) {
  border-color: var(--color-success-200);
  color: var(--color-success-700);
  background: var(--color-success-50);
}

.resource-context-panel-shell :deep(.detail-action.warning) {
  border-color: var(--color-warning-200);
  color: var(--color-warning-700);
  background: var(--color-warning-50);
}

.resource-context-panel-shell :deep(.detail-action.danger),
.resource-context-panel-shell :deep(.detail-action.stop) {
  border-color: var(--color-danger-200);
  color: var(--color-danger-600);
  background: var(--color-danger-50);
}

.resource-context-panel-shell :deep(.detail-action.terminal) {
  border-color: var(--color-info-200);
  background: var(--color-info-50);
  color: var(--color-info-700);
}

.resource-context-panel-shell :deep(.detail-action:hover:not(:disabled)) {
  transform: translateY(-1px);
  filter: saturate(1.08);
}

.resource-context-panel-shell :deep(.detail-action:disabled) {
  cursor: not-allowed;
  opacity: 0.45;
}

.resource-context-panel-shell :deep(.context-grid) {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.resource-context-panel-shell :deep(.context-grid > div),
.resource-context-panel-shell :deep(.context-section) {
  box-sizing: border-box;
  min-width: 0;
  padding: 10px;
  border: 1px solid var(--resource-line);
  border-radius: 10px;
  background: var(--resource-surface);
}

.resource-context-panel-shell :deep(.context-grid span),
.resource-context-panel-shell :deep(.section-label) {
  margin: 0;
  color: var(--resource-muted);
  font-size: 0.68rem;
  font-weight: 800;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.resource-context-panel-shell :deep(.context-grid strong) {
  display: block;
  overflow: hidden;
  margin-top: 4px;
  color: var(--resource-ink);
  font-size: 0.92rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.resource-context-panel-shell :deep(.detail-lines) {
  display: grid;
  gap: 8px;
  margin: 10px 0 0;
  padding: 0;
  list-style: none;
}

.resource-context-panel-shell :deep(.detail-lines span) {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
  color: var(--resource-ink);
  font-size: 0.76rem;
}

.resource-context-panel-shell :deep(.detail-lines span) {
  overflow-wrap: anywhere;
  word-break: break-word;
}

.resource-context-panel-shell :deep(.detail-lines b) {
  flex: 0 0 auto;
  color: var(--resource-muted);
  font-family: var(--font-mono, 'JetBrains Mono', ui-monospace, monospace);
  font-size: 0.66rem;
}

.resource-context-panel-shell :deep(.live-log-preview.log-stream-preview) {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  max-width: 100%;
  overflow: hidden;
}

@media (max-width: 1500px) {
  .resource-context-panel-shell {
    min-height: 360px;
  }
}
</style>
