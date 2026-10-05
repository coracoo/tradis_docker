<template>
  <Modal
    v-model:visible="localVisible"
    title="从 GitHub 导入"
    width="560px"
    :show-close="!submitting"
    :close-on-esc="!submitting"
    @close="handleClose"
  >
    <div class="git-import-form">
      <div class="form-group">
        <label class="form-label">GitHub 仓库地址 <span class="required">*</span></label>
        <input
          v-model.trim="form.repoUrl"
          class="form-input"
          placeholder="https://github.com/owner/repository"
          @blur="fillProjectName"
        />
        <p class="form-hint">目前仅支持无需认证的公开 GitHub HTTPS 仓库；支持根目录、一级 docker/、deploy/ 及最多二级目录中的标准 Compose 文件。</p>
      </div>

      <div class="form-row">
        <div class="form-group">
          <label class="form-label">项目名称 <span class="required">*</span></label>
          <input v-model.trim="form.name" class="form-input" placeholder="repository" />
        </div>
        <div class="form-group">
          <label class="form-label">分支或标签</label>
          <input v-model.trim="form.branch" class="form-input" placeholder="留空使用默认分支" />
        </div>
      </div>

      <div class="form-group">
        <label class="form-label">Git 加速地址</label>
        <input
          v-model.trim="form.acceleratorUrl"
          class="form-input"
          placeholder="例如：https://ghfast.top/"
        />
        <p class="form-hint">可留空。保存后后续同步继续复用；支持普通前缀或包含 <code>{url}</code> 的模板。</p>
      </div>

      <div class="flow-hint">
        仓库只会下载并登记来源，不会立即启动。下载完成后请检查 YAML 和 .env，再点击项目“启动”。
      </div>
    </div>

    <template #footer>
      <button v-ripple class="dialog-btn" :disabled="submitting" @click="handleClose">取消</button>
      <button v-ripple class="dialog-btn primary" :disabled="submitting" @click="handleImport">
        <span v-if="submitting" class="btn-spinner" aria-hidden="true"></span>
        {{ submitting ? '正在提交...' : '下载仓库' }}
      </button>
    </template>
  </Modal>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import Modal from '@/components/feedback/Modal.vue'
import { compose } from '@edition/api'
import { useUiStore } from '@/stores/ui.js'

const props = defineProps({
  visible: { type: Boolean, default: false },
  existingNames: { type: Array, default: () => [] }
})

const emit = defineEmits(['update:visible', 'task-started'])
const uiStore = useUiStore()

const localVisible = computed({
  get: () => props.visible,
  set: value => emit('update:visible', value)
})

const form = reactive({
  repoUrl: '',
  name: '',
  branch: '',
  acceleratorUrl: ''
})
const submitting = ref(false)

watch(() => props.visible, visible => {
  if (!visible) return
  form.repoUrl = ''
  form.name = ''
  form.branch = ''
  form.acceleratorUrl = ''
  submitting.value = false
})

function normalizeProjectName(value) {
  return String(value || '')
    .toLowerCase()
    .replace(/\.git$/i, '')
    .replace(/[^a-z0-9_-]/g, '-')
    .replace(/^-+|-+$/g, '')
}

function fillProjectName() {
  if (form.name || !form.repoUrl) return
  try {
    const url = new URL(form.repoUrl)
    const segments = url.pathname.split('/').filter(Boolean)
    form.name = normalizeProjectName(segments.at(-1))
  } catch {
    // 提交时统一提示地址错误。
  }
}

function validateForm() {
  let url
  try {
    url = new URL(form.repoUrl)
  } catch {
    uiStore.toastWarning('请输入有效的 GitHub HTTPS 地址')
    return ''
  }
  if (url.protocol !== 'https:' || url.hostname.toLowerCase() !== 'github.com') {
    uiStore.toastWarning('目前仅支持 https://github.com/owner/repository 格式')
    return ''
  }

  fillProjectName()
  const name = normalizeProjectName(form.name)
  if (!name || !/^[a-z0-9][a-z0-9_-]*$/.test(name)) {
    uiStore.toastWarning('项目名不合法：仅支持小写字母、数字、_ 和 -')
    return ''
  }
  form.name = name
  return name
}

async function handleImport() {
  if (submitting.value) return
  const name = validateForm()
  if (!name) return

  const overwrite = props.existingNames.some(item => String(item).toLowerCase() === name)
  submitting.value = true
  try {
    if (overwrite) {
      const confirmed = await uiStore.confirm({
        title: '覆盖已有 Git 项目',
        message: `发现重复项目 "${name}"。继续导入会替换项目目录并覆盖其中的同名本地文件，请先做好备份。`,
        type: 'warning',
        confirmText: '继续导入',
        cancelText: '取消'
      })
      if (!confirmed) return
    }
    const result = await compose.importGit({
      repoUrl: form.repoUrl,
      name,
      branch: form.branch,
      acceleratorUrl: form.acceleratorUrl,
      overwrite
    })
    if (!result?.taskId) {
      throw new Error('未获取到任务 ID')
    }
    emit('task-started', { name, taskId: String(result.taskId) })
    emit('update:visible', false)
  } catch (error) {
    uiStore.toastError('Git 导入失败: ' + (error?.message || '未知错误'))
  } finally {
    submitting.value = false
  }
}

function handleClose() {
  if (submitting.value) return
  emit('update:visible', false)
}
</script>

<style scoped>
.git-import-form {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.form-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.form-label {
  color: var(--text-primary);
  font-size: 0.875rem;
  font-weight: 500;
}

.required {
  color: var(--color-danger-500);
}

.form-input {
  width: 100%;
  padding: 9px 11px;
  color: var(--text-primary);
  background: var(--bg-primary);
  border: 1px solid var(--border-default);
  border-radius: 8px;
}

.form-hint {
  margin: 0;
  color: var(--text-tertiary);
  font-size: 0.75rem;
  line-height: 1.5;
}

.flow-hint {
  padding: 10px 12px;
  color: var(--text-secondary);
  background: var(--color-primary-50);
  border: 1px solid var(--color-primary-200);
  border-radius: 8px;
  font-size: 0.8125rem;
  line-height: 1.6;
}

.dialog-btn {
  min-height: 38px;
  padding: 0 14px;
  color: var(--text-secondary);
  background: var(--bg-secondary);
  border: 1px solid var(--border-default);
  border-radius: 8px;
  cursor: pointer;
}

.btn-spinner {
  width: 14px;
  height: 14px;
  border: 2px solid currentColor;
  border-right-color: transparent;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.dialog-btn.primary {
  color: var(--text-inverse);
  background: var(--color-primary-500);
  border-color: var(--color-primary-500);
}

.form-input:focus {
  outline: none;
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.dialog-btn:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

@media (max-width: 640px) {
  .form-row {
    grid-template-columns: 1fr;
  }
}
</style>
