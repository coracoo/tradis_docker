import { computed, reactive } from 'vue'
import { defineStore } from 'pinia'
import { images, containers, volumes, networks, compose } from '@edition/api'
import { currentEnvironmentId } from '@edition/current-environment'
import { applyEnvironmentHeaders } from '@edition/request-environment'

const DEFAULT_TTL = 15000
const COMPOSE_TTL = 8000
const CONTAINERS_TTL = 2000

function createResource(initialData) {
  return {
    data: cloneResourceData(initialData),
    initialData: cloneResourceData(initialData),
    environmentId: '',
    loaded: false,
    updatedAt: 0,
    generation: 0,
    loading: false,
    refreshing: false,
    error: null
  }
}

function cloneResourceData(value) {
  return JSON.parse(JSON.stringify(value))
}

function containerName(container) {
  return (container.Names?.[0] || '').replace(/^\//, '') || container.Id?.substring(0, 12) || '未命名'
}

function composeContainerItem(container) {
  const labels = container.Labels || container.Config?.Labels || {}
  return {
    Id: container.Id,
    Names: container.Names || [`/${containerName(container)}`],
    State: container.State || 'unknown',
    Status: container.Status || '',
    HealthStatus: container.HealthStatus || container.healthStatus || '',
    RunningTime: container.RunningTime || container.runningTime || '',
    Image: container.Image || '-',
    Created: container.Created,
    Ports: container.Ports || [],
    UpdateAvailable: Boolean(container.UpdateAvailable ?? container.updateAvailable),
    updateAvailable: Boolean(container.UpdateAvailable ?? container.updateAvailable),
    _name: containerName(container),
    _service: labels['com.docker.compose.service'] || '',
    _cpu: '0%',
    _memory: '0 MB'
  }
}

function isVisibleComposeProject(project) {
  const name = String(project?.name || '').trim()
  return !name.startsWith('.')
}

export function buildComposeProjects(composeList = [], allContainers = [], containerRemarks = {}) {
  const containersByProject = new Map()
  const projects = []

  for (const container of allContainers || []) {
    const labels = container.Labels || container.Config?.Labels || {}
    const projectName = labels['com.docker.compose.project']
    if (!projectName) continue
    if (!containersByProject.has(projectName)) {
      containersByProject.set(projectName, [])
    }
    containersByProject.get(projectName).push(composeContainerItem(container))
  }

  for (const project of composeList || []) {
    if (!isVisibleComposeProject(project)) continue
    const composeProjectName = project.composeProjectName || project.name
    projects.push({
      ...project,
      containers: containersByProject.get(composeProjectName) || [],
      type: 'compose'
    })
  }

  for (const container of allContainers || []) {
    const labels = container.Labels || container.Config?.Labels || {}
    if (labels['com.docker.compose.project']) continue
    const name = containerName(container)
    projects.push({
      name,
      type: 'container',
      containers: [composeContainerItem(container)],
      path: '-',
      createTime: container.Created,
      isSelf: container.isSelf || false,
      remark: containerRemarks[name] || '',
      _container: container
    })
  }

  return projects
}

export const useDockerResourcesStore = defineStore('dockerResources', () => {
  const resources = reactive({
    containers: createResource([]),
    images: createResource({ images: [], containers: [] }),
    volumes: createResource({ Volumes: [], Warnings: [] }),
    networks: createResource([]),
    composeProjects: createResource([])
  })

  const inFlight = new Map()
  const inFlightGenerations = new Map()
  const pendingForcedFetch = new Map()

  async function loadResource(key, fetcher, options = {}) {
    const resource = resources[key]
    const environmentId = options.environmentId || currentEnvironmentId()
    const requestKey = `${environmentId}:${key}`
    const ttl = options.ttl ?? DEFAULT_TTL
    const now = Date.now()

    if (resource.environmentId !== environmentId) {
      resource.data = cloneResourceData(resource.initialData)
      resource.loaded = false
      resource.updatedAt = 0
      resource.loading = false
      resource.refreshing = false
      resource.error = null
      resource.environmentId = environmentId
      resource.generation++
    }

    const force = !!options.force || (inFlight.has(requestKey) && inFlightGenerations.get(requestKey) !== resource.generation)

    if (!force && resource.loaded) {
      if (now - resource.updatedAt < ttl) {
        return resource.data
      }
      if (!options.fresh) {
        if (!inFlight.has(requestKey)) {
          void runFetch(key, fetcher, environmentId).catch(() => {})
        }
        return resource.data
      }
    }

    if (inFlight.has(requestKey)) {
      if (force) {
        if (pendingForcedFetch.has(requestKey)) {
          return pendingForcedFetch.get(requestKey)
        }

        const activeRequest = inFlight.get(requestKey)
        const trailingRequest = activeRequest
          .catch(() => undefined)
          .then(() => runFetch(key, fetcher, environmentId))
        const trackedRequest = trailingRequest.finally(() => {
          if (pendingForcedFetch.get(requestKey) === trackedRequest) {
            pendingForcedFetch.delete(requestKey)
          }
        })
        pendingForcedFetch.set(requestKey, trackedRequest)
        return trackedRequest
      }
      return inFlight.get(requestKey)
    }
    return runFetch(key, fetcher, environmentId)
  }

  async function runFetch(key, fetcher, environmentId) {
    const resource = resources[key]
    const requestKey = `${environmentId}:${key}`
    const generation = resource.generation
    if (resource.environmentId === environmentId) {
      resource.loading = !resource.loaded
      resource.refreshing = resource.loaded
      resource.error = null
    }

    const promise = Promise.resolve()
      .then(() => fetcher(environmentId))
      .then((data) => {
        if (resource.environmentId === environmentId && resource.generation === generation) {
          resource.data = data
          resource.loaded = true
          resource.updatedAt = Date.now()
        }
        return data
      })
      .catch((error) => {
        if (resource.environmentId === environmentId && resource.generation === generation) resource.error = error
        throw error
      })
      .finally(() => {
        if (resource.environmentId === environmentId) {
          resource.loading = false
          resource.refreshing = false
        }
        inFlight.delete(requestKey)
        inFlightGenerations.delete(requestKey)
      })

    inFlight.set(requestKey, promise)
    inFlightGenerations.set(requestKey, generation)
    return promise
  }

  function invalidate(key) {
    if (resources[key]) {
      resources[key].updatedAt = 0
      resources[key].generation++
    }
    if (key === 'images' || key === 'composeProjects') invalidate('containers')
  }

  function invalidateAll() {
    Object.keys(resources).forEach(invalidate)
  }

  function loadImages(options = {}) {
    return loadResource('images', async (environmentId) => {
      const [imageList, containerList] = await Promise.all([
        images.list({ ...readOptions('/images', environmentId), force: !!options.force }),
        loadContainers(environmentId, options.force)
      ])
      return {
        images: imageList || [],
        containers: containerList || []
      }
    }, { ttl: DEFAULT_TTL, ...options })
  }

  function readOptions(path, environmentId) {
    return { headers: applyEnvironmentHeaders(path, 'GET', {}, environmentId) }
  }

  function loadContainers(environmentId, force = false) {
    return loadResource('containers', () => containers.list({ all: true }, readOptions('/containers', environmentId)), {
      ttl: CONTAINERS_TTL, fresh: true, force, environmentId
    })
  }

  function loadVolumes(options = {}) {
    return loadResource('volumes', (environmentId) => volumes.list(readOptions('/volumes', environmentId)), { ttl: DEFAULT_TTL, ...options })
  }

  function loadNetworks(options = {}) {
    return loadResource('networks', (environmentId) => networks.list({ brief: true }, readOptions('/networks', environmentId)), { ttl: DEFAULT_TTL, ...options })
  }

  function loadComposeProjects(options = {}) {
    return loadResource('composeProjects', async (environmentId) => {
      const force = !!options.force
      const [composeList, allContainers, containerRemarks] = await Promise.all([
        compose.list(force ? { force: '1', _: Date.now().toString() } : {}, readOptions('/compose/list', environmentId)),
        loadContainers(environmentId, force),
        environmentId === 'local' ? compose.listContainerRemarks(readOptions('/compose/container-remarks', environmentId)) : Promise.resolve({})
      ])
      return buildComposeProjects(composeList || [], allContainers || [], containerRemarks || {})
    }, { ttl: COMPOSE_TTL, ...options })
  }

  return {
    resources,
    imageList: computed(() => resources.images.data.images || []),
    imageContainerList: computed(() => resources.images.data.containers || []),
    volumeList: computed(() => resources.volumes.data?.Volumes || []),
    networkList: computed(() => resources.networks.data || []),
    composeProjectList: computed(() => resources.composeProjects.data || []),
    loadImages,
    loadVolumes,
    loadNetworks,
    loadComposeProjects,
    invalidate,
    invalidateAll
  }
})
