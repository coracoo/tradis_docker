<template>
  <label class="animated-checkbox" :class="{ 'is-disabled': disabled }">
    <input
      ref="inputRef"
      class="animated-checkbox__input"
      type="checkbox"
      :checked="checked"
      :disabled="disabled"
      :name="name || undefined"
      :value="value"
      :aria-checked="indeterminate ? 'mixed' : String(checked)"
      @change="handleChange"
    />
    <span class="animated-checkbox__box" aria-hidden="true">
      <svg v-if="!indeterminate" viewBox="0 0 12 12" fill="none">
        <path class="animated-checkbox__path" d="M1.5 6.2L4.7 9.3L10.5 2.4" />
      </svg>
      <span v-else class="animated-checkbox__mixed"></span>
    </span>
    <span v-if="label || $slots.default" class="animated-checkbox__label">
      <slot>{{ label }}</slot>
    </span>
  </label>
</template>

<script setup>
import { computed, ref, watchEffect } from 'vue'

const props = defineProps({
  modelValue: { type: [Boolean, Array], default: false },
  value: { type: [String, Number, Boolean], default: true },
  label: { type: String, default: '' },
  name: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
  indeterminate: { type: Boolean, default: false }
})

const emit = defineEmits(['update:modelValue', 'change'])
const inputRef = ref(null)
const checked = computed(() => Array.isArray(props.modelValue)
  ? props.modelValue.includes(props.value)
  : Boolean(props.modelValue))

watchEffect(() => {
  if (inputRef.value) inputRef.value.indeterminate = props.indeterminate
})

function handleChange(event) {
  const nextChecked = event.target.checked
  const nextValue = Array.isArray(props.modelValue)
    ? nextChecked
      ? [...props.modelValue, props.value]
      : props.modelValue.filter(item => item !== props.value)
    : nextChecked
  emit('update:modelValue', nextValue)
  emit('change', nextValue)
}
</script>

<style scoped>
.animated-checkbox {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--text-primary);
  font-size: 0.875rem;
  line-height: 1.35;
  cursor: pointer;
}

.animated-checkbox__input {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
  pointer-events: none;
}

.animated-checkbox__box {
  display: grid;
  width: 16px;
  height: 16px;
  flex: 0 0 auto;
  place-items: center;
  border: 1px solid var(--border-default);
  border-radius: 4px;
  background: var(--bg-elevated);
  transition:
    background-color var(--motion-duration-fast) var(--motion-ease-out),
    border-color var(--motion-duration-fast) var(--motion-ease-out),
    box-shadow var(--motion-duration-fast) var(--motion-ease-out);
}

.animated-checkbox__box svg {
  width: 11px;
  height: 11px;
  overflow: visible;
}

.animated-checkbox__path {
  fill: none;
  stroke: white;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
  stroke-dasharray: 15;
  stroke-dashoffset: 15;
  transition: stroke-dashoffset 160ms var(--motion-ease-out);
}

.animated-checkbox__input:checked + .animated-checkbox__box,
.animated-checkbox__input:indeterminate + .animated-checkbox__box {
  border-color: var(--color-primary-500);
  background: var(--color-primary-500);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary-500) 12%, transparent);
}

.animated-checkbox__input:checked + .animated-checkbox__box .animated-checkbox__path {
  stroke-dashoffset: 0;
  transition-duration: 220ms;
}

.animated-checkbox__mixed {
  width: 8px;
  height: 2px;
  border-radius: 1px;
  background: white;
}

.animated-checkbox__input:focus-visible + .animated-checkbox__box {
  outline: 2px solid var(--color-primary-500);
  outline-offset: 2px;
}

.animated-checkbox.is-disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

@media (prefers-reduced-motion: reduce) {
  .animated-checkbox__box,
  .animated-checkbox__path { transition: none !important; }
}
</style>
