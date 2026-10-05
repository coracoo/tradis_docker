<template>
  <div class="build-cache-panel">
    <!-- 加载状态 -->
    <div v-if="loading && buildCache.length === 0" class="loading-state">
      <DynamicIcon name="loading" :size="32" />
      <p>加载中...</p>
    </div>

    <!-- 空状态 -->
    <div v-else-if="!loading && buildCache.length === 0" class="empty-state">
      <DynamicIcon name="hard-drive" :size="48" />
      <p>暂无构建缓存</p>
    </div>

    <!-- 缓存表格 - 复用 images 列表视图样式 -->
    <div v-else class="list-view">
      <table class="data-table">
        <thead>
          <tr>
            <th>ID</th>
            <th>类型</th>
            <th>描述</th>
            <th>大小</th>
            <th>状态</th>
            <th>使用次数</th>
            <th>创建时间</th>
            <th>最后使用</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in buildCache" :key="item.ID">
            <td class="cell-id" :title="item.ID">{{ item.ID?.substring(0, 12) }}</td>
            <td>
              <span class="type-badge">{{ item.Type || '-' }}</span>
            </td>
            <td class="cell-desc" :title="item.Description">{{ item.Description || '-' }}</td>
            <td class="cell-size">{{ formatBytes(item.Size) }}</td>
            <td>
              <span
                class="status-badge"
                :class="item.InUse ? 'in-use' : 'unused'"
                title="当前占用状态；是否可清理由 Docker 判断，共享缓存可能保留"
              >
                {{ (item.InUse ? '使用中' : '空闲') + (item.Shared ? '（共享）' : '') }}
              </span>
            </td>
            <td>{{ item.UsageCount }}</td>
            <td class="cell-time">{{ formatTime(item.CreatedAt) }}</td>
            <td class="cell-time">{{ item.LastUsedAt ? formatTime(item.LastUsedAt) : '-' }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <ConfirmDialog
      :visible="showConfirm"
      type="danger"
      :title="confirmTitle"
      :message="confirmMessage"
      :confirm-text="pruning ? '清理中...' : '确认清理'"
      @confirm="confirmPrune"
      @cancel="showConfirm = false"
    />

  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { getBuildCache, pruneBuildCache } from '../../../api/images.js'
import { formatBytes } from '../../../utils/format.js'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import { useUiStore } from '@/stores/ui.js'

const uiStore = useUiStore()

const buildCache = ref([])
const totalSize = ref(0)
const activeCount = ref(0)
const loading = ref(false)
const pruning = ref(false)
const showConfirm = ref(false)
const confirmTitle = ref('')
const confirmMessage = ref('')
const confirmAction = ref(null)

function formatTime(ts) {
  if (!ts) return '-'
  const d = new Date(ts)
  if (isNaN(d.getTime())) {
    const n = Number(ts)
    if (!isNaN(n) && n > 0) {
      return new Date(n * 1000).toLocaleString('zh-CN')
    }
    return '-'
  }
  return d.toLocaleString('zh-CN')
}

async function loadData(force = true) {
  loading.value = true
  try {
    const data = await getBuildCache(force ? { force: '1' } : {})
    buildCache.value = data.build_cache || []
    totalSize.value = data.total_size || 0
    activeCount.value = data.active_count || 0
  } catch (e) {
    console.error('获取构建缓存失败:', e)
    showToast('获取构建缓存失败', 'error')
  } finally {
    loading.value = false
  }
}

function pruneUnused() {
  confirmTitle.value = '清理悬空构建缓存'
  confirmMessage.value = '仅清理 Docker 判定的悬空构建缓存；其他空闲缓存可能保留，此操作不可撤销。'
  confirmAction.value = () => doPrune({ all: false })
  showConfirm.value = true
}

function pruneAll() {
  confirmTitle.value = '清理全部未使用的构建缓存'
  confirmMessage.value = '将清理所有未使用的构建缓存，正在使用的缓存不会删除。后续构建可能需要重新生成缓存，此操作不可撤销。'
  confirmAction.value = () => doPrune({ all: true })
  showConfirm.value = true
}

async function confirmPrune() {
  showConfirm.value = false
  if (confirmAction.value) {
    await confirmAction.value()
  }
}

async function doPrune(opts) {
  pruning.value = true
  try {
    const result = await pruneBuildCache(opts)
    showToast(
      `已清理 ${result.deleted_count || 0} 项缓存，释放 ${formatBytes(result.space_reclaimed || 0)}`,
      'success'
    )
    await loadData()
  } catch (e) {
    console.error('清理构建缓存失败:', e)
    showToast('清理构建缓存失败', 'error')
  } finally {
    pruning.value = false
  }
}

function showToast(message, type = 'info') {
  if (type === 'success') uiStore.toastSuccess(message)
  else if (type === 'error') uiStore.toastError(message)
  else uiStore.toastInfo(message)
}

onMounted(() => {
  loadData(false)
})

defineExpose({
  buildCache,
  totalSize,
  activeCount,
  loading,
  pruning,
  loadData,
  pruneUnused,
  pruneAll
})
</script>

<style scoped>
.build-cache-panel {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.loading-state,
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 4rem 2rem;
  color: var(--text-tertiary);
  gap: 1rem;
}

/* 复用 images 列表视图样式 */
.list-view {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  overflow-y: auto;
  max-height: calc(100vh - 280px);
}

.data-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.875rem;
}

.data-table thead {
  position: sticky;
  top: 0;
  z-index: 1;
}

.data-table th {
  text-align: left;
  padding: 12px 16px;
  font-weight: 600;
  color: var(--text-secondary);
  background: var(--bg-elevated);
  border-bottom: 1px solid var(--border-subtle);
  white-space: nowrap;
  font-size: 0.75rem;
  text-transform: uppercase;
}

html.dark .data-table th {
  background: var(--bg-secondary);
}

.data-table td {
  padding: 12px 16px;
  border-bottom: 1px solid var(--border-subtle);
  color: var(--text-primary);
  vertical-align: middle;
  font-size: 0.8125rem;
}

.data-table tbody tr:hover td {
  background: var(--color-primary-50);
}

.data-table tbody tr:last-child td {
  border-bottom: none;
}

.cell-id {
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.75rem;
  color: var(--text-secondary);
}

.cell-desc {
  max-width: 250px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cell-size {
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.75rem;
  color: var(--text-secondary);
}

.cell-time {
  font-size: 0.75rem;
  color: var(--text-secondary);
  white-space: nowrap;
}

.type-badge {
  padding: 0.125rem 0.5rem;
  border-radius: 999px;
  font-size: 0.75rem;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
}

.status-badge {
  padding: 0.125rem 0.5rem;
  border-radius: 999px;
  font-size: 0.75rem;
  white-space: nowrap;
}

.status-badge.in-use {
  background: var(--color-success-100);
  color: var(--color-success-700);
}

.status-badge.unused {
  background: var(--color-warning-100);
  color: var(--color-warning-700);
}

</style>
