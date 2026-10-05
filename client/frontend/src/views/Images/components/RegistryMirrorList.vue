<template>
  <section class="mirror-status">
    <div class="mirror-status__header">
      <div class="mirror-status__tabs" role="tablist" aria-label="镜像仓库">
        <button v-for="registry in registries" :key="registry.id" type="button" role="tab"
          :aria-selected="activeRegistry === registry.id" :data-registry="registry.id"
          @click="activeRegistry = registry.id">{{ registry.label }}</button>
      </div>
      <a href="https://status.anye.xyz/" target="_blank" rel="noopener noreferrer" class="mirror-status__reference">
        镜像源监控 <DynamicIcon name="external-link" :size="14" />
      </a>
    </div>
    <div class="mirror-status__meta">
      <span>站点状态：未获取</span>
      <span>设备直连 · 非镜像拉取测试</span>
      <span>连接失败不代表镜像源离线</span>
    </div>
    <p v-if="environmentId !== 'local'" class="mirror-status__error">远程 Agent 暂不支持镜像源检测</p>
    <form v-if="activeRegistry === 'ghcr'" class="mirror-status__input" @submit.prevent="addGhcr">
      <input v-model="ghcrInput" data-ghcr-input type="url" aria-label="GHCR 镜像源地址" placeholder="https://镜像源域名" maxlength="512" />
      <button type="submit" data-add-ghcr :disabled="busy" title="添加 GHCR 检测地址" aria-label="添加 GHCR 检测地址" @click.prevent="addGhcr">
        <DynamicIcon name="plus" :size="16" />
      </button>
    </form>
    <div class="mirror-status__scroll">
      <table class="mirror-status__table">
        <thead><tr><th v-if="activeRegistry === 'dockerhub'" class="mirror-status__selection">选择</th><th>镜像源</th><th>设备检测</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="candidate in candidates" :key="candidate.url">
            <td v-if="activeRegistry === 'dockerhub'">
              <AnimatedCheckbox v-model="selected" :value="candidate.url" :disabled="configured.has(candidate.url) || busy || !normalizeMirrorURL(candidate.url)"
                :aria-label="`选择 ${candidate.label}`" />
            </td>
            <td class="mirror-status__source">
              <span>
                <a v-if="candidate.website" :href="candidate.website" target="_blank" rel="noopener noreferrer"
                  title="免费通道仅支持平台允许的镜像">{{ candidate.label }}</a>
                <template v-else>{{ candidate.label }}</template>
                <small v-if="activeRegistry === 'dockerhub' && configured.has(candidate.url)">已配置</small>
              </span>
              <span class="mirror-status__url" :title="candidate.url">{{ candidate.url }}</span>
            </td>
            <td class="mirror-status__result">
              <template v-if="results[candidate.url] && environmentId === 'local'">
                <span :class="['mirror-status__state', `is-${results[candidate.url].status}`]">{{ probeLabels[results[candidate.url].status] }}</span>
                <span>{{ results[candidate.url].latencyMs }} ms<span v-if="isProbeStale(results[candidate.url], now)"> · 已过期</span></span>
                <time :datetime="results[candidate.url].checkedAt">{{ formatCheckedAt(results[candidate.url].checkedAt) }}</time>
              </template>
              <span v-else>未检测</span>
              <span v-if="errors[candidate.url]" class="mirror-status__error" role="status">{{ errors[candidate.url] }}</span>
            </td>
            <td>
              <button type="button" data-check :disabled="busy || environmentId !== 'local' || !normalizeMirrorURL(candidate.url)"
                :title="checking === candidate.url ? '正在检测' : '检测设备直连状态'" aria-label="检测设备直连状态"
                @click="checkCandidates([candidate])">
                <DynamicIcon :name="checking === candidate.url ? 'loader-2' : 'refresh-cw'" :size="16" :class="{ 'animate-spin': checking === candidate.url }" />
              </button>
              <button v-if="activeRegistry === 'ghcr' && candidate.removable" type="button" :disabled="busy"
                title="移除检测地址" aria-label="移除检测地址" @click="removeGhcr(candidate.url)">
                <DynamicIcon name="x" :size="14" />
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-if="inputError" class="mirror-status__error" role="alert">{{ inputError }}</div>
    <div class="mirror-status__actions">
      <button type="button" data-check-all :disabled="busy || environmentId !== 'local'" @click="checkCandidates(candidates)">
        <DynamicIcon name="refresh-cw" :size="14" /> 检测全部
      </button>
      <button v-if="activeRegistry === 'dockerhub'" type="button" data-add-selected :disabled="!selected.length || busy || environmentId !== 'local'" @click="addSelected">
        <DynamicIcon name="plus" :size="14" /> 添加所选
      </button>
    </div>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import AnimatedCheckbox from '@/components/ui/AnimatedCheckbox.vue'
import { checkRegistryMirror } from '@/api/registryMirrors.js'
import { currentEnvironmentId } from '@edition/current-environment'
import { defaultHubMirrors, defaultGhcrMirrors, normalizeMirrorURL, probeLabels, readProbeCache, writeProbeCache,
  validProbeResult, isProbeStale, readGhcrCandidates, ghcrCandidatesKey } from './registryMirrorStatus.js'

const props = defineProps({
  hubMirrors: { type: String, default: '' },
  environmentId: { type: String, default: 'local' }
})
const emit = defineEmits(['add'])
const registries = [{ id: 'dockerhub', label: 'Docker Hub' }, { id: 'ghcr', label: 'GHCR' }]
const activeRegistry = ref('dockerhub')
const configured = computed(() => new Set(props.hubMirrors.split('\n').map(url => normalizeMirrorURL(url) || url.trim()).filter(Boolean)))
const candidates = computed(() => {
  if (activeRegistry.value === 'ghcr') {
    const presets = new Map(defaultGhcrMirrors.map(item => [item.url, item]))
    for (const url of ghcrCandidates.value) if (!presets.has(url)) presets.set(url, { label: '自定义 GHCR 源', url, removable: true })
    return [...presets.values()]
  }
  const presets = new Map(defaultHubMirrors.map(item => [item.url, item]))
  for (const url of configured.value) if (!presets.has(url)) presets.set(url, { label: '自定义 Docker Hub 源', url })
  return [...presets.values()].slice(0, 20)
})
const ghcrCandidates = ref(readGhcrCandidates())
const ghcrInput = ref('')
const selected = ref([])
const results = ref(readProbeCache())
const errors = ref({})
const inputError = ref('')
const busy = ref(false)
const checking = ref('')
const now = ref(Date.now())
const clock = setInterval(() => { now.value = Date.now() }, 30000)
let controller = null

function stopChecks() {
  controller?.abort()
  controller = null
  busy.value = false
  checking.value = ''
}

watch(() => props.environmentId, stopChecks)
watch(activeRegistry, stopChecks)
onBeforeUnmount(() => { stopChecks(); clearInterval(clock) })

function formatCheckedAt(value) {
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function addGhcr() {
  const url = normalizeMirrorURL(ghcrInput.value)
  if (!url) { inputError.value = '请输入不含凭证、参数或仓库路径的 HTTPS 地址'; return }
  if (ghcrCandidates.value.length >= 20) { inputError.value = '最多保存 20 个 GHCR 检测地址'; return }
  ghcrCandidates.value = [...new Set([...ghcrCandidates.value, url])]
  try { localStorage.setItem(ghcrCandidatesKey, JSON.stringify(ghcrCandidates.value)) } catch { /* Optional browser cache. */ }
  ghcrInput.value = ''
  inputError.value = ''
}

function removeGhcr(url) {
  ghcrCandidates.value = ghcrCandidates.value.filter(candidate => candidate !== url)
  try { localStorage.setItem(ghcrCandidatesKey, JSON.stringify(ghcrCandidates.value)) } catch { /* Optional browser cache. */ }
}

function addSelected() {
  if (activeRegistry.value !== 'dockerhub' || props.environmentId !== 'local') return
  const available = new Set(candidates.value.map(item => item.url))
  for (const url of selected.value) if (available.has(url) && !configured.value.has(url)) emit('add', url)
  selected.value = []
}

async function checkCandidates(items) {
  if (busy.value || props.environmentId !== 'local' || currentEnvironmentId() !== 'local') return
  const work = items.filter(item => normalizeMirrorURL(item.url))
  const current = new AbortController()
  controller = current
  busy.value = true
  try {
    for (const candidate of work) {
      if (current.signal.aborted || currentEnvironmentId() !== 'local') break
      checking.value = candidate.url
      const url = normalizeMirrorURL(candidate.url)
      try {
        const result = await checkRegistryMirror(url, { signal: current.signal, environmentId: 'local' })
        if (current.signal.aborted || currentEnvironmentId() !== 'local') break
        if (!validProbeResult(result, url)) throw new Error('检测返回了无效结果')
        results.value = { ...results.value, [candidate.url]: result }
        delete errors.value[candidate.url]
        writeProbeCache(results.value)
      } catch (error) {
        if (current.signal.aborted) break
        errors.value[candidate.url] = error.message || '检测失败'
      }
    }
  } finally {
    if (controller === current) stopChecks()
  }
}
</script>

<style scoped>
.mirror-status { border-top: 1px solid var(--border-subtle); padding-top: 14px; }
.mirror-status__header, .mirror-status__meta, .mirror-status__actions, .mirror-status__input { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.mirror-status__header { justify-content: space-between; }
.mirror-status__tabs { display: flex; gap: 4px; }
.mirror-status button { display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-height: 32px; padding: 6px 8px; border: 1px solid var(--border-subtle); border-radius: 4px; color: var(--text-primary); background: var(--bg-secondary); cursor: pointer; font-size: 0.8125rem; }
.mirror-status button[aria-selected="true"] { color: var(--color-primary-600); border-color: var(--color-primary-500); background: var(--bg-elevated); }
.mirror-status button:disabled { opacity: 0.5; cursor: not-allowed; }
.mirror-status__reference { display: inline-flex; align-items: center; gap: 4px; color: var(--color-primary-600); font-size: 0.8125rem; }
.mirror-status__meta { margin: 10px 0; color: var(--text-secondary); font-size: 0.75rem; }
.mirror-status__scroll { max-height: 290px; overflow: auto; }
.mirror-status__table { width: 100%; border-collapse: collapse; text-align: left; font-size: 0.8125rem; table-layout: fixed; }
.mirror-status__table th, .mirror-status__table td { padding: 8px 6px; border-bottom: 1px solid var(--border-subtle); }
.mirror-status__table .mirror-status__selection { width: 44px; }
.mirror-status__table th:last-child { width: 78px; }
.mirror-status__table th:nth-last-child(2) { width: 190px; }
.mirror-status__source > span, .mirror-status__result > span, .mirror-status__result time { display: block; }
.mirror-status__source small { margin-left: 6px; color: var(--text-secondary); }
.mirror-status__url { overflow-wrap: anywhere; color: var(--text-secondary); font-size: 0.75rem; }
.mirror-status__result { color: var(--text-secondary); font-size: 0.75rem; overflow-wrap: anywhere; }
.mirror-status__source a { color: var(--color-primary-600); }
.mirror-status__state.is-reachable, .mirror-status__state.is-auth_required { color: var(--color-success-600); }
.mirror-status__error { color: var(--color-danger-600); font-size: 0.75rem; overflow-wrap: anywhere; }
.mirror-status__input { margin-bottom: 8px; flex-wrap: nowrap; }
.mirror-status__input input { min-width: 0; flex: 1; padding: 8px; border: 1px solid var(--border-subtle); border-radius: 4px; background: var(--bg-secondary); color: var(--text-primary); font-size: 0.8125rem; }
.mirror-status__actions { margin-top: 10px; }
@media (max-width: 540px) {
  .mirror-status__table { min-width: 430px; }
}
</style>
