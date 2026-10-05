// UI 状态管理
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export const useUiStore = defineStore('ui', () => {
  // State
  const isLoading = ref(false)
  const loadingText = ref('加载中...')
  const toasts = ref([])
  const modals = ref([])

  // Getters
  const hasToast = computed(() => toasts.value.length > 0)
  const currentModal = computed(() => modals.value[0] || null)

  // Toast 相关
  let toastId = 0

  function showToast(message, options = {}) {
    const { type = 'info', duration = 3000 } = options
    const id = ++toastId
    
    const toast = {
      id,
      message,
      type,
      duration
    }
    
    toasts.value.push(toast)
    
    if (duration > 0) {
      setTimeout(() => {
        removeToast(id)
      }, duration)
    }
    
    return id
  }

  function removeToast(id) {
    const index = toasts.value.findIndex(t => t.id === id)
    if (index > -1) {
      toasts.value.splice(index, 1)
    }
  }

  function clearToasts() {
    toasts.value = []
  }

  // 快捷方法
  function toastSuccess(message) {
    return showToast(message, { type: 'success' })
  }

  function toastError(message) {
    return showToast(message, { type: 'error' })
  }

  function toastWarning(message) {
    return showToast(message, { type: 'warning' })
  }

  function toastInfo(message) {
    return showToast(message, { type: 'info' })
  }

  // 全局加载
  function showLoading(text = '加载中...') {
    isLoading.value = true
    loadingText.value = text
  }

  function hideLoading() {
    isLoading.value = false
  }

  // Modal 相关
  function openModal(component, props = {}) {
    const id = Date.now()
    modals.value.push({ id, component, props })
    return id
  }

  function closeModal(id) {
    if (id) {
      const index = modals.value.findIndex(m => m.id === id)
      if (index > -1) {
        modals.value.splice(index, 1)
      }
    } else {
      modals.value.shift()
    }
  }

  function closeAllModals() {
    modals.value = []
  }

  // Confirm 对话框
  // 每个 Promise 通过 onConfirm/onCancel 闭包独立持有自己的 resolve，
  // 因此即便并发/连续调用 confirm()，每次的 resolve 都能被正确触发。
  // （此前存在一个共享的 confirmResolve ref，但它只被赋值、从未被读取，属于死代码，已移除。）
  function confirm(options = {}) {
    const { title = '确认', message = '确定执行此操作？', type = 'warning', confirmText = '确定', cancelText = '取消' } = options

    return new Promise((resolve) => {
      openModal('ConfirmDialog', {
        title,
        message,
        type,
        confirmText,
        cancelText,
        onConfirm: () => {
          resolve(true)
          closeModal()
        },
        onCancel: () => {
          resolve(false)
          closeModal()
        }
      })
    })
  }

  return {
    // State
    isLoading,
    loadingText,
    toasts,
    modals,
    // Getters
    hasToast,
    currentModal,
    // Actions
    showToast,
    removeToast,
    clearToasts,
    toastSuccess,
    toastError,
    toastWarning,
    toastInfo,
    showLoading,
    hideLoading,
    openModal,
    closeModal,
    closeAllModals,
    confirm
  }
})
