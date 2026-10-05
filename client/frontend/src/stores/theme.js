import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

// 可用主题色（forest 默认第一）
const themes = [
  { id: 'forest', name: '森林绿', color: '#10b981' },
  { id: 'ocean', name: '海洋蓝', color: '#3b82f6' },
  { id: 'violet', name: '紫罗兰', color: '#8b5cf6' },
  { id: 'amber', name: '琥珀橙', color: '#f59e0b' },
  { id: 'periwinkle', name: '长春花蓝', color: '#6667ab' },
  { id: 'magenta', name: '非凡洋红', color: '#be3455' },
  { id: 'marsala', name: '玛萨拉红', color: '#955251' },
  { id: 'mocha', name: '摩卡慕斯', color: '#a47764' },
  { id: 'orchid', name: '兰紫', color: '#b565a7' },
  { id: 'ultraviolet', name: '紫外光', color: '#5f4b8b' },
  { id: 'tangerine', name: '橘红', color: '#f05442' },
]

// 图标库选项
const iconLibraries = [
  { id: 'lucide', name: 'Lucide' },
  { id: 'tabler', name: 'Tabler' },
]

export const useThemeStore = defineStore('theme', () => {
  // State
  const isDark = ref(false)
  const currentTheme = ref('forest')
  const currentIconLib = ref('lucide')
  const isInitialized = ref(false)

  // Getters
  const currentThemeColor = computed(() => {
    return themes.find(t => t.id === currentTheme.value)?.color
  })

  const themeLabel = computed(() => {
    return themes.find(t => t.id === currentTheme.value)?.name
  })

  const iconLibLabel = computed(() => {
    return iconLibraries.find(lib => lib.id === currentIconLib.value)?.name
  })

  // Actions
  function init() {
    if (isInitialized.value) return

    // 从 localStorage 读取
    const storedDark = localStorage.getItem('darkMode')
    const storedTheme = localStorage.getItem('colorTheme')
    const storedIconLib = localStorage.getItem('iconLibrary')

    // 设置暗色模式
    if (storedDark !== null) {
      isDark.value = storedDark === 'true'
    } else {
      // 检测系统偏好
      isDark.value = window.matchMedia('(prefers-color-scheme: dark)').matches
    }

    // 设置主题色
    if (storedTheme && themes.find(t => t.id === storedTheme)) {
      currentTheme.value = storedTheme
    }

    // 设置图标库
    if (storedIconLib && iconLibraries.find(lib => lib.id === storedIconLib)) {
      currentIconLib.value = storedIconLib
    }

    applyTheme()
    isInitialized.value = true
  }

  function applyTheme() {
    const html = document.documentElement

    // 应用暗色模式
    if (isDark.value) {
      html.classList.add('dark')
    } else {
      html.classList.remove('dark')
    }

    // 应用主题色
    html.setAttribute('data-theme', currentTheme.value)
  }

  function toggleDarkMode() {
    isDark.value = !isDark.value
    localStorage.setItem('darkMode', isDark.value)
    applyTheme()
  }

  function setTheme(themeId) {
    if (themes.find(t => t.id === themeId)) {
      currentTheme.value = themeId
      localStorage.setItem('colorTheme', themeId)
      applyTheme()
    }
  }

  function setIconLibrary(libId) {
    if (iconLibraries.find(lib => lib.id === libId)) {
      currentIconLib.value = libId
      localStorage.setItem('iconLibrary', libId)
      // 触发全局事件
      window.dispatchEvent(new CustomEvent('iconlibrary-change', { detail: libId }))
    }
  }

  return {
    // State
    isDark,
    currentTheme,
    currentIconLib,
    isInitialized,
    // Getters
    currentThemeColor,
    themeLabel,
    iconLibLabel,
    // Actions
    init,
    toggleDarkMode,
    setTheme,
    setIconLibrary,
  }
})

// 导出配置
export { themes, iconLibraries }
