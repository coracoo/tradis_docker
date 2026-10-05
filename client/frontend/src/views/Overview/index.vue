<template>
  <ResourceWorkbench>
    <div class="overview-page" role="main" aria-label="仪表盘概览">
    <!-- 顶部统计卡片 -->
    <section class="stats-section">
      <div class="stats-grid">
        <div
          v-for="stat in orderedStatistics"
          :key="stat.key"
          class="stat-card"
          :class="`stat-${stat.type}`"
          draggable="true"
          @dragstart="handleStatDragStart(stat.key)"
          @dragover.prevent
          @drop="handleStatDrop(stat.key)"
          @dragend="draggingStatKey = ''"
          @click="handleStatCardClick(stat)"
        >
          <div class="stat-content">
            <div class="stat-value">{{ statDisplayValue(stat.key) }}</div>
            <div class="stat-label">{{ stat.label }}</div>
          </div>
          <div class="stat-icon-wrapper">
            <DynamicIcon :name="stat.icon" :size="18" class="stat-icon" />
          </div>
        </div>
      </div>
    </section>

    <!-- 新手引导横幅：未完成且未关闭时显示 -->
    <section v-if="onboarding.showEntryBanner" class="onboarding-banner">
      <DynamicIcon name="rocket" :size="20" class="onboarding-banner-icon" />
      <div class="onboarding-banner-copy">
        <strong>欢迎使用 TRADIS</strong>
        <span>花几分钟完成初始设置：基础配置、镜像加速、部署第一个应用。</span>
      </div>
      <div class="onboarding-banner-actions">
        <button type="button" class="onboarding-banner-btn ghost" @click="onboarding.skip()">以后再说</button>
        <button type="button" class="onboarding-banner-btn primary" @click="onboarding.start()">开始引导</button>
      </div>
    </section>

    <RemoteEnvironmentRail v-if="isFullEdition" />

    <!-- 快速入口 -->
    <section class="quick-links-section">
      <h2 class="section-title">快捷入口</h2>
      <div class="quick-links-grid">
        <button class="quick-link-card nas-store" @click="router.push('/nas-store')">
          <div class="quick-link-icon">
            <DynamicIcon name="hard-drive" :size="24" />
          </div>
          <div class="quick-link-info">
            <span class="quick-link-title">NAS 选购</span>
            <span class="quick-link-desc">主流 NAS 参数对比与导购</span>
          </div>
          <DynamicIcon name="chevron-right" :size="18" />
        </button>
        <button class="quick-link-card image-maintenance" @click="showImageMaintenance = true">
          <div class="quick-link-icon">
            <DynamicIcon name="cloud-download" :size="24" />
          </div>
          <div class="quick-link-info">
            <span class="quick-link-title">镜像拉取维护</span>
            <span class="quick-link-desc">维护镜像加速地址和拉取网络</span>
          </div>
          <DynamicIcon name="chevron-right" :size="18" />
        </button>
        <button v-if="isFullEdition" class="quick-link-card ai-maintenance" @click="router.push('/settings#ai-settings')">
          <div class="quick-link-icon">
            <DynamicIcon name="ai" :size="24" />
          </div>
          <div class="quick-link-info">
            <span class="quick-link-title">AI 维护</span>
            <span class="quick-link-desc">配置模型、密钥和 AI 场景提示词</span>
          </div>
          <DynamicIcon name="chevron-right" :size="18" />
        </button>
        <button class="quick-link-card onboarding-entry" @click="onboarding.start({ resume: true })">
          <div class="quick-link-icon">
            <DynamicIcon name="compass" :size="24" />
          </div>
          <div class="quick-link-info">
            <span class="quick-link-title">新手引导</span>
            <span class="quick-link-desc">跟着步骤完成首次部署</span>
          </div>
          <DynamicIcon name="chevron-right" :size="18" />
        </button>
      </div>
    </section>

    <!-- 下方左右分栏 -->
    <div class="bottom-sections">
      <!-- 左侧系统资源 -->
      <section class="resources-section">
        <h2 class="section-title">系统资源</h2>
        <div class="resources-card">
          <div v-for="(res, key) in resources" :key="key" class="resource-item">
            <div class="resource-header">
              <div class="resource-info">
                <div class="resource-icon" :class="`icon-${key}`">
                  <svg v-html="res.icon" width="20" height="20" viewBox="0 0 24 24"></svg>
                </div>
                <span class="resource-name">{{ res.name }}</span>
              </div>
              <span class="resource-percent" :class="getUsageColorClass(res.percent)">
                {{ formatResourcePercent(res.percent) }}
              </span>
            </div>
            <div class="progress-bar">
              <div
                class="progress-fill"
                :class="getUsageColorClass(res.percent)"
                :style="{ width: resourceProgressWidth(res.percent) }"
              ></div>
            </div>
            <div class="resource-meta">
              <span>已用: {{ res.used }}</span>
              <span>总量: {{ res.total }}</span>
            </div>
          </div>
        </div>
      </section>

      <!-- 右侧系统事件 -->
      <section class="events-section">
        <h2 class="section-title">
          系统事件
          <span class="section-subtitle">（最近 30 条）</span>
        </h2>
        <div class="events-card">
          <div v-if="eventLogs.length === 0" class="events-empty">
              <DynamicIcon name="file-text" :size="48" />
            <p>{{ isRemoteEnvironment ? '远程设备暂不提供系统事件' : '暂无系统事件' }}</p>
          </div>
          <template v-else>
            <div class="events-list-scroll">
              <div class="events-list">
                <div
                  v-for="log in displayedLogs"
                  :key="log.id"
                  class="event-item"
                  role="button"
                  tabindex="0"
                  :title="`点击复制：${log.message}`"
                  @click="copyEvent(log)"
                  @keydown.enter.prevent="copyEvent(log)"
                >
                  <div class="event-status" :class="`status-${log.type}`">
                    <svg v-html="getEventIcon(log.type)" width="20" height="20" viewBox="0 0 24 24"></svg>
                  </div>
                  <div class="event-content">
                    <span class="event-type" :class="`type-${log.type}`">
                      {{ getEventLabel(log.type) }}
                    </span>
                    <span class="event-message">{{ log.message }}</span>
                    <span class="event-time">{{ log.time }}</span>
                  </div>
                </div>
              </div>
            </div>
            <!-- 分页 -->
            <div v-if="totalPages > 1" class="events-pagination">
              <button
                class="page-btn"
                :disabled="currentPage === 1"
                @click="currentPage--"
              >
                <DynamicIcon name="chevron-left" :size="16" />
              </button>
              <span class="page-info">{{ currentPage }} / {{ totalPages }}</span>
              <button
                class="page-btn"
                :disabled="currentPage === totalPages"
                @click="currentPage++"
              >
                <DynamicIcon name="chevron-right" :size="16" />
              </button>
            </div>
          </template>
        </div>
      </section>
    </div>
      <DockerSettings v-model="showImageMaintenance" initial-tab="mirrors" />
    </div>
  </ResourceWorkbench>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/utils/request.js'
import settingsApi from '@/api/settings.js'
import { formatBytes, formatTime } from '@/utils/format.js'
import { copyToClipboard } from '@/utils/helpers.js'
import { useToast } from '@/composables/useToast.js'
import { useCountUp } from '@/composables/useAnimation.js'
import { useEnvironmentContext } from '@/composables/useEnvironmentContext.js'
import { useOnboardingStore } from '@/stores/onboarding.js'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import ResourceWorkbench from '@/components/resource-workbench/ResourceWorkbench.vue'
import DockerSettings from '@/views/Images/components/DockerSettings.vue'
import { RemoteEnvironmentRail } from '@edition/overview-remote-environment-rail'

const isFullEdition = __TRADIS_EDITION__ === 'full'

const router = useRouter()
const onboarding = useOnboardingStore()
const showImageMaintenance = ref(false)
const { environmentId, isRemoteEnvironment } = useEnvironmentContext()

watch(() => onboarding.openDockerSettings, value => {
  if (value) showImageMaintenance.value = true
})
watch(showImageMaintenance, value => {
  if (!value && onboarding.openDockerSettings) onboarding.openDockerSettings = false
})
const managementMode = import.meta.env.VITE_MANAGEMENT_MODE || 'CS'
const STAT_ORDER_STORAGE_KEY = 'overview_stat_order_v1'
const draggingStatKey = ref('')
const suppressStatClick = ref(false)

// SVG 图标
const icons = {
  container: '<rect x="2" y="5" width="20" height="14" rx="2"/><line x1="2" y1="10" x2="22" y2="10"/>',
  running: '<circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>',
  stopped: '<circle cx="12" cy="12" r="10"/><line x1="8" y1="12" x2="16" y2="12"/>',
  image: '<rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><circle cx="8.5" cy="8.5" r="1.5"/><polyline points="21 15 16 10 5 21"/>',
  volume: '<path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/>',
  network: '<circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>',
  cpu: '<rect x="4" y="4" width="16" height="16" rx="2"/><rect x="9" y="9" width="6" height="6"/>',
  memory: '<path d="M2 12h20"/><path d="M2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6"/><path d="M2 12V6a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v6"/>',
  disk: '<path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>',
  success: '<circle cx="12" cy="12" r="10"/><polyline points="9 12 12 15 16 10"/>',
  warning: '<circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>',
  error: '<circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/>',
  info: '<circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/>',
}

// 统计数据
const statistics = ref([
  { label: '容器总数', value: 0, icon: 'container', type: 'primary', key: 'containers' },
  { label: '运行中', value: 0, icon: 'activity', type: 'success', key: 'running' },
  { label: '已停止', value: 0, icon: 'stop', type: 'danger', key: 'stopped' },
  { label: '镜像总数', value: 0, icon: 'image', type: 'secondary', key: 'images' },
  { label: '卷总数', value: 0, icon: 'database', type: 'accent', key: 'volumes' },
  { label: '网络总数', value: 0, icon: 'network', type: 'primary', key: 'networks' },
])

// 资源使用数据
const resources = ref({
  cpu: { name: 'CPU', icon: icons.cpu, percent: null, used: '—', total: '—' },
  memory: { name: '内存', icon: icons.memory, percent: 0, used: '—', total: '—' },
  disk: { name: '磁盘', icon: icons.disk, percent: 0, used: '—', total: '—' },
})

// 系统事件
const eventLogs = ref([])
const currentPage = ref(1)
const pageSize = 5
const maxEvents = 30
let dashboardRequestGeneration = 0

// 计算属性
const totalPages = computed(() => Math.ceil(Math.min(eventLogs.value.length, maxEvents) / pageSize))

const displayedLogs = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  const end = start + pageSize
  return eventLogs.value.slice(start, end)
})

const orderedStatistics = computed(() => statistics.value)

// 统计数字滚动动画：每个 key 一个 count-up 实例，轮询刷新时平滑过渡
const statCountUps = {}
for (const stat of statistics.value) {
  const target = computed(() => {
    const current = statistics.value.find(item => item.key === stat.key)
    return typeof current?.value === 'number' ? current.value : 0
  })
  statCountUps[stat.key] = useCountUp(target, { duration: 600 })
}

function statDisplayValue(key) {
  return statCountUps[key]?.formattedValue.value ?? 0
}

function setStatValue(key, value) {
  const target = statistics.value.find(stat => stat.key === key)
  if (target) target.value = value
}

function applyStatOrder(order) {
  if (!Array.isArray(order) || order.length === 0) return
  statistics.value = statistics.value
    .slice()
    .sort((a, b) => {
      const ai = order.indexOf(a.key)
      const bi = order.indexOf(b.key)
      if (ai === -1 && bi === -1) return 0
      if (ai === -1) return 1
      if (bi === -1) return -1
      return ai - bi
    })
}

async function loadStatOrder() {
  try {
    const raw = localStorage.getItem(STAT_ORDER_STORAGE_KEY)
    const order = raw ? JSON.parse(raw) : []
    applyStatOrder(order)
  } catch {
    // 忽略本地布局损坏，使用默认顺序。
  }
  try {
    const res = await settingsApi.getKVSetting('overview_stat_order')
    const order = res?.value ? JSON.parse(res.value) : []
    applyStatOrder(order)
    if (Array.isArray(order) && order.length > 0) {
      localStorage.setItem(STAT_ORDER_STORAGE_KEY, JSON.stringify(order))
    }
  } catch {
    // 后端布局不可用时继续使用本地布局。
  }
}

function persistStatOrder() {
  const raw = JSON.stringify(statistics.value.map(stat => stat.key))
  localStorage.setItem(STAT_ORDER_STORAGE_KEY, raw)
  settingsApi.setKVSetting('overview_stat_order', raw).catch(error => {
    console.error('保存仪表盘布局失败:', error)
  })
}

function handleStatDragStart(key) {
  draggingStatKey.value = key
}

function handleStatDrop(targetKey) {
  const sourceKey = draggingStatKey.value
  draggingStatKey.value = ''
  if (!sourceKey || sourceKey === targetKey) return
  const list = statistics.value.slice()
  const sourceIndex = list.findIndex(stat => stat.key === sourceKey)
  const targetIndex = list.findIndex(stat => stat.key === targetKey)
  if (sourceIndex === -1 || targetIndex === -1) return
  const [moved] = list.splice(sourceIndex, 1)
  list.splice(targetIndex, 0, moved)
  statistics.value = list
  persistStatOrder()
  suppressStatClick.value = true
  setTimeout(() => {
    suppressStatClick.value = false
  }, 0)
}

// 获取使用颜色类
function getUsageColorClass(percent) {
  if (!Number.isFinite(percent)) return 'muted'
  if (percent >= 90) return 'danger'
  if (percent >= 75) return 'warning'
  return 'success'
}

function formatResourcePercent(percent) {
  return Number.isFinite(percent) ? `${percent}%` : '-'
}

function resourceProgressWidth(percent) {
  return Number.isFinite(percent) ? `${Math.min(Math.max(percent, 0), 100)}%` : '0%'
}

// 获取事件图标
function getEventIcon(type) {
  switch (type) {
    case 'success': return icons.success
    case 'warning': return icons.warning
    case 'error': return icons.error
    default: return icons.info
  }
}

// 获取事件标签
function getEventLabel(type) {
  switch (type) {
    case 'success': return '成功'
    case 'warning': return '警告'
    case 'error': return '错误'
    default: return '信息'
  }
}

const toast = useToast()

async function copyEvent(log) {
  const ok = await copyToClipboard(log?.message || '')
  if (ok) toast.success('已复制事件内容')
  else toast.error('复制失败，请手动复制')
}

function normalizeSystemEvents(list) {
  const seenIds = new Set()
  const normalized = []

  for (let index = 0; index < list.length && normalized.length < maxEvents; index += 1) {
    const log = list[index] || {}
    const sourceId = String(log.id ?? '').trim()
    if (sourceId && seenIds.has(sourceId)) continue
    if (sourceId) seenIds.add(sourceId)

    const timestamp = log.timestamp ?? log.time ?? ''
    const message = log.message || ''
    normalized.push({
      id: sourceId || `system-event-${index}-${timestamp}-${message}`,
      type: (log.typeClass || log.type || 'info').toLowerCase(),
      time: formatTime(timestamp || new Date()),
      message,
    })
  }

  return normalized
}

// 处理统计卡片点击
function handleStatClick(stat) {
  if (stat.key === 'containers' || stat.key === 'running' || stat.key === 'stopped') {
    const path = managementMode === 'DS' ? '/containers' : '/compose'
    const status = stat.key === 'running' ? 'running' : stat.key === 'stopped' ? 'stopped' : ''
    router.push({ path, query: status ? { status } : undefined })
  } else if (stat.key === 'images') {
    router.push('/images')
  } else if (stat.key === 'volumes') {
    router.push('/volumes')
  } else if (stat.key === 'networks') {
    router.push('/networks')
  }
}

function handleStatCardClick(stat) {
  if (suppressStatClick.value) return
  handleStatClick(stat)
}

// 获取系统信息
async function fetchSystemInfo() {
	const generation = dashboardRequestGeneration
	const requestedEnvironment = environmentId.value
  try {
    const data = await api.system.info()
		if (generation !== dashboardRequestGeneration || requestedEnvironment !== environmentId.value) return
    
    // 更新统计数据
    setStatValue('containers', data.Containers || 0)
    setStatValue('running', data.ContainersRunning || 0)
    setStatValue('stopped', data.ContainersStopped || 0)
    setStatValue('images', data.Images || 0)
    setStatValue('volumes', data.Volumes || 0)
    setStatValue('networks', data.Networks || 0)
    
    // 更新资源使用
    if (data.CpuSampleReady === false) {
      resources.value.cpu.percent = null
    } else if (data.CpuUsage !== undefined) {
      resources.value.cpu.percent = parseFloat(data.CpuUsage.toFixed(1))
    }
    
		if (Number.isFinite(data.MemTotal) && Number.isFinite(data.MemUsage) && data.MemTotal > 0) {
      const memPercent = (data.MemUsage / data.MemTotal) * 100
      resources.value.memory.percent = parseFloat(memPercent.toFixed(1))
      resources.value.memory.used = formatBytes(data.MemUsage)
      resources.value.memory.total = formatBytes(data.MemTotal)
    }
    
		if (Number.isFinite(data.DiskTotal) && Number.isFinite(data.DiskUsage) && data.DiskTotal > 0) {
      const diskPercent = (data.DiskUsage / data.DiskTotal) * 100
      resources.value.disk.percent = parseFloat(diskPercent.toFixed(1))
      resources.value.disk.used = formatBytes(data.DiskUsage)
      resources.value.disk.total = formatBytes(data.DiskTotal)
    }
  } catch (error) {
    console.error('获取系统信息失败:', error)
  }
}

// 获取系统事件
async function fetchSystemEvents() {
	const generation = dashboardRequestGeneration
	const requestedEnvironment = environmentId.value
  try {
    const data = await api.system.events()
		if (generation !== dashboardRequestGeneration || requestedEnvironment !== environmentId.value) return
    const list = Array.isArray(data) ? data : (data.data || [])
    
    // 过滤
    const filteredList = list.filter(log => {
      const msg = (log.message || '').toLowerCase()
      if (msg.includes('container')) {
        return msg.includes('create') || msg.includes('destroy')
      }
      return true
    })
    
    eventLogs.value = normalizeSystemEvents(filteredList)
    const lastPage = Math.max(1, Math.ceil(eventLogs.value.length / pageSize))
    if (currentPage.value > lastPage) currentPage.value = lastPage
  } catch (error) {
    console.error('获取系统事件失败:', error)
  }
}

function resetDashboardTargetData() {
	dashboardRequestGeneration += 1
	for (const stat of statistics.value) stat.value = 0
	for (const resource of Object.values(resources.value)) {
		resource.percent = null
		resource.used = '—'
		resource.total = '—'
	}
	eventLogs.value = []
	currentPage.value = 1
}

watch(environmentId, () => {
	resetDashboardTargetData()
	void Promise.all([fetchSystemInfo(), fetchSystemEvents()])
})

// 自适应刷新间隔
function getRefreshInterval() {
  return 5000
}

// 定时器
let refreshTimer = null

function startRefresh() {
  stopRefresh()
  
  const tick = async () => {
    if (document.visibilityState === 'visible') {
      await fetchSystemInfo()
      await fetchSystemEvents()
    }
    refreshTimer = setTimeout(tick, getRefreshInterval())
  }
  
  tick()
}

function stopRefresh() {
  if (refreshTimer) {
    clearTimeout(refreshTimer)
    refreshTimer = null
  }
}

// 页面可见性变化处理：可见时恢复刷新，隐藏时停止（具名引用以便卸载时移除）
function handleVisibilityChange() {
  if (document.visibilityState === 'visible') {
    startRefresh()
  } else {
    stopRefresh()
  }
}

// 生命周期
onMounted(() => {
  loadStatOrder()
  startRefresh()
  document.addEventListener('visibilitychange', handleVisibilityChange)
})

onUnmounted(() => {
  stopRefresh()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>

<style scoped>
.overview-page {
  box-sizing: border-box;
  min-height: 0;
  height: 100%;
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding-bottom: 22px;
  overflow: auto;
  color: var(--resource-ink);
}

.section-title,
.section-subtitle {
  margin: 0;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  color: var(--resource-ink);
  font-size: 0.875rem;
  font-weight: 700;
  letter-spacing: 0;
}

.section-subtitle {
  color: var(--resource-muted);
  font-size: 0.72rem;
  font-weight: 400;
}

.stats-section,
.quick-links-section {
  width: 100%;
}

.onboarding-banner {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  margin-bottom: 12px;
  padding: 12px 16px;
  border: 1px solid color-mix(in srgb, var(--color-primary-500, var(--color-primary)) 35%, transparent);
  border-radius: 12px;
  background: color-mix(in srgb, var(--color-primary-500, var(--color-primary)) 8%, var(--bg-elevated));
  box-shadow: 0 10px 28px color-mix(in srgb, var(--text-primary) 6%, transparent);
}

.onboarding-banner-icon {
  flex: 0 0 auto;
  color: var(--color-primary-600, var(--color-primary));
}

.onboarding-banner-copy {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex: 1 1 auto;
}

.onboarding-banner-copy strong {
  color: var(--text-primary);
  font-size: 0.9rem;
  font-weight: 650;
}

.onboarding-banner-copy span {
  color: var(--text-secondary);
  font-size: 0.78rem;
}

.onboarding-banner-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 0 0 auto;
}

.onboarding-banner-btn {
  min-height: 34px;
  padding: 0 14px;
  border: 1px solid var(--border-default);
  border-radius: 9px;
  background: var(--bg-elevated);
  color: var(--text-primary);
  font-family: inherit;
  font-size: 0.8125rem;
  font-weight: 500;
  cursor: pointer;
  transition: background-color var(--motion-duration-quick) var(--motion-ease-out), border-color var(--motion-duration-quick) var(--motion-ease-out), color var(--motion-duration-quick) var(--motion-ease-out);
}

.onboarding-banner-btn.ghost {
  border-color: transparent;
  color: var(--text-tertiary);
}

.onboarding-banner-btn.ghost:hover {
  color: var(--text-secondary);
}

.onboarding-banner-btn.primary {
  border-color: var(--color-primary-600, var(--color-primary));
  background: linear-gradient(135deg, var(--color-primary-500, var(--color-primary)), var(--color-primary-700, var(--color-primary)));
  color: var(--text-inverse, #fff);
}

.onboarding-banner-btn.primary:hover {
  filter: brightness(1.04);
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(6, minmax(150px, 1fr));
  gap: 8px;
}

.stat-card {
  min-width: 0;
  min-height: 72px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--resource-line);
  border-radius: 10px;
  background: var(--resource-surface);
  color: var(--resource-ink);
  text-align: left;
  cursor: pointer;
  user-select: none;
  box-shadow: 0 10px 24px color-mix(in srgb, var(--text-primary) 4%, transparent);
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out), background var(--motion-duration-quick) var(--motion-ease-out), transform var(--motion-duration-quick) var(--motion-ease-out);
}

.stat-card:hover {
  border-color: var(--resource-line-strong);
  background: var(--bg-elevated);
  transform: translateY(-1px);
}

.stat-content {
  min-width: 0;
}

.stat-value,
.stat-label {
  display: block;
}

.stat-value {
  color: var(--resource-ink);
  font-size: 1.2rem;
  font-weight: 800;
  line-height: 1;
}

.stat-label {
  margin-top: 7px;
  color: var(--resource-muted);
  font-size: 0.7rem;
}

.stat-icon-wrapper,
.quick-link-icon,
.resource-icon {
  display: inline-grid;
  place-items: center;
  flex: 0 0 auto;
  width: 34px;
  height: 34px;
  border-radius: 9px;
  background: var(--ops-muted-soft);
  color: var(--resource-muted);
}

.stat-icon {
  display: block;
  flex: 0 0 18px;
  width: 18px;
  height: 18px;
}

.stat-primary .stat-icon-wrapper,
.stat-success .stat-icon-wrapper,
.quick-link-icon,
.icon-disk {
  background: var(--ops-green-soft);
  color: var(--ops-green-strong);
}

.stat-secondary .stat-icon-wrapper {
  background: var(--ops-muted-soft);
  color: var(--resource-muted);
}

.stat-accent .stat-icon-wrapper,
.icon-cpu {
  background: var(--ops-cyan-soft);
  color: var(--ops-cyan);
}

.stat-warning .stat-icon-wrapper,
.icon-memory {
  background: var(--ops-amber-soft);
  color: var(--ops-amber);
}

.stat-danger .stat-icon-wrapper {
  background: var(--color-danger-100);
  color: var(--ops-red);
}

.quick-links-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(240px, 1fr));
  gap: 8px;
}

.quick-link-card {
  min-width: 0;
  min-height: 70px;
  display: grid;
  grid-template-columns: 36px minmax(0, 1fr) 18px;
  align-items: center;
  gap: 11px;
  padding: 10px 12px;
  border: 1px solid var(--resource-line);
  border-radius: 10px;
  background: var(--resource-surface);
  color: var(--resource-ink);
  text-align: left;
  cursor: pointer;
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out), background var(--motion-duration-quick) var(--motion-ease-out), transform var(--motion-duration-quick) var(--motion-ease-out);
}

.quick-link-card:hover {
  border-color: color-mix(in srgb, var(--resource-accent) 45%, var(--resource-line));
  background: var(--resource-row-hover);
  transform: translateY(-1px);
}

.quick-link-info {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.quick-link-title {
  color: var(--resource-ink);
  font-size: 0.82rem;
  font-weight: 700;
}

.quick-link-desc {
  overflow: hidden;
  color: var(--resource-muted);
  font-size: 0.7rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.quick-link-card > :last-child {
  color: var(--resource-muted);
}

.bottom-sections {
  min-height: 390px;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  align-items: stretch;
  gap: 14px;
}

.resources-section,
.events-section {
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.resources-card,
.events-card {
  min-height: 0;
  flex: 1;
  border: 1px solid var(--resource-line);
  border-radius: 12px;
  background: var(--resource-panel);
  box-shadow: 0 14px 34px color-mix(in srgb, var(--text-primary) 5%, transparent);
  overflow: hidden;
}

.resources-card {
  padding: 8px 14px;
}

.resource-item {
  padding: 14px 0;
  border-bottom: 1px solid var(--resource-line);
}

.resource-item:last-child,
.event-item:last-child {
  border-bottom: 0;
}

.resource-header,
.resource-info,
.resource-meta {
  display: flex;
  align-items: center;
}

.resource-header,
.resource-meta {
  justify-content: space-between;
  gap: 10px;
}

.resource-info {
  gap: 9px;
}

.resource-icon svg,
.event-status svg,
.page-btn svg {
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.resource-name {
  color: var(--resource-ink);
  font-size: 0.8rem;
  font-weight: 700;
}

.resource-percent {
  color: var(--ops-green-strong);
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 0.78rem;
  font-weight: 800;
}

.resource-percent.warning { color: var(--ops-amber); }
.resource-percent.danger { color: var(--ops-red); }
.resource-percent.muted { color: var(--resource-muted); }

.progress-bar {
  width: 100%;
  height: 6px;
  margin: 9px 0 7px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--ops-muted-soft);
}

.progress-fill {
  height: 100%;
  border-radius: inherit;
  background: var(--ops-green);
  transition: width var(--motion-duration-slow) var(--motion-ease-out);
}

.progress-fill.warning { background: var(--ops-amber); }
.progress-fill.danger { background: var(--ops-red); }
.progress-fill.muted { background: transparent; }

.resource-meta {
  color: var(--resource-muted);
  font-size: 0.68rem;
}

.events-card {
  display: flex;
  flex-direction: column;
}

.events-list-scroll {
  min-height: 0;
  flex: 1;
  overflow: hidden;
}

.events-list {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.events-empty {
  min-height: 260px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 10px;
  color: var(--resource-muted);
  font-size: 0.75rem;
}

.event-item {
  min-height: 0;
  flex: 1;
  display: grid;
  grid-template-columns: 24px minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  border-bottom: 1px solid var(--resource-line);
  cursor: pointer;
  transition: background var(--motion-duration-quick) var(--motion-ease-out);
}

.event-item:hover {
  background: var(--resource-row-hover);
}

.event-status {
  width: 24px;
  height: 24px;
  display: inline-grid;
  place-items: center;
  border-radius: 7px;
  background: var(--ops-muted-soft);
  color: var(--resource-muted);
}

.event-status.info { background: var(--ops-cyan-soft); color: var(--ops-cyan); }
.event-status.success { background: var(--ops-green-soft); color: var(--ops-green-strong); }
.event-status.warning { background: var(--ops-amber-soft); color: var(--ops-amber); }
.event-status.error { background: var(--color-danger-100); color: var(--ops-red); }

.event-content {
  min-width: 0;
  display: grid;
  grid-template-columns: 48px minmax(0, 1fr) 92px;
  align-items: center;
  gap: 8px;
}

.event-type {
  justify-self: start;
  padding: 3px 7px;
  border-radius: 999px;
  background: var(--ops-muted-soft);
  color: var(--resource-muted);
  font-size: 0.66rem;
  font-weight: 700;
}

.event-type.info { background: var(--ops-cyan-soft); color: var(--ops-cyan); }
.event-type.success { background: var(--ops-green-soft); color: var(--ops-green-strong); }
.event-type.warning { background: var(--ops-amber-soft); color: var(--ops-amber); }
.event-type.error { background: var(--color-danger-100); color: var(--ops-red); }

.event-message {
  overflow: hidden;
  color: var(--resource-ink);
  font-size: 0.72rem;
  text-overflow: ellipsis;
  white-space: nowrap;
  border-bottom: 1px dashed transparent;
  transition: color var(--motion-duration-quick) var(--motion-ease-out), border-color var(--motion-duration-quick) var(--motion-ease-out);
}

.event-item:hover .event-message,
.event-item:focus-within .event-message {
  color: var(--color-primary-600, var(--color-primary));
  border-bottom-color: var(--color-primary-400, var(--color-primary));
}

.event-item:focus-visible {
  outline: 2px solid var(--color-primary-500, var(--color-primary));
  outline-offset: -2px;
}

.event-time {
  color: var(--resource-muted);
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 0.65rem;
  text-align: right;
  white-space: nowrap;
}

.events-pagination {
  min-height: 46px;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 9px;
  padding: 0 12px;
  border-top: 1px solid var(--resource-line);
  color: var(--resource-muted);
  font-size: 0.72rem;
}

.page-btn {
  width: 28px;
  height: 28px;
  display: inline-grid;
  place-items: center;
  padding: 0;
  border: 1px solid var(--resource-line);
  border-radius: 8px;
  background: var(--bg-elevated);
  color: var(--resource-muted);
  cursor: pointer;
}

.page-btn:hover:not(:disabled) {
  border-color: var(--resource-line-strong);
  color: var(--resource-ink);
}

.page-btn:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.page-info {
  min-width: 42px;
  text-align: center;
}

@media (max-width: 1350px) {
  .stats-grid {
    grid-template-columns: repeat(3, minmax(150px, 1fr));
  }
}

@media (max-width: 900px) {
  .overview-page {
    height: auto;
    min-height: 100%;
    overflow: visible;
  }

  .quick-links-grid,
  .bottom-sections {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 620px) {
  .stats-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .event-content {
    grid-template-columns: 44px minmax(0, 1fr);
  }

  .event-time {
    display: none;
  }
}
</style>
