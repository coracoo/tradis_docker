import { vi } from 'vitest'
import { config } from '@vue/test-utils'

config.global.directives = {
  ...config.global.directives,
  ripple: {}
}
config.global.stubs = {
  ...config.global.stubs,
  IconEpArrowUp: { template: '<span class="icon-ep-arrow-up-stub" />' },
  IconEpArrowDown: { template: '<span class="icon-ep-arrow-down-stub" />' }
}

// 模拟 IntersectionObserver
global.IntersectionObserver = class IntersectionObserver {
  constructor() {}
  disconnect() {}
  observe() {}
  unobserve() {}
}

// 模拟 matchMedia
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation(query => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
})
