<template>
  <ResourceWorkbench class="app-deploy-workbench">
    <template #toolbar>
      <div class="toolbar workbench-toolbar deploy-toolbar">
        <div class="toolbar-left workbench-toolbar-left">
          <button class="back-btn" @click="goBack">
            <DynamicIcon name="arrow-left" :size="18" />
            返回应用商店
          </button>
          <h1 class="page-title">
            <span v-if="app.name">部署 {{ app.name }}</span>
            <span v-else>部署应用</span>
          </h1>
        </div>
        <div v-if="(deploying || deployLogs.length > 0) && !showDeployModal" class="toolbar-right workbench-toolbar-right">
          <button class="btn secondary small" type="button" @click="showDeployModal = true">
            <DynamicIcon name="activity" :size="14" />
            {{ deploying ? '查看部署进度' : '查看部署结果' }}
          </button>
        </div>
      </div>
    </template>

    <div class="app-deploy-page">

    <!-- 加载状态 -->
    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <p>加载应用配置...</p>
    </div>

    <!-- 错误状态 -->
    <div v-else-if="error" class="error-state">
      <DynamicIcon name="alert-circle" :size="48" />
      <h3>加载失败</h3>
      <p>{{ error }}</p>
      <button class="btn primary" @click="fetchAppData">重试</button>
    </div>

    <!-- 部署表单 -->
    <div v-else class="deploy-layout">
      <div class="left-panel">
        <div class="info-card">
          <DynamicAppIcon 
            :name="app.name" 
            :src="app.logo" 
            size="xl" 
            rounded="12px"
          />
          <div class="app-info">
            <h3>{{ app.name }}</h3>
            <p class="app-desc">{{ app.description || '暂无描述' }}</p>
            <div class="app-tags">
              <span class="tag category">{{ getCategoryCN(app.category) }}</span>
              <span class="tag version">{{ app.version || 'latest' }}</span>
            </div>
          </div>
        </div>

        <div v-if="warnings.length > 0" class="warnings-box">
          <div class="warning-title">
            <DynamicIcon name="alert-triangle" :size="14" />
            注意事项
          </div>
          <ul>
            <li v-for="(w, idx) in warnings" :key="idx">{{ w }}</li>
          </ul>
        </div>

        <div v-if="parseErrors.length > 0" class="warnings-box error">
          <div class="warning-title">
            <DynamicIcon name="alert-circle" :size="14" />
            Compose 解析错误
          </div>
          <ul>
            <li v-for="(e, idx) in parseErrors" :key="idx">{{ e }}</li>
          </ul>
        </div>

        <div v-if="advancedModeEnabled" class="section-card quick-entry-card active">
          <button class="quick-entry-btn" type="button" @click="openYamlEditor">
            <div class="quick-entry-main">
              <span class="quick-entry-title">高级编辑</span>
              <span class="quick-entry-desc">{{ parsingVars ? '正在重新解析字段...' : '直接编辑部署前的 compose YAML' }}</span>
            </div>
            <span class="service-count">YAML</span>
          </button>
        </div>

        <div class="section-card service-list-card">
          <div class="service-list-header">
            <span class="service-name">配置分组</span>
            <span class="field-hint">基础配置默认展开，高级配置默认收拢</span>
          </div>
          <div v-if="serviceGroups.length > 0" class="service-list">
            <button
              v-for="group in serviceGroups"
              :key="group.serviceName"
              type="button"
              class="service-list-item"
              :class="{ active: activePanelMode === 'service' && selectedServiceName === group.serviceName }"
              @click="selectService(group.serviceName)"
            >
              <div class="service-list-main">
                <span class="service-list-name">{{ group.displayName }}</span>
                <span class="service-list-desc">基础配置 {{ group.basic.length + group.basicMappings.length }} 项</span>
              </div>
              <span class="service-advanced-count">高级配置 {{ group.advanced.length + group.advancedMappings.length }} 项</span>
            </button>
          </div>
          <div v-else class="no-config compact">
            <DynamicIcon name="check-circle" :size="24" />
            <p>该应用无 schema 配置</p>
          </div>
        </div>
      </div>

      <div class="right-panel">
        <div v-if="advancedModeEnabled && hasProjectEnvInputs" class="section-card dotenv-card">
          <div class="service-header">
            <div class="service-info">
              <span class="service-name">.env</span>
              <span class="service-count">环境变量文件</span>
            </div>
          </div>
          <div class="section-content">
            <p class="field-hint">部署时会将这些键值写入项目目录的 <code>.env</code> 文件，供 Compose 运行时引用。</p>
            <div class="dotenv-editor">
              <textarea
                v-model="dotenvText"
                class="dotenv-textarea auto-height"
                placeholder="KEY=value"
                @focus="dotenvEditorFocused = true"
                @blur="applyDotenvEditor"
              ></textarea>
            </div>
          </div>
        </div>

        <div v-if="activePanelMode === 'yaml' && advancedModeEnabled" class="section-card advanced-yaml-card">
          <div class="service-header">
            <div class="service-info">
              <span class="service-name">Compose YAML</span>
              <span class="service-count">高级模式</span>
            </div>
          </div>
          <div class="section-content no-margin">
            <p class="field-hint yaml-editor-hint">
              当前已启用高级模式，你可以直接编辑部署前的 compose YAML。表单变量和 .env 仍会按当前值一起提交。
            </p>
            <div class="yaml-actions">
              <button class="btn secondary small" type="button" @click="resetComposeYaml">恢复模板</button>
            </div>
            <div class="yaml-editor">
              <CodeEditor
                v-model="composeText"
                language="yaml"
                aria-label="Compose YAML 编辑器"
                height="360px"
              />
            </div>
          </div>
        </div>

        <div v-else-if="selectedServiceGroup" class="section-card service-config-card">
          <div class="service-header">
            <div class="service-info">
              <span class="service-name">{{ selectedServiceGroup.displayName }}</span>
              <span class="service-count">{{ selectedServiceGroup.configs.length + selectedServiceGroup.basicMappings.length + selectedServiceGroup.advancedMappings.length }} 项配置</span>
            </div>
            <div v-if="selectedServiceGroup.serviceName !== 'Global'" class="service-actions">
              <select v-model="addKinds[selectedServiceGroup.serviceName]" class="field-input compact-select" aria-label="新增配置类型">
                <option value="port">端口</option>
                <option value="bind">路径</option>
                <option value="device">设备</option>
                <option value="environment">环境变量</option>
              </select>
              <button class="btn secondary small add-config-btn" type="button" @click="addServiceMapping(selectedServiceGroup.serviceName)">
                <DynamicIcon name="plus" :size="14" />
                新增
              </button>
            </div>
          </div>

          <div class="section-content no-margin">
            <div v-if="selectedServiceGroup.basic.length > 0 || selectedServiceGroup.basicMappings.length > 0" class="config-section">
              <div class="section-title-bar">
                <span class="title-text">基础配置</span>
              </div>
              <div class="config-rows">
                <div
                  v-for="config in selectedServiceGroup.basic"
                  :key="config._uid"
                  class="config-row"
                >
                  <div class="type-tag">
                    <span class="tag">{{ getParamTypeLabel(config) }}</span>
                  </div>
                  <div class="param-name-col">
                    <div class="input-wrapper">
                      <input
                        :value="config.label || config.name"
                        type="text"
                        class="field-input"
                        readonly
                        :placeholder="config.name"
                      />
                    </div>
                  </div>
                  <div class="arrow-col">
                    <DynamicIcon name="arrow-right" :size="16" />
                  </div>
                  <div class="param-value-col">
                    <div class="input-wrapper">
                      <input
                        v-model="formValues[config._uid]"
                        :type="config.kind === 'secret' || config.sensitive ? 'password' : 'text'"
                        class="field-input"
                        :placeholder="config.kind === 'secret' || config.sensitive ? '请输入密钥' : (config.default || '请输入')"
                      />
                    </div>
                  </div>
                </div>
                <div
                  v-for="mapping in selectedServiceGroup.basicMappings"
                  :key="mapping.id"
                  class="config-row mapping-row"
                >
                  <div class="type-tag">
                    <span class="tag">{{ getMappingTypeLabel(mapping) }}</span>
                  </div>
                  <div class="param-name-col">
                    <div class="input-wrapper">
                      <input
                        :value="mapping.source"
                        type="text"
                        class="field-input"
                        :disabled="mapping.kind === 'volume'"
                        :placeholder="mappingPlaceholders(mapping)[0]"
                        @input="updateMappingField(mapping, 'source', $event.target.value)"
                      />
                    </div>
                  </div>
                  <div class="arrow-col">
                    <DynamicIcon name="arrow-right" :size="16" />
                  </div>
                  <div class="param-value-col">
                    <div class="input-wrapper mapping-target-wrapper">
                      <input
                        :value="mapping.target"
                        type="text"
                        class="field-input"
                        :disabled="mapping.kind === 'volume'"
                        :placeholder="mappingPlaceholders(mapping)[1]"
                        @input="updateMappingField(mapping, 'target', $event.target.value)"
                      />
                      <button
                        v-if="mapping.added"
                        class="table-btn danger mapping-remove-btn"
                        type="button"
                        title="删除新增配置"
                        @click="removeServiceMapping(mapping)"
                      >
                        <DynamicIcon name="trash-2" :size="14" />
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <div v-if="selectedServiceGroup.advanced.length > 0 || selectedServiceGroup.advancedMappings.length > 0" class="config-section advanced">
              <div
                class="section-title-bar collapsible"
                role="button"
                tabindex="0"
                :aria-expanded="Boolean(expandedAdvanced[selectedServiceGroup.serviceName])"
                @click="toggleAdvanced(selectedServiceGroup.serviceName)"
                @keydown.enter.prevent="toggleAdvanced(selectedServiceGroup.serviceName)"
                @keydown.space.prevent="toggleAdvanced(selectedServiceGroup.serviceName)"
              >
                <div class="title-left">
                  <DynamicIcon name="settings" :size="16" class="title-icon" />
                  <span class="title-text">高级配置</span>
                  <span class="badge">{{ selectedServiceGroup.advanced.length + selectedServiceGroup.advancedMappings.length }}</span>
                </div>
                <DynamicIcon
                  name="chevron-down"
                  :size="18"
                  :class="['toggle-arrow', { rotated: expandedAdvanced[selectedServiceGroup.serviceName] }]"
                />
              </div>
              <!-- grid-rows 折叠动画要求保持挂载 -->
              <div
                class="motion-collapse"
                :class="{ 'motion-collapse-open': expandedAdvanced[selectedServiceGroup.serviceName] }"
              >
                <div class="motion-collapse-inner">
                  <div class="config-rows">
                    <div
                      v-for="config in selectedServiceGroup.advanced"
                      :key="config._uid"
                      class="config-row"
                    >
                      <div class="type-tag">
                        <span class="tag">{{ getParamTypeLabel(config) }}</span>
                      </div>
                      <div class="param-name-col">
                        <div class="input-wrapper">
                          <input
                            :value="config.label || config.name"
                            type="text"
                            class="field-input"
                            readonly
                            :placeholder="config.name"
                          />
                        </div>
                      </div>
                      <div class="arrow-col">
                        <DynamicIcon name="arrow-right" :size="16" />
                      </div>
                      <div class="param-value-col">
                        <div class="input-wrapper">
                          <input
                            v-model="formValues[config._uid]"
                            :type="config.kind === 'secret' || config.sensitive ? 'password' : 'text'"
                            class="field-input"
                            :placeholder="config.kind === 'secret' || config.sensitive ? '请输入密钥' : (config.default || '请输入')"
                          />
                        </div>
                      </div>
                    </div>
                    <div
                      v-for="mapping in selectedServiceGroup.advancedMappings"
                      :key="mapping.id"
                      class="config-row mapping-row"
                    >
                      <div class="type-tag">
                        <span class="tag">{{ getMappingTypeLabel(mapping) }}</span>
                      </div>
                      <div class="param-name-col">
                        <div class="input-wrapper">
                          <input
                            :value="mapping.source"
                            type="text"
                            class="field-input"
                            :disabled="mapping.kind === 'volume'"
                            :placeholder="mappingPlaceholders(mapping)[0]"
                            @input="updateMappingField(mapping, 'source', $event.target.value)"
                          />
                        </div>
                      </div>
                      <div class="arrow-col">
                        <DynamicIcon name="arrow-right" :size="16" />
                      </div>
                      <div class="param-value-col">
                        <div class="input-wrapper mapping-target-wrapper">
                          <input
                            :value="mapping.target"
                            type="text"
                            class="field-input"
                            :disabled="mapping.kind === 'volume'"
                            :placeholder="mappingPlaceholders(mapping)[1]"
                            @input="updateMappingField(mapping, 'target', $event.target.value)"
                          />
                          <button
                            v-if="mapping.added"
                            class="table-btn danger mapping-remove-btn"
                            type="button"
                            title="删除新增配置"
                            @click="removeServiceMapping(mapping)"
                          >
                            <DynamicIcon name="trash-2" :size="14" />
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <div v-if="selectedServiceGroup.configs.length === 0 && selectedServiceGroup.basicMappings.length === 0 && selectedServiceGroup.advancedMappings.length === 0" class="no-config compact">
              <DynamicIcon name="check-circle" :size="24" />
              <p>当前容器没有额外配置项</p>
            </div>
          </div>
        </div>

        <div v-else class="no-config panel-empty">
          <DynamicIcon name="layout-panel-left" :size="28" />
          <p>请选择左侧容器，或打开高级编辑</p>
        </div>

        <div class="action-bar">
          <button class="btn secondary" @click="goBack">取消</button>
          <button class="btn primary" :disabled="deploying || !canDeployAppStore" :title="canDeployAppStore ? '' : '当前远程设备需要支持部署能力，请更新 Agent'" @click="startDeploy">
            <span v-if="deploying" class="spinner"></span>
            <DynamicIcon v-else name="rocket" :size="14" />
            {{ deploying ? '部署中...' : '开始部署' }}
          </button>
        </div>
      </div>
    </div>

    <!-- 部署日志对话框 -->
    <Modal
      v-model:visible="showDeployModal"
      title="部署日志"
      width="700px"
      :show-close="!deploying"
      :close-on-overlay="!deploying"
      :close-on-esc="!deploying"
    >
      <div class="deploy-logs ops-console">
        <div v-if="deployLogs.length === 0" class="logs-empty">
          <div class="spinner"></div>
          <p>正在准备部署...</p>
        </div>
        <div v-else ref="logsContainer" class="logs-content ops-console__body">
          <div
            v-for="(log, idx) in deployLogs"
            :key="idx"
            :class="['log-line', log.type]"
          >
            <span class="log-time">{{ log.time }}</span>
            <span class="log-icon">
              <DynamicIcon
                :name="log.type === 'error'
                  ? 'alert-circle'
                  : (log.type === 'success' || log.type === 'result')
                    ? 'circle-check'
                    : log.type === 'warning'
                      ? 'triangle-alert'
                      : 'info'"
                :size="14"
              />
            </span>
            {{ log.message }}
          </div>
        </div>
      </div>
      <template #footer>
        <button v-if="!deploying" class="btn primary" @click="finishDeploy">完成</button>
        <template v-else>
          <button class="btn secondary" :disabled="!deployTaskId" @click="handleBackgroundDeploy">后台运行并关闭</button>
          <button class="btn danger" :disabled="!deployTaskId || abortingDeploy" @click="handleAbortDeploy">
            {{ abortingDeploy ? '中止中...' : '中止部署' }}
          </button>
          <span class="btn" disabled>
            <span class="spinner"></span>
            部署中...
          </span>
        </template>
      </template>
    </Modal>
    </div>
  </ResourceWorkbench>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Modal from '@/components/feedback/Modal.vue'
import CodeEditor from '@/components/ui/CodeEditor.vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import DynamicAppIcon from '@/components/ui/DynamicAppIcon.vue'
import ResourceWorkbench from '@/components/resource-workbench/ResourceWorkbench.vue'
import {
  cancelDeployTask,
  deployApp,
  getAppDetail,
  getAppVars,
  getDeployTask,
  getDeployTaskEventsUrl,
  parseAppVars,
  preflightDeployApp
} from '@/api/appstore.js'
import { useUiStore } from '@/stores/ui.js'
import { formatTime } from '@/utils/format.js'
import { buildAppStoreDeployRequest, hasAppStoreSourceChanges } from './deployRequest.js'
import { createTemplateManifestForm, manifestInputLevel, manifestInputsToSchema, normalizeManifestGroup } from './useTemplateManifestForm.js'
import { remoteCapabilityAllowed, useEnvironmentContext } from '@/composables/useEnvironmentContext.js'
import { deploymentPreflightConfirmation } from '@/utils/deploymentPreflight.js'
import { reportSuccessfulAppStoreDeployment } from '@edition/appstore-post-deploy'

const uiStore = useUiStore()

const { isRemoteEnvironment, environment } = useEnvironmentContext()
const canDeployAppStore = computed(() => !isRemoteEnvironment.value || [
  'deployment.preflight', 'deployment.apply', 'deployment.inspect', 'deployment.cancel'
].every(capability => remoteCapabilityAllowed(environment.value, capability)))

const DEPLOY_TASK_STATE_KEY = 'tradis:appstore:deploy-task'

// 分类中文映射
const categoryMap = {
  'media': '媒体', 'development': '开发', 'tools': '工具',
  'productivity': '生产力', 'database': '数据库', 'storage': '存储',
  'network': '网络', 'monitoring': '监控', 'security': '安全',
  'automation': '自动化', 'ai': 'AI工具', 'multimedia': '多媒体',
  'other': '其他', 'others': '其他', 'utilities': '工具',
  'dev': '开发', 'web': '网站', 'blog': '博客', 'cms': '内容管理',
  'cloud': '云存储', 'communication': '通讯', 'chat': '聊天',
  'game': '游戏', 'finance': '金融', 'shopping': '购物',
  'social': '社交', 'learning': '学习', 'reading': '阅读',
  'music': '音乐', 'video': '视频', 'photo': '图片',
  'download': '下载', 'proxy': '代理', 'vpn': 'VPN',
  'dns': 'DNS', 'mail': '邮件', 'git': '版本控制',
  'ci': '持续集成', 'office': '办公', 'note': '笔记',
  'wiki': '知识库', 'home': '智能家居', 'iot': '物联网'
}

const route = useRoute()
const router = useRouter()

// 状态
const loading = ref(true)
const error = ref('')
const app = ref({})
const schema = ref([])
const templateManifest = ref({})
const templateInitialValues = ref({})
const sourceFiles = ref([])
const warnings = ref([])
const parseErrors = ref([])
const parsingVars = ref(false)
const expandedAdvanced = ref({})
const expandedSections = ref({})
const deploying = ref(false)
const showDeployModal = ref(false)
const deployLogs = ref([])
const logsContainer = ref(null)
const advancedModeEnabled = ref(false)
const activePanelMode = ref('service')
const selectedServiceName = ref('')
const addKinds = ref({})
const deployTaskId = ref('')
const abortingDeploy = ref(false)
let eventSource = null
let composeParseTimer = null
let composeParseSeq = 0

// 表单数据 - 从 schema 初始化
const projectName = ref('')
const templateForm = createTemplateManifestForm({}, {})
const manifest = templateForm.manifest
const formValues = ref(templateForm.valuesByInputID)
const mappingOverlays = templateForm.mappingOverlays
const dotenvText = ref('')
const dotenvEditorFocused = ref(false)
const composeText = ref('')
const hasProjectEnvInputs = computed(() => (manifest.value?.inputs || []).some(input => input.scope === 'project'))

function firstBinding(param) {
  return Array.isArray(param?.bindings) && param.bindings.length > 0
    ? param.bindings[0]
    : null
}

function normalizeSchemaItems(items = []) {
  return Array.isArray(items) ? items.map((item, index) => {
    const serviceName = item.serviceName || item.service_name || 'Global'
    const paramType = item.paramType || item.param_type || item.type || 'env'
    const key = item.name || item.key || ''
    return {
      ...item,
      _uid: item.inputId || item.input_id || item.id || `${serviceName}:${paramType}:${key || 'param'}:${index}`,
      name: key,
      label: item.label || key,
      default: item.default ?? item.defaultValue ?? item.value ?? '',
      category: normalizeManifestGroup(item.category),
      serviceName,
      paramType,
      kind: item.kind || 'env',
      required: !!item.required,
      envFile: item.envFile || item.env_file || ''
    }
  }) : []
}

function schemaMetaByKey(items = []) {
  const out = new Map()
  normalizeSchemaItems(items).forEach(item => {
    if (!item.name || out.has(item.name)) return
    out.set(item.name, item)
  })
  return out
}

function normalizeParamsToSchema(params = [], sourceSchema = []) {
  const meta = schemaMetaByKey(sourceSchema)
  return Array.isArray(params) ? params.map((param, index) => {
    const key = param.key || param.name || ''
    const matched = meta.get(key) || {}
    const binding = firstBinding(param)
    const kind = param.kind || 'env'
    const paramType = kind === 'secret'
      ? 'secret'
      : (matched.paramType || matched.type || 'env')
    const serviceName = matched.serviceName || binding?.serviceName || 'Global'
    const category = normalizeManifestGroup(matched.category)
    return {
      ...matched,
      _uid: param.inputId || param.input_id || `${serviceName}:${kind}:${paramType}:${key || 'param'}:${index}`,
      name: key,
      label: matched.label || key,
      default: param.value ?? param.defaultValue ?? matched.default ?? '',
      category,
      serviceName,
      paramType,
      kind,
      required: !!param.required,
      envFile: matched.envFile || binding?.file || '',
      bindings: Array.isArray(param.bindings) ? param.bindings : [],
      sources: Array.isArray(param.sources) ? param.sources : [],
      usages: Array.isArray(param.usages) ? param.usages : [],
      examples: Array.isArray(param.examples) ? param.examples : []
    }
  }) : []
}

function normalizeVariablesToSchema(variables = []) {
  return Array.isArray(variables) ? variables.map((item, index) => ({
    _uid: item.inputId || item.input_id || `Global:env:${item.name || item.key || 'param'}:${index}`,
    name: item.name || item.key || '',
    label: item.name || item.key || '',
    default: item.value ?? item.defaultValue ?? '',
    category: 'basic',
    serviceName: 'Global',
    paramType: 'env',
    kind: 'env',
    required: !!item.required,
    sources: Array.isArray(item.sources) ? item.sources : [],
    examples: Array.isArray(item.examples) ? item.examples : []
  })) : []
}

function fieldStableKey(item = {}) {
  return item.inputId || item.input_id || item._uid || ''
}

function getCurrentAppId() {
  return String(route.params.id || '')
}

function isTerminalStatus(status) {
  return ['success', 'completed', 'error', 'failed', 'canceled'].includes(String(status || '').toLowerCase())
}

function normalizeLogTime(value) {
  if (!value) return formatTime(new Date())
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return String(value)
  return formatTime(parsed)
}

function saveDeployTaskState(taskId, extra = {}) {
  if (!taskId) return
  const state = {
    taskId: String(taskId),
    appId: getCurrentAppId(),
    appName: app.value?.name || '',
    projectName: projectName.value || app.value?.name || '',
    environmentId: environment.value?.id || 'local',
    updatedAt: new Date().toISOString(),
    ...extra
  }
  localStorage.setItem(DEPLOY_TASK_STATE_KEY, JSON.stringify(state))
}

function loadDeployTaskState() {
  try {
    const raw = localStorage.getItem(DEPLOY_TASK_STATE_KEY)
    if (!raw) return null
    const state = JSON.parse(raw)
    if (!state?.taskId) return null
    return state
  } catch {
    return null
  }
}

function clearDeployTaskState(taskId = '') {
  if (!taskId) {
    localStorage.removeItem(DEPLOY_TASK_STATE_KEY)
    return
  }
  const state = loadDeployTaskState()
  if (state?.taskId === String(taskId)) {
    localStorage.removeItem(DEPLOY_TASK_STATE_KEY)
  }
}

// 按服务分组 Schema。Global 作为“全局配置”展示，不再隐藏在 .env 文本里。
const serviceGroups = computed(() => {
  const groups = new Map()

  const ensureGroup = (serviceName) => {
    if (!groups.has(serviceName)) {
      groups.set(serviceName, {
        serviceName,
        displayName: serviceName === 'Global' ? '全局配置' : serviceName,
        configs: [],
        basic: [],
        advanced: [],
        basicMappings: [],
        advancedMappings: []
      })
    }
    return groups.get(serviceName)
  }

  templateForm.serviceNames().forEach(ensureGroup)

  schema.value.forEach(config => {
    const serviceName = config.serviceName || 'Global'
    const group = ensureGroup(serviceName)
    group.configs.push(config)

    if (normalizeManifestGroup(config.category) === 'advanced') {
      group.advanced.push(config)
    } else {
      group.basic.push(config)
    }
  })

  // 映射按 Server 分级进入基础/高级区块；未暴露为输入的固定结构（named volume、
  // 未发布端口）不渲染为配置项；页面新增的映射归基础。
  ;(manifest.value?.mappings || []).forEach(mapping => {
    const level = manifestInputLevel(manifest.value, mapping.id)
    if (!level) return
    ensureGroup(mapping.service || 'Global')[`${level}Mappings`].push(effectiveMapping(mapping))
  })
  mappingOverlays
    .filter(overlay => !(manifest.value?.mappings || []).some(mapping => mapping.id === overlay.id))
    .forEach(mapping => ensureGroup(mapping.service || 'Global').basicMappings.push({ ...mapping, added: true, editable: true }))

  // 转换为数组并设置展开状态（基础配置默认展开，高级字段仍单独收拢）
  const result = Array.from(groups.values()).sort((a, b) => {
    if (a.serviceName === 'Global') return -1
    if (b.serviceName === 'Global') return 1
    return a.serviceName.localeCompare(b.serviceName)
  })
  result.forEach(group => {
    if (expandedSections.value[group.serviceName] === undefined) {
      expandedSections.value[group.serviceName] = group.basic.length + group.basicMappings.length > 0
    }
  })

  return result
})

const hasSchemaConfig = computed(() => schema.value.length > 0)
const selectedServiceGroup = computed(() => {
  if (!selectedServiceName.value) return null
  return serviceGroups.value.find(group => group.serviceName === selectedServiceName.value) || null
})

// 获取参数类型标签
function getParamTypeLabel(config) {
  if (config.kind === 'secret') return '密钥'
  const paramType = config.paramType?.toLowerCase() || config.type?.toLowerCase() || ''
  const map = {
    'port': '端口',
    'path': '路径',
    'env': '环境变量',
    'environment': '环境变量',
    'secret': '密钥',
    'hardware': '硬件',
    'other': '其他'
  }
  return map[paramType] || '其他'
}

function getMappingTypeLabel(mapping) {
  return {
    port: '端口',
    bind: '路径',
    volume: '数据卷',
    device: '设备',
    environment: '环境变量'
  }[mapping.kind] || '映射'
}

function mappingPlaceholders(mapping) {
  const labels = {
    port: ['宿主机端口', '容器端口'],
    bind: ['宿主机路径', '容器路径'],
    volume: ['数据卷', '容器路径'],
    device: ['宿主机设备', '容器设备'],
    environment: ['变量名', '变量值']
  }
  return labels[mapping.kind] || ['来源', '目标']
}

function effectiveMapping(mapping) {
  const overlay = mappingOverlays.find(item => item.id === mapping.id)
  return overlay ? { ...mapping, ...overlay } : { ...mapping }
}

function updateMappingField(mapping, field, value) {
  if (mapping.kind === 'volume') return
  templateForm.updateMapping(mapping.id, { [field]: value })
}

function addServiceMapping(serviceName) {
  if (!serviceName || serviceName === 'Global') return
  const kind = addKinds.value[serviceName] || 'port'
  const sequence = mappingOverlays.length + 1
  templateForm.addMapping({
    id: `custom:${kind}:${serviceName}:${Date.now()}:${sequence}`,
    kind,
    service: serviceName,
    source: '',
    target: '',
    protocol: kind === 'port' ? 'tcp' : '',
    mode: ''
  })
}

function removeServiceMapping(mapping) {
  if (mapping.added) templateForm.removeAddedMapping(mapping.id)
}

// 获取占位符
function getPlaceholder(config) {
  if (config.description) return config.description
  if (config.kind === 'secret') return '请输入密钥'
  const paramType = config.paramType?.toLowerCase() || config.type?.toLowerCase() || ''
  if (paramType === 'port') return '宿主机端口，如: 8080'
  if (paramType === 'path') return '宿主机路径，如: /data'
  return '请输入值'
}

// 切换高级配置
function toggleAdvanced(serviceName) {
  expandedAdvanced.value[serviceName] = !expandedAdvanced.value[serviceName]
}

// 切换服务卡片展开/收拢
function toggleSection(serviceName) {
  expandedSections.value[serviceName] = !expandedSections.value[serviceName]
}

// 获取分类中文名
function getCategoryCN(category) {
  if (!category) return '其他'
  return categoryMap[category.toLowerCase()] || category
}

// 初始化表单 - 从服务器返回的 schema 读取默认值
function initFormFromSchema(nextSchema = app.value?.schema || [], options = {}) {
  if (!Array.isArray(nextSchema)) return

  const oldByStableKey = new Map()
  if (options.preserveValues) {
    schema.value.forEach(item => {
      const value = formValues.value[item._uid]
      if (value === undefined) return
      oldByStableKey.set(fieldStableKey(item), value)
    })
  }
  
  // 设置 schema
  schema.value = normalizeSchemaItems(nextSchema)
  const nextValues = {}
  
  // 初始化表单值 - 使用 schema 中的 default 值
  schema.value.forEach(config => {
    const defaultValue = config.default || ''
    const stableKey = fieldStableKey(config)
    nextValues[config._uid] = oldByStableKey.has(stableKey)
      ? oldByStableKey.get(stableKey)
      : defaultValue
  })
  for (const key of Object.keys(formValues.value)) delete formValues.value[key]
  Object.assign(formValues.value, nextValues)
}

function applyVarsResponse(varsRes, sourceSchema = app.value?.schema || [], options = {}) {
  if (!varsRes) return
  warnings.value = Array.isArray(varsRes.warnings) ? varsRes.warnings : []
  parseErrors.value = Array.isArray(varsRes.errors) ? varsRes.errors : []

  if (varsRes.manifest?.manifest_version) {
    templateForm.replaceManifest(varsRes.manifest, varsRes.initial_values || {}, options)
    const nextSchema = manifestInputsToSchema(varsRes.manifest)
    app.value.manifest = varsRes.manifest
    app.value.manifest_digest = varsRes.manifest.manifest_digest || ''
    app.value.schema = nextSchema
    schema.value = nextSchema
  } else if (Array.isArray(varsRes.params) && varsRes.params.length > 0) {
    const nextSchema = normalizeParamsToSchema(varsRes.params, sourceSchema)
    app.value.schema = nextSchema
    initFormFromSchema(nextSchema, options)
  } else if (Array.isArray(varsRes.schema)) {
    app.value.schema = varsRes.schema
    initFormFromSchema(varsRes.schema, options)
  } else if (Array.isArray(varsRes.variables) && varsRes.variables.length > 0) {
    const nextSchema = normalizeVariablesToSchema(varsRes.variables)
    app.value.schema = nextSchema
    initFormFromSchema(nextSchema, options)
  }
}

function sourceHasChanges() {
  return hasAppStoreSourceChanges({
    app: app.value,
    composeText: composeText.value,
    sourceFiles: sourceFiles.value
  })
}

function restoreOfficialManifestProjection() {
  if (!templateManifest.value?.manifest_version) return
  templateForm.replaceManifest(templateManifest.value, templateInitialValues.value, { preserveValues: true })
  const nextSchema = manifestInputsToSchema(templateManifest.value)
  app.value.manifest = templateManifest.value
  app.value.manifest_digest = templateManifest.value.manifest_digest || ''
  app.value.schema = nextSchema
  schema.value = nextSchema
}

async function parseComposeFields() {
  if (!advancedModeEnabled.value) return true
  const compose = String(composeText.value || '').trim()
  if (!compose) return false
  const parseSeq = ++composeParseSeq
  parsingVars.value = true
  try {
    const res = await parseAppVars({
      compose: composeText.value,
      dotenv: app.value?.dotenv || '',
      source_files: sourceFiles.value,
      input_metadata: app.value?.input_metadata || {},
      schema: app.value?.schema || []
    })
    if (parseSeq !== composeParseSeq) return
    applyVarsResponse(res, app.value?.schema || [], { preserveValues: true })
    return true
  } catch (err) {
    if (parseSeq !== composeParseSeq) return
    parseErrors.value = [err?.message || 'Compose 解析失败']
    return false
  } finally {
    if (parseSeq === composeParseSeq) {
      parsingVars.value = false
    }
  }
}

function scheduleComposeParse() {
  if (!advancedModeEnabled.value) return
  if (!sourceHasChanges()) {
    restoreOfficialManifestProjection()
    return
  }
  if (composeParseTimer) {
    clearTimeout(composeParseTimer)
  }
  composeParseTimer = setTimeout(() => {
    composeParseTimer = null
    parseComposeFields()
  }, 600)
}

function syncAdvancedMode() {
  advancedModeEnabled.value = localStorage.getItem('advancedMode') === '1'
}

function handleStorageChange(event) {
  if (!event || event.key === 'advancedMode') {
    syncAdvancedMode()
  }
}

function resetComposeYaml() {
  composeParseSeq++
  composeText.value = app.value.compose || ''
  parseErrors.value = []
  templateForm.replaceManifest(templateManifest.value, templateInitialValues.value)
  const nextSchema = manifestInputsToSchema(templateManifest.value)
  app.value.manifest = templateManifest.value
  app.value.schema = nextSchema
  schema.value = nextSchema
  syncDotenvEditorFromRegistry(app.value.dotenv || '')
}

function syncDotenvEditorFromRegistry(sourceText = dotenvText.value || app.value?.dotenv || '') {
  if (dotenvEditorFocused.value) return
  dotenvText.value = templateForm.renderProjectDotenv(sourceText)
}

function applyDotenvEditor() {
  dotenvEditorFocused.value = false
  const result = templateForm.applyProjectDotenv(dotenvText.value)
  if (!result.ok) {
    parseErrors.value = result.errors
    return
  }
  parseErrors.value = []
  syncDotenvEditorFromRegistry(dotenvText.value)
}

function openYamlEditor() {
  activePanelMode.value = 'yaml'
}

function selectService(serviceName) {
  selectedServiceName.value = serviceName
  activePanelMode.value = 'service'
  if (expandedAdvanced.value[serviceName] === undefined) {
    expandedAdvanced.value[serviceName] = false
  }
}

// 获取应用数据
async function fetchAppData() {
  const appId = route.params.id
  if (!appId) {
    error.value = '应用ID不能为空'
    loading.value = false
    return
  }

  loading.value = true
  error.value = ''

  try {
    // 1. 获取应用详情（包含 schema）
    const detail = await getAppDetail(appId).catch(() => null)
    
    if (detail) {
      app.value = detail
      sourceFiles.value = Array.isArray(detail.source_files) ? detail.source_files : []
      projectName.value = detail.name || ''
      composeText.value = detail.compose || ''
      dotenvText.value = detail.dotenv || ''

      if (detail.manifest?.manifest_version) {
        templateManifest.value = detail.manifest
        templateInitialValues.value = Object.fromEntries(
          (detail.manifest.inputs || []).map(input => [input.id, input.default_value ?? ''])
        )
        templateForm.replaceManifest(templateManifest.value, templateInitialValues.value)
        schema.value = manifestInputsToSchema(templateManifest.value)
      } else {
        initFormFromSchema(Array.isArray(detail.schema) ? detail.schema : [])
      }
    } else {
      error.value = '获取应用详情失败'
      loading.value = false
      return
    }

    // 2. 获取变量和 dotenv
    const varsRes = await getAppVars(appId).catch(() => null)
    
    if (varsRes) {
      // 使用返回的 dotenv
      dotenvText.value = varsRes.dotenv || ''
      applyVarsResponse(varsRes, app.value?.schema || [])
      if (varsRes.manifest?.manifest_version) {
        templateManifest.value = varsRes.manifest
        templateInitialValues.value = { ...(varsRes.initial_values || {}) }
      }
    }

    await restoreDeployTask()
  } catch (err) {
    console.error('获取应用数据失败:', err)
    error.value = err.message || '获取应用数据失败'
  } finally {
    loading.value = false
  }
}

// 返回上一页
function goBack() {
  router.push('/appstore')
}

// 构建部署请求数据
function buildDeployRequest() {
  return buildAppStoreDeployRequest({
    app: app.value,
    composeText: composeText.value,
    baseManifestDigest: templateManifest.value?.manifest_digest || '',
    overrideManifestDigest: manifest.value?.manifest_digest || '',
    valuesByInputID: formValues.value,
    mappingOverlays,
    sourceFiles: sourceFiles.value
  })
}

// 开始部署
let localDeployAttempt = null
async function startDeploy() {
  if (deploying.value) return
  if (advancedModeEnabled.value && !String(composeText.value || '').trim()) {
    uiStore.toastWarning('高级模式下 compose YAML 不能为空')
    return
  }
  if (advancedModeEnabled.value && sourceHasChanges()) {
    if (composeParseTimer) {
      clearTimeout(composeParseTimer)
      composeParseTimer = null
    }
    const sourceReady = await parseComposeFields()
    if (!sourceReady || parseErrors.value.length > 0) {
      uiStore.toastWarning('请先修复高级 YAML 的解析错误')
      return
    }
  }
  const missing = schema.value.find(item => {
    if (!item.required) return false
    const value = String(formValues.value[item._uid] ?? '').trim()
    return value === ''
  })
  if (missing) {
    uiStore.toastWarning(`请填写必填项：${missing.label || missing.name}`)
    return
  }
  const incompleteMapping = mappingOverlays.find(item => item.id.startsWith('custom:') && (
    !String(item.source || '').trim() || (item.kind !== 'environment' && !String(item.target || '').trim())
  ))
  if (incompleteMapping) {
    uiStore.toastWarning(`请完整填写新增的${getMappingTypeLabel(incompleteMapping)}配置`)
    return
  }
  deploying.value = true
  deployLogs.value = []
  showDeployModal.value = true

  try {
    const deployData = buildDeployRequest()
    if (isRemoteEnvironment.value) {
      const preflight = await preflightDeployApp(app.value.id, deployData)
      const resolvedProjectName = String(preflight?.resolved_project_name || '').trim()
      if (!resolvedProjectName) {
        throw new Error('远程设备未返回项目名称')
      }
      if (preflight?.requires_confirmation && !window.confirm(deploymentPreflightConfirmation(preflight))) {
        deploying.value = false
        showDeployModal.value = false
        return
      }
      projectName.value = resolvedProjectName
      deployData.projectName = resolvedProjectName
      deployData.preflightToken = preflight.preflight_token
      deployData.idempotencyKey = preflight.idempotency_key
      deployLogs.value.push({
        type: 'info',
        message: `目标设备将使用项目名称：${resolvedProjectName}`,
        time: formatTime(new Date())
      })
    } else {
      const signature = JSON.stringify(deployData)
      if (localDeployAttempt?.signature !== signature) {
        const randomPart = globalThis.crypto?.randomUUID?.()
          || `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
        localDeployAttempt = { signature, key: `appstore-local-${randomPart}` }
      }
      deployData.idempotencyKey = localDeployAttempt.key
    }
    const res = await deployApp(app.value.id, deployData)

    const taskId = res?.taskId
    if (taskId) {
      localDeployAttempt = null
      deployTaskId.value = String(taskId)
      const taskEnvironmentId = environment.value?.id || 'local'
      saveDeployTaskState(taskId, { status: 'running', environmentId: taskEnvironmentId })
      connectDeployLogs(taskId, taskEnvironmentId)
    } else {
      deployLogs.value.push({
        type: 'success',
        message: '部署任务已启动',
        time: formatTime(new Date())
      })
      deploying.value = false
    }
  } catch (err) {
    deployLogs.value.push({
      type: 'error',
      message: '部署失败: ' + (err.message || '未知错误'),
      time: formatTime(new Date())
    })
    clearDeployTaskState()
    deploying.value = false
  }
}

// 连接部署日志 SSE
function connectDeployLogs(taskId, taskEnvironmentId = environment.value?.id || 'local') {
  const url = getDeployTaskEventsUrl(taskId, taskEnvironmentId)

  if (eventSource) {
    eventSource.close()
    eventSource = null
  }

  eventSource = new EventSource(url)

  eventSource.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data)
      const type = String(data.type || 'info').toLowerCase()
      const status = String(data.status || '').toLowerCase()
      const statusText = ['success', 'completed'].includes(status)
        ? '部署成功'
        : status === 'canceled'
          ? '部署已中止'
          : ['error', 'failed'].includes(status)
            ? '部署失败'
            : `任务结束：${status}`
      const message = String(data.message || (type === 'result' && status ? statusText : ''))

      deployLogs.value.push({
        type,
        message,
        time: normalizeLogTime(data.time)
      })

      nextTick(() => {
        if (logsContainer.value) {
          logsContainer.value.scrollTop = logsContainer.value.scrollHeight
        }
      })

      if (type === 'result') {
        deploying.value = false
        abortingDeploy.value = false
        deployTaskId.value = ''
        eventSource.close()
        eventSource = null
        if (status === 'success' || status === 'completed') {
          reportSuccessfulAppStoreDeployment(app.value?.id, taskEnvironmentId)
        }
        clearDeployTaskState(taskId)
      }
    } catch (e) {
      deployLogs.value.push({
        type: 'info',
        message: String(event.data),
        time: formatTime(new Date())
      })
    }
  }

  eventSource.onerror = () => {
    if (!deployLogs.value.some(log => log.type === 'warning' && log.message === '日志连接中断，正在自动重连...')) {
      deployLogs.value.push({
        type: 'warning',
        message: '日志连接中断，正在自动重连...',
        time: formatTime(new Date())
      })
    }
    saveDeployTaskState(taskId, { status: 'reconnecting', environmentId: taskEnvironmentId })
  }
}

async function restoreDeployTask() {
  const state = loadDeployTaskState()
  if (!state?.taskId || state.appId !== getCurrentAppId()) return

  try {
    const taskEnvironmentId = state.environmentId || 'local'
    const task = await getDeployTask(state.taskId, taskEnvironmentId)
    if (!task?.id) {
      clearDeployTaskState(state.taskId)
      return
    }

    showDeployModal.value = true
    deploying.value = !isTerminalStatus(task.status)
    deployLogs.value = []
    deployTaskId.value = String(state.taskId)
    connectDeployLogs(state.taskId, taskEnvironmentId)
  } catch {
    clearDeployTaskState(state.taskId)
  }
}

// 完成部署
function finishDeploy() {
  showDeployModal.value = false
  deployTaskId.value = ''
  router.push('/compose')
}

// 后台运行：只关闭日志弹窗，不中止后端任务；SSE 继续接收结果并清理任务状态
function handleBackgroundDeploy() {
  const taskId = deployTaskId.value
  if (!deploying.value || !taskId) return
  const taskEnvironmentId = loadDeployTaskState()?.environmentId || environment.value?.id || 'local'
  saveDeployTaskState(taskId, { status: 'running', environmentId: taskEnvironmentId })
  showDeployModal.value = false
  const name = projectName.value || app.value?.name || ''
  uiStore.toastInfo(name ? `部署任务已后台运行：${name}` : '部署任务已后台运行')
}

// 中止部署
async function handleAbortDeploy() {
  const taskId = deployTaskId.value
  if (!taskId || abortingDeploy.value) return
  abortingDeploy.value = true
  try {
    const taskEnvironmentId = loadDeployTaskState()?.environmentId || environment.value?.id || 'local'
    await cancelDeployTask(taskId, taskEnvironmentId)
    uiStore.toastInfo('已请求中止部署')
  } catch (error) {
    abortingDeploy.value = false
    uiStore.toastError('中止失败: ' + (error.message || '未知错误'))
  }
}

onUnmounted(() => {
  if (eventSource) {
    eventSource.close()
  }
  if (composeParseTimer) {
    clearTimeout(composeParseTimer)
    composeParseTimer = null
  }
  window.removeEventListener('storage', handleStorageChange)
})

onMounted(() => {
  syncAdvancedMode()
  window.addEventListener('storage', handleStorageChange)
  fetchAppData()
})

watch(serviceGroups, (groups) => {
  if (!Array.isArray(groups) || groups.length === 0) {
    selectedServiceName.value = ''
    if (activePanelMode.value === 'service') {
      activePanelMode.value = advancedModeEnabled.value ? 'yaml' : 'service'
    }
    return
  }

  const exists = groups.some(group => group.serviceName === selectedServiceName.value)
  if (!exists) {
    selectedServiceName.value = groups[0].serviceName
  }

  if (activePanelMode.value !== 'yaml') {
    activePanelMode.value = 'service'
  }
}, { immediate: true })

watch(composeText, () => {
  scheduleComposeParse()
})

watch(formValues, () => {
  syncDotenvEditorFromRegistry()
}, { deep: true })

watch(advancedModeEnabled, (enabled) => {
  if (enabled) {
    syncDotenvEditorFromRegistry()
    scheduleComposeParse()
  }
})
</script>

<style scoped>
.app-deploy-workbench {
  --resource-accent: var(--color-primary-500);
}

.app-deploy-page {
  box-sizing: border-box;
  width: 100%;
  height: 100%;
  min-height: 0;
  padding: 0 0 22px;
  display: flex;
  flex-direction: column;
}

.back-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 38px;
  padding: 0 12px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: 9px;
  font-size: 0.8125rem;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.back-btn:hover {
  border-color: var(--color-primary-300);
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

.page-title {
  min-width: 0;
  overflow: hidden;
  font-size: 0.9375rem;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 加载/错误状态 */
.loading-state,
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  flex: 1;
  gap: 16px;
  color: var(--text-secondary);
}

.spinner {
  width: 40px;
  height: 40px;
  border: 3px solid var(--border-subtle);
  border-top-color: var(--color-primary-500);
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

.btn .spinner {
  width: 14px;
  height: 14px;
  border-width: 2px;
}

.error-state :deep(svg) {
  stroke: var(--color-danger-500);
}

.error-state h3 {
  font-size: 1rem;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
}

.error-state p {
  margin: 0;
  font-size: 0.8125rem;
}

/* 左右布局 - 左侧两列卡片，右侧环境变量 */
.deploy-layout {
  display: grid;
  grid-template-columns: 320px minmax(0, 1fr);
  gap: 16px;
  flex: 1;
  overflow: hidden;
  min-height: 0;
  align-items: stretch;
}

.left-panel,
.right-panel {
  min-height: 0;
  overflow-y: auto;
}

.left-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding-right: 4px;
}

/* 卡片样式 */
.section-card {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  padding: 16px;
}

/* 字段包装器 */
.field-wrapper {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.field-label {
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--text-primary);
}

.required {
  color: var(--color-danger-500);
  margin-left: 2px;
}

.field-input {
  width: 100%;
  padding: 8px 10px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--bg-primary);
  color: var(--text-primary);
  font-size: 0.8125rem;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.field-input:focus {
  outline: none;
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.field-input.readonly {
  background: var(--bg-tertiary);
  color: var(--text-tertiary);
}

.field-hint {
  font-size: 0.75rem;
  color: var(--text-tertiary);
  margin: 0;
}

.service-list-header {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 12px;
}

.service-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.service-list-item,
.quick-entry-btn {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px;
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  background: var(--bg-secondary);
  text-align: left;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.service-list-item:hover,
.service-list-item.active,
.quick-entry-btn:hover {
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.quick-entry-card.active {
  border-color: var(--color-primary-200);
}

.service-list-main,
.quick-entry-main {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.service-list-name,
.quick-entry-title {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--text-primary);
}

.service-list-desc,
.quick-entry-desc {
  font-size: 0.75rem;
  color: var(--text-tertiary);
}

.yaml-editor-hint {
  margin-bottom: 10px;
}

/* 服务头部 */
.service-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-bottom: 12px;
}

.service-header.collapsible {
  cursor: pointer;
  user-select: none;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.section-card.collapsed .service-header {
  padding-bottom: 0;
}

.service-info {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.service-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.compact-select {
  width: 112px;
  min-height: 32px;
  padding: 5px 28px 5px 9px;
}

.add-config-btn {
  flex: none;
  min-height: 32px;
}

.service-name {
  font-size: 1rem;
  font-weight: 600;
  color: var(--text-primary);
}

.service-count {
  font-size: 0.75rem;
  padding: 2px 8px;
  background: var(--color-primary-100);
  color: var(--color-primary-700);
  border-radius: 10px;
  font-weight: 500;
}

.service-advanced-count {
  font-size: 0.75rem;
  padding: 2px 8px;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  border-radius: 10px;
  font-weight: 500;
  white-space: nowrap;
  flex-shrink: 0;
}

.section-toggle-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  border: none;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.section-toggle-btn:hover {
  background: var(--border-medium);
  color: var(--text-primary);
}

.section-toggle-arrow {
  color: inherit;
  transition: transform var(--motion-duration-quick) var(--motion-ease-out);
}

.section-toggle-arrow.rotated {
  transform: rotate(180deg);
}

.section-content {
  margin-top: 12px;
}

.section-content.no-margin {
  margin-top: 0;
}

/* 配置区域 */
.config-section {
  background: var(--bg-secondary);
  border-radius: 10px;
  overflow: hidden;
}

.config-section + .config-section {
  margin-top: 12px;
}

/* 区域标题栏 */
.section-title-bar {
  display: flex;
  align-items: center;
  padding: 10px 14px;
  background: var(--bg-tertiary);
  border-bottom: 1px solid var(--border-subtle);
}

.section-title-bar.collapsible {
  cursor: pointer;
  user-select: none;
  transition: background var(--motion-duration-quick) var(--motion-ease-out);
}

.section-title-bar.collapsible:hover {
  background: var(--border-subtle);
}

.title-left {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
}

.title-icon {
  color: var(--text-secondary);
}

.title-text {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--text-primary);
}

.badge {
  font-size: 0.75rem;
  padding: 1px 6px;
  background: var(--color-primary-500);
  color: var(--text-inverse);
  border-radius: 10px;
  font-weight: 500;
}

.toggle-arrow {
  color: var(--text-secondary);
  transition: transform var(--motion-duration-quick) var(--motion-ease-out);
}

.toggle-arrow.rotated {
  transform: rotate(180deg);
}

/* 高级配置特殊样式 - 仅图标变色 */
.config-section.advanced .title-icon {
  color: var(--color-primary-500);
}

/* 配置行列表 */
.config-rows {
  padding: 8px;
}

.config-row {
  display: grid;
  grid-template-columns: 70px 1fr 36px 1fr;
  gap: 8px;
  align-items: center;
  padding: 8px;
  border-radius: 6px;
  transition: background var(--motion-duration-quick) var(--motion-ease-out);
}

.config-row:hover {
  background: var(--bg-tertiary);
}

.config-row + .config-row {
  border-top: 1px solid var(--border-subtle);
  margin-top: 4px;
}

/* 类型标签 */
.type-tag {
  display: flex;
  align-items: center;
  justify-content: center;
}

.type-tag .tag {
  font-size: 0.75rem;
  padding: 2px 8px;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  border-radius: 4px;
  font-weight: 500;
  border: 1px solid var(--border-subtle);
}

/* 参数名列 */
.param-name-col,
.param-value-col {
  display: flex;
  flex-direction: column;
  justify-content: center;
}

/* 输入包装器 */
.input-wrapper {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.mapping-target-wrapper {
  position: relative;
  flex-direction: row;
  align-items: center;
  gap: 8px;
}

.mapping-target-wrapper .field-input {
  min-width: 0;
}

.mapping-remove-btn {
  flex: 0 0 30px;
  width: 30px;
  height: 30px;
}

.field-input:disabled {
  cursor: not-allowed;
  color: var(--text-tertiary);
  background: var(--bg-tertiary);
}

.input-label {
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--text-primary);
  display: flex;
  align-items: center;
  gap: 6px;
}

.help-tooltip {
  color: var(--text-tertiary);
  cursor: help;
  display: inline-flex;
  align-items: center;
}

/* 箭头列 */
.arrow-col {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-tertiary);
}

/* 无配置提示 */
.no-config {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 48px;
  color: var(--text-secondary);
  gap: 12px;
}

.no-config.compact {
  padding: 18px;
  gap: 8px;
  text-align: center;
}

.panel-empty {
  min-height: 180px;
  background: var(--bg-primary);
  border: 1px dashed var(--border-medium);
  border-radius: 12px;
}

.no-config :deep(svg) {
  stroke: var(--color-success-500);
}

.right-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.advanced-yaml-card .service-count {
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

.service-config-card {
  flex: 1;
}

/* 信息卡片 */
.info-card {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  padding: 16px;
  display: flex;
  gap: 12px;
}

.app-info {
  flex: 1;
  min-width: 0;
}

.app-info h3 {
  font-size: 1rem;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 6px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.app-desc {
  font-size: 0.8125rem;
  color: var(--text-secondary);
  margin: 0 0 10px;
  line-height: 1.5;
  max-height: 7.5em;
  overflow-y: auto;
  overflow-wrap: anywhere;
  overscroll-behavior: contain;
  scrollbar-width: none;
  white-space: pre-line;
}

.app-desc::-webkit-scrollbar {
  display: none;
}

.app-tags {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.tag {
  font-size: 0.75rem;
  padding: 2px 8px;
  border-radius: 6px;
  font-weight: 500;
}

.tag.category {
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

.tag.version {
  background: var(--bg-tertiary);
  color: var(--text-tertiary);
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
}

/* 警告框 */
.warnings-box {
  background: var(--color-warning-50);
  border: 1px solid var(--color-warning-200);
  border-radius: 10px;
  padding: 12px 14px;
}

.warning-title {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--color-warning-700);
  font-size: 0.8125rem;
  font-weight: 500;
  margin-bottom: 8px;
}

.warnings-box ul {
  margin: 0;
  padding-left: 18px;
  color: var(--color-warning-800);
  font-size: 0.8125rem;
  line-height: 1.6;
}

.warnings-box li {
  margin: 4px 0;
}

/* 侧边卡片 */
.side-card {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  overflow: hidden;
}

.side-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 14px;
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--text-primary);
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border-subtle);
}

.side-header.collapsible {
  cursor: pointer;
  user-select: none;
}

.toggle-icon {
  margin-left: auto;
  color: var(--text-secondary);
  transition: transform var(--motion-duration-quick) var(--motion-ease-out);
}

.toggle-icon.rotated {
  transform: rotate(180deg);
}

/* Dotenv 编辑器 */
.dotenv-editor {
  padding: 10px 12px;
}

.yaml-actions {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 10px;
}

.yaml-editor {
  padding: 0 12px 12px;
}

.dotenv-textarea {
  width: 100%;
  padding: 10px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--bg-primary);
  color: var(--text-primary);
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 0.8125rem;
  line-height: 1.5;
  resize: vertical;
  min-height: 140px;
}

.dotenv-textarea:focus {
  outline: none;
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

/* 操作栏 */
.action-bar {
  position: sticky;
  bottom: 0;
  z-index: 2;
  display: flex;
  gap: 10px;
  padding: 10px;
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  background: color-mix(in srgb, var(--bg-elevated) 94%, transparent);
}

.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px 14px;
  border-radius: 8px;
  font-size: 0.8125rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  border: none;
  flex: 1;
}

.btn.small {
  padding: 6px 10px;
  font-size: 0.75rem;
}

.btn.primary {
  background: var(--color-primary-500);
  color: var(--text-inverse);
}

.btn.primary:hover:not(:disabled) {
  background: var(--color-primary-600);
}

.btn.secondary {
  background: var(--bg-tertiary);
  color: var(--text-primary);
  border: 1px solid var(--border-subtle);
}

.btn.secondary:hover {
  background: var(--bg-secondary);
  border-color: var(--border-medium);
}

.btn.danger {
  background: var(--bg-elevated);
  color: #b0705c;
  border: 1px solid #e3c3b3;
}

.btn.danger:hover:not(:disabled) {
  background: #f6e8e0;
  border-color: #ddbbaa;
}

.btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

/* 部署日志 */
.deploy-logs {
  min-height: 300px;
  max-height: 500px;
}

.logs-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 300px;
  color: var(--text-secondary);
  gap: 16px;
}

.logs-content {
  height: 400px;
  padding: 16px;
}

.log-line {
  display: flex;
  gap: 8px;
  padding: 4px 0;
  color: var(--text-secondary);
  word-break: break-all;
}

.log-time {
  color: var(--text-tertiary);
  flex-shrink: 0;
}

.log-icon {
  flex-shrink: 0;
}

.log-line.error {
  color: var(--color-danger-500);
}

.log-line.success,
.log-line.result {
  color: var(--color-success-600);
}

.log-line.warning {
  color: var(--color-warning-600);
}

/* 响应式 */
@media (max-width: 1024px) {
  .deploy-layout {
    grid-template-columns: 1fr;
    overflow-y: auto;
  }

  .left-panel,
  .right-panel {
    overflow: visible;
  }

  .config-row {
    grid-template-columns: 1fr;
    gap: 8px;
  }

  .type-tag,
  .arrow-col {
    display: none;
  }

  .service-list-item,
  .quick-entry-btn {
    padding: 10px;
  }

  .service-header {
    align-items: flex-start;
    gap: 10px;
  }

  .service-actions {
    flex-wrap: wrap;
  }
}
</style>
