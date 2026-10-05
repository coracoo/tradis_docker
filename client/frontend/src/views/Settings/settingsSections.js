export const settingCategories = [
  { key: 'general', label: '常规' },
  { key: 'deployment', label: '部署' },
  { key: 'services', label: '服务' },
  { key: 'notifications', label: '通知与备份' }
]

export const SETTINGS_CATEGORY_STORAGE_KEY = 'tradis-settings-category'

const fullEdition = __TRADIS_EDITION__ === 'full'

export const settingSections = [
  { key: 'appearance', category: 'general', column: 'left', elementId: 'appearance-settings', label: '外观设置', keywords: '主题色 暗色模式 图标风格 lucide tabler' },
  { key: 'security', category: 'general', column: 'left', elementId: 'security-settings', label: '安全设置', keywords: '管理员 密码 安全' },
  { key: 'onboarding', category: 'general', column: 'left', elementId: 'onboarding-settings', label: '新手引导', keywords: '新手 引导 巡游 入门 onboarding' },
  { key: 'advanced', category: 'general', column: 'right', elementId: 'advanced-settings', label: '高级选项', keywords: '高级模式 yaml 编辑' },
  { key: 'diagnostics', category: 'general', column: 'right', elementId: 'diagnostics-settings', label: '系统诊断', keywords: '诊断包 排障 导出 docker 任务 事件' },
  ...(fullEdition ? [
    { key: 'license', category: 'general', column: 'right', elementId: 'license-settings', label: 'Go 授权', keywords: 'go 许可 订阅 激活 实例 爱发电 订单 购买' }
  ] : []),
  { key: 'notifications', category: 'notifications', column: 'left', elementId: 'notification-settings', label: '通知偏好', keywords: '部署 git 导航 备份 系统通知 webhook 企业微信 ntfy gotify bark pushplus' },
  { key: 'service', category: 'services', column: 'left', elementId: 'service-settings', label: '服务配置', keywords: '内网 外网 rss cdn 镜像更新 教程缓存' },
  { key: 'ai', category: 'services', column: 'left', elementId: 'ai-settings', label: 'AI 配置', keywords: '模型 base url api key 导航 compose prompt' },
  { key: 'nasDefaults', category: 'deployment', column: 'left', elementId: 'nas-defaults-settings', label: 'NAS 部署默认值', keywords: 'nas 部署 默认值 用户 puid pgid 时区 媒体 照片 音乐 漫画 小说 路径 host 网络' },
  ...(fullEdition ? [
    { key: 'remoteNodes', category: 'deployment', column: 'right', elementId: 'remote-management-settings', label: '远程设备', keywords: '远程 多主机 节点 nas agent 登录 设备 管理' },
    { key: 'github', category: 'services', column: 'right', elementId: 'github-token-settings', label: 'GitHub Token', keywords: 'github api token 同步 翻译 应用发现' }
  ] : []),
  { key: 'ports', category: 'deployment', column: 'right', elementId: 'port-settings', label: '端口管理', keywords: '端口范围 自动分配' },
  { key: 'backup', category: 'notifications', column: 'right', elementId: 'volume-backup-settings', label: '卷备份', keywords: '数据卷 s3 webdav cron 归档 远程备份' }
]

export function resolveStoredSettingCategory(value) {
  const key = String(value || '').trim()
  return settingCategories.some(category => category.key === key) ? key : 'general'
}

export function resolveSettingSectionTarget(sectionKey) {
  const key = String(sectionKey || '').trim()
  const section = settingSections.find(item => item.key === key)
  if (!section) return null
  return {
    key: section.key,
    category: section.category,
    elementId: section.elementId
  }
}

export function getVisibleSettingSectionKeys({ activeCategory, query, isGo }) {
  const keyword = String(query || '').trim().toLowerCase()

  return new Set(settingSections
    .filter((section) => keyword || section.category === activeCategory)
    .filter((section) => section.key !== 'github' || isGo)
    .filter((section) => !keyword || `${section.label} ${section.keywords}`.toLowerCase().includes(keyword))
    .map((section) => section.key))
}
