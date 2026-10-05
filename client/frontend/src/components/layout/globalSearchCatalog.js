import { settingSections } from '@/views/Settings/settingsSections.js'

export const GLOBAL_SEARCH_TYPE_LABELS = {
  container: '容器', compose: 'Compose', image: '镜像', volume: '数据卷', network: '网络',
  navigation: '导航应用', 'scheduled-task': '定时任务', action: '功能', setting: '设置', page: '页面'
}
export const GLOBAL_SEARCH_TYPE_ICONS = {
  container: 'container', compose: 'compose', image: 'image', volume: 'database', network: 'network',
  navigation: 'navigation', 'scheduled-task': 'clock', action: 'plus', setting: 'settings', page: 'layout-dashboard'
}


export const GLOBAL_SEARCH_PAGES = [
  {
    id: 'page-overview',
    type: 'page',
    name: '仪表盘',
    description: '查看系统概览与运行状态',
    icon: 'dashboard',
    keywords: ['首页', '概览', '状态'],
    route: { path: '/' }
  },
  {
    id: 'page-containers',
    type: 'page',
    name: '容器',
    description: '管理 Docker 容器',
    icon: 'container',
    keywords: ['docker', '运行', '服务', '强制停止', '批量操作', '容器更新', '日志', '终端', '健康检查'],
    route: { path: '/containers' }
  },
  {
    id: 'page-compose',
    type: 'page',
    name: 'Compose',
    description: '管理 Compose 项目',
    icon: 'compose',
    keywords: ['项目', 'yaml', 'docker compose', 'git 导入', '同步', '安全更新', '备注', '日志', '批量操作', '强制停止'],
    route: { path: '/compose' }
  },
  {
    id: 'page-images',
    type: 'page',
    name: '镜像',
    description: '管理镜像与构建缓存',
    icon: 'image',
    keywords: ['image', '缓存', '拉取', '更新检测', '检测更新', '构建缓存', '清理', '导入'],
    route: { path: '/images' }
  },
  {
    id: 'page-volumes',
    type: 'page',
    name: '数据卷',
    description: '管理持久化存储与备份',
    icon: 'database',
    keywords: ['volume', '卷', '存储', '备份', '使用关系', '清理'],
    route: { path: '/volumes' }
  },
  {
    id: 'page-networks',
    type: 'page',
    name: '网络',
    description: '管理 Docker 网络',
    icon: 'network',
    keywords: ['network', 'bridge', 'macvlan', '使用关系', '清理'],
    route: { path: '/networks' }
  },
  {
    id: 'page-ports',
    type: 'page',
    name: '端口',
    description: '查看端口占用与自动分配范围',
    icon: 'plug',
    keywords: ['port', 'tcp', 'udp', '映射'],
    route: { path: '/ports' }
  },
  {
    id: 'page-cleanup',
    type: 'page',
    name: '空间清理',
    description: '评估并清理未使用的 Docker 资源',
    icon: 'brush',
    keywords: ['清理', '空间', '磁盘', 'prune', 'build cache', '缓存'],
    route: { path: '/cleanup' }
  },
  {
    id: 'page-navigation',
    type: 'page',
    name: '导航页',
    description: '维护应用访问入口',
    icon: 'navigation',
    keywords: ['应用导航', '网址', '入口', '隐藏', '自动隐藏', '恢复'],
    route: { path: '/navigation' }
  },
  {
    id: 'page-appstore',
    type: 'page',
    name: '应用商店',
    description: '浏览和部署应用模板',
    icon: 'store',
    keywords: ['模板', '安装', '部署', '后台部署', '高级 yaml', '.env', '环境变量'],
    route: { path: '/appstore' }
  },
  {
    id: 'page-nas-store',
    type: 'page',
    name: 'NAS 选购',
    description: '筛选和比较 NAS 设备',
    icon: 'hard-drive',
    keywords: ['nas', '导购', '设备', '型号', '处理器', '内存', '价格', '对比'],
    route: { path: '/nas-store' }
  },
  {
    id: 'page-tutorials',
    type: 'page',
    name: '教程中心',
    description: '阅读部署教程',
    icon: 'book-open',
    keywords: ['教程', '文章', '部署指南'],
    route: { path: '/tutorials' }
  },
  {
    id: 'page-scheduled-tasks',
    type: 'page',
    name: '定时任务',
    description: '管理后台调度任务',
    icon: 'clock',
    keywords: ['定时', '计划', '调度', 'cron', '执行历史', '应用保护计划'],
    route: { path: '/scheduled-tasks' }
  },
  {
    id: 'page-settings',
    type: 'page',
    name: '设置',
    description: '配置系统与功能服务',
    icon: 'settings',
    keywords: ['配置', '主题', '服务'],
    route: { path: '/settings' }
  }
]

export const GLOBAL_SEARCH_ACTIONS = [
  {
    id: 'create-container',
    type: 'action',
    name: '新建容器',
    description: '打开容器创建表单',
    icon: 'container',
    keywords: ['创建', '新增', 'docker run'],
    route: { path: '/containers', query: { action: 'create' } }
  },
  {
    id: 'create-compose',
    type: 'action',
    name: '新建 Compose 项目',
    description: '编写或粘贴 Compose YAML',
    icon: 'compose',
    keywords: ['创建', '新增', '项目', 'yaml'],
    route: { path: '/compose', query: { action: 'create' } }
  },
  {
    id: 'import-compose-git',
    type: 'action',
    name: 'Git 导入 Compose',
    description: '从 GitHub 仓库导入项目',
    icon: 'github',
    keywords: ['git 导入', 'github 导入', '仓库', '项目'],
    route: { path: '/compose', query: { action: 'git-import' } }
  },
  {
    id: 'pull-image',
    type: 'action',
    name: '拉取镜像',
    description: '从镜像仓库拉取新镜像',
    icon: 'download',
    keywords: ['新增', '下载', 'pull', 'image'],
    route: { path: '/images', query: { action: 'pull' } }
  },
  {
    id: 'import-image',
    type: 'action',
    name: '导入镜像',
    description: '从本地归档文件导入镜像',
    icon: 'upload',
    keywords: ['新增', '上传', 'tar', 'image'],
    route: { path: '/images', query: { action: 'import' } }
  },
  {
    id: 'create-volume',
    type: 'action',
    name: '新建数据卷',
    description: '创建持久化数据卷',
    icon: 'database',
    keywords: ['创建', '新增', 'volume', '存储'],
    route: { path: '/volumes', query: { action: 'create' } }
  },
  {
    id: 'create-network',
    type: 'action',
    name: '新建网络',
    description: '创建 Docker 网络',
    icon: 'network',
    keywords: ['创建', '新增', 'network', 'bridge'],
    route: { path: '/networks', query: { action: 'create' } }
  },
  {
    id: 'create-navigation',
    type: 'action',
    name: '添加导航应用',
    description: '在导航页添加应用入口',
    icon: 'navigation',
    keywords: ['创建', '新增', '添加应用', '网址'],
    route: { path: '/navigation', query: { action: 'create' } }
  },
  {
    id: 'create-scheduled-task',
    type: 'action',
    name: '新建定时任务',
    description: '创建后台调度任务',
    icon: 'clock',
    keywords: ['创建', '新增', '计划', 'cron'],
    route: { path: '/scheduled-tasks', query: { action: 'create' } }
  },
  {
    id: 'configure-port-range',
    type: 'action',
    name: '设置自动端口范围',
    description: '配置自动部署可分配的端口范围',
    icon: 'settings',
    keywords: ['端口设置', '范围设置', '自动端口', 'port'],
    route: { path: '/ports', query: { action: 'range' } }
  },
  {
    id: 'check-image-updates',
    type: 'action',
    name: '检测镜像更新',
    description: '检查本地镜像是否存在可用更新',
    icon: 'refresh',
    keywords: ['镜像更新', '检测更新', 'image update'],
    route: { path: '/images', query: { action: 'check-updates' } }
  },
]

export const GLOBAL_SEARCH_SETTINGS = settingSections.map(section => ({
  id: `setting-${section.key}`,
  type: 'setting',
  name: section.label,
  description: `打开设置中的${section.label}`,
  icon: 'settings',
  keywords: String(section.keywords || '').split(/\s+/).filter(Boolean),
  route: { path: '/settings', query: { section: section.key } }
}))

export function searchGlobalCatalog(query) {
  const terms = String(query || '')
    .trim()
    .toLowerCase()
    .split(/\s+/)
    .filter(Boolean)

  if (terms.length === 0) return []

  return [...GLOBAL_SEARCH_ACTIONS, ...GLOBAL_SEARCH_SETTINGS, ...GLOBAL_SEARCH_PAGES].filter(item => {
    const haystack = [item.name, item.description, ...(item.keywords || [])]
      .join(' ')
      .toLowerCase()
    return terms.every(term => haystack.includes(term))
  })
}

export async function loadEditionSearchData() {
  return {}
}

export function searchEditionResources() {
  return []
}
