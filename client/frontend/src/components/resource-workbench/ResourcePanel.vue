<template>
  <section class="resource-panel">
    <div class="resource-panel__body">
      <slot />
    </div>
    <footer v-if="$slots.footer" class="resource-panel__footer" data-resource-pagination>
      <slot name="footer" />
    </footer>
  </section>
</template>

<style scoped>
.resource-panel {
  box-sizing: border-box;
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  border: 1px solid var(--resource-line, var(--border-subtle));
  border-radius: 14px;
  background: var(--resource-panel, var(--bg-primary));
  box-shadow: 0 18px 48px color-mix(in srgb, var(--text-primary) 7%, transparent);
}

.resource-panel__body {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
  overflow: auto;
  scrollbar-gutter: stable;
}

.resource-panel__body :deep(.card-grid-wrapper) {
  box-sizing: border-box;
  width: 100%;
  min-height: 100%;
  padding: 16px;
}

.resource-panel__body :deep(.card-grid) {
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  align-content: start;
  align-items: stretch;
  gap: 16px;
  padding: 0;
}

.resource-panel__body :deep(.card-grid > *) {
  min-height: 236px;
}

.resource-panel__body :deep(.card-grid > *:hover) {
  transform: none;
}

.resource-panel__body :deep(.card-grid > .is-selected) {
  border-color: var(--resource-accent, var(--color-success-500));
  box-shadow:
    inset 4px 0 0 var(--resource-accent, var(--color-success-500)),
    0 16px 36px color-mix(in srgb, var(--resource-accent, var(--color-success-500)) 12%, transparent);
  animation: resource-card-select var(--motion-duration-fast) var(--motion-ease-out);
}

.resource-panel__footer {
  flex: 0 0 auto;
  padding: 10px 14px;
  border-top: 1px solid var(--resource-line, var(--border-subtle));
  background: var(--resource-surface, var(--bg-elevated));
}

@keyframes resource-card-select {
  0% { transform: scale(0.985); }
  70% { transform: scale(1.006); }
  100% { transform: scale(1); }
}

@media (prefers-reduced-motion: reduce) {
  .resource-panel__body :deep(.card-grid > *),
  .resource-panel__body :deep(.card-grid > .is-selected) {
    animation: none;
    transition: none;
  }
}

@media (max-width: 1500px) {
  .resource-panel {
    min-height: 560px;
  }
}

@media (max-width: 720px) {
  .resource-panel__body :deep(.card-grid) {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
