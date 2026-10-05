export const sharedSidebarCatalog = [
  {
    key: 'overview',
    title: '总览',
    items: [
      { path: '/', label: '仪表盘', ariaLabel: '仪表盘', icon: 'dashboard' },
      { path: '/navigation', label: '导航页', ariaLabel: '应用导航', icon: 'navigation' }
    ]
  },
  {
    key: 'services',
    title: '服务',
    items: [
      { path: '/appstore', label: '应用商店', ariaLabel: '应用商店', icon: 'store' },
      { path: '/tutorials', label: '教程', ariaLabel: '教程中心', icon: 'book-open' },
      { path: '/scheduled-tasks', label: '定时任务', ariaLabel: '定时任务', icon: 'clock', remoteRestricted: true }
    ]
  },
  {
    key: 'docker',
    title: 'Docker 管理',
    items: [
      { path: '/compose', label: 'Compose', ariaLabel: 'Compose 项目', icon: 'compose' },
      { path: '/containers', label: '容器', ariaLabel: '容器管理', icon: 'container' },
      { path: '/images', label: '镜像', ariaLabel: '镜像管理', icon: 'image' },
      { path: '/volumes', label: '数据卷', ariaLabel: '数据卷管理', icon: 'database' },
      { path: '/networks', label: '网络', ariaLabel: '网络管理', icon: 'network' },
      { path: '/ports', label: '端口', ariaLabel: '端口管理', icon: 'server', remoteRestricted: true },
      { path: '/cleanup', label: '空间清理', ariaLabel: '空间清理', icon: 'brush', remoteRestricted: true }
    ]
  }
]
