<template>
  <Modal v-model:visible="dialogVisible" title="Docker 设置" width="700px">
    <!-- 自定义 Tabs -->
    <div class="tabs">
      <div class="tab-headers">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          class="tab-btn"
          :class="{ active: activeTab === tab.key }"
          @click="activeTab = tab.key"
        >
          {{ tab.label }}
        </button>
      </div>
      
      <!-- 注册表 -->
      <div v-if="activeTab === 'registry'" class="tab-content">
        <div class="section-header">
          <button class="btn btn-primary" @click="addRegistry">添加注册表</button>
        </div>
        
        <div class="registry-table">
          <table>
            <thead>
              <tr>
                <th>名称</th>
                <th>地址</th>
                <th>用户名</th>
                <th>密码</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="reg in registryList" :key="reg.key">
                <td>
                  <input 
                    v-model="reg.name" 
                    class="form-input" 
                    placeholder="名称"
                    :disabled="reg.key === 'docker.io'"
                  />
                </td>
                <td>
                  <input 
                    v-model="reg.url" 
                    class="form-input" 
                    placeholder="地址"
                    :disabled="reg.key === 'docker.io'"
                  />
                </td>
                <td>
                  <input v-model="reg.username" class="form-input" placeholder="可选" />
                </td>
                <td>
                  <input v-model="reg.password" type="password" class="form-input" placeholder="可选" />
                </td>
                <td>
                  <button 
                    v-if="reg.key !== 'docker.io'"
                    class="btn btn-text danger"
                    @click="removeRegistry(reg.key)"
                  >
                    删除
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        
        <div class="form-actions">
          <button class="btn btn-primary" @click="saveRegistries">保存注册表</button>
        </div>
      </div>
      
      <!-- 镜像加速 -->
      <div v-if="activeTab === 'mirrors'" class="tab-content">
        <div class="form-group">
          <label class="form-label">Docker Hub 加速地址</label>
          <textarea
            v-model="mirrorForm.mirrors"
            rows="4"
            class="form-textarea"
            data-tour="mirror-input"
            placeholder="每行一个 Docker Hub 加速地址"
          />
        </div>
        
        <RegistryMirrorList v-if="dialogVisible" :hub-mirrors="mirrorForm.mirrors"
          :environment-id="environmentId" @add="addMirror" />
        
        <div class="form-actions">
          <button class="btn btn-primary" @click="saveMirrors">保存 Docker Hub 配置</button>
        </div>
      </div>
      
      <!-- 代理设置 -->
      <div v-if="activeTab === 'proxy'" class="tab-content">
        <div class="form-group">
          <label class="checkbox-label">
            <input v-model="proxyForm.enabled" type="checkbox" />
            <span>启用代理</span>
          </label>
        </div>
        
        <template v-if="proxyForm.enabled">
          <div class="form-group">
            <label class="form-label">HTTP 代理</label>
            <input v-model="proxyForm.http" class="form-input" placeholder="例如: http://192.168.1.1:7890" />
          </div>
          <div class="form-group">
            <label class="form-label">HTTPS 代理</label>
            <input v-model="proxyForm.https" class="form-input" placeholder="例如: http://192.168.1.1:7890" />
          </div>
          <div class="form-group">
            <label class="form-label">不代理的地址</label>
            <input v-model="proxyForm.no" class="form-input" placeholder="例如: localhost,127.0.0.1,.local" />
          </div>
        </template>
        
        <div class="form-actions">
          <button class="btn btn-primary" @click="saveProxy">保存</button>
        </div>
      </div>
    </div>
  </Modal>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { api } from '@/utils/request.js'
import { getRegistries, updateRegistries } from '@/api/imageRegistry.js'
import Modal from '@/components/feedback/Modal.vue'
import { useUiStore } from '@/stores/ui.js'
import { useEnvironmentContext } from '@/composables/useEnvironmentContext.js'
import RegistryMirrorList from './RegistryMirrorList.vue'

const props = defineProps({
  modelValue: {
    type: Boolean,
    default: false
  },
  initialTab: {
    type: String,
    default: 'registry',
    validator: (value) => ['registry', 'mirrors', 'proxy'].includes(value)
  }
})

const emit = defineEmits(['update:modelValue'])
const uiStore = useUiStore()
const { environmentId } = useEnvironmentContext()

const dialogVisible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})

const tabs = [
  { key: 'registry', label: '注册表' },
  { key: 'mirrors', label: '镜像加速' },
  { key: 'proxy', label: '代理设置' }
]

const activeTab = ref(props.initialTab)

// 注册表
const registryForm = ref({
  registries: {
    'docker.io': {
      name: 'Docker Hub',
      url: 'docker.io',
      username: '',
      password: ''
    }
  }
})

const registryList = computed(() => {
  return Object.entries(registryForm.value.registries).map(([key, value]) => ({
    key,
    ...value
  }))
})

const mirrorForm = ref({
  mirrors: ''
})

const proxyForm = ref({
  enabled: false,
  http: '',
  https: '',
  no: ''
})

// 添加镜像到列表
const addMirror = (url) => {
  const current = mirrorForm.value.mirrors
    .split('\n')
    .map(line => line.trim())
    .filter(line => line)
  if (!current.includes(url)) {
    current.push(url)
    mirrorForm.value.mirrors = current.join('\n')
    uiStore.toastSuccess('已添加')
  } else {
    uiStore.toastWarning('该镜像已存在')
  }
}

// 添加注册表
const addRegistry = () => {
  const key = 'registry-' + Date.now()
  registryForm.value.registries[key] = {
    name: '',
    url: '',
    username: '',
    password: ''
  }
}

// 删除注册表
const removeRegistry = (key) => {
  if (key === 'docker.io') {
    uiStore.toastWarning('不能删除默认仓库')
    return
  }
  delete registryForm.value.registries[key]
}

// 保存注册表
const saveRegistries = async () => {
  try {
    // 验证
    for (const [key, reg] of Object.entries(registryForm.value.registries)) {
      if (!reg.name.trim()) {
        uiStore.toastWarning('注册表名称不能为空')
        return
      }
      if (!reg.url.trim()) {
        uiStore.toastWarning('注册表地址不能为空')
        return
      }
    }
    
    await updateRegistries({ registries: registryForm.value.registries })
    uiStore.toastSuccess('注册表配置已保存')
    dialogVisible.value = false
  } catch (error) {
    console.error('保存注册表失败:', error)
    uiStore.toastError('保存失败: ' + (error.message || '未知错误'))
  }
}

// 加载配置
const loadSettings = async () => {
  try {
    // 加载代理和镜像加速配置
    const proxyData = await api.images.getProxy()
    if (proxyData) {
      // 镜像加速
      if (proxyData['registry-mirrors']) {
        mirrorForm.value.mirrors = proxyData['registry-mirrors'].join('\n')
      }
      // 代理
      proxyForm.value.enabled = proxyData.enabled || false
      proxyForm.value.http = proxyData['HTTP Proxy'] || ''
      proxyForm.value.https = proxyData['HTTPS Proxy'] || ''
      proxyForm.value.no = proxyData['No Proxy'] || ''
    }
    
    // 加载注册表配置
    try {
      const registriesData = await getRegistries()
      if (registriesData && registriesData.registries) {
        // 确保默认仓库存在
        if (!registriesData.registries['docker.io']) {
          registriesData.registries['docker.io'] = {
            name: 'Docker Hub',
            url: 'docker.io',
            username: '',
            password: ''
          }
        }
        registryForm.value.registries = registriesData.registries
      }
    } catch (e) {
      console.log('加载注册表配置失败，使用默认值')
    }
  } catch (error) {
    console.error('加载设置失败:', error)
  }
}

// 保存镜像加速
const saveMirrors = async () => {
  try {
    const mirrors = mirrorForm.value.mirrors
      .split('\n')
      .map(line => line.trim())
      .filter(line => line)
    
    await api.images.updateProxy({ 
      'registry-mirrors': mirrors 
    })
    uiStore.toastSuccess('镜像加速配置已保存，请重启 Docker 服务以生效')
    dialogVisible.value = false
  } catch (error) {
    console.error('保存失败:', error)
    uiStore.toastError('保存失败: ' + (error.message || '未知错误'))
  }
}

// 保存代理设置
const saveProxy = async () => {
  try {
    const config = {
      enabled: proxyForm.value.enabled,
      'HTTP Proxy': proxyForm.value.enabled ? proxyForm.value.http : '',
      'HTTPS Proxy': proxyForm.value.enabled ? proxyForm.value.https : '',
      'No Proxy': proxyForm.value.enabled ? proxyForm.value.no : ''
    }
    await api.images.updateProxy(config)
    uiStore.toastSuccess('代理配置已保存，请重启 Docker 服务以生效')
    dialogVisible.value = false
  } catch (error) {
    console.error('保存失败:', error)
    uiStore.toastError('保存失败: ' + (error.message || '未知错误'))
  }
}

watch(dialogVisible, (val) => {
  if (val) {
    activeTab.value = props.initialTab
    loadSettings()
  }
})

onMounted(() => {
  if (dialogVisible.value) {
    loadSettings()
  }
})
</script>

<style scoped>
/* Tabs */
.tabs {
  margin-top: -16px;
}

.tab-headers {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--border-subtle);
  margin-bottom: 20px;
}

.tab-btn {
  padding: 12px 20px;
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  color: var(--text-secondary);
  font-size: 0.875rem;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.tab-btn:hover {
  color: var(--text-primary);
}

.tab-btn.active {
  color: var(--color-primary-600);
  border-bottom-color: var(--color-primary-500);
  font-weight: 500;
}

.tab-content {
  padding: 4px 0;
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
.form-textarea {
  width: 100%;
  padding: 10px 12px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--bg-secondary);
  color: var(--text-primary);
  font-size: 0.875rem;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.form-input:focus,
.form-textarea:focus {
  outline: none;
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.form-textarea {
  resize: vertical;
  min-height: 100px;
  font-family: 'JetBrains Mono', monospace;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 0.875rem;
  color: var(--text-primary);
  cursor: pointer;
}

.checkbox-label input[type="checkbox"] {
  width: 16px;
  height: 16px;
}

/* 按钮 */
.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 36px;
  padding: 0 16px;
  border-radius: 8px;
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  border: none;
}

.btn-primary {
  background: linear-gradient(135deg, var(--color-primary-500), var(--color-primary-600));
  color: var(--text-inverse);
}

.btn-primary:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px var(--color-primary-500-30);
}

.btn-text {
  background: transparent;
  color: var(--text-secondary);
  padding: 4px 8px;
  height: auto;
}

.btn-text:hover {
  color: var(--color-danger-600);
}

.btn-text.danger {
  color: var(--color-danger-600);
}

.btn-text.danger:hover {
  background: var(--color-danger-100);
}

/* 区域头部 */
.section-header {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 16px;
}

/* 表格 */
.registry-table {
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  overflow: hidden;
  margin-bottom: 16px;
}

.registry-table table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.875rem;
}

.registry-table th {
  background: var(--bg-tertiary);
  padding: 10px 12px;
  text-align: left;
  font-weight: 600;
  color: var(--text-secondary);
  border-bottom: 1px solid var(--border-subtle);
}

.registry-table td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--border-subtle);
}

.registry-table tr:last-child td {
  border-bottom: none;
}

.registry-table .form-input {
  height: 32px;
  padding: 0 8px;
  font-size: 0.8125rem;
}

/* 操作按钮 */
.form-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 20px;
  padding-top: 16px;
  border-top: 1px solid var(--border-subtle);
}

</style>
