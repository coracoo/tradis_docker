import { nextTick, onBeforeUnmount, ref, unref, watch } from 'vue'

let overlaySequence = 0
const overlayStack = []
let savedBodyOverflow = ''
let savedBodyPaddingRight = ''

function lockBodyScroll() {
  if (overlayStack.length !== 1) return
  savedBodyOverflow = document.body.style.overflow
  savedBodyPaddingRight = document.body.style.paddingRight
  const scrollbarWidth = window.innerWidth - document.documentElement.clientWidth
  document.body.style.overflow = 'hidden'
  if (scrollbarWidth > 0) {
    document.body.style.paddingRight = `${scrollbarWidth}px`
  }
}

function unlockBodyScroll() {
  if (overlayStack.length !== 0) return
  document.body.style.overflow = savedBodyOverflow
  document.body.style.paddingRight = savedBodyPaddingRight
}

function registerOverlay(id) {
  const existingIndex = overlayStack.indexOf(id)
  if (existingIndex >= 0) overlayStack.splice(existingIndex, 1)
  overlayStack.push(id)
  lockBodyScroll()
}

function unregisterOverlay(id) {
  const index = overlayStack.indexOf(id)
  if (index >= 0) overlayStack.splice(index, 1)
  unlockBodyScroll()
}

function isTopOverlay(id) {
  return overlayStack.at(-1) === id
}

function getFocusableElements(container) {
  if (!container) return []
  const selector = [
    'a[href]',
    'button:not([disabled])',
    'input:not([disabled]):not([type="hidden"])',
    'select:not([disabled])',
    'textarea:not([disabled])',
    '[tabindex]:not([tabindex="-1"])'
  ].join(',')
  return Array.from(container.querySelectorAll(selector)).filter(element => {
    return !element.hasAttribute('hidden') && element.getAttribute('aria-hidden') !== 'true'
  })
}

export function useOverlayController(options) {
  const id = `tradis-overlay-${++overlaySequence}`
  let previousActiveElement = null
  let registered = false
  const overlayZIndex = ref(2000)

  function focusOverlay() {
    const container = unref(options.containerRef)
    if (!container) return
    const autofocus = container.querySelector('[autofocus]')
    const target = autofocus || container
    target.focus?.({ preventScroll: true })
  }

  function restoreFocus() {
    const target = previousActiveElement
    previousActiveElement = null
    const anotherOverlayIsOpen = overlayStack.length > 0
    const targetBelongsToOverlay = target?.closest?.('[role="dialog"][aria-modal="true"]')
    if (target?.isConnected && (!anotherOverlayIsOpen || targetBelongsToOverlay)) {
      target.focus?.({ preventScroll: true })
    }
  }

  function handleKeydown(event) {
    if (!isTopOverlay(id)) return

    if (event.key === 'Escape' && unref(options.closeOnEscape) !== false) {
      event.preventDefault()
      options.onClose?.('escape')
      return
    }

    if (event.key !== 'Tab') return
    const container = unref(options.containerRef)
    const focusable = getFocusableElements(container)
    if (focusable.length === 0) {
      event.preventDefault()
      container?.focus?.({ preventScroll: true })
      return
    }

    const first = focusable[0]
    const last = focusable.at(-1)
    if (event.shiftKey && (document.activeElement === first || document.activeElement === container)) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault()
      first.focus()
    }
  }

  function open() {
    if (registered) return
    registered = true
    previousActiveElement = document.activeElement
    registerOverlay(id)
    overlayZIndex.value = 2000 + (overlayStack.length - 1) * 20
    document.addEventListener('keydown', handleKeydown)
    nextTick(focusOverlay)
  }

  function close({ restore = true } = {}) {
    if (!registered) return
    registered = false
    document.removeEventListener('keydown', handleKeydown)
    unregisterOverlay(id)
    if (restore) nextTick(restoreFocus)
  }

  watch(
    () => Boolean(unref(options.visible)),
    visible => {
      if (visible) open()
      else close()
    },
    { immediate: true }
  )

  onBeforeUnmount(() => close({ restore: true }))

  return {
    overlayId: id,
    overlayZIndex
  }
}
