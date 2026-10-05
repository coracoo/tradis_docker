<template>
  <div class="app-description-tooltip">
    <p
      class="app-description-tooltip__trigger"
      :tabindex="hasDescription ? 0 : undefined"
      :data-tooltip="hasDescription ? text : undefined"
    >
      {{ displayText }}
    </p>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  text: {
    type: String,
    default: ''
  },
  fallback: {
    type: String,
    default: '暂无描述'
  }
})

const hasDescription = computed(() => Boolean(props.text?.trim()))
const displayText = computed(() => hasDescription.value ? props.text : props.fallback)
</script>

<style scoped>
.app-description-tooltip {
  position: relative;
  flex: 1;
  min-width: 0;
}

.app-description-tooltip__trigger {
  display: -webkit-box;
  max-height: 4.5em;
  margin: 0;
  overflow: hidden;
  color: var(--resource-muted);
  font-size: 0.73rem;
  line-height: 1.5;
  overflow-wrap: anywhere;
  text-overflow: ellipsis;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
}

.app-description-tooltip__trigger:focus-visible {
  border-radius: 4px;
  outline: 2px solid color-mix(in srgb, var(--resource-accent) 45%, transparent);
  outline-offset: 2px;
}

</style>
