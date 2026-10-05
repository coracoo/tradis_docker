const blockedRemoteTargets = [
	{ prefix: '/scheduled-tasks', label: '定时任务' },
	{ prefix: '/ports', label: '端口管理' },
	{ prefix: '/cleanup', label: '空间清理' },
	{ prefix: '/appstore/deploy', label: '应用部署' }
]

function blockedTarget(path = '') {
	return blockedRemoteTargets.find(({ prefix }) => path === prefix || path.startsWith(`${prefix}/`))
}

export function remoteTargetNavigationReason(path, isRemoteEnvironment) {
	if (!isRemoteEnvironment) return ''
	const target = blockedTarget(path)
	return target ? `当前远程设备暂不支持 ${target.label}，请先切回本机` : ''
}

export function resolveRemoteTargetRedirect(path, environmentId) {
	if (!environmentId || environmentId === 'local' || !blockedTarget(path)) return null
	return { path: '/', query: { remoteUnavailable: '1' } }
}
