<template>
  <Modal
    v-model:visible="localVisible"
    title="配置历史"
    width="920px"
    :show-close="!restoring"
    :close-on-esc="!restoring"
    @close="close"
  >
    <div class="history-shell">
      <aside class="history-list" aria-label="Compose 配置历史列表">
        <div class="history-list-heading">
          <span>{{ projectName }}</span>
          <small>最近 {{ historyItems.length }} 份</small>
        </div>
        <div v-if="loadingList" class="history-loading">正在读取配置历史...</div>
        <div v-else class="history-items">
          <button
            v-for="item in allItems"
            :key="item.id"
            type="button"
            class="history-item"
            :class="{ 'is-current': item.current, 'is-selected': selectedId === item.id }"
            :data-history-id="item.id"
            @click="selectHistory(item)"
          >
            <span class="history-marker" aria-hidden="true"></span>
            <span class="history-copy">
              <strong>{{ sourceLabel(item.source) }}</strong>
              <small>{{ formatDate(item.createdAt) }}</small>
            </span>
            <span v-if="item.summary?.total" class="history-count">{{ item.summary.total }}</span>
          </button>
          <div v-if="allItems.length === 0" class="history-empty">尚无配置历史</div>
        </div>
      </aside>

      <section class="history-preview" aria-live="polite">
        <div v-if="selectedItem" class="history-preview-heading">
          <div>
            <span>{{ selectedItem.composePath || 'Compose YAML' }}</span>
            <strong>{{ sourceLabel(selectedItem.source) }}</strong>
          </div>
          <small>{{ formatDate(selectedItem.createdAt) }}</small>
        </div>
        <div v-if="previewLoading" class="history-loading">正在比较配置...</div>
        <div v-else-if="selectedItem?.current" class="history-current-note">
          这是当前正在使用的配置。选择左侧历史版本可查看恢复变化。
        </div>
        <ComposeChangePreview v-else-if="preview" :preview="preview" />
        <div v-else class="history-empty">选择一份历史配置查看变化</div>
      </section>
    </div>

    <template #footer>
      <button type="button" class="action-btn secondary" :disabled="restoring" @click="close">关闭</button>
      <button
        v-if="selectedItem && !selectedItem.current && preview?.hasChanges"
        type="button"
        class="action-btn primary"
        :disabled="restoring || previewLoading"
        @click="restoreSelected"
      >
        {{ restoring ? '恢复中...' : '恢复此版本' }}
      </button>
    </template>
  </Modal>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import Modal from '@/components/feedback/Modal.vue'
import { compose } from '@edition/api'
import { useUiStore } from '@/stores/ui.js'
import ComposeChangePreview from './ComposeChangePreview.vue'

const props = defineProps({
  visible: { type: Boolean, default: false },
  projectName: { type: String, default: '' }
})

const emit = defineEmits(['update:visible', 'restored'])
const uiStore = useUiStore()
const localVisible = computed({
  get: () => props.visible,
  set: value => emit('update:visible', value)
})

const currentItem = ref(null)
const historyItems = ref([])
const selectedId = ref('')
const preview = ref(null)
const loadingList = ref(false)
const previewLoading = ref(false)
const restoring = ref(false)
let requestVersion = 0

const allItems = computed(() => [currentItem.value, ...historyItems.value].filter(Boolean))
const selectedItem = computed(() => allItems.value.find(item => item.id === selectedId.value) || null)

watch(
  [() => props.visible, () => props.projectName],
  ([visible, projectName]) => {
    if (visible && projectName) loadHistory()
    if (!visible) resetState()
  },
  { immediate: true }
)

async function loadHistory () {
  const version = ++requestVersion
  loadingList.value = true
  preview.value = null
  try {
    const result = await compose.listHistory(props.projectName)
    if (version !== requestVersion) return
    currentItem.value = result?.current || null
    historyItems.value = Array.isArray(result?.items) ? result.items.slice(0, 10) : []
    const initial = historyItems.value[0] || currentItem.value
    selectedId.value = initial?.id || ''
    if (initial && !initial.current) await loadPreview(initial, version)
  } catch (error) {
    if (version === requestVersion) {
      uiStore.toastError('读取配置历史失败: ' + (error.message || '未知错误'))
    }
  } finally {
    if (version === requestVersion) loadingList.value = false
  }
}

async function selectHistory (item) {
  selectedId.value = item.id
  preview.value = null
  if (item.current) return
  await loadPreview(item, requestVersion)
}

async function loadPreview (item, version = requestVersion) {
  previewLoading.value = true
  try {
    const result = await compose.previewHistory(props.projectName, item.id)
    if (version === requestVersion && selectedId.value === item.id) preview.value = result
  } catch (error) {
    if (version === requestVersion && selectedId.value === item.id) {
      uiStore.toastError('比较历史配置失败: ' + (error.message || '未知错误'))
    }
  } finally {
    if (version === requestVersion && selectedId.value === item.id) previewLoading.value = false
  }
}

async function restoreSelected () {
  const item = selectedItem.value
  if (!item || item.current || !preview.value?.baseHash) return
  const confirmed = await uiStore.confirm({
    title: '恢复 Compose 配置',
    message: `确定将项目 “${props.projectName}” 恢复到所选配置吗？恢复后不会自动部署，也不会修改 .env。`,
    type: 'warning',
    confirmText: '恢复配置',
    cancelText: '取消'
  })
  if (!confirmed) return

  restoring.value = true
  try {
    const result = await compose.restoreHistory(props.projectName, item.id, { baseHash: preview.value.baseHash })
    const message = result?.message || '配置已恢复，尚未部署'
    uiStore.toastSuccess(message)
    emit('restored', { name: props.projectName })
    emit('update:visible', false)
  } catch (error) {
    uiStore.toastError('恢复配置失败: ' + (error.message || '未知错误'))
  } finally {
    restoring.value = false
  }
}

function close () {
  if (restoring.value) return
  emit('update:visible', false)
}

function resetState () {
  requestVersion++
  currentItem.value = null
  historyItems.value = []
  selectedId.value = ''
  preview.value = null
  loadingList.value = false
  previewLoading.value = false
}

function sourceLabel (source) {
  return ({ current: '当前配置', manual_edit: '手动编辑', history_restore: '历史恢复' })[source] || '配置记录'
}

function formatDate (value) {
  const raw = String(value || '').trim()
  if (!raw) return '-'
  const parsed = new Date(raw.includes('T') ? raw : raw.replace(' ', 'T') + '+08:00')
  if (Number.isNaN(parsed.getTime())) return raw
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false
  }).format(parsed)
}
</script>

<style scoped>
.history-shell {
  height: min(62vh, 600px);
  min-height: 420px;
  display: grid;
  grid-template-columns: 240px minmax(0, 1fr);
  overflow: hidden;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--bg-elevated);
}

.history-list {
  min-width: 0;
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--border-subtle);
  background: color-mix(in srgb, var(--bg-secondary) 70%, var(--bg-elevated));
}

.history-list-heading,
.history-preview-heading {
  min-height: 52px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--border-subtle);
}

.history-list-heading span,
.history-preview-heading strong {
  overflow: hidden;
  color: var(--text-primary);
  font: 600 0.8125rem/1.35 var(--font-mono, ui-monospace, monospace);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.history-list-heading small,
.history-preview-heading span,
.history-preview-heading small {
  color: var(--text-tertiary);
  font-size: 0.75rem;
  white-space: nowrap;
}

.history-preview-heading > div {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.history-items,
.history-preview {
  min-height: 0;
  overflow-y: auto;
  scrollbar-color: var(--border-default) transparent;
  scrollbar-width: thin;
}

.history-item {
  position: relative;
  width: 100%;
  min-height: 58px;
  display: grid;
  grid-template-columns: 3px minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  padding: 8px 12px 8px 10px;
  border: 0;
  border-bottom: 1px solid var(--border-subtle);
  background: transparent;
  color: var(--text-secondary);
  text-align: left;
  cursor: pointer;
  transition: background-color var(--motion-duration-quick) var(--motion-ease-out), color var(--motion-duration-quick) var(--motion-ease-out);
}

.history-item:hover {
  background: var(--bg-tertiary);
  color: var(--text-primary);
}

.history-item.is-selected {
  background: var(--color-primary-50);
  color: var(--text-primary);
}

.history-marker {
  align-self: stretch;
  border-radius: 2px;
  background: var(--border-default);
}

.history-item.is-current .history-marker { background: var(--color-success-500); }
.history-item.is-selected .history-marker { background: var(--color-primary-500); }

.history-copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.history-copy strong {
  color: inherit;
  font-size: 0.8125rem;
  font-weight: 600;
}

.history-copy small,
.history-count {
  color: var(--text-tertiary);
  font-size: 0.75rem;
}

.history-count {
  font-family: var(--font-mono, ui-monospace, monospace);
}

.history-preview {
  min-width: 0;
  padding: 0 16px 16px;
}

.history-preview-heading {
  margin: 0 -16px 14px;
}

.history-loading,
.history-empty,
.history-current-note {
  min-height: 140px;
  display: grid;
  place-items: center;
  padding: 20px;
  color: var(--text-tertiary);
  font-size: 0.8125rem;
  line-height: 1.6;
  text-align: center;
}

.action-btn {
  min-height: 38px;
  padding: 0 16px;
  border: 1px solid var(--border-default);
  border-radius: 8px;
  background: var(--bg-elevated);
  color: var(--text-secondary);
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
}

.action-btn:hover { background: var(--bg-secondary); color: var(--text-primary); }
.action-btn.primary { border-color: var(--color-primary-600); background: var(--color-primary-500); color: var(--text-inverse); }
.action-btn.primary:hover { background: var(--color-primary-600); }
.action-btn:disabled { opacity: 0.6; cursor: not-allowed; }

@media (max-width: 720px) {
  .history-shell {
    height: min(68vh, 640px);
    min-height: 500px;
    grid-template-columns: 1fr;
    grid-template-rows: minmax(180px, 42%) minmax(0, 1fr);
  }

  .history-list {
    border-right: 0;
    border-bottom: 1px solid var(--border-subtle);
  }
}
</style>
