<template>
  <section class="resource-relation-list" :aria-label="title">
    <header>
      <span>{{ title }}</span>
      <strong>{{ items.length }}</strong>
    </header>
    <ul v-if="items.length">
      <li v-for="item in items" :key="item.id || item.name">
        <component
          :is="clickable ? 'button' : 'div'"
          :type="clickable ? 'button' : undefined"
          class="resource-relation-list__button"
          :class="{ clickable }"
          @click="clickable && $emit('select', item)"
        >
          <span class="resource-relation-list__icon">
            <DynamicIcon :name="icon" :size="14" />
          </span>
          <span class="resource-relation-list__copy">
            <strong :title="item.name">{{ item.name }}</strong>
            <small :title="item.meta">{{ item.meta || '-' }}</small>
          </span>
        </component>
      </li>
    </ul>
    <p v-else>{{ emptyText }}</p>
  </section>
</template>

<script setup>
import DynamicIcon from '@/components/ui/DynamicIcon.vue'

defineProps({
  title: {
    type: String,
    default: '关联容器'
  },
  items: {
    type: Array,
    default: () => []
  },
  icon: {
    type: String,
    default: 'container'
  },
  emptyText: {
    type: String,
    default: '无关联容器'
  },
  clickable: {
    type: Boolean,
    default: false
  }
})

defineEmits(['select'])
</script>

<style scoped>
.resource-relation-list {
  display: grid;
  gap: 7px;
  min-height: 0;
}

.resource-relation-list > header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  color: var(--ops-muted, var(--text-secondary));
  font-size: 0.71875rem;
  font-weight: 800;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.resource-relation-list > header strong {
  min-width: 22px;
  padding: 1px 6px;
  border-radius: 999px;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  font-size: 0.6875rem;
  text-align: center;
}

.resource-relation-list ul {
  display: grid;
  gap: 2px;
  max-height: 196px;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  list-style: none;
}

.resource-relation-list li {
  min-width: 0;
  border-bottom: 1px solid var(--ops-line, var(--border-subtle));
}

.resource-relation-list__button {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 42px;
  padding: 5px 6px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  text-align: left;
}

.resource-relation-list__button.clickable {
  cursor: pointer;
}

.resource-relation-list__button.clickable:hover,
.resource-relation-list__button.clickable:focus-visible {
  outline: none;
  background: var(--resource-row-hover, var(--bg-secondary));
}

.resource-relation-list li:last-child {
  border-bottom: 0;
}

.resource-relation-list__icon {
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 7px;
  background: color-mix(in srgb, var(--color-primary-500) 10%, var(--bg-elevated));
  color: var(--color-primary-600);
}

.resource-relation-list__copy {
  display: grid;
  gap: 2px;
  min-width: 0;
}

.resource-relation-list__copy strong,
.resource-relation-list__copy small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.resource-relation-list__copy strong {
  color: var(--ops-ink, var(--text-primary));
  font-size: 0.78125rem;
  font-weight: 650;
}

.resource-relation-list__copy small {
  color: var(--ops-muted, var(--text-secondary));
  font-family: var(--font-mono, ui-monospace, monospace);
  font-size: 0.6875rem;
}

.resource-relation-list > p {
  margin: 0;
  padding: 12px 0;
  color: var(--text-tertiary);
  font-size: 0.75rem;
  text-align: center;
}
</style>
