import { getProxy } from '@/api/images.js'
import { listContainers } from '@/api/containers.js'
import { listCompose } from '@/api/compose.js'

export const TOUR_VERSION = 1

function asArray(value) {
  if (Array.isArray(value)) return value
  if (value && Array.isArray(value.containers)) return value.containers
  if (value && Array.isArray(value.projects)) return value.projects
  if (value && Array.isArray(value.list)) return value.list
  return []
}

async function mirrorsConfigured() {
  try {
    const data = await getProxy()
    const mirrors = data?.['registry-mirrors']
    return Array.isArray(mirrors) && mirrors.length > 0
  } catch {
    return false
  }
}

async function anyContainer() {
  try {
    return asArray(await listContainers()).length > 0
  } catch {
    return false
  }
}

async function anyComposeProject() {
  try {
    return asArray(await listCompose()).length > 0
  } catch {
    return false
  }
}

export const tourSteps = [
  {
    id: 'settings',
    title: '基础设置',
    body: '先把时区、通知等基础项调好，后面用起来更顺手。',
    ctaLabel: '去设置',
    route: '/settings',
    anchor: '[data-tour="settings-categories"]'
  },
  {
    id: 'mirrors',
    title: '配置镜像加速',
    body: '在这里粘贴加速地址，拉镜像又快又稳；也可以先用预设一键填入。',
    ctaLabel: '打开镜像设置',
    route: '/',
    openDockerSettings: true,
    anchor: '[data-tour="mirror-input"]',
    detect: mirrorsConfigured
  },
  {
    id: 'appstore',
    title: '逛应用商店',
    body: '常用应用一键部署，不用手写 Compose。',
    ctaLabel: '去应用商店',
    route: '/appstore',
    anchor: '[data-tour="appstore-deploy"]',
    detect: anyContainer
  },
  {
    id: 'compose',
    title: '部署第一个 Compose',
    body: '多容器应用用 Compose 编排，点这里新建项目。',
    ctaLabel: '去 Compose',
    route: '/compose',
    anchor: '[data-tour="compose-create"]',
    detect: anyComposeProject
  }
]

export const TOUR_STEP_IDS = tourSteps.map(step => step.id)
