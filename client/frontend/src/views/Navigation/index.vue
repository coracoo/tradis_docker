<template>
  <ResourceWorkbench>
    <template #toolbar>
      <div class="toolbar workbench-toolbar">
        <div class="toolbar-left workbench-toolbar-left">
        <SearchInput
          v-model="searchQuery"
          placeholder="搜索应用名称..."
          @search="handleSearch"
        />
        <div class="filter-group">
          <label class="visibility-option">
            <input
              type="checkbox"
              :checked="showDeleted"
              @change="persistShowDeleted($event.target.checked)"
            />
            <span>显示隐藏</span>
          </label>
        </div>
        </div>
        <div class="toolbar-right">
        <button
          v-ripple
          class="icon-btn resource-refresh-btn"
          :class="{ 'is-spinning': loading }"
          :disabled="loading"
          title="刷新应用导航"
          @click="refreshData"
        >
          <DynamicIcon class="resource-refresh-icon" name="refresh" :size="18" />
        </button>
        <button v-if="navigationAIEnabled" class="icon-btn" title="重新识别" @click="confirmRegenerate" type="button">
          <DynamicIcon name="sparkles" :size="18" />
        </button>
        <button class="icon-btn" title="分类管理" @click="categoryDialogVisible = true">
          <DynamicIcon name="tags" :size="18" />
        </button>
        <button class="primary-btn" @click="openAddDialog">
          <DynamicIcon name="plus" :size="16" />
          添加应用
        </button>
        </div>
      </div>
    </template>

    <div class="navigation-page" role="main" aria-label="应用导航">

    <!-- 应用列表 -->
    <div class="navigation-content">
      <div v-if="loading" class="loading-state">
        <div v-for="n in 3" :key="n" class="category-skeleton">
          <div class="skeleton-header"></div>
          <div class="skeleton-grid">
            <div v-for="m in 4" :key="m" class="skeleton-card"></div>
          </div>
        </div>
      </div>
      <EmptyState
        v-else-if="!groupedApps.length"
        icon="box"
        title="暂无应用"
        description="点击右上角添加应用按钮创建"
      />
      <div v-else class="category-list motion-reveal">
        <div
          v-for="category in groupedApps"
          :key="category.name"
          class="category-section"
          :class="{ 'is-collapsed': collapsedCategories.has(category.name) }"
        >
          <div class="category-header">
            <div class="category-title" @click="toggleCategory(category.name)">
              <DynamicIcon name="chevron-down" :size="16" class="collapse-icon" />
              <span class="category-name">{{ category.name }}</span>
              <span class="category-count">{{ category.apps.length }}</span>
            </div>
            <div class="category-actions">
              <button 
                class="category-more-btn" 
                @click.stop="toggleCategoryMenu(category.name)"
              >
                <DynamicIcon name="more-vertical" :size="16" />
              </button>
              <div v-if="activeCategoryMenu === category.name" class="category-dropdown">
                <div class="dropdown-item" @click.stop="renameCategory(category.name)">
                  <DynamicIcon name="edit" :size="14" />
                  编辑分类
                </div>
                <div class="dropdown-item danger" @click.stop="deleteCategory(category.name)">
                  <DynamicIcon name="delete" :size="14" />
                  删除分类
                </div>
              </div>
            </div>
          </div>
          <div class="category-body">
            <div class="app-grid">
              <div
                v-for="app in category.apps"
                :key="app.id"
                class="app-card"
                :class="{ 
                  'is-hidden': app.isDeleted,
                  'is-auto': app.source === 'auto',
                  'is-ai': app.source === 'ai'
                }"
              >
                <!-- 应用图标 -->
                <div class="app-icon" @click="openApp(app)">
                  <img
                    v-if="isUrlIcon(getAppIcon(app)) && !isBrokenIcon(getAppIcon(app))"
                    :src="getAppIcon(app)"
                    alt="icon" 
                    @error="markIconBroken(getAppIcon(app))"
                  />
                  <DynamicIcon
                    v-else-if="isUrlIcon(getAppIcon(app))"
                    :name="getAppFallbackIcon(app)"
                    :size="32"
                    class="icon-fallback"
                  />
                  <DynamicIcon v-else-if="getAppIcon(app)" :name="getAppIcon(app)" :size="32" />
                  <DynamicIcon v-else name="box" :size="32" />
                </div>
                <!-- 应用信息 -->
                <div class="app-info" @click="openApp(app)">
                  <div class="app-name">{{ app.name }}</div>
                  <div class="app-tags">
                    <span v-if="app.source === 'auto'" class="tag auto">Auto</span>
                    <span v-if="navigationAIEnabled && app.source === 'ai'" class="tag ai">AI</span>
                    <span v-if="app.isDeleted" class="tag hidden-status" :title="getHiddenReasonTitle(app)">{{ getHiddenLabel(app) }}</span>
                  </div>
                </div>
                <!-- 快捷访问按钮 -->
                <div class="app-quick-actions">
                  <button 
                    v-if="app.lanUrl && !app.isDeleted" 
                    class="quick-btn" 
                    title="内网访问"
                    @click.stop="openUrl(app.lanUrl)"
                  >
                    <DynamicIcon name="link" :size="14" />
                    内网
                  </button>
                  <button 
                    v-if="app.wanUrl && !app.isDeleted" 
                    class="quick-btn" 
                    title="外网访问"
                    @click.stop="openUrl(app.wanUrl)"
                  >
                    <DynamicIcon name="globe" :size="14" />
                    外网
                  </button>
                </div>
                <!-- 操作按钮 -->
                <div class="app-actions">
                  <button v-if="!app.isDeleted" class="action-btn" title="编辑" @click.stop="editApp(app)">
                    <DynamicIcon name="edit" :size="14" />
                  </button>
                  <button
                    v-if="navigationAIEnabled && !app.isDeleted && app.source !== 'manual'"
                    class="action-btn"
                    :class="{ 'is-loading': aiEnrichingIds.has(app.id) }"
                    :disabled="aiEnrichingIds.has(app.id)"
                    :title="aiEnrichingIds.has(app.id) ? 'AI识别中...' : 'AI识别'"
                    @click.stop="aiEnrich(app)"
                  >
                    <DynamicIcon :name="aiEnrichingIds.has(app.id) ? 'loader' : 'sparkles'" :size="14" />
                  </button>
                  <button v-if="!app.isDeleted" class="action-btn" title="隐藏" @click.stop="deleteApp(app)">
                    <DynamicIcon name="delete" :size="14" />
                  </button>
                  <button v-if="app.isDeleted" class="action-btn restore" title="恢复" @click.stop="restoreApp(app)">
                    <DynamicIcon name="backup" :size="14" />
                  </button>
                  <button v-if="app.isDeleted" class="action-btn danger" title="彻底删除" @click.stop="purgeApp(app)">
                    <DynamicIcon name="delete" :size="14" />
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 添加/编辑应用对话框 -->
    <Modal v-model:visible="appDialogVisible" :title="isEdit ? '编辑应用' : '添加应用'" width="480px">
      <div class="form-group">
        <label class="form-label">应用名称</label>
        <input v-model="appForm.name" type="text" class="form-input" placeholder="输入应用名称" />
      </div>
      <div class="form-group">
        <label class="form-label">分类</label>
        <select v-model="appForm.category" class="form-select">
          <option value="">选择或输入分类</option>
          <option v-for="cat in categories" :key="cat" :value="cat">{{ cat }}</option>
        </select>
        <input v-model="appForm.category" type="text" class="form-input mt-2" placeholder="或输入新分类" />
      </div>
      <div class="form-group">
        <label class="form-label">图标</label>
        <div class="icon-input-group">
          <input v-model="appForm.icon" type="text" class="form-input" placeholder="输入图标名称或URL" />
          <div v-if="appForm.icon" class="icon-preview">
            <img v-if="isUrlIcon(appForm.icon) && !isBrokenIcon(appForm.icon)" :src="appForm.icon" alt="preview" @error="markIconBroken(appForm.icon)" />
            <DynamicIcon v-else-if="isUrlIcon(appForm.icon)" name="box" :size="24" />
            <DynamicIcon v-else :name="appForm.icon" :size="24" />
          </div>
        </div>
        <div class="icon-tools">
          <input
            ref="iconFileInput"
            type="file"
            class="upload-input"
            accept="image/png,image/jpeg,image/webp,image/gif,image/svg+xml,image/x-icon"
            @change="handleIconFileChange"
          />
          <button class="btn btn-default btn-sm" type="button" :disabled="!isEdit || iconUploading" @click="chooseIconFile">
            <span v-if="iconUploading" class="btn-spinner"></span>
            <DynamicIcon v-else name="upload" :size="14" />
            {{ iconUploading ? '上传中...' : '上传图标' }}
          </button>
          <p class="form-hint">{{ isEdit ? '支持 Lucide 图标名、mdi:前缀、图片 URL 或上传本地图标' : '新增应用保存后可上传本地图标' }}</p>
        </div>
      </div>
      <div class="form-group">
        <label class="form-label">内网访问地址</label>
        <input v-model="appForm.lanUrl" type="text" class="form-input" placeholder="http://localhost:8080" />
      </div>
      <div class="form-group">
        <label class="form-label">外网访问地址</label>
        <input v-model="appForm.wanUrl" type="text" class="form-input" placeholder="https://example.com" />
      </div>
      <template #footer>
        <button class="btn btn-default" @click="appDialogVisible = false">取消</button>
        <button class="btn btn-primary" :disabled="saving" @click="saveApp">
          <span v-if="saving" class="btn-spinner"></span>
          {{ saving ? '保存中...' : '保存' }}
        </button>
      </template>
    </Modal>

    <!-- 分类管理对话框 -->
    <Modal v-model:visible="categoryDialogVisible" title="分类管理" width="400px">
      <div class="category-list-manage">
        <!-- 新增分类 -->
        <div class="add-category-row">
          <input 
            v-model="newCategoryName" 
            type="text" 
            class="form-input" 
            placeholder="输入新分类名称"
            @keyup.enter="addCategory"
          />
          <button class="btn btn-primary btn-sm" @click="addCategory" :disabled="!newCategoryName.trim()">
            <DynamicIcon name="plus" :size="14" />
            新增
          </button>
        </div>
        <!-- 分类列表 -->
        <div v-for="cat in categories" :key="cat" class="category-item">
          <span class="category-item-name">{{ cat }}</span>
          <div class="category-item-actions">
            <button class="action-btn-sm" @click="renameCategory(cat)">
              <DynamicIcon name="edit" :size="14" />
            </button>
            <button class="action-btn-sm danger" @click="deleteCategory(cat)">
              <DynamicIcon name="delete" :size="14" />
            </button>
          </div>
        </div>
      </div>
      <template #footer>
        <button class="btn btn-default" @click="categoryDialogVisible = false">关闭</button>
      </template>
    </Modal>

    <!-- 重命名分类对话框 -->
    <Modal v-model:visible="renameDialogVisible" title="重命名分类" width="360px">
      <div class="form-group">
        <label class="form-label">分类名称</label>
        <input
          v-model="renameForm.newName"
          type="text"
          class="form-input"
          placeholder="输入新的分类名称"
          @keyup.enter="submitRenameCategory"
        />
      </div>
      <template #footer>
        <button class="btn btn-default" @click="renameDialogVisible = false">取消</button>
        <button class="btn btn-primary" :disabled="saving || !renameForm.newName.trim()" @click="submitRenameCategory">
          {{ saving ? '保存中...' : '保存' }}
        </button>
      </template>
    </Modal>

    <!-- 确认对话框 -->
    <Modal v-model:visible="confirmDialog.visible" :title="confirmDialog.title" width="420px">
      <p class="confirm-message">{{ confirmDialog.message }}</p>
      <template #footer>
        <button class="btn btn-default" @click="closeConfirmDialog">取消</button>
        <button
          class="btn"
          :class="confirmDialog.type === 'danger' ? 'btn-danger' : 'btn-primary'"
          :disabled="saving"
          @click="handleConfirmAction"
        >
          {{ saving ? '处理中...' : confirmDialog.confirmText }}
        </button>
      </template>
    </Modal>
    </div>
  </ResourceWorkbench>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, reactive } from 'vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import Modal from '@/components/feedback/Modal.vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import ResourceWorkbench from '@/components/resource-workbench/ResourceWorkbench.vue'
import { usePageAction } from '@/composables/usePageAction.js'
import { listNavigation, addNavigation, updateNavigation, deleteNavigation, purgeNavigation, restoreNavigation, uploadIcon } from '@/api/navigation.js'
import { enrichNavigation, enrichNavigationById, navigationAIEnabled } from '@edition/navigation-ai'
import { useToast } from '@/composables/useToast.js'
import { usePersistentBooleanPreference } from '@/composables/usePersistentBooleanPreference.js'

const toast = useToast()

// 状态
const loading = ref(false)
const searchQuery = ref('')
const { value: showDeleted, persist: persistShowDeleted } = usePersistentBooleanPreference(
  'navigation_show_hidden',
  false
)
const apps = ref([])
const collapsedCategories = ref(new Set())
const activeCategoryMenu = ref(null)
const brokenIconUrls = ref(new Set())
const aiEnrichingIds = reactive(new Set())
const recognitionPollers = new Map()

// 对话框状态
const appDialogVisible = ref(false)
const categoryDialogVisible = ref(false)
const renameDialogVisible = ref(false)
const isEdit = ref(false)
const newCategoryName = ref('')
const saving = ref(false)
const iconUploading = ref(false)
const iconFileInput = ref(null)
const appForm = reactive({
  id: null,
  name: '',
  category: '',
  icon: '',
  lanUrl: '',
  wanUrl: ''
})
const renameForm = reactive({
  oldName: '',
  newName: ''
})
const confirmDialog = reactive({
  visible: false,
  title: '',
  message: '',
  confirmText: '确定',
  type: 'warning',
  action: '',
  target: null
})

// 计算属性 - 分组的应用
const groupedApps = computed(() => {
  let filtered = apps.value
  
  // 搜索过滤
  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    filtered = filtered.filter(app => 
      app.name.toLowerCase().includes(query) ||
      app.category.toLowerCase().includes(query)
    )
  }
  
  // 已隐藏过滤
  if (!showDeleted.value) {
    filtered = filtered.filter(app => !app.isDeleted)
  }
  
  // 按分类分组
  const groups = {}
  filtered.forEach(app => {
    const category = app.category || '未分类'
    if (!groups[category]) {
      groups[category] = []
    }
    groups[category].push(app)
  })
  
  // 转换为数组
  return Object.keys(groups)
    .sort()
    .map(name => ({
      name,
      apps: groups[name].sort((a, b) => a.name.localeCompare(b.name))
    }))
})

// 所有分类
const categories = computed(() => {
  const cats = new Set()
  apps.value.forEach(app => {
    if (app.category) {
      cats.add(app.category)
    }
  })
  return Array.from(cats).sort()
})

// 方法
function isUrlIcon(icon) {
  return icon && (icon.startsWith('http://') || icon.startsWith('https://') || icon.startsWith('/'))
}

function isBrokenIcon(icon) {
  return brokenIconUrls.value.has(icon)
}

function markIconBroken(icon) {
  if (!icon) return
  const next = new Set(brokenIconUrls.value)
  next.add(icon)
  brokenIconUrls.value = next
}

function getAppIcon(app) {
  const icon = (app?.icon || '').trim()
  if (icon && icon !== 'mdi-docker' && icon !== 'mdi:docker') {
    return icon
  }
  // 后端会在服务在线时发现并缓存 favicon。没有缓存图标时使用稳定的本地图标，
  // 不直接依赖业务容器的 /favicon.ico，避免容器停止后导航图标消失。
  return 'container'
}

function getAppFallbackIcon(app) {
  const configuredIcon = (app?.icon || '').trim()
  if (configuredIcon && !isUrlIcon(configuredIcon)) {
    return configuredIcon
  }
  return 'container'
}

function getHiddenLabel(app) {
  switch (app?.hiddenReason) {
    case 'manual':
      return '已隐藏'
    case 'container_removed':
      return '已自动隐藏'
    case 'container_stopped':
      return '已自动隐藏'
    case 'container_paused':
      return '已自动隐藏'
    case 'container_missing':
      return '已自动隐藏'
    case 'container_unavailable':
      return '已自动隐藏'
    default:
      return app?.isAuto ? '已自动隐藏' : '已隐藏'
  }
}

function getHiddenReasonTitle(app) {
  switch (app?.hiddenReason) {
    case 'manual':
      return '手动隐藏'
    case 'container_removed':
      return '容器已删除或销毁'
    case 'container_stopped':
      return '容器已停止'
    case 'container_paused':
      return '容器已暂停'
    case 'container_missing':
      return '容器不存在'
    case 'container_unavailable':
      return '容器暂不可用或没有可用端口'
    default:
      return app?.isAuto ? '自动隐藏' : '手动隐藏'
  }
}

function toggleCategory(name) {
  if (collapsedCategories.value.has(name)) {
    collapsedCategories.value.delete(name)
  } else {
    collapsedCategories.value.add(name)
  }
}

function toggleCategoryMenu(name) {
  if (activeCategoryMenu.value === name) {
    activeCategoryMenu.value = null
  } else {
    activeCategoryMenu.value = name
  }
}

function handleSearch() {
  // 搜索逻辑在计算属性中处理
}

async function refreshData({ quiet = false } = {}) {
  if (!quiet) loading.value = true
  try {
    const res = await listNavigation({ include_deleted: true })
    // 后端返回数组，需要映射字段名
    apps.value = (res || []).map(item => ({
      id: item.id,
      name: item.title,
      category: item.category,
      icon: item.icon_url || item.icon,
      lanUrl: item.lan_url,
      wanUrl: item.wan_url,
      source: item.ai_generated ? 'ai' : (item.is_auto ? 'auto' : 'manual'),
      isAuto: item.is_auto,
      isDeleted: item.is_deleted,
      hiddenReason: item.hidden_reason || '',
      containerId: item.container_id,
      updatedAt: item.updated_at
    }))
  } catch (err) {
    console.error('加载导航失败:', err)
    toast.error('加载导航失败')
  } finally {
    if (!quiet) loading.value = false
  }
}

function navigationFingerprint(app) {
  if (!app) return ''
  return [app.name, app.category, app.icon, app.updatedAt].join('|')
}

function scheduleRecognitionRefresh(navId = null, previousFingerprint = '', onComplete = null) {
  const key = navId ? `nav-${navId}` : 'all'
  if (recognitionPollers.has(key)) {
    clearTimeout(recognitionPollers.get(key))
  }
  let attempts = 0
  const poll = async () => {
    attempts += 1
    await refreshData({ quiet: true })
    const current = navId ? apps.value.find(item => item.id === navId) : null
    const changed = navId
      ? current && navigationFingerprint(current) !== previousFingerprint
      : false
    const timedOut = attempts >= 20 && !changed
    if (changed || timedOut) {
      recognitionPollers.delete(key)
      if (onComplete) {
        onComplete(changed, timedOut)
      }
      return
    }
    recognitionPollers.set(key, setTimeout(poll, 1500))
  }
  recognitionPollers.set(key, setTimeout(poll, 800))
}

function openAddDialog() {
  isEdit.value = false
  appForm.id = null
  appForm.name = ''
  appForm.category = ''
  appForm.icon = ''
  appForm.lanUrl = ''
  appForm.wanUrl = ''
  appDialogVisible.value = true
}

function editApp(app) {
  isEdit.value = true
  appForm.id = app.id
  appForm.name = app.name
  appForm.category = app.category
  appForm.icon = app.icon
  appForm.lanUrl = app.lanUrl
  appForm.wanUrl = app.wanUrl
  appDialogVisible.value = true
}

function buildNavigationPayload(app) {
  return {
    title: app.name,
    category: app.category,
    icon_url: app.icon,
    lan_url: app.lanUrl,
    wan_url: app.wanUrl
  }
}

async function saveApp() {
  if (!appForm.name.trim()) {
    toast.error('请输入应用名称')
    return
  }
  
  saving.value = true
  try {
    const data = buildNavigationPayload(appForm)
    
    if (isEdit.value) {
      await updateNavigation(appForm.id, data)
      toast.success('应用已更新')
    } else {
      await addNavigation(data)
      toast.success('应用已添加')
    }
    
    appDialogVisible.value = false
    await refreshData()
  } catch (err) {
    console.error('保存应用失败:', err)
    toast.error('保存失败')
  } finally {
    saving.value = false
  }
}

function deleteApp(app) {
  openConfirmDialog({
    title: '隐藏应用',
    message: `确定要隐藏应用 "${app.name}" 吗？`,
    confirmText: '隐藏',
    type: 'warning',
    action: 'delete-app',
    target: app
  })
}

async function performDeleteApp(app) {
  try {
    await deleteNavigation(app.id)
    toast.success('应用已隐藏')
    await refreshData()
  } catch (err) {
    console.error('隐藏应用失败:', err)
    toast.error('隐藏失败')
  }
}

function purgeApp(app) {
  openConfirmDialog({
    title: '彻底删除应用',
    message: `确定要彻底删除应用 "${app.name}" 吗？此操作不可恢复。`,
    confirmText: '彻底删除',
    type: 'danger',
    action: 'purge-app',
    target: app
  })
}

async function performPurgeApp(app) {
  try {
    await purgeNavigation(app.id)
    toast.success('应用已彻底删除')
    await refreshData()
  } catch (err) {
    console.error('彻底删除应用失败:', err)
    toast.error('删除失败')
  }
}

async function restoreApp(app) {
  try {
    await restoreNavigation(app.id)
    toast.success('应用已恢复')
    await refreshData()
  } catch (err) {
    console.error('恢复应用失败:', err)
    toast.error('恢复失败')
  }
}

async function aiEnrich(app) {
  try {
    const previousFingerprint = navigationFingerprint(app)
    aiEnrichingIds.add(app.id)
    await enrichNavigationById({ navId: app.id, force: true })
    toast.success('AI识别已在后台启动')
    scheduleRecognitionRefresh(app.id, previousFingerprint, (changed, timedOut) => {
      aiEnrichingIds.delete(app.id)
      if (timedOut) {
        toast.warning(`「${app.name}」AI识别仍在进行，请稍后手动刷新`)
      } else if (changed) {
        toast.success(`「${app.name}」AI识别完成`)
      } else {
        toast.info(`「${app.name}」AI识别完成，无变化`)
      }
    })
  } catch (err) {
    aiEnrichingIds.delete(app.id)
    console.error('AI识别失败:', err)
    toast.error(err.message || 'AI识别失败')
  }
}

function confirmRegenerate() {
  openConfirmDialog({
    title: '重新识别导航',
    message: '确定要重新识别所有导航吗？这会清空自动发现的导航数据并重新生成。',
    confirmText: '重新识别',
    type: 'warning',
    action: 'regenerate'
  })
}

async function regenerateNavigation() {
  try {
    await enrichNavigation({ cleanup: true, force: true })
    toast.success('重新识别已在后台启动，页面将自动刷新')
    scheduleRecognitionRefresh()
  } catch (err) {
    console.error('[Navigation] 重新识别失败:', err)
    toast.error('重新识别失败: ' + (err?.message || '未知错误'))
  }
}

function chooseIconFile() {
  if (!isEdit.value || !appForm.id) {
    toast.error('请先保存应用后再上传图标')
    return
  }
  iconFileInput.value?.click()
}

async function handleIconFileChange(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) return
  if (!isEdit.value || !appForm.id) {
    toast.error('请先保存应用后再上传图标')
    return
  }

  iconUploading.value = true
  try {
    const res = await uploadIcon(appForm.id, file)
    appForm.icon = res?.icon_url || res?.iconUrl || appForm.icon
    toast.success('图标已上传')
    await refreshData()
  } catch (err) {
    console.error('上传图标失败:', err)
    toast.error('图标上传失败')
  } finally {
    iconUploading.value = false
  }
}

function openApp(app) {
  if (app.isDeleted) {
    toast.error('该应用已隐藏')
    return
  }
  const url = app.lanUrl || app.wanUrl
  openUrl(url)
}

function openUrl(url) {
  if (!url) {
    toast.error('该应用未设置访问地址')
    return
  }
  // URL安全校验
  if (!url.startsWith('http://') && !url.startsWith('https://')) {
    toast.error('访问地址格式不正确')
    return
  }
  window.open(url, '_blank')
}

async function addCategory() {
  const name = newCategoryName.value.trim()
  if (!name) return
  
  // 检查是否已存在
  if (categories.value.includes(name)) {
    toast.error('分类名称已存在')
    return
  }
  
  // 创建一个空的分类（通过创建一个示例应用，然后删除它）
  // 或者直接添加到分类列表中
  try {
    // 创建一个临时应用来建立分类
    await addNavigation({
      title: '_temp_category_' + Date.now(),
      category: name,
      icon_url: 'box',
      lan_url: '',
      wan_url: ''
    })
    toast.success('分类已创建')
    newCategoryName.value = ''
    await refreshData()
  } catch (err) {
    console.error('创建分类失败:', err)
    toast.error('创建分类失败')
  }
}

function renameCategory(oldName) {
  activeCategoryMenu.value = null
  renameForm.oldName = oldName
  renameForm.newName = oldName
  renameDialogVisible.value = true
}

async function submitRenameCategory() {
  const oldName = renameForm.oldName
  const newName = renameForm.newName.trim()
  if (!newName || newName === oldName) {
    renameDialogVisible.value = false
    return
  }
  if (categories.value.includes(newName)) {
    toast.error('分类名称已存在')
    return
  }

  saving.value = true
  try {
    // 更新该分类下的所有应用
    const appsInCategory = apps.value.filter(app => app.category === oldName)
    for (const app of appsInCategory) {
      await updateNavigation(app.id, buildNavigationPayload({ ...app, category: newName }))
    }
    toast.success('分类已重命名')
    renameDialogVisible.value = false
    await refreshData()
  } catch (err) {
    console.error('重命名分类失败:', err)
    toast.error('重命名失败')
  } finally {
    saving.value = false
  }
}

function deleteCategory(name) {
  activeCategoryMenu.value = null
  openConfirmDialog({
    title: '删除分类',
    message: `确定要删除分类 "${name}" 吗？该分类下的应用将被设为未分类。`,
    confirmText: '删除',
    type: 'danger',
    action: 'delete-category',
    target: name
  })
}

async function performDeleteCategory(name) {
  try {
    const appsInCategory = apps.value.filter(app => app.category === name)
    for (const app of appsInCategory) {
      await updateNavigation(app.id, buildNavigationPayload({ ...app, category: '' }))
    }
    toast.success('分类已删除')
    await refreshData()
  } catch (err) {
    console.error('删除分类失败:', err)
    toast.error('删除失败')
  }
}

function openConfirmDialog({ title, message, confirmText = '确定', type = 'warning', action, target = null }) {
  Object.assign(confirmDialog, {
    visible: true,
    title,
    message,
    confirmText,
    type,
    action,
    target
  })
}

function closeConfirmDialog() {
  confirmDialog.visible = false
  confirmDialog.action = ''
  confirmDialog.target = null
}

async function handleConfirmAction() {
  saving.value = true
  try {
    switch (confirmDialog.action) {
      case 'delete-app':
        await performDeleteApp(confirmDialog.target)
        break
      case 'purge-app':
        await performPurgeApp(confirmDialog.target)
        break
      case 'regenerate':
        await regenerateNavigation()
        break
      case 'delete-category':
        await performDeleteCategory(confirmDialog.target)
        break
      default:
        break
    }
    closeConfirmDialog()
  } finally {
    saving.value = false
  }
}

usePageAction({ create: openAddDialog })

onMounted(() => {
  refreshData()
})

onUnmounted(() => {
  for (const timer of recognitionPollers.values()) {
    clearTimeout(timer)
  }
  recognitionPollers.clear()
})
</script>

<style scoped>
.navigation-page {
  box-sizing: border-box;
  min-height: 0;
  height: 100%;
  display: flex;
  flex-direction: column;
  padding-bottom: 22px;
  color: var(--resource-ink);
}

/* 工具栏 */
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin: 0;
}

.toolbar-left {
  display: flex;
  align-items: center;
  gap: 12px;
  flex: 1;
  min-width: 0;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.filter-group {
  display: flex;
  align-items: center;
  gap: 7px;
}

.visibility-option {
  min-height: 38px;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 0 11px;
  border: 1px solid var(--resource-line);
  border-radius: 9px;
  background: var(--bg-elevated);
  color: var(--resource-muted);
  font-size: 0.78rem;
  cursor: pointer;
  user-select: none;
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out), background var(--motion-duration-quick) var(--motion-ease-out), color var(--motion-duration-quick) var(--motion-ease-out);
}

.visibility-option:hover {
  border-color: var(--border-default);
  color: var(--text-primary);
}

.visibility-option:has(input:checked) {
  border-color: color-mix(in srgb, var(--resource-accent) 42%, var(--resource-line));
  background: var(--resource-accent-soft);
  color: var(--resource-accent-strong);
}

.visibility-option input {
  width: 15px;
  height: 15px;
  margin: 0;
  accent-color: var(--resource-accent);
  cursor: pointer;
}

.icon-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  border-radius: 10px;
  background: var(--bg-elevated);
  border: 1px solid var(--resource-line);
  color: var(--resource-muted);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.icon-btn:hover {
  background: var(--bg-secondary);
  border-color: var(--border-default);
  color: var(--text-primary);
}

.icon-btn.is-spinning svg {
  animation: spin 1s linear infinite;
}

.icon-btn svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.primary-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 14px;
  height: 38px;
  background: linear-gradient(135deg, var(--color-primary-500), var(--color-primary-600));
  color: var(--text-inverse);
  border: none;
  border-radius: 10px;
  font-size: 0.875rem;
  font-weight: 600;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.primary-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px var(--color-primary-500-30);
}

.primary-btn svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

/* 内容区域 */
.navigation-content {
  min-height: 0;
  flex: 1;
  overflow: auto;
}

/* 加载状态 */
.loading-state {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.category-skeleton {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.skeleton-header {
  height: 24px;
  width: 120px;
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
  border-radius: 4px;
}

.skeleton-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 12px;
}

.skeleton-card {
  height: 148px;
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
  border-radius: 10px;
}

/* 分类列表 */
.category-list {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.category-section {
  background: var(--resource-panel);
  border: 1px solid var(--resource-line);
  border-radius: 12px;
  box-shadow: 0 14px 34px color-mix(in srgb, var(--text-primary) 5%, transparent);
  overflow: hidden;
}

.category-header {
  min-height: 52px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 0 14px;
  border-bottom: 1px solid var(--resource-line);
  transition: background var(--motion-duration-quick);
}

.category-header:hover {
  background: var(--resource-row-hover);
}

.category-title {
  min-height: 36px;
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  flex: 1;
}

.category-actions {
  position: relative;
}

.category-more-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 6px;
  background: transparent;
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick);
  opacity: 0;
}

.category-header:hover .category-more-btn {
  opacity: 1;
}

.category-more-btn:hover {
  background: var(--bg-tertiary);
  color: var(--text-primary);
}

.category-more-btn svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.category-dropdown {
  position: absolute;
  top: calc(100% + 4px);
  right: 0;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  box-shadow: var(--shadow-lg);
  padding: 4px;
  min-width: 140px;
  z-index: 100;
}

.category-dropdown .dropdown-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 0.8125rem;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick);
}

.category-dropdown .dropdown-item:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

.category-dropdown .dropdown-item.danger:hover {
  background: var(--color-danger-100);
  color: var(--color-danger-600);
}

.category-dropdown .dropdown-item svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
  flex-shrink: 0;
}

.collapse-icon {
  transition: transform var(--motion-duration-quick);
  color: var(--text-secondary);
}

.category-section.is-collapsed .collapse-icon {
  transform: rotate(-90deg);
}

.category-name {
  color: var(--resource-ink);
  font-size: 0.86rem;
  font-weight: 700;
}

.category-count {
  min-width: 24px;
  padding: 2px 7px;
  border-radius: 999px;
  background: var(--ops-muted-soft);
  color: var(--resource-muted);
  font-size: 0.68rem;
  text-align: center;
}

.category-body {
  padding: 14px;
}

.category-section.is-collapsed .category-body {
  display: none;
}

/* 应用网格 */
.app-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(168px, 180px));
  align-items: start;
  gap: 12px;
  justify-content: start;
}

.app-card {
  box-sizing: border-box;
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 7px;
  min-height: 148px;
  padding: 11px 10px;
  background: color-mix(in srgb, var(--bg-secondary) 72%, var(--bg-elevated));
  border: 1px solid var(--resource-line);
  border-radius: 10px;
  color: var(--resource-ink);
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out), background var(--motion-duration-quick) var(--motion-ease-out), transform var(--motion-duration-quick) var(--motion-ease-out);
  position: relative;
}

.app-card:hover {
  border-color: var(--resource-line-strong);
  background: var(--bg-elevated);
  transform: translateY(-1px);
}

.app-card.is-hidden {
  opacity: 0.6;
}

/* 快捷访问按钮 */
.app-quick-actions {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 5px;
  width: 100%;
  margin-top: 1px;
}

.quick-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  min-height: 25px;
  padding: 0 8px;
  background: var(--bg-elevated);
  border: 1px solid var(--resource-line);
  border-radius: 6px;
  color: var(--resource-muted);
  font-size: 0.66rem;
  white-space: nowrap;
  cursor: pointer;
  transition: all var(--motion-duration-quick);
}

.app-quick-actions .quick-btn:only-child {
  grid-column: 1 / -1;
}

.quick-btn:hover {
  background: var(--color-primary-50);
  border-color: var(--color-primary-200);
  color: var(--color-primary-600);
}

.app-card:hover .quick-btn {
  opacity: 1;
}

.icon-fallback {
  width: 32px;
  height: 32px;
  color: var(--color-primary-500);
  flex-shrink: 0;
}

.app-icon {
  width: 42px;
  height: 42px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-elevated);
  border-radius: 10px;
  color: var(--color-primary-500);
  cursor: pointer;
  transition: transform var(--motion-duration-quick);
}

.app-icon:hover {
  transform: scale(1.05);
}

.app-icon img {
  width: 32px;
  height: 32px;
  object-fit: contain;
}

.app-info {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  cursor: pointer;
  width: 100%;
}

.app-name {
  color: var(--resource-ink);
  font-size: 0.8rem;
  font-weight: 700;
  text-align: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}

.app-tags {
  min-height: 18px;
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
  justify-content: center;
}

.tag {
  padding: 2px 5px;
  border-radius: 5px;
  font-size: 0.6rem;
  font-weight: 700;
  text-transform: uppercase;
}

.tag.auto {
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

.tag.ai {
  background: var(--color-success-100);
  color: var(--color-success-700);
}

.tag.hidden-status {
  background: var(--color-warning-100);
  color: var(--color-warning-700);
}

.app-actions {
  position: absolute;
  top: 7px;
  right: 7px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  opacity: 0;
  transition: opacity var(--motion-duration-quick);
  z-index: 1;
}

.app-card:hover .app-actions {
  opacity: 1;
}

.action-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border-radius: 6px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick);
}

.action-btn:hover {
  background: var(--color-primary-100);
  border-color: var(--color-primary-200);
  color: var(--color-primary-600);
}

.action-btn.restore:hover {
  background: var(--color-success-100);
  border-color: var(--color-success-200);
  color: var(--color-success-600);
}

.action-btn.danger:hover {
  background: var(--color-danger-100);
  border-color: var(--color-danger-200);
  color: var(--color-danger-600);
}

.action-btn svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.action-btn.is-loading {
  cursor: not-allowed;
  opacity: 0.8;
  color: var(--color-primary-600);
  border-color: var(--color-primary-200);
}

.action-btn.is-loading svg {
  animation: spin 1s linear infinite;
}

/* 表单 */
.form-group {
  margin-bottom: 16px;
}

.form-label {
  display: block;
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--text-primary);
  margin-bottom: 6px;
}

.form-input,
.form-select {
  width: 100%;
  height: 40px;
  padding: 0 12px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--bg-elevated);
  color: var(--text-primary);
  font-size: 0.875rem;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.form-input:focus,
.form-select:focus {
  outline: none;
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.mt-2 {
  margin-top: 8px;
}

.form-hint {
  font-size: 0.75rem;
  color: var(--text-tertiary);
  margin: 6px 0 0;
}

.icon-input-group {
  display: flex;
  align-items: center;
  gap: 10px;
}

.icon-preview {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-secondary);
  border-radius: 8px;
}

.icon-preview img {
  width: 24px;
  height: 24px;
  object-fit: contain;
}

.icon-tools {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 8px;
  flex-wrap: wrap;
}

.upload-input {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

/* 按钮 */
.btn {
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.btn-default {
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  color: var(--text-primary);
}

.btn-default:hover {
  background: var(--bg-tertiary);
}

.btn-primary {
  background: linear-gradient(135deg, var(--color-primary-500), var(--color-primary-600));
  border: none;
  color: var(--text-inverse);
}

.btn-primary:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px var(--color-primary-500-30);
}

.btn-primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
  transform: none;
}

.btn-danger {
  background: var(--color-danger-600);
  border: none;
  color: var(--text-inverse);
}

.btn-danger:hover {
  background: var(--color-danger-700);
}

.btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
  transform: none;
}

.btn-spinner {
  display: inline-block;
  width: 16px;
  height: 16px;
  border: 2px solid color-mix(in srgb, var(--text-inverse) 30%, transparent);
  border-top-color: var(--text-inverse);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  margin-right: 6px;
  vertical-align: middle;
}

/* 分类管理 */
.category-list-manage {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.add-category-row {
  display: flex;
  gap: 8px;
  padding: 4px 0 12px;
  border-bottom: 1px solid var(--border-subtle);
  margin-bottom: 4px;
}

.add-category-row .form-input {
  flex: 1;
}

.add-category-row .btn-sm {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 12px;
  height: 36px;
}

.category-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  background: var(--bg-secondary);
  border-radius: 8px;
}

.category-item-name {
  font-size: 0.875rem;
  color: var(--text-primary);
}

.category-item-actions {
  display: flex;
  gap: 6px;
}

.action-btn-sm {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 6px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick);
}

.action-btn-sm:hover {
  background: var(--color-primary-100);
  border-color: var(--color-primary-200);
  color: var(--color-primary-600);
}

.action-btn-sm.danger:hover {
  background: var(--color-danger-100);
  border-color: var(--color-danger-200);
  color: var(--color-danger-600);
}

.action-btn-sm svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.confirm-message {
  margin: 0;
  color: var(--text-secondary);
  line-height: 1.6;
}

@media (max-width: 900px) {
  .navigation-page {
    height: auto;
    min-height: 100%;
    overflow: visible;
  }

  .toolbar-left,
  .toolbar-right {
    width: 100%;
    flex-wrap: wrap;
  }

  .toolbar-right {
    justify-content: flex-end;
  }

  .navigation-content {
    overflow: visible;
  }
}
</style>
