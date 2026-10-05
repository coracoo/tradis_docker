// Compose 项目的行身份：同名多路径的外部/受管项目共用 compose 项目名，
// 选择、高亮与摘要解析必须按“名字+路径”定位真实项目，而不是只按名字。
export function composeProjectIdentity(project) {
  const name = String(project?.name ?? '')
  const path = String(project?.path ?? '')
  if (!name) return ''
  return path ? `${name}|${path}` : name
}

export function remoteComposeOperationName(project) {
	return String(project?.composeProjectName || project?.name || '').trim()
}

export function sameComposeProject(a, b) {
  return Boolean(a && b) && composeProjectIdentity(a) === composeProjectIdentity(b)
}

export function findComposeProjectByIdentity(projects, identity, fallbackName = '') {
  const normalizedIdentity = String(identity || '')
  if (normalizedIdentity) {
    const exact = (projects || []).find(project => composeProjectIdentity(project) === normalizedIdentity)
    if (exact) return exact
  }
  const normalizedName = String(fallbackName || '')
  if (!normalizedName) return null
  const matches = (projects || []).filter(project => String(project?.name || '') === normalizedName)
  return matches.length === 1 ? matches[0] : null
}
