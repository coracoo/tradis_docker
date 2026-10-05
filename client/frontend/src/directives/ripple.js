/**
 * Ripple 点击涟漪指令
 * 用法：<button v-ripple>点击我</button>
 * 可配置颜色：<button v-ripple="'rgba(255,255,255,0.3)'">
 */

function createRipple(event, el, color) {
  const rect = el.getBoundingClientRect()
  const size = Math.max(rect.width, rect.height)
  const x = event.clientX - rect.left - size / 2
  const y = event.clientY - rect.top - size / 2

  const ripple = document.createElement('span')
  ripple.className = 'ripple-effect'
  ripple.style.cssText = `
    position: absolute;
    border-radius: 50%;
    transform: scale(0);
    animation: ripple-animation 0.5s cubic-bezier(0.4, 0, 0.2, 1);
    background-color: ${color || 'currentColor'};
    opacity: 0.25;
    pointer-events: none;
    width: ${size}px;
    height: ${size}px;
    left: ${x}px;
    top: ${y}px;
    z-index: 0;
  `

  el.appendChild(ripple)

  const remove = () => {
    if (ripple.parentNode === el) {
      el.removeChild(ripple)
    }
  }

  ripple.addEventListener('animationend', remove)
  // 保险：500ms 后强制移除
  setTimeout(remove, 520)
}

function addKeydownRipple(el, color) {
  const handler = (e) => {
    if (e.key !== 'Enter' && e.key !== ' ') return
    if (el.disabled) return
    createRipple(
      { clientX: 0, clientY: 0 },
      el,
      color
    )
  }
  el._rippleKeydownHandler = handler
  el.addEventListener('keydown', handler)
}

export const vRipple = {
  mounted(el, binding) {
    if (getComputedStyle(el).position === 'static') {
      el.style.position = 'relative'
    }
    if (getComputedStyle(el).overflow === 'visible') {
      el.style.overflow = 'hidden'
    }

    const color = typeof binding.value === 'string' ? binding.value : ''
    const handler = (e) => {
      if (el.disabled) return
      createRipple(e, el, color)
    }
    el._rippleHandler = handler
    el.addEventListener('click', handler)
    addKeydownRipple(el, color)
  },
  updated(el, binding) {
    const color = typeof binding.value === 'string' ? binding.value : ''
    if (el._rippleHandler) {
      el._rippleColor = color
    }
  },
  unmounted(el) {
    if (el._rippleHandler) {
      el.removeEventListener('click', el._rippleHandler)
      delete el._rippleHandler
    }
    if (el._rippleKeydownHandler) {
      el.removeEventListener('keydown', el._rippleKeydownHandler)
      delete el._rippleKeydownHandler
    }
  }
}

export default vRipple
