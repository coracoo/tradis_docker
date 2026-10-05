<template>
  <Modal
    v-model:visible="localVisible"
    title="AI生成与模板"
    width="720px"
    @close="handleClose"
  >
    <template #footer>
      <button v-ripple class="action-btn secondary" @click="handleClose">关闭</button>
    </template>
    
    <div class="ai-template-dialog">
      <!-- 模板选择 -->
      <div class="template-section">
        <div class="section-title">
          <DynamicIcon name="file" :size="16" />
          快速模板
        </div>
        <div class="template-grid">
          <button 
            v-for="tpl in templates" 
            :key="tpl.key"
            v-ripple
            class="template-btn"
            @click="handleInsertTemplate(tpl.key)"
          >
            <span class="template-icon">
              <DynamicIcon :name="tpl.icon" :size="20" />
            </span>
            <span class="template-name">{{ tpl.name }}</span>
          </button>
        </div>
      </div>
      
      <div class="divider"></div>
      
      <!-- AI生成 -->
      <div class="ai-section">
        <div class="section-title">
          <DynamicIcon name="sparkles" :size="16" />
          AI 生成 Compose
        </div>
        <div class="ai-form">
          <textarea
            v-model="localPrompt"
            class="ai-textarea"
            rows="6"
            placeholder="例如：部署 n8n + postgres，持久化数据到 ./data，映射 5678 端口，设置 TZ=Asia/Shanghai"
          ></textarea>
          <div class="ai-options">
            <label class="checkbox-label">
              <input v-model="useExistingCompose" type="checkbox" />
              参考当前 Compose
            </label>
            <label class="checkbox-label">
              <input v-model="useExistingEnv" type="checkbox" />
              参考当前 .env
            </label>
          </div>
          <div v-if="warnings.length" class="ai-alerts warning">
            <div class="alert-title">Warnings ({{ warnings.length }})</div>
            <div v-for="(w, i) in warnings" :key="i" class="alert-item">{{ w }}</div>
          </div>
          <div v-if="notes.length" class="ai-alerts info">
            <div class="alert-title">Notes ({{ notes.length }})</div>
            <div v-for="(n, i) in notes" :key="i" class="alert-item">{{ n }}</div>
          </div>
          <button 
            v-ripple
            class="ai-generate-btn" 
            :disabled="generating || !localPrompt.trim()"
            @click="handleGenerate"
          >
            <span v-if="generating" class="spinner"></span>
            <DynamicIcon v-else name="sparkles" :size="16" />
            {{ generating ? '生成中...' : '生成并填充' }}
          </button>
        </div>
      </div>
    </div>
  </Modal>
</template>

<script setup>
import { ref, watch, computed } from 'vue'
import Modal from '@/components/feedback/Modal.vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import { generateCompose } from '@/api/aiFree.js'
import { useUiStore } from '@/stores/ui.js'

const props = defineProps({
  visible: { type: Boolean, default: false },
  currentYaml: { type: String, default: '' },
  currentEnv: { type: String, default: '' }
})

const emit = defineEmits(['update:visible', 'generated', 'insert-template'])
const uiStore = useUiStore()

// 本地visible状态，用于v-model
const localVisible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val)
})

// 本地状态
const localPrompt = ref('')
const useExistingCompose = ref(true)
const useExistingEnv = ref(true)
const generating = ref(false)
const warnings = ref([])
const notes = ref([])

// 模板列表
const templates = [
  { key: 'nginx', name: 'Nginx', icon: 'server' },
  { key: 'mysql', name: 'MySQL', icon: 'database' },
  { key: 'redis', name: 'Redis', icon: 'zap' },
  { key: 'wordpress', name: 'WordPress', icon: 'layout-template' }
]

// 监听visible变化，打开时重置
watch(() => props.visible, (newVal) => {
  if (newVal) {
    localPrompt.value = ''
    warnings.value = []
    notes.value = []
    useExistingCompose.value = true
    useExistingEnv.value = true
  }
})

function handleClose() {
  emit('update:visible', false)
}

function handleInsertTemplate(type) {
  emit('insert-template', type)
  handleClose()
}

async function handleGenerate() {
  const prompt = String(localPrompt.value || '').trim()
  if (!prompt) {
    uiStore.toastWarning('请先输入需求描述')
    return
  }
  
  generating.value = true
  warnings.value = []
  notes.value = []
  
  try {
    const res = await generateCompose({
      prompt,
      existingCompose: useExistingCompose.value ? String(props.currentYaml || '') : '',
      existingDotenv: useExistingEnv.value ? String(props.currentEnv || '') : ''
    })
    
    const data = res || {}
    const yaml = String(data.composeYaml || '').trim()
    const dotenv = String(data.dotenvText || '')
    const notesData = Array.isArray(data.notes) ? data.notes : []
    const warningsData = Array.isArray(data.warnings) ? data.warnings : []
    
    notes.value = notesData.map(v => String(v || '').trim()).filter(Boolean)
    warnings.value = warningsData.map(v => String(v || '').trim()).filter(Boolean)
    
    if (!yaml) {
      uiStore.toastWarning('AI 未返回 composeYaml')
      return
    }
    
    const resultYaml = yaml.endsWith('\n') ? yaml : `${yaml}\n`
    const resultEnv = dotenv.trim() !== '' 
      ? (dotenv.endsWith('\n') ? dotenv : `${dotenv}\n`)
      : ''
    
    emit('generated', { yaml: resultYaml, env: resultEnv })
    handleClose()
  } catch (error) {
    uiStore.toastError('生成失败: ' + (error.message || '未知错误'))
  } finally {
    generating.value = false
  }
}
</script>

<style scoped>
.ai-template-dialog {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 0.9375rem;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 12px;
}

.section-title svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
}

.template-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
}

.template-btn {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 16px 12px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.template-btn:hover {
  border-color: var(--color-primary-300);
  background: var(--bg-secondary);
}

.template-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border-radius: 9px;
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

.template-name {
  font-size: 0.8125rem;
  color: var(--text-secondary);
}

.divider {
  height: 1px;
  background: var(--border-subtle);
}

.ai-form {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.ai-textarea {
  width: 100%;
  padding: 12px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--bg-elevated);
  color: var(--text-primary);
  font-size: 0.875rem;
  font-family: inherit;
  resize: vertical;
  min-height: 100px;
}

.ai-textarea:focus {
  outline: none;
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.ai-options {
  display: flex;
  gap: 20px;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 0.8125rem;
  color: var(--text-secondary);
  cursor: pointer;
}

.checkbox-label input[type="checkbox"] {
  width: 16px;
  height: 16px;
  cursor: pointer;
}

.ai-alerts {
  padding: 12px;
  border-radius: 8px;
  font-size: 0.8125rem;
}

.ai-alerts.warning {
  background: var(--color-warning-100);
  color: var(--color-warning-700);
}

.ai-alerts.info {
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

.alert-title {
  font-weight: 600;
  margin-bottom: 6px;
}

.alert-item {
  padding: 2px 0;
  padding-left: 12px;
  position: relative;
}

.alert-item::before {
  content: '•';
  position: absolute;
  left: 0;
}

.ai-generate-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 38px;
  padding: 0 16px;
  background: var(--color-primary-500);
  color: var(--text-inverse);
  border: none;
  border-radius: 8px;
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  align-self: flex-start;
}

.ai-generate-btn:hover:not(:disabled) {
  background: var(--color-primary-600);
}

.ai-generate-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid var(--white-30);
  border-top-color: var(--text-inverse);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
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

.action-btn.secondary {
  background: var(--bg-tertiary);
  color: var(--text-secondary);
}

.action-btn.secondary:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
}
</style>
