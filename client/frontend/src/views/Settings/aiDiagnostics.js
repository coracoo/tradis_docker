function positiveLatency(value) {
  const latency = Number(value)
  return Number.isFinite(latency) && latency >= 0 ? Math.round(latency) : 0
}

export function formatAIDiagnosticResult(result) {
  const transport = result?.transport || {}
  const inference = result?.inference || {}

  if (result?.ok) {
    const transportLatency = positiveLatency(transport.latencyMs)
    const inferenceLatency = positiveLatency(inference.latencyMs)
    const totalLatency = positiveLatency(result.latencyMs) || transportLatency + inferenceLatency
    const details = [
      `连接与鉴权 ${transportLatency}ms`,
      `模型推理 ${inferenceLatency}ms`
    ]
    if (result.model) details.push(result.model)
    return {
      ok: true,
      message: `连接与推理正常（${totalLatency}ms）`,
      detail: details.join(' · ')
    }
  }

  const inferenceWasRun = inference.code && inference.code !== 'skipped'
  const stage = inferenceWasRun ? inference : transport
  const stageName = inferenceWasRun ? '模型推理' : '连接与鉴权'
  const detail = []
  if (stage.code) detail.push(`错误类型 ${stage.code}`)
  if (stage.traceId) detail.push(`Trace ${stage.traceId}`)
  return {
    ok: false,
    message: `${stageName}：${stage.message || '检测未通过'}`,
    detail: detail.join(' · ')
  }
}
