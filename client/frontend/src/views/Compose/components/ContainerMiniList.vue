<template>
  <div class="container-mini-list">
    <section class="container-switch-rail" aria-label="Compose 容器列表">
      <div class="rail-heading">
        <span class="rail-label">容器</span>
        <span class="rail-count">{{ containers.length }}</span>
      </div>

      <div v-if="!containers.length" class="empty-containers">暂无容器</div>
      <div v-else class="containers-chips">
        <button
          v-for="container in containers"
          :key="container.Id"
          type="button"
          class="container-chip"
          :class="{
            'is-selected': selectedId === container.Id,
            'is-running': isRunning(container.State),
            'is-stopped': !isRunning(container.State)
          }"
          :title="getContainerName(container)"
          @click="handleSelect(container)"
        >
          <span class="chip-signal"></span>
          <span class="chip-name">{{ getContainerName(container) }}</span>
          <span class="chip-meta">{{ getPortCount(container.Ports) }}</span>
          <span class="chip-update-statuses">
            <span class="chip-update-status update-status" :class="`is-${updateState(container).tone}`">
              {{ updateState(container).label }}
            </span>
            <span v-if="hasUpdate(container) && serviceUpdateForContainer(container)" class="chip-update-available update-status is-warning">
              镜像可更新
            </span>
          </span>
          <span v-if="hasUpdate(container)" class="chip-update-dot" title="镜像可更新" aria-label="镜像可更新"></span>
        </button>
      </div>

      <div v-if="serviceUpdates.length" class="rail-update-time">
        最近更新：{{ formatDateTime(updateSummary.finishedAt) }}
      </div>
      <div v-if="missingServiceUpdates.length" class="missing-service-updates" role="status">
        <span class="missing-service-heading">无当前容器的服务</span>
        <div v-for="service in missingServiceUpdates" :key="service.service" class="missing-service-row">
          <strong>{{ service.service }}</strong>
          <span class="update-status" :class="`is-${serviceUpdateState(service).tone}`">{{ serviceUpdateState(service).label }}</span>
          <span class="missing-service-reason">{{ service.message || '当前没有该服务的容器，请检查项目配置和更新日志' }}</span>
        </div>
      </div>
    </section>

    <section v-if="selectedContainer" class="container-detail" aria-label="容器详情">
      <div v-if="detailError" class="detail-load-error" role="alert">
        <span>详情读取失败</span>
        <button type="button" @click="emit('retry-detail')">重试</button>
      </div>
      <header class="detail-command-strip">
        <div class="container-identity">
          <span class="identity-icon">
            <DynamicIcon name="container" :size="18" />
          </span>
          <div class="identity-copy">
            <h4 class="detail-container-name">{{ getContainerName(selectedContainer) }}</h4>
            <div class="identity-meta">
              <span class="detail-state-text" :class="{ running: isRunning(selectedContainer.State) }">
                <span class="state-dot"></span>
                {{ toCnState(selectedContainer.State) || '-' }}
              </span>
              <span class="meta-divider"></span>
              <span>{{ getPortCount(selectedContainer.Ports) }}</span>
            </div>
          </div>
        </div>

        <div class="detail-update-badges">
          <span v-if="hasUpdate(selectedContainer)" class="detail-update-badge detail-update-available update-status is-warning">
            <DynamicIcon name="cloud-download" :size="13" />
            镜像可更新
          </span>
          <span v-if="selectedServiceUpdate" class="detail-service-update-badge update-status" :class="`is-${serviceUpdateState(selectedServiceUpdate).tone}`">
            {{ serviceUpdateState(selectedServiceUpdate).label }}
          </span>
        </div>
      </header>

      <div v-if="selectedServiceUpdate" class="detail-update-result" role="status">
        <span class="update-result-heading">最近更新 · {{ formatDateTime(updateSummary.finishedAt) }}</span>
        <span>服务 {{ selectedServiceUpdate.service }}：{{ serviceUpdateState(selectedServiceUpdate).label }}</span>
        <span v-if="selectedServiceUpdate.message">{{ selectedServiceUpdate.message }}</span>
        <span v-if="selectedServiceUpdate.status === 'applied'">版本变化未确认</span>
      </div>

      <div class="detail-actions-compact" data-remote-write>
        <button
          v-if="hasUpdate(selectedContainer)"
          v-ripple
          class="action-item update"
          :disabled="isProcessing(selectedContainer?.Id)"
          @click="handleAction('update')"
        >
          <span v-if="isProcessing(selectedContainer?.Id)" class="btn-spinner"></span>
          <DynamicIcon v-else name="cloud-download" :size="14" />
          <span>更新</span>
        </button>
        <button v-ripple class="action-item terminal" :disabled="isProcessing(selectedContainer?.Id)" @click="handleAction('terminal')">
          <span v-if="isProcessing(selectedContainer?.Id)" class="btn-spinner"></span>
          <DynamicIcon v-else name="terminal" :size="14" />
          <span>终端</span>
        </button>
        <button v-ripple class="action-item info" :disabled="isProcessing(selectedContainer?.Id)" @click="handleAction('logs')">
          <span v-if="isProcessing(selectedContainer?.Id)" class="btn-spinner"></span>
          <DynamicIcon v-else name="file-text" :size="14" />
          <span>日志</span>
        </button>
        <button
          v-if="isRunning(selectedContainer.State)"
          v-ripple
          class="action-item stop"
          :disabled="isProcessing(selectedContainer?.Id)"
          @click="handleAction('stop')"
        >
          <span v-if="isProcessing(selectedContainer?.Id)" class="btn-spinner"></span>
          <DynamicIcon v-else name="stop" :size="14" />
          <span>停止</span>
        </button>
        <button
          v-else
          v-ripple
          class="action-item start"
          :disabled="isProcessing(selectedContainer?.Id)"
          @click="handleAction('start')"
        >
          <span v-if="isProcessing(selectedContainer?.Id)" class="btn-spinner"></span>
          <DynamicIcon v-else name="play" :size="14" />
          <span>启动</span>
        </button>
        <button v-ripple class="action-item restart" :disabled="isProcessing(selectedContainer?.Id)" @click="handleAction('restart')">
          <span v-if="isProcessing(selectedContainer?.Id)" class="btn-spinner"></span>
          <DynamicIcon v-else name="refresh" :size="14" />
          <span>重启</span>
        </button>
        <button v-ripple class="action-item delete" :disabled="isProcessing(selectedContainer?.Id)" @click="handleAction('delete')">
          <span v-if="isProcessing(selectedContainer?.Id)" class="btn-spinner"></span>
          <DynamicIcon v-else name="delete" :size="14" />
          <span>删除</span>
        </button>
      </div>

      <div class="detail-body-grid">
        <div class="detail-info-card identity-card">
          <div class="section-heading">
            <span>身份</span>
            <span class="section-line"></span>
          </div>

          <dl class="detail-lines">
            <div>
              <dt>容器 ID</dt>
              <dd class="mono" :title="selectedContainer.Id || '-'">{{ selectedContainer.Id?.substring(0, 12) || '-' }}</dd>
            </div>
            <div>
              <dt>镜像</dt>
              <dd class="mono" :title="selectedContainer.Image || '-'">{{ formatDockerImageReference(selectedContainer.Image) || '-' }}</dd>
            </div>
            <div>
              <dt>创建时间</dt>
              <dd>{{ formatDateTime(selectedContainer.Created) }}</dd>
            </div>
            <div>
              <dt>网络</dt>
              <dd>
                <span
                  v-for="(net, idx) in getNetworkNames(selectedContainer)"
                  :key="idx"
                  class="inline-chip network"
                  :title="net"
                >
                  {{ net }}
                </span>
                <span v-if="!getNetworkNames(selectedContainer).length" class="inline-chip muted">
                  {{ detailLoading ? '读取中' : '-' }}
                </span>
              </dd>
            </div>
          </dl>
        </div>

        <div class="detail-info-card port-card">
          <div class="section-heading">
            <span>端口映射</span>
            <span class="section-line"></span>
          </div>

          <div v-if="!getPublicPorts(selectedContainer.Ports).length" class="empty-map">
            {{ detailLoading ? '读取中' : '无公开端口' }}
          </div>
          <div v-else class="wire-map">
            <div
              v-for="(port, idx) in getPublicPorts(selectedContainer.Ports)"
              :key="idx"
              class="wire-row port"
              :title="port.full"
            >
              <span class="wire-node"></span>
              <span class="wire-from mono">{{ port.publicLabel }}</span>
              <span class="wire-arrow">→</span>
              <span class="wire-to mono">{{ port.privateLabel }}</span>
              <span v-if="port.ipCount > 1" class="wire-scope">双栈</span>
            </div>
          </div>
        </div>

        <div class="detail-info-card mount-card">
          <div class="section-heading">
            <span>卷挂载</span>
            <span class="section-line"></span>
          </div>

          <div v-if="!getMountsList(selectedContainer).length" class="empty-map">
            {{ detailLoading ? '读取中' : '无挂载' }}
          </div>
          <div v-else class="wire-map mounts">
            <div
              v-for="(mount, idx) in getMountsList(selectedContainer)"
              :key="idx"
              class="wire-row mount"
              :title="mount"
            >
              <span class="wire-node"></span>
              <span class="wire-path mono">{{ mount }}</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <div v-else class="empty-detail">
      <DynamicIcon name="container" :size="44" class="empty-icon" />
      <p>点击上方容器查看详情</p>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import { formatDateTime, formatDockerImageReference } from '@/utils/format.js'

const props = defineProps({
  containers: { type: Array, default: () => [] },
  selectedId: { type: String, default: '' },
  selectedContainer: { type: Object, default: null },
  updateSummary: { type: Object, default: null },
  detailLoading: { type: Boolean, default: false },
  detailError: { type: String, default: '' },
  processingIds: {
    type: [Set, Object],
    default: () => new Set()
  }
})

const emit = defineEmits(['select', 'action', 'retry-detail'])

const serviceUpdates = computed(() => Array.isArray(props.updateSummary?.services) ? props.updateSummary.services : [])
const selectedServiceUpdate = computed(() => serviceUpdateForContainer(props.selectedContainer))
const missingServiceUpdates = computed(() => {
  const existingServices = new Set(props.containers.map(getServiceName).filter(Boolean))
  return serviceUpdates.value.filter(service => service.service && !existingServices.has(service.service))
})

function getServiceName(container) {
  return container?.Labels?.['com.docker.compose.service']
    || container?.Config?.Labels?.['com.docker.compose.service']
    || container?._service
    || ''
}

function serviceUpdateForContainer(container) {
  const service = getServiceName(container)
  return service ? serviceUpdates.value.find(result => result.service === service) : null
}

function serviceUpdateState(result) {
  const states = {
    updated: { label: '已更新', tone: 'success' },
    unchanged: { label: '无需更新', tone: 'muted' },
    pull_failed: { label: '拉取失败', tone: 'warning' },
    blocked: { label: '更新受阻', tone: 'warning' },
    apply_failed: { label: '应用失败', tone: 'error' },
    applied: { label: '已应用', tone: 'info' }
  }
  return states[result?.status] || { label: '更新结果未确认', tone: 'muted' }
}

function updateState(container) {
  const result = serviceUpdateForContainer(container)
  if (result) return serviceUpdateState(result)
  return hasUpdate(container)
    ? { label: '镜像可更新', tone: 'warning' }
    : { label: '暂无已知更新', tone: 'muted' }
}

function isProcessing(id) {
  if (props.processingIds instanceof Set) {
    return props.processingIds.has(id)
  }
  return props.processingIds?.has?.(id)
}

function hasUpdate(container) {
  return Boolean(container?.UpdateAvailable ?? container?.updateAvailable)
}

function isRunning(state) {
  const s = String(state || '').toLowerCase()
  return s === 'running' || s === '运行中'
}

function toCnState(s) {
  if (!s) return ''
  const map = {
    running: '运行中',
    exited: '已停止',
    created: '已创建',
    paused: '已暂停',
    restarting: '重启中',
    dead: '异常'
  }
  return map[String(s).toLowerCase()] || s
}

function getContainerName(container) {
  return (container.Names?.[0] || '').replace(/^\//, '') || container.name || container.Id?.substring(0, 12) || '未知'
}

function getPortCount(ports) {
  if (!ports?.length) return '无端口'
  const publicCount = getPublicPorts(ports).length
  if (publicCount > 0) return publicCount === 1 ? '1端口' : `${publicCount}端口`

  const internalPorts = new Set(
    ports
      .filter(port => port.PrivatePort)
      .map(port => `${port.PrivatePort}/${port.Type || 'tcp'}`)
  )
  const count = internalPorts.size || ports.length
  return count === 1 ? '1端口' : `${count}端口`
}

function getNetworkNames(container) {
  const networkNames = Object.keys(container.NetworkSettings?.Networks || {})
  if (networkNames.length) return networkNames
  const networkMode = String(container.HostConfig?.NetworkMode || '').trim()
  if (networkMode && networkMode !== 'default') return [networkMode]
  return []
}

function getPublicPorts(ports) {
  if (!ports || !Array.isArray(ports)) return []

  const grouped = new Map()
  ports
    .filter(port => port.PublicPort)
    .forEach(port => {
      const type = port.Type || 'tcp'
      const key = `${port.PublicPort}:${port.PrivatePort}:${type}`
      const item = grouped.get(key) || {
        publicLabel: `${port.PublicPort}/${type}`,
        privateLabel: `${port.PrivatePort}/${type}`,
        bindings: []
      }
      item.bindings.push(`${port.IP || '0.0.0.0'}:${port.PublicPort}/${type} → ${port.PrivatePort}/${type}`)
      grouped.set(key, item)
    })

  return Array.from(grouped.values()).map(item => ({
    ...item,
    short: `${item.publicLabel}→${item.privateLabel}`,
    full: item.bindings.join('\n'),
    ipCount: item.bindings.length
  }))
}

function getMountsList(container) {
  const mounts = container.Mounts || []
  if (mounts.length > 0) {
    return mounts.map(m => `${m.Source || '-'} → ${m.Destination || m.Target || '-'}`)
  }

  const binds = container.HostConfig?.Binds || []
  if (binds.length > 0) {
    return binds.map(b => {
      const parts = b.split(':')
      if (parts.length >= 2) {
        return `${parts[0]} → ${parts[1]}`
      }
      return b
    })
  }

  return []
}

function handleSelect(container) {
  emit('select', container)
}

function handleAction(action) {
  emit('action', action)
}
</script>

<style scoped>
.container-mini-list {
  --ops-bg: var(--bg-secondary);
  --ops-surface: var(--bg-elevated);
  --ops-panel: var(--bg-primary);
  --ops-ink: var(--text-primary);
  --ops-muted: var(--text-secondary);
  --ops-line: var(--border-subtle);
  --ops-strong-line: var(--border-default);
  --ops-green: var(--color-success-500);
  --ops-green-strong: var(--color-success-700);
  --ops-cyan: var(--color-info-600);
  --ops-blue: var(--color-primary-600);
  --ops-amber: var(--color-warning-600);
  --ops-red: var(--color-danger-600);
  --ops-green-soft: var(--color-success-100);
  --ops-blue-soft: var(--color-primary-100);
  --ops-cyan-soft: var(--color-info-100);
  --ops-amber-soft: var(--color-warning-100);
  --ops-muted-soft: var(--bg-tertiary);

  display: flex;
  flex-direction: column;
  height: 100%;
  gap: 14px;
  overflow: hidden;
  color: var(--ops-ink);
}

.container-switch-rail {
  display: grid;
  grid-template-columns: auto 1fr;
  align-items: center;
  gap: 12px;
  padding: 12px;
  border: 1px solid var(--ops-line);
  border-radius: 10px;
  background:
    linear-gradient(90deg, color-mix(in srgb, var(--ops-ink) 4%, transparent) 1px, transparent 1px),
    linear-gradient(180deg, color-mix(in srgb, var(--ops-ink) 4%, transparent) 1px, transparent 1px),
    color-mix(in srgb, var(--ops-surface) 92%, transparent);
  background-size: 18px 18px;
  flex-shrink: 0;
}

.rail-heading {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  min-width: 58px;
}

.rail-label {
  color: var(--ops-muted);
  font-size: 0.72rem;
  font-weight: 800;
  letter-spacing: 0.08em;
}

.rail-count {
  color: var(--ops-green-strong);
  font-size: 0.88rem;
  font-weight: 800;
  font-family: 'JetBrains Mono', 'SFMono-Regular', Consolas, monospace;
}

.containers-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  min-width: 0;
}

.container-chip {
  position: relative;
  display: inline-grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 7px;
  max-width: 230px;
  min-height: 32px;
  padding: 6px 10px;
  border: 1px solid var(--ops-line);
  border-radius: 8px;
  background: color-mix(in srgb, var(--ops-panel) 84%, transparent);
  color: var(--ops-ink);
  cursor: pointer;
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out), background var(--motion-duration-quick) var(--motion-ease-out), box-shadow var(--motion-duration-quick) var(--motion-ease-out);
}

.container-chip:hover,
.container-chip:focus-visible {
  border-color: var(--ops-strong-line);
  background: color-mix(in srgb, var(--ops-panel) 72%, var(--ops-green) 8%);
  outline: none;
}

.container-chip.is-selected {
  border-color: var(--ops-green-strong);
  background: color-mix(in srgb, var(--ops-green) 14%, var(--ops-panel));
  box-shadow: inset 3px 0 0 var(--ops-green), 0 10px 24px color-mix(in srgb, var(--ops-green) 12%, transparent);
}

.chip-signal,
.state-dot,
.wire-node {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--ops-muted);
  flex-shrink: 0;
}

.container-chip.is-running .chip-signal,
.detail-state-text.running .state-dot {
  background: var(--ops-green);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--ops-green) 16%, transparent);
}

.chip-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: 'JetBrains Mono', 'SFMono-Regular', Consolas, monospace;
  font-size: 0.76rem;
  font-weight: 700;
}

.chip-meta {
  color: var(--ops-muted);
  font-size: 0.68rem;
  font-weight: 800;
}

.chip-update-dot {
  position: absolute;
  top: 5px;
  right: 5px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--ops-red);
}

.chip-update-statuses {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  grid-column: 2 / -1;
  justify-self: start;
}

.update-status {
  display: inline-flex;
  align-items: center;
  padding: 3px 7px;
  border: 1px solid var(--ops-line);
  border-radius: 999px;
  color: var(--ops-muted);
  background: var(--ops-muted-soft);
  font-size: 0.7rem;
  font-weight: 700;
  white-space: nowrap;
}

.update-status.is-success {
  color: var(--ops-green-strong);
  background: var(--ops-green-soft);
  border-color: color-mix(in srgb, var(--ops-green) 40%, var(--ops-line));
}

.update-status.is-warning {
  color: var(--ops-amber);
  background: var(--ops-amber-soft);
  border-color: color-mix(in srgb, var(--ops-amber) 40%, var(--ops-line));
}

.update-status.is-error {
  color: var(--ops-red);
  background: color-mix(in srgb, var(--ops-red) 8%, var(--ops-panel));
  border-color: color-mix(in srgb, var(--ops-red) 35%, var(--ops-line));
}

.update-status.is-info {
  color: var(--ops-blue);
  background: var(--ops-blue-soft);
  border-color: color-mix(in srgb, var(--ops-blue) 35%, var(--ops-line));
}

.rail-update-time,
.missing-service-updates {
  grid-column: 1 / -1;
  color: var(--ops-muted);
  font-size: 0.75rem;
}

.missing-service-updates {
  display: grid;
  gap: 8px;
  max-height: 160px;
  overflow-y: auto;
  padding-top: 10px;
  border-top: 1px solid var(--ops-line);
}

.missing-service-heading {
  font-weight: 700;
}

.missing-service-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 7px;
}

.missing-service-reason {
  width: 100%;
  overflow-wrap: anywhere;
}

.detail-update-result {
  display: grid;
  gap: 6px;
  margin-top: 10px;
  padding: 10px 12px;
  border: 1px solid var(--ops-line);
  border-radius: 8px;
  background: var(--ops-panel);
  color: var(--ops-ink);
  font-size: 0.78rem;
  overflow-wrap: anywhere;
}

.update-result-heading {
  color: var(--ops-muted);
  font-size: 0.72rem;
}

.empty-containers {
  color: var(--ops-muted);
  font-size: 0.8125rem;
}

.container-detail {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 14px;
  border: 1px solid var(--ops-line);
  border-radius: 10px;
  background: color-mix(in srgb, var(--ops-surface) 92%, transparent);
}

.container-detail::-webkit-scrollbar {
  width: 8px;
}

.container-detail::-webkit-scrollbar-track {
  background: transparent;
}

.container-detail::-webkit-scrollbar-thumb {
  background: color-mix(in srgb, var(--ops-muted) 32%, transparent);
  border-radius: 999px;
}

.detail-load-error {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
  padding: 8px 10px;
  border: 1px solid color-mix(in srgb, var(--ops-red) 35%, var(--ops-line));
  border-radius: 8px;
  background: color-mix(in srgb, var(--ops-red) 8%, var(--ops-panel));
  color: var(--ops-red);
  font-size: 0.75rem;
}

.detail-load-error button {
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  font-weight: 700;
  cursor: pointer;
}

.detail-command-strip {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
  padding: 14px;
  border: 1px solid var(--ops-line);
  border-radius: 10px;
  background: var(--ops-panel);
  box-shadow: 0 14px 32px color-mix(in srgb, var(--ops-ink) 7%, transparent);
}

.container-identity {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.identity-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  border: 1px solid color-mix(in srgb, var(--ops-green) 50%, var(--ops-line));
  border-radius: 10px;
  background: color-mix(in srgb, var(--ops-green) 14%, var(--ops-panel));
  color: var(--ops-green-strong);
  flex-shrink: 0;
}

.identity-copy {
  min-width: 0;
}

.detail-container-name {
  margin: 0;
  color: var(--ops-ink);
  font-family: 'JetBrains Mono', 'SFMono-Regular', Consolas, monospace;
  font-size: 1rem;
  font-weight: 800;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.identity-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 5px;
  color: var(--ops-muted);
  font-size: 0.75rem;
  font-weight: 700;
}

.detail-state-text {
  display: inline-flex;
  align-items: center;
  gap: 7px;
}

.detail-state-text.running {
  color: var(--ops-green-strong);
}

.meta-divider {
  width: 1px;
  height: 12px;
  background: var(--ops-line);
}

.detail-update-badge,
.inline-chip,
.wire-scope {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border-radius: 999px;
  font-size: 0.7rem;
  font-weight: 800;
  white-space: nowrap;
}

.detail-update-badge,
.detail-service-update-badge {
  padding: 5px 9px;
}

.detail-update-badges {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
  flex-shrink: 0;
}

.detail-actions-compact {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 12px 0;
}

.action-item {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 32px;
  padding: 7px 10px;
  border: 1px solid var(--ops-line);
  border-radius: 8px;
  background: var(--ops-panel);
  color: var(--ops-ink);
  font-size: 0.75rem;
  font-weight: 800;
  cursor: pointer;
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out), background var(--motion-duration-quick) var(--motion-ease-out), transform var(--motion-duration-quick) var(--motion-ease-out);
}

.action-item:hover {
  border-color: var(--ops-strong-line);
  background: color-mix(in srgb, var(--ops-panel) 74%, var(--ops-green) 9%);
}

.action-item:active {
  transform: translateY(1px);
}

.action-item.start {
  border-color: color-mix(in srgb, var(--ops-green) 55%, var(--ops-line));
  background: var(--ops-green-soft);
  color: var(--ops-green-strong);
}

.action-item.stop,
.action-item.delete {
  border-color: color-mix(in srgb, var(--ops-red) 34%, var(--ops-line));
  background: color-mix(in srgb, var(--ops-red) 9%, var(--ops-panel));
  color: var(--ops-red);
}

.action-item.restart {
  border-color: color-mix(in srgb, var(--ops-amber) 40%, var(--ops-line));
  background: var(--ops-amber-soft);
  color: var(--ops-amber);
}

.action-item.update {
  border-color: color-mix(in srgb, var(--ops-amber) 40%, var(--ops-line));
  background: var(--ops-amber-soft);
  color: var(--ops-amber);
}

.action-item.info {
  border-color: color-mix(in srgb, var(--ops-blue) 34%, var(--ops-line));
  background: var(--ops-blue-soft);
  color: var(--ops-blue);
}

.action-item.terminal {
  border-color: color-mix(in srgb, var(--ops-cyan) 40%, var(--ops-line));
  background: var(--ops-cyan-soft);
  color: var(--ops-cyan);
}

.action-item:disabled {
  opacity: 0.65;
  cursor: not-allowed;
}

.btn-spinner {
  display: inline-block;
  width: 14px;
  height: 14px;
  border: 2px solid currentColor;
  border-right-color: transparent;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.detail-body-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(220px, 0.8fr);
  gap: 12px;
}

.detail-info-card {
  min-width: 0;
  padding: 12px;
  border: 1px solid var(--ops-line);
  border-radius: 10px;
  background: var(--ops-panel);
}

.identity-card {
  grid-row: span 2;
}

.section-heading {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
  color: var(--ops-muted);
  font-size: 0.72rem;
  font-weight: 900;
  letter-spacing: 0.08em;
}

.section-line {
  height: 1px;
  flex: 1;
  background: var(--ops-line);
}

.detail-lines {
  display: grid;
  gap: 9px;
  margin: 0;
}

.detail-lines div {
  display: grid;
  grid-template-columns: 72px minmax(0, 1fr);
  align-items: center;
  gap: 10px;
  padding: 8px 0;
  border-bottom: 1px solid color-mix(in srgb, var(--ops-line) 70%, transparent);
}

.detail-lines div:last-child {
  border-bottom: none;
}

.detail-lines dt {
  color: var(--ops-muted);
  font-size: 0.72rem;
  font-weight: 800;
}

.detail-lines dd {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  min-width: 0;
  margin: 0;
  color: var(--ops-ink);
  font-size: 0.78rem;
  font-weight: 700;
  overflow: hidden;
}

.mono {
  font-family: 'JetBrains Mono', 'SFMono-Regular', Consolas, monospace;
}

.detail-lines dd.mono {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.inline-chip {
  max-width: 100%;
  padding: 4px 8px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.inline-chip.network {
  border: 1px solid color-mix(in srgb, var(--ops-cyan) 36%, var(--ops-line));
  background: var(--ops-cyan-soft);
  color: var(--ops-cyan);
}

.inline-chip.muted {
  border: 1px solid var(--ops-line);
  background: var(--ops-muted-soft);
  color: var(--ops-muted);
}

.wire-map {
  display: grid;
  gap: 8px;
}

.wire-row {
  display: grid;
  align-items: center;
  gap: 8px;
  min-width: 0;
  padding: 9px 10px;
  border: 1px solid color-mix(in srgb, var(--ops-line) 84%, transparent);
  border-radius: 8px;
  background:
    linear-gradient(90deg, color-mix(in srgb, var(--ops-ink) 5%, transparent) 1px, transparent 1px),
    color-mix(in srgb, var(--ops-surface) 70%, transparent);
  background-size: 16px 16px;
}

.wire-row.port {
  grid-template-columns: auto minmax(0, 1fr) auto minmax(0, 1fr) auto;
}

.wire-row.mount {
  grid-template-columns: auto minmax(0, 1fr);
}

.wire-row.port .wire-node {
  background: var(--ops-blue);
}

.wire-row.mount .wire-node {
  background: var(--ops-cyan);
}

.wire-from,
.wire-to,
.wire-path {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--ops-ink);
  font-size: 0.74rem;
  font-weight: 800;
}

.wire-arrow {
  color: var(--ops-muted);
  font-weight: 900;
}

.wire-scope {
  padding: 3px 7px;
  background: var(--ops-blue-soft);
  color: var(--ops-blue);
}

.empty-map {
  display: flex;
  align-items: center;
  min-height: 40px;
  padding: 0 10px;
  border: 1px dashed var(--ops-line);
  border-radius: 8px;
  color: var(--ops-muted);
  font-size: 0.78rem;
  font-weight: 700;
}

.empty-detail {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  color: var(--ops-muted);
}

.empty-detail svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 1.5;
  margin-bottom: 16px;
}

.empty-detail p {
  font-size: 0.875rem;
}

@media (max-width: 720px) {
  .container-switch-rail,
  .detail-body-grid {
    grid-template-columns: 1fr;
  }

  .detail-command-strip {
    flex-direction: column;
  }

  .wire-row.port {
    grid-template-columns: auto minmax(0, 1fr);
  }

  .wire-row.port .wire-arrow,
  .wire-row.port .wire-to,
  .wire-row.port .wire-scope {
    grid-column: 2;
  }
}
</style>
