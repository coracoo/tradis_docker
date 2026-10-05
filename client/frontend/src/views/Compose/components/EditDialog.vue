<template>
  <Modal
    v-model:visible="localVisible"
    :title="editMode ? '编辑项目' : '新增项目'"
    width="1100px"
    :show-close="!saving && !deploying"
    :close-on-esc="!saving && !deploying"
    @close="handleClose"
  >
    <template #footer>
      <div class="footer-left">
        <label class="checkbox-label">
          <input v-model="pullLatest" type="checkbox" />
          <span>拉取最新镜像</span>
        </label>
        <label class="checkbox-label">
          <input v-model="rebuildImage" type="checkbox" />
          <span>重构镜像</span>
        </label>
      </div>
      <div class="footer-right">
        <button v-ripple class="action-btn secondary" @click="handleClose">取消</button>
        <button v-if="!editMode && deploying" v-ripple class="action-btn" :disabled="!currentDeployTaskId" @click="handleBackground">后台运行</button>
        <button v-if="deploying" v-ripple class="action-btn danger" :disabled="!currentDeployTaskId" @click="handleAbort">中止</button>
        <button v-if="composeAIEnabled" v-ripple class="action-btn" @click="handleOpenAi">AI生成与模板</button>
        <template v-if="editMode">
          <button v-ripple class="action-btn" :disabled="saving || deploying || !validProjectName" @click="handleSaveOnly">
            {{ saving ? '处理中...' : '仅保存' }}
          </button>
          <button v-ripple class="action-btn primary" :disabled="saving || deploying || !validProjectName" @click="handleSaveAndDeploy">
            {{ deploying ? '部署中...' : '保存并部署' }}
          </button>
        </template>
        <button v-else v-ripple class="action-btn primary" :disabled="saving || deploying || !validProjectName" @click="handleDeploy">
          {{ deploying ? '部署中...' : '立即部署' }}
        </button>
      </div>
    </template>
    
    <div class="dialog-body with-logs">
      <div class="form-section">
        <div class="edit-form">
          <div class="form-group">
            <label class="form-label">项目名称 <span class="required">*</span></label>
            <input 
              v-model="form.name" 
              type="text" 
              class="form-input"
              :class="{ 'is-invalid': projectNameError }"
              placeholder="请输入项目名称"
              :disabled="editMode"
              :aria-invalid="projectNameError ? 'true' : 'false'"
            />
            <p v-if="projectNameError" class="project-name-error" role="alert">
              {{ projectNameError }}
            </p>
          </div>
          <div class="form-group">
            <label class="form-label">
              宿主机存放路径
              <span
                class="path-hint"
                title="根据 Docker 宿主机配置的 PROJECT_ROOT 自动生成，不是 Client 容器内路径"
                aria-label="路径根据 Docker 宿主机配置的 PROJECT_ROOT 自动生成，不是 Client 容器内路径"
                tabindex="0"
              >
                <DynamicIcon name="info" :size="14" />
              </span>
            </label>
            <input 
              v-model="form.path" 
              type="text" 
              class="form-input" 
              placeholder="根据项目名称自动生成"
              :disabled="editMode"
            />
          </div>
          <div class="editor-heading">
            <label class="form-label" for="compose-yaml-editor">
              docker-compose.yml <span class="required">*</span>
            </label>
            <button
              type="button"
              class="env-panel-toggle"
              :class="{ active: envExpanded }"
              :aria-expanded="envExpanded"
              @click="envExpanded = !envExpanded"
            >
              <DynamicIcon name="file-code" :size="14" />
              <span>.env</span>
            </button>
          </div>
          <div class="editor-workspace" :class="{ 'env-open': envExpanded }">
            <CodeEditor
              id="compose-yaml-editor"
              v-model="form.yaml"
              language="yaml"
              aria-label="docker-compose.yml 编辑器"
              height="430px"
            />
            <aside v-if="envExpanded" class="env-panel">
              <header class="env-panel__header">
                <span>.env 环境变量</span>
                <button type="button" title="收起 .env" aria-label="收起 .env" @click="envExpanded = false">
                  <DynamicIcon name="close" :size="14" />
                </button>
              </header>
              <CodeEditor
                v-model="form.env"
                language="ini"
                aria-label=".env 编辑器"
                height="380px"
              />
            </aside>
          </div>
        </div>
      </div>
      
      <!-- 部署日志区域 -->
      <div class="logs-section ops-console">
        <div class="logs-header ops-console__header">
          <DynamicIcon name="file-text" :size="14" />
          部署日志
        </div>
        <div ref="logsContainer" class="logs-content ops-console__body">
          <div v-if="logs.length === 0" class="logs-empty">
            暂无部署日志，点击"立即部署"后将在此展示实时输出。
          </div>
          <div
            v-for="(log, index) in logs"
            :key="index"
            :class="['log-line', log.type]"
          >
            {{ log.message }}
          </div>
        </div>
      </div>
    </div>
  </Modal>

  <Modal
    v-model:visible="previewVisible"
    title="确认配置变更"
    width="820px"
    :show-close="!previewApplying"
    :close-on-esc="!previewApplying"
    @close="cancelConfigPreview"
  >
    <div class="preview-heading">
      <div>
        <span>Compose 项目</span>
        <strong>{{ form.name }}</strong>
      </div>
      <p>确认后先写入配置；选择保存并部署时将继续原有部署任务。</p>
    </div>
    <div class="preview-scroll">
      <ComposeChangePreview :preview="configPreview" />
    </div>
    <template #footer>
      <button v-ripple class="action-btn secondary" type="button" :disabled="previewApplying" @click="cancelConfigPreview">
        取消
      </button>
      <button v-ripple class="action-btn primary" type="button" :disabled="previewApplying" @click="confirmConfigChanges">
        {{ previewApplying ? '保存中...' : pendingAction === 'deploy' ? '确认保存并部署' : '确认保存' }}
      </button>
    </template>
  </Modal>
</template>

<script setup>
import { ref, watch, nextTick, computed, onBeforeUnmount } from 'vue'
import Modal from '@/components/feedback/Modal.vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import CodeEditor from '@/components/ui/CodeEditor.vue'
import ComposeChangePreview from './ComposeChangePreview.vue'
import { compose } from '@edition/api'
import { useUiStore } from '@/stores/ui.js'
import { deploymentPreflightConfirmation } from '@/utils/deploymentPreflight.js'
import { composeAIEnabled } from '@edition/compose-features'

const props = defineProps({
  visible: { type: Boolean, default: false },
  isEdit: { type: Boolean, default: false },
  initialData: { type: Object, default: () => ({}) },
  projectRoot: { type: String, default: '' },
  existingNames: { type: Array, default: () => [] },
  remoteDeployment: { type: Boolean, default: false }
})

const emit = defineEmits([
  'update:visible', 
  'saved', 
  'deployed', 
  'open-ai',
  'background',
  'task-started'
])
const uiStore = useUiStore()

// 首次部署失败后，后端已创建项目目录；此时把对话框切到“编辑该项目”语义，
// 后续保存/部署走编辑路径（saveYaml → updateTask），不再撞 deployTask 的“项目已存在”。
const editMode = ref(props.isEdit)

// 本地visible状态，用于v-model
const localVisible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val)
})

// 表单状态
const form = ref({
  name: '',
  path: '',
  yaml: '',
  env: ''
})

// 操作状态
const saving = ref(false)
const deploying = ref(false)
const pullLatest = ref(localStorage.getItem('compose_pull_latest_image') !== 'false')
const rebuildImage = ref(localStorage.getItem('compose_rebuild_image') === 'true')
const logs = ref([])
const logsContainer = ref(null)
const currentDeployTaskId = ref('')
const previewVisible = ref(false)
const previewApplying = ref(false)
const pendingAction = ref('save')
const configPreview = ref(null)
const envExpanded = ref(false)
let eventSource = null
let deploymentLogTimeout = null
const composeProjectNamePattern = /^[a-z0-9][a-z0-9_-]*$/

const validProjectName = computed(() => normalizeProjectName(form.value.name))
const projectNameError = computed(() => {
  const raw = String(form.value.name || '').trim()
  if (!raw || validProjectName.value) return ''
  return '项目名仅支持小写字母、数字、_ 和 -，并且必须以字母或数字开头'
})

// 监听pullLatest变化
watch(pullLatest, (newVal) => {
  localStorage.setItem('compose_pull_latest_image', newVal)
})

watch(rebuildImage, (newVal) => {
  localStorage.setItem('compose_rebuild_image', newVal)
})

// 监听visible变化
watch(() => props.visible, (newVal) => {
  if (newVal) {
    editMode.value = props.isEdit
    if (props.isEdit && props.initialData) {
      form.value = {
        name: props.initialData.name || '',
        path: props.initialData.path || '',
        yaml: props.initialData.yaml || '',
        env: props.initialData.env || ''
      }
    } else {
      form.value = { name: '', path: '', yaml: '', env: '' }
    }
    logs.value = []
    saving.value = false
    deploying.value = false
    currentDeployTaskId.value = ''
    previewVisible.value = false
    previewApplying.value = false
    pendingAction.value = 'save'
    configPreview.value = null
    envExpanded.value = false
  }
})

// 宿主机根目录可能在弹窗打开后异步返回，变化时必须同步刷新展示路径。
watch([() => form.value.name, () => props.projectRoot], ([newName, projectRoot]) => {
  if (editMode.value) return
  const basePath = String(projectRoot || '${PROJECT_ROOT}').trim().replace(/\/+$/, '')
  const normalized = normalizeProjectName(newName)
  form.value.path = normalized ? `${basePath}/${normalized}` : ''
})

// 组件卸载时关闭 SSE 连接，避免 EventSource 和回调闭包泄漏
// （visible watcher 只在打开时重置状态，不会在隐藏/卸载时关闭 eventSource）
onBeforeUnmount(() => {
  cleanup()
})

function normalizeProjectName(name) {
  const normalized = String(name || '').trim().toLowerCase()
  return composeProjectNamePattern.test(normalized) ? normalized : ''
}

function validateProjectForm({ checkDuplicate = false } = {}) {
  const rawName = String(form.value.name || '').trim()
  if (!rawName || !form.value.yaml.trim()) {
    uiStore.toastWarning('请填写项目名称和 YAML 配置')
    return ''
  }
  const normalizedName = normalizeProjectName(form.value.name)
  if (!normalizedName) {
    uiStore.toastWarning('项目名不合法：仅支持小写字母/数字，可包含 _ -，并以字母或数字开头')
    return ''
  }
  if (normalizedName !== rawName) {
    form.value.name = normalizedName
  }
  if (checkDuplicate && props.existingNames.some(name => String(name).toLowerCase() === normalizedName)) {
    uiStore.toastWarning(`项目 "${normalizedName}" 已存在`)
    return ''
  }
  if (!form.value.yaml.includes('services:')) {
    uiStore.toastWarning('YAML格式错误：缺少services定义')
    return ''
  }
  return normalizedName
}

function handleClose() {
  cleanup()
  previewVisible.value = false
  configPreview.value = null
  emit('update:visible', false)
}

function handleOpenAi() {
  emit('open-ai', { yaml: form.value.yaml, env: form.value.env })
}

function applyAiDraft(payload = {}) {
  if (Object.prototype.hasOwnProperty.call(payload, 'yaml')) {
    form.value.yaml = String(payload.yaml || '')
  }
  if (Object.prototype.hasOwnProperty.call(payload, 'env')) {
    form.value.env = String(payload.env || '')
  }
}

function handleBackground() {
  emit('background', {
    name: normalizeProjectName(form.value.name),
    taskId: currentDeployTaskId.value
  })
  handleClose()
}

async function handleAbort() {
  const taskId = currentDeployTaskId.value
  if (!taskId) return
  try {
    await compose.cancelTask(taskId)
    uiStore.toastInfo('已请求中止部署')
  } catch (error) {
    uiStore.toastError('中止失败: ' + (error.message || '未知错误'))
  }
}

function cleanup() {
  clearDeploymentLogTimeout()
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
  deploying.value = false
  saving.value = false
  currentDeployTaskId.value = ''
}

function clearDeploymentLogTimeout() {
  if (deploymentLogTimeout !== null) {
    clearTimeout(deploymentLogTimeout)
    deploymentLogTimeout = null
  }
}

function connectTaskLogs(taskId, successMessage) {
  return new Promise((resolve, reject) => {
    const url = compose.getTaskEventsUrl(taskId)

    if (eventSource) {
      eventSource.close()
    }

    eventSource = new EventSource(url)
    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data || '{}')
        const type = String(data.type || 'info').toLowerCase()
        const message = String(data.message || '')

        if (type === 'result') {
          const status = String(data.status || '').toLowerCase()
          eventSource.close()
          eventSource = null
          if (status === 'success' || status === 'completed') {
            logs.value.push({ type: 'success', message: successMessage })
            resolve()
          } else if (status === 'canceled') {
            logs.value.push({ type: 'warning', message: '部署已中止' })
            const err = new Error(data.error || '部署已中止')
            err.canceled = true
            reject(err)
          } else {
            reject(new Error(data.error || '任务失败'))
          }
          return
        }

        if (message) {
          logs.value.push({ type, message })
        }
        nextTick(() => {
          if (logsContainer.value) {
            logsContainer.value.scrollTop = logsContainer.value.scrollHeight
          }
        })
      } catch {
        logs.value.push({ type: 'info', message: String(event.data || '') })
      }
    }
    eventSource.onerror = () => {
      eventSource?.close()
      eventSource = null
      reject(new Error('任务日志连接失败'))
    }
  })
}

async function requestConfigPreview(action) {
  const projectName = validateProjectForm()
  if (!projectName) return

  saving.value = true
  try {
    const preview = await compose.previewConfig(projectName, {
      yaml: form.value.yaml,
      env: String(form.value.env || '')
    })
    if (!preview?.hasChanges) {
      uiStore.toastInfo('配置未发生变化')
      return
    }
    pendingAction.value = action
    configPreview.value = preview
    previewVisible.value = true
  } catch (error) {
    uiStore.toastError('预览失败: ' + (error.message || '未知错误'))
  } finally {
    saving.value = false
  }
}

function handleSaveOnly() {
  return requestConfigPreview('save')
}

function handleSaveAndDeploy() {
  return requestConfigPreview('deploy')
}

function cancelConfigPreview() {
  if (previewApplying.value) return
  previewVisible.value = false
  configPreview.value = null
}

async function confirmConfigChanges() {
  const projectName = validateProjectForm()
  if (!projectName || !configPreview.value?.baseHash) return

  const action = pendingAction.value
  previewApplying.value = true
  saving.value = true
  try {
    const result = await compose.applyConfig(projectName, {
      yaml: form.value.yaml,
      env: String(form.value.env || ''),
      baseHash: configPreview.value.baseHash
    })
    previewVisible.value = false
    configPreview.value = null
    if (!result?.changed) {
      uiStore.toastInfo(result?.message || '配置未发生变化')
      return
    }
    if (action === 'save') {
      emit('saved')
      handleClose()
      return
    }
  } catch (error) {
    uiStore.toastError('保存失败: ' + (error.message || '未知错误'))
    previewVisible.value = false
    configPreview.value = null
    return
  } finally {
    saving.value = false
    previewApplying.value = false
  }

  await deployEditedProject(projectName)
}

async function deployEditedProject(projectName) {
  deploying.value = true
  logs.value = []

  try {
    const res = pullLatest.value || rebuildImage.value
      ? await compose.updateTask(projectName, { pull: pullLatest.value, rebuild: rebuildImage.value })
      : await compose.startTask(projectName)
    const taskId = String(res?.taskId || '')
    if (!taskId) throw new Error('未获取到任务ID')
    currentDeployTaskId.value = taskId
    emit('task-started', { name: projectName, taskId })
    await connectTaskLogs(taskId, `项目 ${projectName} 已重新部署，新配置已生效`)

    setTimeout(() => {
      emit('deployed')
      handleClose()
    }, 1500)
  } catch (error) {
    if (error?.canceled) {
      uiStore.toastInfo('部署已中止')
    } else {
      uiStore.toastError('部署失败: ' + (error.message || '未知错误'))
    }
  } finally {
    deploying.value = false
  }
}

// 部署（新建模式）
async function handleDeploy() {
  const projectName = validateProjectForm({ checkDuplicate: true })
  if (!projectName) return
  
  // 修复常见错误
  if (form.value.yaml.includes('/binsh')) {
    logs.value.push({
      type: 'warning',
      message: '警告：检测到可能的路径错误，"/binsh" 应该为 "/bin/sh"'
    })
    form.value.yaml = form.value.yaml.replace(/\/binsh\b/g, '/bin/sh')
  }
  
  deploying.value = true
  logs.value = []
  
  if (eventSource) {
    eventSource.close()
  }
  clearDeploymentLogTimeout()
  
  try {
    const request = {
      name: projectName,
      compose: form.value.yaml,
      dotenv: String(form.value.env || ''),
      env: '',
      autoStart: true,
      pull: pullLatest.value,
      rebuild: rebuildImage.value
    }
    let res
    if (props.remoteDeployment) {
      // The target Agent resolves its own project path and conflict state. A
      // replacement remains an explicit browser confirmation before apply.
      const preflight = await compose.preflightDeployment({ ...request, replace: true })
      if (preflight?.requires_confirmation && !window.confirm(deploymentPreflightConfirmation(preflight))) {
        deploying.value = false
        return
      }
      res = await compose.deployTask({
        ...request,
        replace: true,
        preflightToken: preflight?.preflight_token,
        idempotencyKey: preflight?.idempotency_key
      })
    } else {
      res = await compose.deployTask(request)
    }
    
    const taskId = res?.taskId
    if (!taskId) {
      deploying.value = false
      uiStore.toastError('未获取到任务ID')
      return
    }
    currentDeployTaskId.value = String(taskId)
    emit('task-started', {
      name: projectName,
      taskId: currentDeployTaskId.value
    })
    
    // 连接 SSE 获取部署日志（通过 Cookie 认证）
    const url = compose.getTaskEventsUrl(taskId)

    eventSource = new EventSource(url)
    
    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data)
        const type = String(data.type || 'info').toLowerCase()
        const message = String(data.message || '')
        
        logs.value.push({ type, message })
        
        nextTick(() => {
          if (logsContainer.value) {
            logsContainer.value.scrollTop = logsContainer.value.scrollHeight
          }
        })
        
        if (type === 'result') {
          const status = String(data.status || '').toLowerCase()
          if (status === 'success' || status === 'completed') {
            deploying.value = false
            clearDeploymentLogTimeout()
            eventSource.close()
            setTimeout(() => {
              emit('deployed')
              handleClose()
            }, 1000)
          } else if (status === 'error' || status === 'failed') {
            deploying.value = false
            clearDeploymentLogTimeout()
            eventSource.close()
            editMode.value = true
            uiStore.toastInfo('项目已创建，已切换为编辑模式，修改后点“保存并部署”重试')
          } else if (status === 'canceled') {
            deploying.value = false
            clearDeploymentLogTimeout()
            eventSource.close()
            uiStore.toastInfo('部署已中止')
          }
        }
      } catch (e) {
        logs.value.push({ type: 'info', message: String(event.data) })
      }
    }
    
    eventSource.onerror = () => {
      deploying.value = false
      clearDeploymentLogTimeout()
      eventSource.close()
    }
    
    // 超时处理
    const activeEventSource = eventSource
    deploymentLogTimeout = setTimeout(() => {
      deploymentLogTimeout = null
      if (eventSource !== activeEventSource) return
      if (deploying.value) {
        logs.value.push({
          type: 'warning',
          message: '日志连接超时，可稍后在进度查询中查看任务状态'
        })
        deploying.value = false
        eventSource.close()
        eventSource = null
      }
    }, 600000)
    
  } catch (error) {
    deploying.value = false
    logs.value.push({
      type: 'error',
      message: '部署失败: ' + (error.message || '未知错误')
    })
  }
}

// 暴露方法给父组件
defineExpose({
  cleanup,
  applyAiDraft,
  getFormData: () => ({ ...form.value }),
  setFormData: (data) => {
    form.value = { ...form.value, ...data }
  }
})
</script>

<style scoped>
.dialog-body {
  display: flex;
  gap: 16px;
}

.dialog-body.with-logs {
  display: grid;
  grid-template-columns: 2fr 1fr;
}

.form-section {
  min-width: 0;
}

.logs-section {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  overflow: hidden;
  background: var(--bg-primary);
  min-height: 400px;
}

.logs-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  background: color-mix(in srgb, var(--bg-secondary) 76%, var(--bg-elevated));
  border-bottom: 1px solid var(--border-subtle);
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--text-secondary);
}

.logs-content {
  flex: 1;
  padding: 12px;
  overflow-y: auto;
  max-height: 500px;
}

.logs-empty {
  color: var(--text-tertiary);
  text-align: center;
  padding: 40px 20px;
}

.log-line {
  margin-bottom: 2px;
  word-break: break-all;
}

.log-line.error {
  color: var(--color-danger-500);
}

.log-line.warning {
  color: var(--color-warning-500);
}

.log-line.success {
  color: var(--color-success-500);
}

.edit-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--text-primary);
}

.form-label .required {
  color: var(--color-danger-500);
}

.path-hint {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 0.75rem;
  font-weight: normal;
  color: var(--text-tertiary);
}

.path-hint svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
}

.editor-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.editor-workspace {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
  min-width: 0;
}

.editor-workspace.env-open {
  grid-template-columns: minmax(0, 1fr) minmax(240px, 34%);
}

.env-panel-toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 30px;
  padding: 0 10px;
  border: 1px solid var(--border-default);
  border-radius: 7px;
  background: var(--bg-elevated);
  color: var(--text-secondary);
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 0.75rem;
  cursor: pointer;
}

.env-panel-toggle:hover,
.env-panel-toggle.active {
  border-color: var(--color-primary-500);
  color: var(--color-primary-600);
  background: var(--color-primary-50);
}

.env-panel {
  min-width: 0;
  padding: 8px;
  border: 1px solid var(--border-subtle);
  border-radius: 9px;
  background: var(--bg-secondary);
}

.env-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 32px;
  padding: 0 2px 8px 4px;
  color: var(--text-secondary);
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 0.75rem;
  font-weight: 600;
}

.env-panel__header button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--text-tertiary);
  cursor: pointer;
}

.env-panel__header button:hover {
  background: var(--bg-tertiary);
  color: var(--text-primary);
}

.form-input,
.form-textarea {
  width: 100%;
  padding: 10px 12px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--bg-primary);
  color: var(--text-primary);
  font-size: 0.875rem;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  font-family: inherit;
}

.form-input:focus,
.form-textarea:focus {
  outline: none;
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.form-input.is-invalid,
.form-input.is-invalid:focus {
  border-color: var(--color-danger-500);
  box-shadow: 0 0 0 3px var(--color-danger-ring);
}

.project-name-error {
  margin: 0;
  color: var(--color-danger-500);
  font-size: 0.75rem;
  line-height: 1.4;
}

.form-input:disabled {
  background: var(--bg-tertiary);
  color: var(--text-tertiary);
  cursor: not-allowed;
}

.form-textarea {
  resize: vertical;
  min-height: 100px;
}

.form-textarea.code {
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 0.8125rem;
  line-height: 1.5;
}

/* Footer 布局 */
.footer-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.footer-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  font-size: 0.875rem;
  color: var(--text-secondary);
  user-select: none;
}

.checkbox-label input[type="checkbox"] {
  width: 16px;
  height: 16px;
  cursor: pointer;
  accent-color: var(--color-primary-500);
}

.checkbox-label:hover {
  color: var(--text-primary);
}

.action-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 38px;
  padding: 0 14px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: 8px;
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.action-btn:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

.action-btn.primary {
  background: var(--color-primary-500);
  border-color: var(--color-primary-600);
  color: var(--text-inverse);
}

.action-btn.primary:hover {
  background: var(--color-primary-600);
}

.action-btn.secondary {
  background: var(--bg-elevated);
  color: var(--text-secondary);
}

.action-btn.secondary:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

.action-btn.danger {
  border-color: #e3c3b3;
  color: #b0705c;
}

.action-btn.danger:hover {
  background: #f6e8e0;
  border-color: #ddbbaa;
  color: #9c5f4c;
}

.action-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.preview-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  padding: 0 2px 14px;
}

.preview-heading > div {
  min-width: 0;
  display: flex;
  align-items: baseline;
  gap: 10px;
}

.preview-heading span,
.preview-heading p {
  color: var(--text-tertiary);
  font-size: 0.75rem;
}

.preview-heading strong {
  overflow: hidden;
  color: var(--text-primary);
  font: 600 0.875rem/1.4 var(--font-mono, ui-monospace, monospace);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.preview-heading p {
  max-width: 390px;
  margin: 0;
  line-height: 1.5;
  text-align: right;
}

.preview-scroll {
  max-height: min(58vh, 560px);
  overflow-y: auto;
  scrollbar-color: var(--border-default) transparent;
  scrollbar-width: thin;
}

@media (max-width: 900px) {
  .dialog-body.with-logs {
    grid-template-columns: 1fr;
  }

  .logs-section {
    min-height: 260px;
  }

  .editor-workspace.env-open {
    grid-template-columns: 1fr;
  }

  .preview-heading {
    flex-direction: column;
    gap: 6px;
  }

  .preview-heading p {
    max-width: none;
    text-align: left;
  }
}
</style>
