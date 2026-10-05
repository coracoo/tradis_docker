export const sharedChildRoutes = [
  {
    path: '',
    name: 'Overview',
    meta: { title: '概览' },
    component: () => import('../views/Overview/index.vue')
  },
  {
    path: '/images',
    name: 'Images',
    meta: { title: '镜像' },
    component: () => import('../views/Images/index.vue')
  },
  {
    path: '/networks',
    name: 'Networks',
    meta: { title: '网络' },
    component: () => import('../views/Networks/index.vue')
  },
  {
    path: '/volumes',
    name: 'Volumes',
    meta: { title: '存储卷' },
    component: () => import('../views/Volumes/index.vue')
  },
  {
    path: '/containers',
    name: 'Containers',
    meta: { title: '容器' },
    component: () => import('../views/Containers/index.vue')
  },
  {
    path: '/compose',
    name: 'Compose',
    meta: { title: 'Compose' },
    component: () => import('../views/Compose/index.vue')
  },
  {
    path: '/navigation',
    name: 'Navigation',
    meta: { title: '应用导航' },
    component: () => import('../views/Navigation/index.vue')
  },
  {
    path: '/appstore',
    name: 'AppStore',
    meta: { title: '应用商店' },
    component: () => import('../views/AppStore/index.vue')
  },
  {
    path: '/appstore/deploy/:id',
    name: 'AppDeploy',
    meta: { title: '应用部署' },
    component: () => import('../views/AppStore/Deploy.vue')
  },
  {
    path: '/nas-store',
    name: 'NasStore',
    meta: { title: 'NAS 选购' },
    component: () => import('../views/NasStore/index.vue')
  },
  {
    path: '/nas-store/topic/:slug',
    name: 'NasTopic',
    meta: { title: 'NAS 专题' },
    component: () => import('../views/NasStore/Topic.vue')
  },
  {
    path: '/nas-store/review/:id',
    name: 'NasReview',
    meta: { title: 'NAS 评测' },
    component: () => import('../views/NasStore/Review.vue')
  },
  {
    path: '/tutorials',
    name: 'Tutorials',
    meta: { title: '教程' },
    component: () => import('../views/Tutorials/index.vue')
  },
  {
    path: '/tutorials/:slug',
    name: 'TutorialDetail',
    meta: { title: '教程' },
    component: () => import('../views/Tutorials/index.vue')
  },
  {
    path: '/ports',
    name: 'Ports',
    meta: { title: '端口' },
    component: () => import('../views/Ports/index.vue')
  },
  {
    path: '/cleanup',
    name: 'Cleanup',
    meta: { title: '空间清理' },
    component: () => import('../views/Cleanup/index.vue')
  },
  {
    path: '/settings',
    name: 'Settings',
    meta: { title: '设置' },
    component: () => import('../views/Settings/index.vue')
  },
  {
    path: '/scheduled-tasks',
    name: 'ScheduledTasks',
    meta: { title: '定时任务' },
    component: () => import('../views/ScheduledTasks/index.vue')
  }
]
