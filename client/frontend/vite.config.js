import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import Icons from 'unplugin-icons/vite'
import IconsResolver from 'unplugin-icons/resolver'
import Components from 'unplugin-vue-components/vite'
import path from 'path'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const edition = String(env.VITE_TRADIS_EDITION || process.env.VITE_TRADIS_EDITION || 'full').trim().toLowerCase()
  if (!['full', 'community'].includes(edition)) {
    throw new Error(`Unsupported TRADIS frontend edition: ${edition}`)
  }
  const isCommunity = edition === 'community'
  const frontendPort = Number.parseInt(env.FRONTEND_PORT || process.env.FRONTEND_PORT || '33339', 10)
  const backendPort = Number.parseInt(env.BACKEND_PORT || process.env.BACKEND_PORT || '8080', 10)
  const backendHost = env.BACKEND_HOST || process.env.BACKEND_HOST || 'localhost'

  return {
    plugins: [
      vue(),
      Components({
        resolvers: [
          IconsResolver({
            prefix: 'Icon',
            enabledCollections: ['ep', 'mdi', 'lucide', 'tabler', 'simple-icons']
          })
        ]
      }),
      Icons({ autoInstall: true })
    ],
    base: '/',
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
        '@edition/layout': path.resolve(__dirname, './src/components/layout/MainLayout.vue'),
        '@edition/layout-environment': path.resolve(__dirname, isCommunity ? './src/community/layoutEnvironment.js' : './src/edition/full/layoutEnvironment.js'),
        '@edition/sidebar-catalog': path.resolve(__dirname, isCommunity ? './src/community/sidebarCatalog.js' : './src/edition/full/sidebarCatalog.js'),
        '@edition/sidebar-update': path.resolve(__dirname, isCommunity ? './src/community/sidebarUpdate.js' : './src/edition/full/sidebarUpdate.js'),
        '@edition/remote-navigation': path.resolve(__dirname, isCommunity ? './src/utils/remoteNavigation.js' : './src/edition/full/remoteNavigation.js'),
        '@edition/global-search-catalog': path.resolve(__dirname, isCommunity ? './src/community/globalSearchCatalog.js' : './src/edition/full/globalSearchCatalog.js'),
        '@edition/global-search-resources': path.resolve(__dirname, isCommunity ? './src/community/globalSearchResources.js' : './src/edition/full/globalSearchResources.js'),
        '@edition/routes': path.resolve(__dirname, isCommunity ? './src/community/routes.js' : './src/edition/full/routes.js'),
        '@edition/overview-remote-environment-rail': path.resolve(__dirname, isCommunity ? './src/community/overviewRemoteEnvironmentRail.js' : './src/edition/full/overviewRemoteEnvironmentRail.js'),
        '@edition/router-guard': path.resolve(__dirname, isCommunity ? './src/community/routerGuard.js' : './src/edition/full/routerGuard.js'),
        '@edition/request-environment': path.resolve(__dirname, isCommunity ? './src/community/requestEnvironment.js' : './src/edition/full/requestEnvironment.js'),
        '@edition/environment-context': path.resolve(__dirname, isCommunity ? './src/community/environmentContext.js' : './src/edition/full/environmentContext.js'),
        '@edition/current-environment': path.resolve(__dirname, isCommunity ? './src/community/currentEnvironment.js' : './src/edition/full/currentEnvironment.js'),
        '@edition/appstore-deployment-metrics': path.resolve(__dirname, isCommunity ? './src/community/appStoreDeploymentMetrics.js' : './src/edition/full/appStoreDeploymentMetrics.js'),
        '@edition/appstore-post-deploy': path.resolve(__dirname, isCommunity ? './src/community/appStorePostDeploy.js' : './src/edition/full/appStorePostDeploy.js'),
        '@edition/resource-workbench-context': path.resolve(__dirname, isCommunity ? './src/community/resourceWorkbenchContext.js' : './src/edition/full/resourceWorkbenchContext.js'),
        '@edition/navigation-ai': path.resolve(__dirname, isCommunity ? './src/community/navigationAI.js' : './src/edition/full/navigationAI.js'),
        '@edition/tutorial-actions': path.resolve(__dirname, isCommunity ? './src/community/tutorialActions.js' : './src/edition/full/tutorialActions.js'),
        '@edition/compose-features': path.resolve(__dirname, isCommunity ? './src/community/composeFeatures.js' : './src/edition/full/composeFeatures.js'),
        '@edition/compose-protection': path.resolve(__dirname, isCommunity ? './src/community/composeProtection.js' : './src/edition/full/composeProtection.js'),
        '@edition/compose-feature-dialogs': path.resolve(__dirname, isCommunity ? './src/community/composeFeatureDialogs.js' : './src/edition/full/composeFeatureDialogs.js'),
        '@edition/settings-dependencies': path.resolve(__dirname, isCommunity ? './src/community/settingsDependencies.js' : './src/edition/full/settingsDependencies.js'),
        '@edition/deployment-defaults': path.resolve(__dirname, isCommunity ? './src/community/deploymentDefaults.js' : './src/edition/full/deploymentDefaults.js'),
        '@edition/api': path.resolve(__dirname, isCommunity ? './src/community/api.js' : './src/edition/full/api.js'),
        '@edition/scheduled-task-features': path.resolve(__dirname, isCommunity ? './src/community/scheduledTaskFeatures.js' : './src/edition/full/scheduledTaskFeatures.js')
      }
    },
    server: {
      host: '0.0.0.0',
      port: Number.isFinite(frontendPort) ? frontendPort : 33339,
      proxy: {
        '/api': {
          target: `http://${backendHost}:${Number.isFinite(backendPort) ? backendPort : 8080}`,
          changeOrigin: true,
          ws: true,
          secure: false
        },
        '/data/pic': {
          target: `http://${backendHost}:${Number.isFinite(backendPort) ? backendPort : 8080}`,
          changeOrigin: true,
          secure: false
        }
      }
    },
    build: {
      // Monaco 编辑器仅在 Compose 编辑时异步加载，其核心 chunk 大于 Vite 默认 500KB 阈值。
      chunkSizeWarningLimit: 3000,
      rollupOptions: {
        output: {
          // 入口文件命名
          entryFileNames: 'assets/[name]-[hash].js',
          // 代码分割后的chunk命名
          chunkFileNames: 'assets/[name]-[hash].js',
          // 静态资源命名
          assetFileNames: (assetInfo) => {
            const info = assetInfo.name.split('.')
            const ext = info[info.length - 1]
            if (/\.(png|jpe?g|gif|svg|webp|ico)$/i.test(assetInfo.name)) {
              return 'assets/img/[name]-[hash][extname]'
            }
            if (/\.(woff2?|eot|ttf|otf)$/i.test(assetInfo.name)) {
              return 'assets/fonts/[name]-[hash][extname]'
            }
            return 'assets/[name]-[hash][extname]'
          }
        }
      },
      // 压缩配置（使用esbuild，Vite默认）
      minify: 'esbuild',
      // 资源内联阈值（小于4KB内联为base64）
      assetsInlineLimit: 4096,
      // 源码映射（生产环境关闭）
      sourcemap: false
    },
    define: {
      __TRADIS_EDITION__: JSON.stringify(edition)
    }
  }
})
