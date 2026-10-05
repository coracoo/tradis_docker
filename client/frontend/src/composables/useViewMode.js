import { ref, watch } from 'vue'

/**
 * 视图模式管理（列表/卡片切换，自动记忆）
 * @param {string} key - 存储键名，如 'images_view_mode'
 * @param {string} defaultMode - 默认模式 'grid' | 'table'
 * @returns {Object} { viewMode, setViewMode }
 */
export function useViewMode(key, defaultMode = 'grid') {
  // 从 localStorage 读取保存的模式
  const storedMode = localStorage.getItem(key)
  const normalizedDefault = defaultMode === 'table' ? 'table' : 'grid'
  const initialMode = storedMode === 'table' || storedMode === 'grid'
    ? storedMode
    : normalizedDefault
  
  const viewMode = ref(initialMode)
  
  // 监听变化并保存到 localStorage
  watch(viewMode, (newMode) => {
    localStorage.setItem(key, newMode)
  }, { flush: 'sync' })
  
  // 设置视图模式
  const setViewMode = (mode) => {
    if (mode === 'grid' || mode === 'table') {
      viewMode.value = mode
    }
  }
  
  return {
    viewMode,
    setViewMode
  }
}

export default useViewMode
