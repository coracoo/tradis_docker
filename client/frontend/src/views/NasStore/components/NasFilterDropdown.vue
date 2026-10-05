<template>
  <div class="nas-filter-dropdown">
    <button
      ref="anchorRef"
      type="button"
      class="filter-menu-trigger"
      :class="{ active: open || count > 0 }"
      :aria-expanded="open"
      aria-haspopup="menu"
      @click="$emit('update:open', !open)"
    >
      <span>{{ label }}</span>
      <span v-if="count > 0" class="filter-menu-count">{{ count }}</span>
      <DynamicIcon name="chevron-down" :size="14" />
    </button>

    <AnchoredMenu
      :open="open"
      :anchor="anchorRef"
      :gap="6"
      align="start"
      :min-width="panelWidth"
      :max-width="`min(${panelWidth}px, calc(100vw - 16px))`"
      @update:open="$emit('update:open', $event)"
    >
      <div
        class="nas-filter-menu"
        :class="{ 'is-price': variant === 'price' }"
        role="group"
        :aria-label="`${label}筛选`"
      >
        <slot />
      </div>
    </AnchoredMenu>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import AnchoredMenu from '@/components/ui/AnchoredMenu.vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'

defineProps({
  label: {
    type: String,
    required: true
  },
  count: {
    type: Number,
    default: 0
  },
  open: {
    type: Boolean,
    default: false
  },
  variant: {
    type: String,
    default: 'options',
    validator: value => ['options', 'price'].includes(value)
  },
  panelWidth: {
    type: Number,
    default: 240
  }
})

defineEmits(['update:open'])

const anchorRef = ref(null)
</script>

<style scoped>
.nas-filter-dropdown {
  flex: 0 0 auto;
}

.filter-menu-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 34px;
  padding: 0 11px;
  border: 1px solid var(--resource-line);
  border-radius: 8px;
  background: var(--bg-elevated);
  color: var(--text-secondary);
  font-family: inherit;
  font-size: 0.8125rem;
  font-weight: 500;
  white-space: nowrap;
  cursor: pointer;
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out), background var(--motion-duration-quick) var(--motion-ease-out), color var(--motion-duration-quick) var(--motion-ease-out);
}

.filter-menu-trigger:hover {
  border-color: var(--color-primary-300);
  color: var(--text-primary);
}

.filter-menu-trigger.active {
  border-color: var(--color-primary-500);
  background: var(--color-primary-500-10);
  color: var(--color-primary-600);
}

.filter-menu-trigger:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--color-primary-500) 24%, transparent);
  outline-offset: 2px;
}

.filter-menu-trigger :deep(svg) {
  transition: transform var(--motion-duration-quick) var(--motion-ease-out);
}

.filter-menu-trigger[aria-expanded='true'] :deep(svg) {
  transform: rotate(180deg);
}

.filter-menu-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 999px;
  background: var(--color-primary-500);
  color: var(--text-inverse, var(--bg-elevated));
  font-size: 0.6875rem;
  font-weight: 700;
}

.nas-filter-menu {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 4px;
  width: 100%;
  max-height: min(320px, calc(100vh - 32px));
  overflow-y: auto;
  padding: 1px;
  scrollbar-width: thin;
}

.nas-filter-menu :deep(.filter-chip) {
  position: relative;
  display: flex;
  width: 100%;
  min-width: 0;
  min-height: 34px;
  align-items: center;
  justify-content: flex-start;
  padding: 0 28px 0 10px;
  overflow: hidden;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--text-secondary);
  font-size: 0.8125rem;
  font-weight: 500;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.nas-filter-menu :deep(.filter-chip:hover),
.nas-filter-menu :deep(.filter-chip:focus-visible) {
  outline: none;
  background: var(--bg-secondary);
  color: var(--text-primary);
}

.nas-filter-menu :deep(.filter-chip.active) {
  background: var(--color-primary-500-10);
  color: var(--color-primary-600);
}

.nas-filter-menu :deep(.filter-chip.active::after) {
  content: '✓';
  position: absolute;
  right: 10px;
  color: var(--color-primary-600);
  font-size: 0.75rem;
  font-weight: 700;
}

.nas-filter-menu.is-price {
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 5px;
}

.nas-filter-menu.is-price :deep(.price-range) {
  grid-column: 1 / -1;
  width: 100%;
  margin-top: 3px;
  padding: 9px 8px 3px;
  border-top: 1px solid var(--border-subtle);
}

.nas-filter-menu.is-price :deep(.price-input) {
  flex: 1 1 0;
  min-width: 0;
  width: auto;
  height: 34px;
}
</style>
