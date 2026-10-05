export function deploymentPreflightConfirmation(preflight = {}) {
  const projectName = String(preflight?.resolved_project_name || '').trim() || '当前项目'
  const reasons = []
  const replacesProject = Array.isArray(preflight?.changes) && preflight.changes.some(change => (
    change?.reason_code === 'project_exists' || change?.reasonCode === 'project_exists'
  ))
  if (replacesProject) reasons.push(`替换目标节点上的同名项目 ${projectName}`)

  const absolutePaths = Array.from(new Set(
    (Array.isArray(preflight?.issues) ? preflight.issues : [])
      .filter(issue => issue?.code === 'absolute_bind_requires_confirmation')
      .map(issue => String(issue?.details?.path || '').trim())
      .filter(Boolean)
  ))
  if (absolutePaths.length > 0) reasons.push(`挂载目标节点的宿主机路径：${absolutePaths.join('、')}`)

  if (reasons.length === 0) reasons.push(`执行项目 ${projectName} 的远程部署变更`)
  return `请确认以下远程部署操作：\n${reasons.map(reason => `- ${reason}`).join('\n')}`
}
