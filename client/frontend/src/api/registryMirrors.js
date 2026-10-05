import { post } from '@/utils/request.js'
import { applyEnvironmentHeaders } from '@edition/request-environment'

export function checkRegistryMirror(url, { signal, environmentId = 'local' } = {}) {
  const path = '/images/mirrors/check'
  return post(path, { url }, {
    signal,
    timeout: 8000,
    headers: applyEnvironmentHeaders(path, 'POST', {}, environmentId)
  })
}
