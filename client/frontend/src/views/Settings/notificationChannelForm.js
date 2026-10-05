// PushPlus 官方渠道契约：空/wechat 为默认微信公众号；cp（企业微信应用）与
// webhook（含企业微信群机器人）必须填写在 PushPlus 个人中心维护的渠道配置编码。
// 历史 cpwebhook 不是合法渠道，编辑时归一化为 webhook 并要求重新填写编码，不自动编造。

export const normalizePushPlusChannel = value => value === 'cpwebhook' ? 'webhook' : (value || '')
export const pushPlusOptionRequired = value => ['cp', 'webhook'].includes(normalizePushPlusChannel(value))
export const pushPlusOptionLabel = value => normalizePushPlusChannel(value) === 'cp'
  ? '企业微信应用编码'
  : 'Webhook 编码'
export const pushPlusOptionMissing = form => pushPlusOptionRequired(String(form?.pushChannel || '')) &&
  !String(form?.pushOption || '').trim()

export function buildPushPlusConfig (form = {}) {
  const config = {}
  const server = String(form.server || '').trim()
  const channel = normalizePushPlusChannel(String(form.pushChannel || '').trim())
  const option = String(form.pushOption || '').trim()
  if (server) config.server = server
  if (channel) config.channel = channel
  if (option) config.option = option
  return config
}
