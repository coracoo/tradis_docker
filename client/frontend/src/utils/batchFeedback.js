function failureText(failure) {
  if (typeof failure === 'string') return failure.slice(0, 180)
  const name = String(failure?.name || '未知对象')
  const reason = String(failure?.reason || '').trim()
  return reason ? `${name}：${reason.slice(0, 180)}` : name
}

export function notifyBatchResult(uiStore, { label, succeeded = 0, failures = [], pending = 0, suffix = '' }) {
  const failed = failures.length
  const summary = `${label}结束：成功 ${succeeded} 个，失败 ${failed} 个${suffix}`
  if (failed === 0) {
    if (pending > 0 || succeeded === 0) {
      uiStore.toastInfo(summary)
    } else {
      uiStore.toastSuccess(summary)
    }
    return
  }

  const visibleFailures = failures.slice(0, 3).map(failureText)
  const remaining = failed > visibleFailures.length ? `，另有 ${failed - visibleFailures.length} 个` : ''
  const message = `${summary}；失败项：${visibleFailures.join('；')}${remaining}`
  if (succeeded > 0) {
    uiStore.toastWarning(message)
  } else {
    uiStore.toastError(message)
  }
}
