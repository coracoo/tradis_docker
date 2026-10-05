import { useUiStore } from '../stores/ui.js'

export function useToast() {
  const uiStore = useUiStore()
  
  return {
    show: uiStore.showToast,
    success: uiStore.toastSuccess,
    error: uiStore.toastError,
    warning: uiStore.toastWarning,
    info: uiStore.toastInfo,
    remove: uiStore.removeToast,
    clear: uiStore.clearToasts
  }
}
