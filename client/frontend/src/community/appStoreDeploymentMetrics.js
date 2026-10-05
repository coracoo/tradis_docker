export const appStoreDeploymentMetrics = {
  enabled: false,
  sortOptions: [],
  fetch: async () => null,
  apply: (apps = []) => apps.map(app => ({
    ...app,
    deployment_count: 0,
    show_deployment_count: false
  })),
  clear: (apps = []) => apps.map(app => ({
    ...app,
    deployment_count: 0,
    show_deployment_count: false
  }))
}
