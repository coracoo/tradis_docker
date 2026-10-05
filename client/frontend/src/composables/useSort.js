import { ref, computed } from 'vue'

/**
 * 表格排序管理
 * @param {string} storageKey - 本地存储键名，如 'sort_images'
 * @param {Object} defaultSort - 默认排序 { prop: 'name', order: 'ascending' }
 * @returns {Object} 排序相关状态和函数
 */
export function useSort(storageKey, defaultSort = { prop: '', order: '' }) {
  // 从 localStorage 读取保存的排序
  const stored = localStorage.getItem(storageKey)
  const initialSort = stored ? JSON.parse(stored) : defaultSort

  const sortState = ref({
    prop: initialSort.prop || '',
    order: initialSort.order || '' // 'ascending' | 'descending' | ''
  })

  // 保存排序到 localStorage
  const saveSort = () => {
    localStorage.setItem(storageKey, JSON.stringify(sortState.value))
  }

  /**
   * 处理排序点击
   * @param {string} prop - 排序字段
   */
  const handleSort = (prop) => {
    if (sortState.value.prop === prop) {
      // 切换排序方向
      if (sortState.value.order === 'ascending') {
        sortState.value.order = 'descending'
      } else if (sortState.value.order === 'descending') {
        sortState.value.prop = ''
        sortState.value.order = ''
      } else {
        sortState.value.order = 'ascending'
      }
    } else {
      // 新字段，默认升序
      sortState.value.prop = prop
      sortState.value.order = 'ascending'
    }
    saveSort()
  }

  const setSort = (prop, order = 'ascending') => {
    sortState.value = {
      prop: prop || '',
      order: prop ? order : ''
    }
    saveSort()
  }

  /**
   * 对数据进行排序
   * @param {Array} data - 原始数据
   * @param {Function} getValue - 获取排序值的函数 (item, prop) => value
   */
  const sortData = (data, getValue) => {
    if (!sortState.value.prop || !sortState.value.order) {
      return data
    }

    return [...data].sort((a, b) => {
      let aVal = getValue(a, sortState.value.prop)
      let bVal = getValue(b, sortState.value.prop)

      // 处理空值
      if (aVal == null && bVal == null) return 0
      if (aVal == null) return sortState.value.order === 'ascending' ? -1 : 1
      if (bVal == null) return sortState.value.order === 'ascending' ? 1 : -1

      // 数字比较
      if (typeof aVal === 'number' && typeof bVal === 'number') {
        return sortState.value.order === 'ascending' ? aVal - bVal : bVal - aVal
      }

      // 字符串比较
      aVal = String(aVal).toLowerCase()
      bVal = String(bVal).toLowerCase()

      if (aVal < bVal) return sortState.value.order === 'ascending' ? -1 : 1
      if (aVal > bVal) return sortState.value.order === 'ascending' ? 1 : -1
      return 0
    })
  }

  /**
   * 获取排序图标
   * @param {string} prop - 字段名
   * @returns {string} 'asc' | 'desc' | ''
   */
  const getSortIcon = (prop) => {
    if (sortState.value.prop !== prop) return ''
    return sortState.value.order === 'ascending' ? 'asc' : 'desc'
  }

  /**
   * 清除排序
   */
  const clearSort = () => {
    sortState.value = { prop: '', order: '' }
    saveSort()
  }

  return {
    sortState,
    handleSort,
    setSort,
    sortData,
    getSortIcon,
    clearSort
  }
}

export default useSort
