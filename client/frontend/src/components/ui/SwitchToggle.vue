<template>
  <button
    type="button"
    class="switch-toggle"
    :class="{ 'is-init': interacted, 'is-disabled': disabled }"
    role="switch"
    :data-on="String(modelValue)"
    :aria-checked="String(modelValue)"
    :aria-label="ariaLabel || label"
    :disabled="disabled"
    @click="toggle"
  >
    <span class="switch-toggle__track" aria-hidden="true">
      <span class="switch-toggle__thumb"></span>
    </span>
    <span v-if="label || $slots.default" class="switch-toggle__label">
      <slot>{{ label }}</slot>
    </span>
  </button>
</template>

<script setup>
import { ref } from 'vue'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  label: { type: String, default: '' },
  ariaLabel: { type: String, default: '' },
  disabled: { type: Boolean, default: false }
})

const emit = defineEmits(['update:modelValue', 'change'])
const interacted = ref(false)

function toggle() {
  if (props.disabled) return
  interacted.value = true
  const nextValue = !props.modelValue
  emit('update:modelValue', nextValue)
  emit('change', nextValue)
}
</script>

<style scoped>
.switch-toggle {
  --motion-switch-duration: 260ms;
  --motion-switch-travel: 16px;
  --motion-switch-overshoot: 0.5px;
  display: inline-flex;
  align-items: center;
  gap: 9px;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--text-primary);
  font: inherit;
  cursor: pointer;
}

.switch-toggle__track {
  position: relative;
  width: 36px;
  height: 20px;
  flex: 0 0 auto;
  border: 1px solid var(--border-default);
  border-radius: 999px;
  background: var(--bg-tertiary);
  transition:
    background-color var(--motion-duration-fast) var(--motion-ease-out),
    border-color var(--motion-duration-fast) var(--motion-ease-out),
    box-shadow var(--motion-duration-fast) var(--motion-ease-out);
}

.switch-toggle__thumb {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: var(--bg-elevated);
  box-shadow: var(--shadow-sm);
  translate: 0 0;
}

.switch-toggle[data-on='true'] .switch-toggle__track {
  border-color: var(--color-primary-500);
  background: var(--color-primary-500);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary-500) 12%, transparent);
}

.switch-toggle[data-on='true'] .switch-toggle__thumb {
  translate: var(--motion-switch-travel) 0;
}

.switch-toggle.is-init[data-on='true'] .switch-toggle__thumb {
  animation: switch-toggle-on var(--motion-switch-duration) var(--motion-ease-out) both;
}

.switch-toggle.is-init[data-on='false'] .switch-toggle__thumb {
  animation: switch-toggle-off var(--motion-switch-duration) var(--motion-ease-out) both;
}

.switch-toggle__label {
  font-size: 0.875rem;
  line-height: 1.35;
}

.switch-toggle:focus-visible {
  outline: 2px solid var(--color-primary-500);
  outline-offset: 3px;
  border-radius: 5px;
}

.switch-toggle.is-disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

@keyframes switch-toggle-on {
  0% { translate: 0 0; }
  62% { translate: calc(var(--motion-switch-travel) + var(--motion-switch-overshoot)) 0; }
  100% { translate: var(--motion-switch-travel) 0; }
}

@keyframes switch-toggle-off {
  0% { translate: var(--motion-switch-travel) 0; }
  62% { translate: calc(0px - var(--motion-switch-overshoot)) 0; }
  100% { translate: 0 0; }
}

@media (prefers-reduced-motion: reduce) {
  .switch-toggle__track,
  .switch-toggle__thumb {
    transition: none !important;
  }

  .switch-toggle__thumb { animation: none !important; }
}
</style>
