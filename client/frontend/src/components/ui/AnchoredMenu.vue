<template>
  <Teleport to="body">
    <Transition name="anchored-menu">
      <div
        v-if="open"
        ref="menuRef"
        class="anchored-menu"
        :data-placement="placement"
        :style="[menuStyle, sizeStyle]"
        role="menu"
        @pointerdown.stop
      >
        <slot />
      </div>
    </Transition>
  </Teleport>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

const props = defineProps({
  open: {
    type: Boolean,
    default: false
  },
  anchor: {
    type: Object,
    default: null
  },
  gap: {
    type: Number,
    default: 8
  },
  viewportPadding: {
    type: Number,
    default: 8
  },
  align: {
    type: String,
    default: 'end',
    validator: value => ['start', 'end'].includes(value)
  },
  minWidth: {
    type: [Number, String],
    default: null
  },
  maxWidth: {
    type: [Number, String],
    default: null
  }
})

const emit = defineEmits(['update:open'])
const menuRef = ref(null)
const placement = ref('below')
const menuStyle = ref({
  position: 'fixed',
  top: '0px',
  left: '0px',
  visibility: 'hidden'
})

function cssSize(value) {
  if (value === null || value === undefined || value === '') return null
  return typeof value === 'number' ? `${value}px` : value
}

const sizeStyle = computed(() => ({
  '--anchored-menu-min-width': cssSize(props.minWidth),
  '--anchored-menu-max-width': cssSize(props.maxWidth)
}))

function close() {
  emit('update:open', false)
}

function updatePosition() {
  const anchor = props.anchor
  const menu = menuRef.value
  if (!props.open || !anchor?.getBoundingClientRect || !menu) return

  const anchorRect = anchor.getBoundingClientRect()
  const menuRect = menu.getBoundingClientRect()
  const menuWidth = menuRect.width || menu.offsetWidth || 152
  const menuHeight = menuRect.height || menu.offsetHeight || 160
  const viewportWidth = window.innerWidth || document.documentElement.clientWidth
  const viewportHeight = window.innerHeight || document.documentElement.clientHeight
  const maxLeft = Math.max(props.viewportPadding, viewportWidth - menuWidth - props.viewportPadding)
  const desiredLeft = props.align === 'start'
    ? anchorRect.left
    : anchorRect.right - menuWidth
  const left = Math.min(Math.max(props.viewportPadding, desiredLeft), maxLeft)

  const belowTop = anchorRect.bottom + props.gap
  const aboveTop = anchorRect.top - menuHeight - props.gap
  const shouldOpenAbove = belowTop + menuHeight > viewportHeight - props.viewportPadding
  placement.value = shouldOpenAbove ? 'above' : 'below'
  const maxTop = Math.max(props.viewportPadding, viewportHeight - menuHeight - props.viewportPadding)
  const top = Math.min(
    Math.max(props.viewportPadding, shouldOpenAbove ? aboveTop : belowTop),
    maxTop
  )

  menuStyle.value = {
    position: 'fixed',
    top: `${Math.round(top)}px`,
    left: `${Math.round(left)}px`,
    visibility: 'visible',
    transformOrigin: `${shouldOpenAbove ? 'bottom' : 'top'} ${props.align === 'start' ? 'left' : 'right'}`
  }
}

function handleOutsidePointer(event) {
  const target = event.target
  if (menuRef.value?.contains(target) || props.anchor?.contains?.(target)) return
  close()
}

function handleKeydown(event) {
  if (event.key === 'Escape') close()
}

function handleScroll() {
  close()
}

function addGlobalListeners() {
  document.addEventListener('pointerdown', handleOutsidePointer, true)
  window.addEventListener('keydown', handleKeydown)
  window.addEventListener('resize', updatePosition)
  window.addEventListener('scroll', handleScroll, true)
}

function removeGlobalListeners() {
  document.removeEventListener('pointerdown', handleOutsidePointer, true)
  window.removeEventListener('keydown', handleKeydown)
  window.removeEventListener('resize', updatePosition)
  window.removeEventListener('scroll', handleScroll, true)
}

watch(
  () => [props.open, props.anchor],
  async ([open]) => {
    removeGlobalListeners()
    if (!open) return

    menuStyle.value = {
      position: 'fixed',
      top: '0px',
      left: '0px',
      visibility: 'hidden'
    }
    placement.value = 'below'
    await nextTick()
    updatePosition()
    addGlobalListeners()
  },
  { immediate: true }
)

onBeforeUnmount(removeGlobalListeners)
</script>

<style scoped>
.anchored-menu {
  --anchored-menu-shift: calc(var(--motion-distance-micro) * -1);
  z-index: 3000;
  display: grid;
  min-width: var(--anchored-menu-min-width, 152px);
  max-width: var(--anchored-menu-max-width, min(240px, calc(100vw - 16px)));
  padding: 6px;
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  background: var(--bg-elevated);
  box-shadow: var(--shadow-lg);
}

.anchored-menu[data-placement='above'] {
  --anchored-menu-shift: var(--motion-distance-micro);
}

.anchored-menu-enter-active,
.anchored-menu-leave-active {
  transition:
    opacity var(--motion-duration-quick) var(--motion-ease-out),
    transform var(--motion-duration-quick) var(--motion-ease-out);
}

.anchored-menu-enter-from,
.anchored-menu-leave-to {
  opacity: 0;
  transform: translateY(var(--anchored-menu-shift)) scale(var(--motion-scale-enter));
}

.anchored-menu :deep([role='menuitem']) {
  width: 100%;
  min-height: 34px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--text-secondary);
  font-size: 0.8125rem;
  font-weight: 500;
  text-align: left;
  white-space: nowrap;
  cursor: pointer;
}

.anchored-menu :deep([role='menuitem']:hover),
.anchored-menu :deep([role='menuitem']:focus-visible) {
  outline: none;
  color: var(--text-primary);
  background: var(--bg-secondary);
}

.anchored-menu :deep([role='menuitem'].danger) {
  color: var(--color-danger-600);
}

.anchored-menu :deep([role='menuitem'].danger:hover),
.anchored-menu :deep([role='menuitem'].danger:focus-visible) {
  background: var(--color-danger-100);
}
</style>
