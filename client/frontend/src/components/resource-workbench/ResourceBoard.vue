<template>
  <section class="resource-board-shell">
    <div class="resource-board__main">
      <slot />
    </div>
    <aside v-if="$slots.context" class="resource-board__context">
      <slot name="context" />
    </aside>
  </section>
</template>

<style scoped>
.resource-board-shell {
  display: grid;
  grid-template-columns: minmax(0, 1fr) clamp(300px, 22vw, 380px);
  align-items: stretch;
  gap: 12px;
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
}

.resource-board__main,
.resource-board__context {
  min-width: 0;
  min-height: 0;
}

.resource-board__main {
  display: flex;
}

.resource-board__context {
  display: flex;
}

@media (max-width: 1500px) {
  .resource-board-shell {
    height: auto;
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
