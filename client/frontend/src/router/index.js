import { createRouter, createWebHistory } from 'vue-router'
import { useThemeStore } from '../stores/theme.js'
import MainLayout from '@edition/layout'
import { editionChildRoutes, editionStandaloneRoutes } from '@edition/routes'
import { resolveEditionNavigation } from '@edition/router-guard'

import Login from '../views/Login/index.vue'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: Login,
    meta: { public: true }
  },
  ...editionStandaloneRoutes,
  {
    path: '/',
    component: MainLayout,
    meta: { requiresAuth: true },
    children: [
      ...editionChildRoutes,
      // 404 页面 - 暂时重定向到首页
      {
        path: ':pathMatch(.*)*',
        redirect: '/'
      }
    ]
  },
  // 全局 404
  {
    path: '/:pathMatch(.*)*',
    redirect: '/'
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior(to) {
    if (to.hash) {
      return {
        el: to.hash,
        top: 20,
        behavior: 'smooth'
      }
    }
    return { top: 0 }
  }
})

// 路由守卫
router.beforeEach(async (to, from, next) => {
  // 初始化主题
  const themeStore = useThemeStore()
  themeStore.init()
  
  // 检查登录状态
  const isLoggedIn = localStorage.getItem('loggedIn') === 'true'
  
  if (to.meta.requiresAuth && !isLoggedIn) {
    next({ path: '/login', query: { redirect: to.fullPath } })
  } else if (to.meta.public && isLoggedIn) {
    next({ path: '/' })
  } else {
		const editionRedirect = resolveEditionNavigation(to.path)
		if (editionRedirect) {
			next(editionRedirect)
			return
		}
    next()
  }
})

export default router
