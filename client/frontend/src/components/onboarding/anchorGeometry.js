export const SPOTLIGHT_PADDING = 6
const POPOVER_GAP = 12
const VIEWPORT_MARGIN = 8

export function clamp(value, min, max) {
  return Math.max(min, Math.min(max, value))
}

export function computeSpotlightStyle(rect, padding = SPOTLIGHT_PADDING) {
  if (!rect) return null
  return {
    top: Math.max(0, rect.top - padding),
    left: Math.max(0, rect.left - padding),
    width: rect.width + padding * 2,
    height: rect.height + padding * 2
  }
}

export function computePopoverPlacement(rect, viewport, popover, gap = POPOVER_GAP) {
  const { width: vw, height: vh } = viewport
  const { width: pw, height: ph } = popover

  if (!rect) {
    return {
      placement: 'center',
      top: clamp((vh - ph) / 2, VIEWPORT_MARGIN, Math.max(VIEWPORT_MARGIN, vh - ph - VIEWPORT_MARGIN)),
      left: clamp((vw - pw) / 2, VIEWPORT_MARGIN, Math.max(VIEWPORT_MARGIN, vw - pw - VIEWPORT_MARGIN))
    }
  }

  const rightFits = rect.right + gap + pw <= vw
  const bottomFits = rect.bottom + gap + ph <= vh
  const leftFits = rect.left - gap - pw >= 0
  const topFits = rect.top - gap - ph >= 0

  if (rightFits) {
    return {
      placement: 'right',
      top: clamp(rect.top, VIEWPORT_MARGIN, vh - ph - VIEWPORT_MARGIN),
      left: rect.right + gap
    }
  }
  if (bottomFits) {
    return {
      placement: 'bottom',
      top: rect.bottom + gap,
      left: clamp(rect.left, VIEWPORT_MARGIN, vw - pw - VIEWPORT_MARGIN)
    }
  }
  if (leftFits) {
    return {
      placement: 'left',
      top: clamp(rect.top, VIEWPORT_MARGIN, vh - ph - VIEWPORT_MARGIN),
      left: rect.left - gap - pw
    }
  }
  if (topFits) {
    return {
      placement: 'top',
      top: rect.top - gap - ph,
      left: clamp(rect.left, VIEWPORT_MARGIN, vw - pw - VIEWPORT_MARGIN)
    }
  }

  return {
    placement: 'bottom',
    top: clamp(rect.bottom + gap, VIEWPORT_MARGIN, vh - ph - VIEWPORT_MARGIN),
    left: clamp(rect.left, VIEWPORT_MARGIN, vw - pw - VIEWPORT_MARGIN)
  }
}

export function waitForSelector(selector, { timeout = 3000, root } = {}) {
  if (typeof document === 'undefined') return Promise.resolve(null)
  const scope = root || document
  const existing = scope.querySelector(selector)
  if (existing) return Promise.resolve(existing)

  return new Promise(resolve => {
    let settled = false
    const finish = value => {
      if (settled) return
      settled = true
      observer.disconnect()
      clearTimeout(safety)
      resolve(value)
    }
    const observer = new MutationObserver(() => {
      const el = scope.querySelector(selector)
      if (el) finish(el)
    })
    observer.observe(scope === document ? document.documentElement : scope, {
      childList: true,
      subtree: true
    })
    const start = Date.now()
    const poll = () => {
      const el = scope.querySelector(selector)
      if (el) return finish(el)
      if (Date.now() - start >= timeout) return finish(null)
      requestAnimationFrame(poll)
    }
    const safety = setTimeout(() => finish(scope.querySelector(selector)), timeout + 50)
    requestAnimationFrame(poll)
  })
}
