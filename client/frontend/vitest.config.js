/// <reference types="vitest" />
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import path from 'path'

const isCommunity = String(process.env.VITE_TRADIS_EDITION || '').trim().toLowerCase() === 'community'

export default defineConfig({
  plugins: [vue()],
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['src/**/*.{test,spec}.{js,ts}'],
    setupFiles: ['./src/test/setup.js'],
    css: false,
    coverage: {
      reporter: ['text', 'json', 'html'],
      exclude: [
        'node_modules/',
        'src/**/*.d.ts',
        'src/**/__tests__/',
      ],
    },
  },
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
    },
  },
  define: {
    __TRADIS_EDITION__: JSON.stringify(isCommunity ? 'community' : 'full')
  },
  css: {
    postcss: {
      plugins: [],
    },
  },
})
