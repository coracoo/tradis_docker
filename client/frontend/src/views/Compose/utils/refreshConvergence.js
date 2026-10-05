import { classifyComposeStatus } from '@/utils/resourceStatus.js'

const RUNNING_ACTIONS = new Set(['start', 'restart', 'build', 'deploy', 'update', 'safe-update'])
const STOPPED_ACTIONS = new Set(['stop', 'force-stop', 'down'])

export function isComposeMutationSettled(project, action) {
  if (RUNNING_ACTIONS.has(action)) {
    return Boolean(project) && classifyComposeStatus(project) === 'running'
  }
  if (STOPPED_ACTIONS.has(action)) {
    return !project || classifyComposeStatus(project) === 'stopped'
  }
  if (action === 'remove' || action === 'destroy') return !project
  return true
}

export async function convergeComposeMutation({
  refresh,
  resolveProject,
  action,
  delays = [750, 1250],
  wait = delay => new Promise(resolve => setTimeout(resolve, delay))
}) {
  for (let attempt = 0; attempt <= delays.length; attempt += 1) {
    await refresh()
    const project = resolveProject()
    if (isComposeMutationSettled(project, action) || attempt === delays.length) {
      return project
    }
    await wait(delays[attempt])
  }
  return resolveProject()
}
