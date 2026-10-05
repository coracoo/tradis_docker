<template>
  <ResourceWorkbench class="compose-page compose-workbench" remote-context>
    <template #rail>
      <section class="resource-rail" aria-label="Compose 状态概览">
        <article v-for="item in composeRail" :key="item.label" class="rail-card" :class="item.tone">
          <span class="rail-icon">
            <DynamicIcon :name="item.icon" :size="18" />
          </span>
          <div class="rail-copy">
            <strong>{{ item.value }}</strong>
            <span>{{ item.label }}</span>
          </div>
          <small>{{ item.note }}</small>
        </article>
      </section>
    </template>

    <!-- 工具栏 -->
    <template #toolbar>
      <div class="toolbar workbench-toolbar">
      <div class="toolbar-left workbench-toolbar-left">
        <SearchInput
          v-model="searchQuery"
          placeholder="搜索项目或容器名称..."
          @search="handleSearch"
        />
        <div class="sort-control">
          <DynamicIcon name="arrow-up-down" :size="16" />
          <select v-model="sortField" aria-label="Compose 项目排序字段">
            <option value="">默认排序</option>
            <option value="name">名称</option>
            <option value="status">运行状态</option>
            <option value="containerCount">容器数量</option>
            <option value="updateCount">可更新数量</option>
            <option value="createTime">创建时间</option>
            <option value="path">项目路径</option>
          </select>
          <button
            class="sort-direction-btn"
            :class="{ 'is-placeholder': !sortField }"
            :disabled="!sortField"
            :aria-hidden="!sortField"
            :title="sortState.order === 'ascending' ? '当前升序，点击切换降序' : '当前降序，点击切换升序'"
            @click="toggleSortDirection"
          >
            <DynamicIcon :name="sortState.order === 'ascending' ? 'arrow-up' : 'arrow-down'" :size="15" />
          </button>
        </div>
        <SegmentedTabs
          v-model="statusFilter"
          :options="statusTabs"
          aria-label="状态筛选"
          compact
          @change="currentPage = 1"
        />
      </div>
      <div class="toolbar-right">
        <template v-if="viewMode !== 'grid' && batchTargets.length > 0 && batchTargets.every(canProjectLifecycle)">
          <button
            v-ripple
            class="primary-btn"
            type="button"
            :disabled="batchWorking"
            title="批量启动已勾选的项目和独立容器"
            @click="batchLifecycle('start', '启动')"
          >
            <DynamicIcon name="play" :size="16" />
            批量启动（{{ batchTargets.length }}）
          </button>
          <button
            v-ripple
            class="secondary-btn"
            type="button"
            :disabled="batchWorking"
            title="批量停止已勾选的项目和独立容器"
            @click="batchLifecycle('stop', '停止')"
          >
            <DynamicIcon name="stop" :size="16" />
            批量停止（{{ batchTargets.length }}）
          </button>
        </template>
        <button v-ripple class="secondary-btn" data-remote-write @click="showGitImportDialog = true">
          <DynamicIcon name="github" :size="16" />
          Git 导入
        </button>
        <ViewToggle v-model="viewMode" />
        <button
          v-ripple
          class="icon-btn resource-refresh-btn"
          :class="{ 'is-spinning': composeBusy }"
          :disabled="composeBusy"
          title="刷新项目列表"
          @click="handleRefresh"
        >
          <DynamicIcon class="resource-refresh-icon" name="refresh" :size="18" />
        </button>
        <button v-ripple class="primary-btn" :disabled="!canCreateCompose" data-tour="compose-create" @click="openCreateDialog">
          <DynamicIcon name="plus" :size="16" />
          新建项目
        </button>
      </div>
      </div>
    </template>

    <!-- 网格视图 -->
    <ResourceBoard>
      <ResourcePanel>
          <CardGrid
            v-if="viewMode === 'grid'"
            :items="paginatedProjects"
            :loading="loading"
            :skeleton-count="12"
            :empty-props="emptyProps"
          >
            <template #default="{ items }">
              <ComposeCard
                v-for="project in items"
                :key="composeProjectIdentity(project)"
                :project="project"
                :selected="sameComposeProject(selectedSummaryProject, project)"
                :processing-action="getProjectProcessingAction(project)"
                @click="handleSelect(project)"
                @start="handleStart"
                @stop="handleStop"
                @restart="handleRestart"
                @down="handleDown"
                @remove="project.type === 'container' ? handleRemoveStandalone : handleRemove"
                @destroy="handleDestroy"
                @update="handleUpdate"
                @sync-git="handleGitSync"
                @edit="handleEdit"
                @remark="openRemarkEditor"
                @logs="openProjectLogs"
                @terminal="handleTerminal"
                @force-stop="handleForceStop"
              />
            </template>
          </CardGrid>

          <!-- 列表视图 -->
          <ProjectList
            v-else
            :projects="paginatedProjects"
            :loading="loading"
            :selected-name="composeProjectIdentity(selectedSummaryProject)"
            :sort-state="sortState"
            :processing-actions="processingProjects"
            :checked-ids="checkedProjectIds"
            @select="handleSelect"
            @sort="handleSort"
            @action="handleListAction"
            @toggle-check="toggleProjectCheck"
            @toggle-check-all="toggleProjectCheckAll"
          />
        <template #footer>
          <Pagination
            v-model:current="currentPage"
            v-model:page-size="pageSize"
            :total="filteredProjects.length"
          />
        </template>
      </ResourcePanel>

      <template #context>
        <ResourceContextPanel
          class="compose-context-panel"
          :motion-key="selectedSummaryProject ? composeProjectIdentity(selectedSummaryProject) : 'overview'"
          aria-label="Compose 上下文"
          @click.stop
        >
          <div class="context-header">
            <span class="resource-kind compose">
              <DynamicIcon name="compose" :size="18" />
            </span>
            <div>
              <strong>{{ selectedSummaryProject ? selectedSummaryProject.name : 'Compose 概览' }}</strong>
              <span v-if="selectedSummaryProject">{{ selectedSummaryProject.path || '-' }}</span>
              <span v-else>{{ filteredProjects.length }} 个匹配项目</span>
            </div>
          </div>

          <div v-if="selectedSummaryProject" class="context-actions">
            <button v-if="!isRemoteEnvironment" type="button" class="detail-action info" @click="openRemarkEditor(selectedSummaryProject)">
              <DynamicIcon name="edit" :size="15" />
              编辑备注
            </button>
            <button type="button" class="detail-action info" @click="openSelectedProjectDrawer">
              <DynamicIcon name="container" :size="15" />
              容器详情
            </button>
            <button v-if="canProjectLogs(selectedSummaryProject)" type="button" class="detail-action info" @click="openProjectLogs(selectedSummaryProject)">
              <DynamicIcon name="file-text" :size="15" />
              {{ selectedSummaryProject.type === 'container' ? '容器日志' : '项目日志' }}
            </button>
            <button v-if="!isRemoteEnvironment && selectedSummaryProject.type !== 'container'" type="button" class="detail-action info" @click="openComposeHistory(selectedSummaryProject)">
              <DynamicIcon name="history" :size="15" />
              配置历史
            </button>
            <button v-if="!isRemoteEnvironment && selectedSummaryProject.type !== 'container'" type="button" class="detail-action info" @click="openProjectProtection(selectedSummaryProject)">
              <DynamicIcon name="shield" :size="15" />
              应用保护
            </button>
            <button v-if="composeSafeUpdatesEnabled && !isRemoteEnvironment && selectedSummaryProject.type !== 'container'" type="button" class="detail-action warning" @click="openSafeUpdate(selectedSummaryProject)">
              <DynamicIcon name="shield-check" :size="15" />
              安全更新
            </button>
            <button
              v-if="canProjectLifecycle(selectedSummaryProject) && getStatusText(selectedSummaryProject) === '运行中'"
              type="button"
              class="detail-action stop"
              @click="handleStop(selectedSummaryProject)"
            >
              <DynamicIcon name="stop" :size="15" />
              停止
            </button>
            <button
              v-if="canProjectLifecycle(selectedSummaryProject) && getStatusText(selectedSummaryProject) !== '运行中'"
              type="button"
              class="detail-action success"
              @click="handleStart(selectedSummaryProject)"
            >
              <DynamicIcon name="play" :size="15" />
              启动
            </button>
          </div>

          <div class="context-grid">
            <div>
              <span>服务</span>
              <strong>{{ selectedSummaryProject ? runningProjectContainers(selectedSummaryProject) : composeCounts.runningContainers }}/{{ selectedSummaryProject ? totalProjectContainers(selectedSummaryProject) : composeCounts.totalContainers }}</strong>
            </div>
            <div>
              <span>已运行时间</span>
              <strong>{{ selectedSummaryProject ? projectUptimeLabel(selectedSummaryProject) : '-' }}</strong>
            </div>
            <div>
              <span>端口</span>
              <strong :title="selectedSummaryProject ? projectPortsTitle(selectedSummaryProject) : ''">{{ selectedSummaryProject ? dedupeProjectPorts(selectedSummaryProject.containers) : composeCounts.exposedPorts }}</strong>
            </div>
            <div>
              <span>可更新</span>
              <strong>{{ selectedSummaryProject ? (selectedSummaryProject.updateCount || 0) : composeCounts.updateCount }}</strong>
            </div>
          </div>

          <div v-if="selectedSummaryProject" class="context-section selected-detail">
            <p class="section-label">容器运行时间</p>
            <div class="detail-lines uptime-lines">
              <span v-for="container in selectedSummaryProject.containers || []" :key="container.Id || getContainerName(container)">
                <b :title="getContainerName(container)">{{ getContainerName(container) }}</b>
                <div class="uptime-cell">
                  {{ formatContainerUptimeCn(container) }}
                  <em v-if="hasContainerUpdate(container)" class="update-tag" title="镜像可更新">更新</em>
                </div>
              </span>
              <span v-if="!selectedSummaryProject.containers?.length">
                <b>容器</b>
                -
              </span>
            </div>
          </div>

          <ComposeLogPreview
            v-if="!selectedSummaryProject || canProjectLogs(selectedSummaryProject)"
            class="context-section log-stream-preview"
            :project="selectedSummaryProject"
            expandable
            @open="openProjectLogs"
          />
        </ResourceContextPanel>
      </template>
    </ResourceBoard>

    <!-- 编辑/部署对话框 -->
    <EditDialog
      ref="editDialogRef"
      v-model:visible="showEditDialog"
      :is-edit="isEdit"
      :initial-data="editFormData"
      :project-root="projectRoot"
      :existing-names="projectsList.map(project => project.name)"
      :remote-deployment="isRemoteEnvironment"
      @saved="onProjectSaved"
      @deployed="onProjectDeployed"
      @open-ai="openAiDialog"
      @background="onBackgroundDeploy"
      @task-started="payload => onDeployTaskStarted(payload, 'deploy')"
    />

    <ComposeHistoryDialog
      v-model:visible="showHistoryDialog"
      :project-name="historyProjectName"
      @restored="onComposeHistoryRestored"
    />

    <GitImportDialog
      v-model:visible="showGitImportDialog"
      :existing-names="projectsList.map(project => project.name)"
      @task-started="payload => onDeployTaskStarted(payload, 'git-import')"
    />

    <!-- AI生成对话框 -->
    <AiGenerateDialog
      v-if="composeAIEnabled"
      v-model:visible="showAiDialog"
      :current-yaml="currentYamlForAi"
      :current-env="currentEnvForAi"
      @generated="onAiGenerated"
      @insert-template="insertTemplate"
    />

    <!-- 更新对话框 -->
    <UpdateDialog
      v-model:visible="showUpdateDialog"
      :project-name="updateProjectName"
      @task-started="payload => onDeployTaskStarted(payload, 'update')"
      @completed="fetchProjects"
    />

    <SafeUpdateDialog
      v-if="composeSafeUpdatesEnabled"
      v-model:visible="showSafeUpdateDialog"
      :project-name="safeUpdateProjectName"
      @task-started="payload => onDeployTaskStarted(payload, 'safe-update')"
      @completed="fetchProjects"
    />

    <!-- 详情抽屉 -->
    <DetailDrawer
      v-model:visible="showDetailDrawer"
      width="750px"
    >
      <template #header-custom v-if="selectedProjectView">
        <div class="drawer-header-compose">
          <div class="drawer-header-main">
            <span class="drawer-header-label">项目名称：</span>
            <h3 class="drawer-header-name">{{ selectedProjectView.name }}</h3>
            <span 
              class="status-badge"
              :class="getProjectStatusClass(selectedProjectView)"
            >
              <span class="status-dot" :class="getProjectStatusClass(selectedProjectView)"></span>
              {{ getProjectStatusText(selectedProjectView) }}
            </span>
          </div>
          <div class="drawer-header-path">
            <DynamicIcon name="folder" :size="14" />
            <span class="drawer-header-label">项目路径：</span>
            <span class="path-value">{{ selectedProjectView.path }}</span>
          </div>
        </div>
      </template>

      <ContainerMiniList
        v-if="selectedProjectView"
        :containers="selectedProjectView.containers || []"
        :selected-id="selectedContainerId"
        :selected-container="selectedContainerView"
        :detail-loading="containerDetailLoading"
        :detail-error="containerDetailError"
        :processing-ids="processingContainers"
        @select="selectContainer"
        @retry-detail="retrySelectedContainerDetail"
        @action="handleContainerAction"
      />
    </DetailDrawer>

    <!-- 终端对话框 -->
    <Modal
      v-model:visible="showTerminalDialog"
      title="容器终端"
      width="80%"
      @close="onTerminalClose"
    >
      <ContainerTerminal
        v-if="showTerminalDialog && currentContainer"
        :container-id="currentContainer.Id"
      />
    </Modal>

    <!-- Compose 备注 -->
    <Modal
      v-model:visible="remarkVisible"
      :title="remarkProject?.type === 'container' ? '编辑容器备注' : '编辑 Compose 备注'"
      width="440px"
    >
      <label class="remark-field">
        <span>备注名</span>
        <input
          v-model="remarkDraft"
          class="remark-input"
          maxlength="128"
          autocomplete="off"
          placeholder="例如：家庭媒体中心"
          @keydown.enter.prevent="saveRemark"
        />
        <small>{{ [...remarkDraft].length }}/128</small>
      </label>
      <template #footer>
        <button type="button" class="secondary-btn" @click="remarkVisible = false">取消</button>
        <button type="button" class="primary-btn" :disabled="remarkSaving" data-testid="remark-save" @click="saveRemark">
          {{ remarkSaving ? '保存中...' : '保存' }}
        </button>
      </template>
    </Modal>

    <Modal
      v-model:visible="destroyVisible"
      :title="destroyProject ? `销毁 ${destroyProject.name}` : '销毁 Compose 项目'"
      width="640px"
      :show-close="!destroySubmitting"
      :close-on-overlay="false"
      :close-on-esc="!destroySubmitting"
      @close="resetDestroyDialog"
    >
      <div class="destroy-dialog" aria-live="polite">
        <div v-if="destroyLoading" class="destroy-dialog-state">
          <DynamicIcon name="loader-2" :size="20" class="destroy-spinner" />
          <span>正在核对项目资源...</span>
        </div>

        <div v-else-if="destroyError" class="destroy-dialog-state is-error">
          <DynamicIcon name="circle-alert" :size="20" />
          <div>
            <strong>无法生成完整销毁清单</strong>
            <span>{{ destroyError }}</span>
          </div>
          <button type="button" class="secondary-btn" @click="loadDestroyPreview">重试</button>
        </div>

        <template v-else-if="destroyInventory">
          <p class="destroy-warning">
            确认后将停止项目并永久删除以下专属资源。此操作不可恢复。
          </p>
          <section class="destroy-resource-section" aria-label="将删除的资源">
            <header>
              <span>将删除</span>
              <b>{{ destroySelected.length }}/{{ destroyInventory.delete?.length || 0 }}</b>
            </header>
            <div class="destroy-resource-list">
              <label v-for="item in destroyInventory.delete" :key="destroyResourceKey(item)" class="destroy-resource-row is-selectable">
                <input
                  v-model="destroySelected"
                  class="destroy-resource-selection"
                  data-testid="destroy-resource-selection"
                  type="checkbox"
                  :value="destroyResourceKey(item)"
                />
                <span class="destroy-resource-icon is-delete">
                  <DynamicIcon :name="destroyResourceIcon(item.kind)" :size="16" />
                </span>
                <div>
                  <strong>{{ item.name }}</strong>
                  <small>{{ destroyResourceLabel(item.kind) }}<template v-if="item.detail"> · {{ item.detail }}</template></small>
                </div>
              </label>
            </div>
          </section>

          <section v-if="destroyInventory.retain?.length" class="destroy-resource-section is-retained" aria-label="将保留的资源">
            <header>
              <span>安全保留</span>
              <b>{{ destroyInventory.retain.length }}</b>
            </header>
            <div class="destroy-resource-list">
              <div v-for="item in destroyInventory.retain" :key="destroyResourceKey(item)" class="destroy-resource-row">
                <span class="destroy-resource-icon is-retained">
                  <DynamicIcon :name="destroyResourceIcon(item.kind)" :size="16" />
                </span>
                <div>
                  <strong>{{ item.name }}</strong>
                  <small>{{ item.reason || destroyResourceLabel(item.kind) }}</small>
                </div>
              </div>
            </div>
          </section>
        </template>
      </div>
      <template #footer>
        <button type="button" class="secondary-btn" :disabled="destroySubmitting" @click="closeDestroyDialog">取消</button>
        <button
          type="button"
          class="destroy-confirm-btn"
          data-testid="destroy-confirm"
          :disabled="destroyLoading || destroySubmitting || !destroyInventory || !destroySelected.length || Boolean(destroyError)"
          @click="confirmDestroy"
        >
          <DynamicIcon name="trash-2" :size="16" />
          {{ destroySubmitting ? '正在提交...' : '确认销毁' }}
        </button>
      </template>
    </Modal>

    <Modal
      v-model:visible="showLogsDialog"
      :title="logsDialogTitle"
      width="80%"
      @close="onLogsClose"
    >
      <ContainerLogs
        v-if="currentLogSource?.id"
        :source="currentLogSource"
      />
    </Modal>
  </ResourceWorkbench>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch, nextTick, defineAsyncComponent } from 'vue'
import { composeProjectIdentity, findComposeProjectByIdentity, remoteComposeOperationName, sameComposeProject } from './utils/projectIdentity.js'
import { convergeComposeMutation } from './utils/refreshConvergence.js'
import { useRouter } from 'vue-router'
import { useDockerResourcesStore } from '@/stores/dockerResources.js'
import { remoteCapabilityAllowed, useEnvironmentContext } from '@/composables/useEnvironmentContext.js'
import { useUiStore } from '@/stores/ui.js'
import CardGrid from '@/components/data-display/CardGrid.vue'
import ComposeCard from '@/components/data-display/ComposeCard.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import ViewToggle from '@/components/ui/ViewToggle.vue'
import SegmentedTabs from '@/components/ui/SegmentedTabs.vue'
import Pagination from '@/components/ui/Pagination.vue'
import Modal from '@/components/feedback/Modal.vue'
import DetailDrawer from '@/components/feedback/DetailDrawer.vue'
import ContainerLogs from '@/components/container/ContainerLogs.vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import ComposeLogPreview from './components/ComposeLogPreview.vue'
import ResourceWorkbench from '@/components/resource-workbench/ResourceWorkbench.vue'
import ResourceBoard from '@/components/resource-workbench/ResourceBoard.vue'
import ResourcePanel from '@/components/resource-workbench/ResourcePanel.vue'
import ResourceContextPanel from '@/components/resource-workbench/ResourceContextPanel.vue'
import { compose, containers, system } from '@edition/api'
import { useViewMode } from '@/composables/useViewMode.js'
import { useSort } from '@/composables/useSort.js'
import { usePageAction } from '@/composables/usePageAction.js'
import { classifyComposeStatus, composeProjectStateLabel, formatContainerUptimeCn, formatElapsed } from '@/utils/resourceStatus.js'
import { notifyBatchResult } from '@/utils/batchFeedback.js'

// 子组件
import ProjectList from './components/ProjectList.vue'
import EditDialog from './components/EditDialog.vue'
import { AiGenerateDialog, SafeUpdateDialog } from '@edition/compose-feature-dialogs'
import UpdateDialog from './components/UpdateDialog.vue'
import ContainerMiniList from './components/ContainerMiniList.vue'
import GitImportDialog from './components/GitImportDialog.vue'
import ComposeHistoryDialog from './components/ComposeHistoryDialog.vue'
import { openComposeProtection } from '@edition/compose-protection'
import { composeAIEnabled, composeSafeUpdatesEnabled } from '@edition/compose-features'

const ContainerTerminal = defineAsyncComponent(() => import('@/components/container/ContainerTerminal.vue'))

const dockerResources = useDockerResourcesStore()
const { isRemoteEnvironment, environment } = useEnvironmentContext()

const canCreateCompose = computed(() => !isRemoteEnvironment.value || [
  'deployment.preflight', 'deployment.apply', 'deployment.inspect', 'deployment.cancel'
].every(capability => remoteCapabilityAllowed(environment.value, capability)))

function projectRemoteCapability(project, kind) {
  return `${project?.type === 'container' ? 'container' : 'compose'}.${kind}`
}

function canProjectLifecycle(project) {
  return !isRemoteEnvironment.value || remoteCapabilityAllowed(environment.value, projectRemoteCapability(project, 'manage'))
}

function canProjectLogs(project) {
  return !isRemoteEnvironment.value || remoteCapabilityAllowed(environment.value, projectRemoteCapability(project, 'logs'))
}
const uiStore = useUiStore()
const router = useRouter()
const remarkVisible = ref(false)
const remarkSaving = ref(false)
const remarkProject = ref(null)
const remarkDraft = ref('')
const destroyVisible = ref(false)
const destroyLoading = ref(false)
const destroySubmitting = ref(false)
const destroyProject = ref(null)
const destroyInventory = ref(null)
const destroyError = ref('')
const destroySelected = ref([])
let destroyPreviewSequence = 0

function getErrorMessage(error) {
  return error?.message || '未知错误'
}

function toastOperationError(action, error) {
  uiStore.toastError(`${action}失败: ${getErrorMessage(error)}`)
}

function confirmComposeAction({ title, message, type = 'warning', confirmText = '确定' }) {
  return uiStore.confirm({
    title,
    message,
    type,
    confirmText,
    cancelText: '取消'
  })
}

// 视图模式
const { viewMode } = useViewMode('compose_view_mode', 'table')

// 排序
const { sortState, handleSort, setSort } = useSort('sort_compose')
const sortField = computed({
  get: () => sortState.value.prop || '',
  set: value => setSort(value, sortState.value.order || 'ascending')
})

function toggleSortDirection() {
  if (!sortState.value.prop) return
  setSort(
    sortState.value.prop,
    sortState.value.order === 'ascending' ? 'descending' : 'ascending'
  )
}

// 状态
const loading = computed(() => {
  const resource = dockerResources.resources.composeProjects
  return resource.loading || resource.refreshing
})
const composeBusy = computed(() => loading.value)
const rawProjectsList = computed(() => dockerResources.composeProjectList)
const searchQuery = ref('')
const statusFilter = ref('all')
const currentPage = ref(1)
const pageSize = ref(20)

// 选中的项目
const selectedProject = ref(null)
const selectedContainer = ref(null)
const selectedContainerId = ref('')
const containerDetailsById = ref(new Map())
const containerDetailLoading = ref(false)
const containerDetailError = ref('')
const isRestoringDrawer = ref(false)

watch(selectedContainer, (container) => {
  selectedContainerId.value = container?.Id || ''
}, { flush: 'sync' })

// 对话框状态
const showEditDialog = ref(false)
const editDialogRef = ref(null)
const showTerminalDialog = ref(false)
const showLogsDialog = ref(false)
const showAiDialog = ref(false)
const showUpdateDialog = ref(false)
const showSafeUpdateDialog = ref(false)
const showHistoryDialog = ref(false)
const historyProjectName = ref('')
const historyProjectIdentity = ref('')
watch(showUpdateDialog, (visible) => {
  if (!visible && updateProjectName.value) {
    if (!hasActiveTaskForProject(updateProjectIdentity.value)) {
      stopProjectProcessing(updateProjectIdentity.value)
    }
    updateProjectName.value = ''
    updateProjectIdentity.value = ''
  }
})
watch(showSafeUpdateDialog, (visible) => {
  if (!visible && safeUpdateProjectName.value) {
    if (!hasActiveTaskForProject(safeUpdateProjectIdentity.value)) {
      stopProjectProcessing(safeUpdateProjectIdentity.value)
    }
    safeUpdateProjectName.value = ''
    safeUpdateProjectIdentity.value = ''
  }
})
const showGitImportDialog = ref(false)
const showDetailDrawer = ref(false)
const isEdit = ref(false)
const containerStatsMap = ref(new Map())
let resourceStatsTimer = null
let resourceStatsLoading = false

const projectsList = computed(() => rawProjectsList.value.map(project => ({
  ...project,
  ResourceStats: getProjectResourceStats(project),
  containers: (project.containers || []).map(container => ({
    ...container,
    ResourceStats: getContainerResourceStats(container)
  }))
})))

// 编辑表单数据
const editFormData = ref({})
const projectRoot = ref('${PROJECT_ROOT}')

// AI对话框当前值
const currentYamlForAi = ref('')
const currentEnvForAi = ref('')

// 更新对话框
const updateProjectName = ref('')
const updateProjectIdentity = ref('')
const safeUpdateProjectName = ref('')
const safeUpdateProjectIdentity = ref('')

// 当前操作的容器
const currentContainer = ref(null)
const currentLogSource = ref(null)
const logsDialogTitle = computed(() => currentLogSource.value?.type === 'compose' ? 'Compose 日志' : '容器日志')
const activeComposeTasks = ref(new Map())
let deployTaskPollTimer = null
const DEPLOY_TASK_STATE_KEY = 'tradis_compose_deploy_task'
const COMPOSE_BATCH_TASK_WAIT_TIMEOUT_MS = 5 * 60 * 1000
let composeViewActive = true

// 项目/容器操作中的加载状态
const processingProjects = ref(new Map())
const processingContainers = ref(new Set())

// 批量启动/停止：仅表格视图提供勾选，卡片视图不展示勾选。
// 勾选键为项目身份（名称+路径），同名多路径项目不会互相干扰；项目本身（isSelf）不可勾选。
const checkedProjectIds = ref([])
const batchWorking = ref(false)
const batchTargets = computed(() => filteredProjects.value.filter(project =>
  !project?.isSelf && checkedProjectIds.value.includes(composeProjectIdentity(project))
))

function toggleProjectCheck(id) {
  const next = new Set(checkedProjectIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  checkedProjectIds.value = [...next]
}

// 表头全选：勾选/清空当前页条目，不影响其它页已勾选的项目。
function toggleProjectCheckAll() {
  const visible = paginatedProjects.value
    .filter(project => !project?.isSelf)
    .map(project => composeProjectIdentity(project))
  const visibleSet = new Set(visible)
  const allChecked = visible.length > 0 && visible.every(id => checkedProjectIds.value.includes(id))
  checkedProjectIds.value = allChecked
    ? checkedProjectIds.value.filter(id => !visibleSet.has(id))
    : [...new Set([...checkedProjectIds.value, ...visible])]
}

// 刷新后剔除已不存在的勾选，避免对已删除条目执行批量操作。
function pruneCheckedProjects() {
  const existing = new Set(projectsList.value.map(project => composeProjectIdentity(project)))
  const next = checkedProjectIds.value.filter(id => existing.has(id))
  if (next.length !== checkedProjectIds.value.length) checkedProjectIds.value = next
}

// 批量操作只在前台等待有限时间；超时后交给现有后台任务跟踪继续处理。
async function waitForComposeTask(taskId) {
  const deadline = Date.now() + COMPOSE_BATCH_TASK_WAIT_TIMEOUT_MS
  while (composeViewActive) {
    const task = await compose.getTask(taskId)
    if (isTaskFinished(task?.status)) return { task, pending: false, detached: false }
    if (Date.now() >= deadline) return { task, pending: true, detached: false }
    await new Promise(resolve => setTimeout(resolve, 1000))
  }
  return { task: null, pending: true, detached: true }
}

// 批量启动/停止：串行逐项执行；独立容器走容器同步接口（返回真实结果），
// 受管项目走 Compose 任务接口并轮询任务终态，成功/失败计数按任务真实结果统计。
// 正在处理中的项目跳过，避免在途任务上叠加第二次操作。
async function batchLifecycle(action, actionLabel) {
  const targets = batchTargets.value
  if (batchWorking.value || targets.length === 0) return
  batchWorking.value = true
  let succeeded = 0
  const failures = []
  let skipped = 0
  let background = 0
  let detached = false
  for (const project of targets) {
    if (isProjectProcessing(project)) {
      skipped += 1
      continue
    }
    startProjectProcessing(project, action)
    let continuesInBackground = false
    try {
      if (project.type === 'container') {
        const container = project?.containers?.[0]
        if (!container?.Id) throw new Error('未找到容器 ID')
        if (container.isSelf) throw new Error('容器化部署模式下，不支持操作自身容器')
        await containers[action](container.Id)
      } else if (isRemoteEnvironment.value) {
        await compose[action](project.name)
      } else {
        const res = await compose[`${action}Task`](project.name)
        if (!res?.taskId) throw new Error('未获取到任务 ID')
        const waitResult = await waitForComposeTask(res.taskId)
        if (waitResult.detached) {
          detached = true
          break
        }
        if (waitResult.pending) {
          startDeployTaskPolling(res.taskId, project.name, action, {
            projectIdentity: composeProjectIdentity(project)
          })
          continuesInBackground = true
          background += 1
          continue
        }
        const task = waitResult.task
        const status = String(task?.status || '').toLowerCase()
        if (!['success', 'completed'].includes(status)) {
          throw new Error(task?.error || `任务未成功（状态：${task?.status || '未知'}）`)
        }
      }
      succeeded += 1
    } catch (error) {
      failures.push({ name: project.name, reason: error.message || '' })
    } finally {
      if (!continuesInBackground) stopProjectProcessing(project)
    }
  }
  batchWorking.value = false
  if (detached || !composeViewActive) return
  checkedProjectIds.value = []
  await fetchProjects({ force: true })
  const suffix = `${skipped > 0 ? `，跳过 ${skipped} 个（进行中）` : ''}${background > 0 ? `，后台继续 ${background} 个` : ''}`
  notifyBatchResult(uiStore, {
    label: `批量${actionLabel}`,
    succeeded,
    failures,
    pending: background,
    suffix
  })
}

// 空状态配置
const emptyProps = {
  title: '暂无项目',
  description: '点击"新建项目"按钮创建第一个 Compose 项目',
  icon: 'box'
}

// 状态筛选选项
const statusTabs = computed(() => [
  { label: '全部', value: 'all', count: projectsList.value.length },
  { label: '运行中', value: 'running', count: projectsList.value.filter(p => classifyComposeStatus(p) === 'running').length },
  { label: '其它', value: 'unhealthy', count: projectsList.value.filter(p => classifyComposeStatus(p) === 'unhealthy').length },
  { label: '已停止', value: 'stopped', count: projectsList.value.filter(p => classifyComposeStatus(p) === 'stopped').length }
])

const composeCounts = computed(() => {
  const total = projectsList.value.length
  const running = projectsList.value.filter(p => classifyComposeStatus(p) === 'running').length
  const unhealthy = projectsList.value.filter(p => classifyComposeStatus(p) === 'unhealthy').length
  const stopped = projectsList.value.filter(p => classifyComposeStatus(p) === 'stopped').length
  const totalContainers = projectsList.value.reduce((sum, project) => sum + totalProjectContainers(project), 0)
  const runningContainers = projectsList.value.reduce((sum, project) => sum + runningProjectContainers(project), 0)
  const updateCount = projectsList.value.reduce((sum, project) => sum + Number(project.updateCount || 0), 0)
  const exposedPorts = projectsList.value.reduce((sum, project) => sum + dedupeProjectPorts(project.containers), 0)
  return { total, running, unhealthy, stopped, totalContainers, runningContainers, updateCount, exposedPorts }
})

const composeRail = computed(() => [
  {
    label: '项目总数',
    value: composeCounts.value.total,
    note: `${filteredProjects.value.length} 个匹配`,
    icon: 'compose',
    tone: 'neutral'
  },
  {
    label: '运行中',
    value: composeCounts.value.running,
    note: `${composeCounts.value.runningContainers}/${composeCounts.value.totalContainers} 服务`,
    icon: 'play',
    tone: 'success'
  },
  {
    label: '不健康',
    value: composeCounts.value.unhealthy,
    note: composeCounts.value.unhealthy > 0 ? '需要关注' : '无异常',
    icon: 'alert',
    tone: 'warning'
  },
  {
    label: '已停止',
    value: composeCounts.value.stopped,
    note: '可启动或清理',
    icon: 'stop',
    tone: 'muted'
  },
  {
    label: '可更新',
    value: composeCounts.value.updateCount,
    note: '镜像标签变化',
    icon: 'cloud-download',
    tone: 'info'
  }
])

const selectedSummaryProject = computed(() => {
  if (selectedProject.value?.name) {
    // 同名多路径项目必须按“名字+路径”身份回解，否则会定位到另一个同名项目。
    const selected = projectsList.value.find(project => sameComposeProject(project, selectedProject.value))
    if (selected) return selected
  }
  return paginatedProjects.value.find(project => getStatusText(project) === '运行中') || paginatedProjects.value[0] || null
})

const selectedProjectView = computed(() => {
  if (!selectedProject.value?.name) return null
  return projectsList.value.find(project => sameComposeProject(project, selectedProject.value))
    || selectedProject.value
})

function normalizedContainerName(container) {
  return (container?.Names?.[0] || '').replace(/^\//, '') || container?._name || container?.name || ''
}

function findReboundContainer(containersList, previous) {
  if (!previous) return containersList?.[0] || null
  return containersList?.find(container => container.Id === previous.Id)
    || (previous._service && containersList?.find(container => container._service === previous._service))
    || containersList?.find(container => normalizedContainerName(container) === normalizedContainerName(previous))
    || containersList?.[0]
    || null
}

const selectedContainerSummary = computed(() => {
  const containersList = selectedProjectView.value?.containers || []
  return containersList.find(container => container.Id === selectedContainerId.value)
    || findReboundContainer(containersList, selectedContainer.value)
})

const selectedContainerView = computed(() => {
  const summary = selectedContainerSummary.value
  if (!summary) return null
  const detail = containerDetailsById.value.get(summary.Id)
  if (!detail) return summary
  return {
    ...summary,
    ...detail,
    Id: summary.Id,
    Names: summary.Names?.length ? summary.Names : detail.Names,
    State: summary.State,
    Status: summary.Status,
    HealthStatus: summary.HealthStatus,
    RunningTime: summary.RunningTime,
    Image: summary.Image,
    UpdateAvailable: summary.UpdateAvailable,
    updateAvailable: summary.updateAvailable,
    ResourceStats: summary.ResourceStats,
    _name: summary._name,
    _service: summary._service
  }
})

function totalProjectContainers(project) {
  return project?.containers?.length || 0
}

function runningProjectContainers(project) {
  return (project?.containers || []).filter(container => isRunning(container.State)).length
}

// 状态相关函数
function isRunning(state) {
  const s = String(state || '').toLowerCase()
  return s === 'running' || s === '运行中'
}

function getStatusText(project) {
  // 详情/排序按 Docker 实际生命周期展示，不沿用粗粒度的"不健康"兜底。
  return composeProjectStateLabel(project)
}

function getProjectStatusText(project) {
  return getStatusText(project)
}

function getProjectStatusClass(project) {
  return classifyComposeStatus(project)
}

function getContainerResourceStats(container) {
  const id = String(container?.Id || '')
  if (!id) return null
  return containerStatsMap.value.get(id) || containerStatsMap.value.get(id.slice(0, 12)) || null
}

function getProjectResourceStats(project) {
  const stats = (project.containers || [])
    .map(container => getContainerResourceStats(container))
    .filter(Boolean)
  if (stats.length === 0) return null
  const cpu = stats.reduce((sum, item) => sum + Number(item.cpu_percent || 0), 0)
  const memoryUsage = stats.reduce((sum, item) => sum + Number(item.memory_usage || 0), 0)
  const memoryLimit = stats.reduce((sum, item) => sum + Number(item.memory_limit || 0), 0)
  const networkRx = stats.reduce((sum, item) => sum + Number(item.network_rx || 0), 0)
  const networkTx = stats.reduce((sum, item) => sum + Number(item.network_tx || 0), 0)
  const blockRead = stats.reduce((sum, item) => sum + Number(item.block_read || 0), 0)
  const blockWrite = stats.reduce((sum, item) => sum + Number(item.block_write || 0), 0)
  return {
    cpu_percent: cpu,
    memory_usage: memoryUsage,
    memory_limit: memoryLimit,
    memory_percent: memoryLimit > 0 ? (memoryUsage / memoryLimit) * 100 : 0,
    network_rx: networkRx,
    network_tx: networkTx,
    block_read: blockRead,
    block_write: blockWrite,
    container_count: stats.length
  }
}

function formatStatPercent(value) {
  if (value === undefined || value === null) return '-'
  const num = Number(value)
  if (Number.isNaN(num)) return '-'
  return `${num.toFixed(num >= 10 ? 0 : 1)}%`
}

// 计算属性：过滤后的项目
const filteredProjects = computed(() => {
  let list = [...projectsList.value]
  
  // 状态筛选
  if (statusFilter.value !== 'all') {
    list = list.filter(p => {
      return classifyComposeStatus(p) === statusFilter.value
    })
  }
  
  // 搜索过滤
  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    list = list.filter(p => {
      if ((p.name || '').toLowerCase().includes(query)) return true
      if ((p.remark || '').toLowerCase().includes(query)) return true
      if (p.containers?.some(c => (c.Names?.[0] || '').toLowerCase().includes(query))) return true
      return false
    })
  }
  
  // 排序
  if (sortState.value.prop && sortState.value.order) {
    list.sort((a, b) => {
      let valA, valB
      switch (sortState.value.prop) {
        case 'name':
          valA = a.name || ''
          valB = b.name || ''
          break
        case 'status':
          valA = getStatusText(a)
          valB = getStatusText(b)
          break
        case 'containerCount':
          valA = a.containers?.length || 0
          valB = b.containers?.length || 0
          break
        case 'updateCount':
          valA = a.updateCount || 0
          valB = b.updateCount || 0
          break
        case 'createTime':
          valA = new Date(a.createTime || 0).getTime()
          valB = new Date(b.createTime || 0).getTime()
          break
        case 'path':
          valA = a.path || ''
          valB = b.path || ''
          break
        default:
          valA = a[sortState.value.prop]
          valB = b[sortState.value.prop]
      }
      
      if (valA < valB) return sortState.value.order === 'ascending' ? -1 : 1
      if (valA > valB) return sortState.value.order === 'ascending' ? 1 : -1
      return 0
    })
  }
  
  return list
})

// 分页后的列表
const paginatedProjects = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  const end = start + pageSize.value
  return filteredProjects.value.slice(start, end)
})

// 批量删除或筛选导致总页数收缩时，把当前页钳制到最后一个非空页。
watch(() => Math.max(1, Math.ceil(filteredProjects.value.length / pageSize.value)), pages => {
  if (currentPage.value > pages) currentPage.value = pages
})

// 方法
function handleSearch() {
  currentPage.value = 1
}

// 获取系统信息
async function fetchSystemInfo() {
  try {
    const info = await system.getInfo()
    const displayRoot = info?.ConfiguredProjectRoot || info?.HostProjectRoot
    if (typeof displayRoot === 'string' && displayRoot.trim()) {
      projectRoot.value = displayRoot.trim().replace(/\/+$/, '')
    }
  } catch (e) {
    console.error('获取系统信息失败:', e)
  }
}

// 获取项目列表
async function fetchProjects(options = {}) {
  try {
    await dockerResources.loadComposeProjects(options)
    syncSelectedProject()
    pruneCheckedProjects()
  } catch (error) {
    console.error('获取项目列表失败:', error)
  }
}

async function fetchResourceStats() {
  if (resourceStatsLoading) return
  resourceStatsLoading = true
  try {
    const data = await system.getStats()
    const map = new Map()
    for (const item of data?.container_stats || []) {
      if (item.container_id) map.set(String(item.container_id), item)
      if (item.id) map.set(String(item.id), item)
    }
    containerStatsMap.value = map
  } catch (error) {
    console.error('获取 Compose 资源占用失败:', error)
  } finally {
    resourceStatsLoading = false
  }
}

function startResourceStatsRefresh() {
  stopResourceStatsRefresh()
  if (isRemoteEnvironment.value) {
    containerStatsMap.value = new Map()
    return
  }
  fetchResourceStats()
  resourceStatsTimer = setInterval(fetchResourceStats, 5000)
}

function stopResourceStatsRefresh() {
  if (resourceStatsTimer) {
    clearInterval(resourceStatsTimer)
    resourceStatsTimer = null
  }
}

function syncSelectedProject() {
  if (!selectedProject.value?.name) return
  // 同名多路径项目必须按身份回解，否则定时同步会跳到另一个同名项目。
  const updated = projectsList.value.find(project => sameComposeProject(project, selectedProject.value))
  if (!updated) return
  const previousContainer = selectedContainer.value
  selectedProject.value = updated
  const rebound = findReboundContainer(updated.containers || [], previousContainer)
  if (previousContainer?.Id && rebound?.Id && previousContainer.Id !== rebound.Id) {
    const nextDetails = new Map(containerDetailsById.value)
    nextDetails.delete(previousContainer.Id)
    containerDetailsById.value = nextDetails
  }
  selectedContainer.value = rebound
  selectedContainerId.value = rebound?.Id || ''
  if (rebound && !containerDetailsById.value.has(rebound.Id)) {
    void loadSelectedContainerDetail(rebound)
  }
}

async function handleRefresh() {
  if (composeBusy.value) return
  await fetchProjects({ force: true })
}

function projectProcessingKey(projectOrIdentity) {
  if (projectOrIdentity && typeof projectOrIdentity === 'object') {
    return composeProjectIdentity(projectOrIdentity)
  }
  return String(projectOrIdentity || '')
}

function startProjectProcessing(projectOrIdentity, action = 'processing') {
  const identity = projectProcessingKey(projectOrIdentity)
  if (!identity) return
  processingProjects.value.set(identity, action)
}

function stopProjectProcessing(projectOrIdentity) {
  processingProjects.value.delete(projectProcessingKey(projectOrIdentity))
}

function isProjectProcessing(projectOrIdentity) {
  return processingProjects.value.has(projectProcessingKey(projectOrIdentity))
}

function getProjectProcessingAction(projectOrIdentity) {
  return processingProjects.value.get(projectProcessingKey(projectOrIdentity)) || ''
}

function hasActiveTaskForProject(projectOrIdentity) {
  const identity = projectProcessingKey(projectOrIdentity)
  for (const task of activeComposeTasks.value.values()) {
    if ((task?.projectIdentity || task?.name) === identity) return true
  }
  return false
}

function startContainerProcessing(id) {
  if (id) processingContainers.value.add(id)
}

function stopContainerProcessing(id) {
  if (id) processingContainers.value.delete(id)
}

function isContainerProcessing(id) {
  return processingContainers.value.has(id)
}

// 上一次 handleSelect 的 AbortController，用于在快速切换项目时取消旧请求，避免旧响应覆盖新状态
let selectAbortController = null

async function loadSelectedContainerDetail(container, options = {}) {
  const id = container?.Id
  if (!id) return
  if (!options.force && containerDetailsById.value.has(id)) {
    selectAbortController?.abort()
    selectAbortController = null
    containerDetailLoading.value = false
    containerDetailError.value = ''
    return
  }

  if (selectAbortController) selectAbortController.abort()
  const controller = new AbortController()
  selectAbortController = controller
  containerDetailLoading.value = true
  containerDetailError.value = ''

  try {
    const detail = await containers.get(id, { signal: controller.signal })
    if (controller.signal.aborted) return
    const next = new Map(containerDetailsById.value)
    next.set(id, detail)
    containerDetailsById.value = next
  } catch (error) {
    if (error?.name !== 'AbortError' && selectedContainerId.value === id) {
      containerDetailError.value = getErrorMessage(error)
      console.error('获取容器详情失败:', error)
    }
  } finally {
    if (selectAbortController === controller) {
      selectAbortController = null
      containerDetailLoading.value = false
    }
  }
}

function retrySelectedContainerDetail() {
  if (selectedContainerSummary.value) {
    void loadSelectedContainerDetail(selectedContainerSummary.value, { force: true })
  }
}

// 选择项目并打开详情抽屉。
// 设计要点：抽屉开关与容器详情刷新解耦 —— 抽屉立即打开（用现有 project.containers 数据），
// 容器详情异步刷新。这样无论异步请求被 abort、还是被后续切换取代，抽屉都能正常打开。
async function handleSelect(project, options = {}) {
  selectedProject.value = project
  const initialContainer = project.containers?.[0] || null
  selectedContainer.value = initialContainer
  selectedContainerId.value = initialContainer?.Id || ''
  containerDetailError.value = ''
  if (options.openDrawer) {
    showDetailDrawer.value = true
  }
  if (initialContainer) await loadSelectedContainerDetail(initialContainer)
}

function openSelectedProjectDrawer() {
  const project = selectedSummaryProject.value
  if (!project) return
  handleSelect(project, { openDrawer: true })
}

function selectContainer(container) {
  selectedContainer.value = container
  selectedContainerId.value = container?.Id || ''
  containerDetailError.value = ''
  if (container) void loadSelectedContainerDetail(container)
}

// 列表操作处理
function handleListAction({ action, project }) {
	if (rejectComposeIdentityConflict(project)) return
  // 目录名不规范的项目无法执行任何 Compose 操作，提示重命名后自动识别。
  if (project?.invalidName) {
    uiStore.toastWarning(project.invalidReason || '目录名不符合 Compose 项目名规范，重命名后即可管理')
    return
  }
  const actionMap = {
    start: handleStart,
    stop: handleStop,
    restart: handleRestart,
    edit: handleEdit,
    remark: openRemarkEditor,
    update: handleUpdate,
    'sync-git': handleGitSync,
    'force-stop': handleForceStop,
    down: handleDown,
    destroy: handleDestroy,
    remove: handleRemoveStandalone,
    terminal: handleTerminalStandalone,
    logs: openProjectLogs,
    protect: openProjectProtection
  }
  
  if (actionMap[action]) {
    actionMap[action](project)
  }
}

function rejectComposeIdentityConflict(project) {
  if (project?.type === 'container' || !project?.identityError) return false
  uiStore.toastWarning(project.identityError)
  return true
}

function openRemarkEditor(project) {
  if (!project) return
  remarkProject.value = project
  remarkDraft.value = String(project.remark || '')
  remarkVisible.value = true
}

async function saveRemark() {
  const project = remarkProject.value
  if (!project || remarkSaving.value) return
  remarkSaving.value = true
  try {
    if (project.type === 'container') {
      await compose.updateContainerRemark(project.name, { remark: remarkDraft.value.trim() })
    } else {
      await compose.updateMetadata(project.name, { remark: remarkDraft.value.trim() })
    }
    await fetchProjects({ force: true })
    remarkVisible.value = false
    uiStore.toastSuccess('备注已保存')
  } catch (error) {
    toastOperationError('保存备注', error)
  } finally {
    remarkSaving.value = false
  }
}

function openProjectProtection(project) {
  if (rejectComposeIdentityConflict(project)) return
  if (!project?.name || project.type === 'container') return
  openComposeProtection(router, project)
}

// 项目操作
async function handleStandaloneLifecycle(project, action, actionLabel) {
  const container = project?.containers?.[0]
  const containerId = container?.Id
  if (!containerId) {
    uiStore.toastWarning('未找到容器 ID')
    return
  }
  if (project?.isSelf || container?.isSelf) {
    uiStore.toastWarning('容器化部署模式下，不支持操作自身容器')
    return
  }
  if (isProjectProcessing(project)) return

  const operation = containers[action]
  if (typeof operation !== 'function') {
    uiStore.toastError(`不支持${actionLabel}该容器`)
    return
  }

  startProjectProcessing(project, action)
  try {
    await operation(containerId)
    await fetchProjects({ force: true })
  } catch (error) {
    toastOperationError(actionLabel, error)
  } finally {
    stopProjectProcessing(project)
  }
}

async function handleRemoteComposeLifecycle(project, action, actionLabel) {
  if (rejectComposeIdentityConflict(project)) return
  if (isProjectProcessing(project)) return
  startProjectProcessing(project, action)
  try {
    await compose[action](remoteComposeOperationName(project))
    await fetchProjects({ force: true })
  } catch (error) {
    toastOperationError(actionLabel, error)
  } finally {
    stopProjectProcessing(project)
  }
}

async function handleStart(project) {
  if (project?.type === 'container') {
    await handleStandaloneLifecycle(project, 'start', '启动')
    return
  }
	if (isRemoteEnvironment.value) {
		await handleRemoteComposeLifecycle(project, 'start', '启动')
		return
	}
  if (rejectComposeIdentityConflict(project)) return
  if (isProjectProcessing(project)) return
  startProjectProcessing(project, 'start')
  try {
    const res = await compose.startTask(project.name)
    if (!res?.taskId) throw new Error('未获取到任务 ID')
    startDeployTaskPolling(res.taskId, project.name, 'start', { projectIdentity: composeProjectIdentity(project) })
  } catch (error) {
    stopProjectProcessing(project)
    toastOperationError('启动', error)
  }
}

async function handleStop(project) {
  if (project?.type === 'container') {
    await handleStandaloneLifecycle(project, 'stop', '停止')
    return
  }
	if (isRemoteEnvironment.value) {
		await handleRemoteComposeLifecycle(project, 'stop', '停止')
		return
	}
  if (rejectComposeIdentityConflict(project)) return
  if (isProjectProcessing(project)) return
  startProjectProcessing(project, 'stop')
  try {
    const res = await compose.stopTask(project.name)
    if (!res?.taskId) throw new Error('未获取到任务 ID')
    startDeployTaskPolling(res.taskId, project.name, 'stop', { projectIdentity: composeProjectIdentity(project) })
  } catch (error) {
    stopProjectProcessing(project)
    toastOperationError('停止', error)
  }
}

// 强制停止：仅发送 SIGKILL，不做优雅停止、不修改重启策略。
// Compose 项目走后端 kill 任务逐个终止项目容器；独立容器直接调容器 kill 接口。
async function handleForceStop(project) {
  if (project?.type === 'container') {
    const container = project?.containers?.[0]
    const containerId = container?.Id
    if (!containerId) {
      uiStore.toastWarning('未找到容器 ID')
      return
    }
    if (project?.isSelf || container?.isSelf) {
      uiStore.toastWarning('容器化部署模式下，不支持操作自身容器')
      return
    }
    const confirmed = await confirmComposeAction({
      type: 'danger',
      title: '强制停止容器',
      message: `将对容器 "${project.name}" 立即发送 SIGKILL，不做优雅停止。容器内未保存的数据可能丢失。`,
      confirmText: '强制停止'
    })
    if (!confirmed || isProjectProcessing(project)) return
    startProjectProcessing(project, 'force-stop')
    try {
      await containers.forceStop(containerId)
      await fetchProjects({ force: true })
    } catch (error) {
      toastOperationError('强制停止', error)
    } finally {
      stopProjectProcessing(project)
    }
    return
  }
  if (rejectComposeIdentityConflict(project)) return
  const confirmed = await confirmComposeAction({
    type: 'danger',
    title: '强制停止项目',
    message: `将对项目 "${project.name}" 的所有容器立即发送 SIGKILL，不做优雅停止。容器内未保存的数据可能丢失。`,
    confirmText: '强制停止'
  })
  if (!confirmed || isProjectProcessing(project)) return
  startProjectProcessing(project, 'force-stop')
  try {
    const res = await compose.forceStop(project.name)
    if (!res?.taskId) throw new Error('未获取到任务 ID')
    startDeployTaskPolling(res.taskId, project.name, 'force-stop', { projectIdentity: composeProjectIdentity(project) })
  } catch (error) {
    stopProjectProcessing(project)
    toastOperationError('强制停止', error)
  }
}

async function handleRestart(project) {
  if (project?.type === 'container') {
    await handleStandaloneLifecycle(project, 'restart', '重启')
    return
  }
	if (isRemoteEnvironment.value) {
		await handleRemoteComposeLifecycle(project, 'restart', '重启')
		return
	}
  if (rejectComposeIdentityConflict(project)) return
  if (isProjectProcessing(project)) return
  startProjectProcessing(project, 'restart')
  try {
    const res = await compose.restartTask(project.name)
    if (!res?.taskId) throw new Error('未获取到任务 ID')
    startDeployTaskPolling(res.taskId, project.name, 'restart', { projectIdentity: composeProjectIdentity(project) })
  } catch (error) {
    stopProjectProcessing(project)
    toastOperationError('重启', error)
  }
}

async function handleDown(project) {
  if (rejectComposeIdentityConflict(project)) return
  const confirmed = await confirmComposeAction({
    title: '清除项目容器',
    message: `确定清除项目 "${project.name}" 的容器吗？这将停止并删除所有容器，但保留项目配置文件。`,
    confirmText: '清除'
  })
  if (!confirmed) {
    return
  }
  if (isProjectProcessing(project)) return
  startProjectProcessing(project, 'down')
  try {
    if (isRemoteEnvironment.value) {
      await compose.down(remoteComposeOperationName(project))
      await fetchProjects({ force: true })
      stopProjectProcessing(project)
      return
    }
    const res = await compose.downTask(project.name)
    if (!res?.taskId) throw new Error('未获取到任务 ID')
    startDeployTaskPolling(res.taskId, project.name, 'down', { projectIdentity: composeProjectIdentity(project) })
  } catch (error) {
    stopProjectProcessing(project)
    toastOperationError('清除', error)
    await fetchProjects({ force: true }).catch(() => {})
  }
}

async function handleRemove(project) {
  if (rejectComposeIdentityConflict(project)) return
  const confirmed = await confirmComposeAction({
    type: 'danger',
    title: '删除项目',
    message: `确定删除项目 "${project.name}" 吗？此操作不可恢复。`,
    confirmText: '删除'
  })
  if (!confirmed) {
    return
  }
  if (isProjectProcessing(project)) return
  startProjectProcessing(project, 'remove')
  try {
    if (isRemoteEnvironment.value) {
      await compose.remove(remoteComposeOperationName(project))
      await fetchProjects({ force: true })
      stopProjectProcessing(project)
      return
    }
    const res = await compose.removeTask(project.name)
    if (!res?.taskId) throw new Error('未获取到任务 ID')
    startDeployTaskPolling(res.taskId, project.name, 'remove', { projectIdentity: composeProjectIdentity(project) })
  } catch (error) {
    stopProjectProcessing(project)
    toastOperationError('删除', error)
    await fetchProjects({ force: true }).catch(() => {})
  }
}

async function handleRemoveStandalone(project) {
  const containerId = project.containers?.[0]?.Id
  if (!containerId) {
    uiStore.toastWarning('未找到容器 ID')
    return
  }
  const confirmed = await confirmComposeAction({
    type: 'danger',
    title: '删除容器',
    message: `确定删除容器 "${project.name}" 吗？此操作不可恢复。`,
    confirmText: '删除'
  })
  if (!confirmed) {
    return
  }
  if (isProjectProcessing(project)) return
  startProjectProcessing(project, 'remove')
  try {
    await containers.remove(containerId, { force: false })
    await fetchProjects({ force: true })
  } catch (error) {
    toastOperationError('删除', error)
  } finally {
    stopProjectProcessing(project)
  }
}

async function handleDestroy(project) {
  if (rejectComposeIdentityConflict(project)) return
  if (isProjectProcessing(project)) return
  if (isRemoteEnvironment.value) {
    uiStore.toastWarning('远程设备暂不支持生成销毁清单，请切回本机操作')
    return
  }

  destroyProject.value = project
  destroyInventory.value = null
  destroyError.value = ''
  destroyVisible.value = true
  await loadDestroyPreview()
}

async function loadDestroyPreview() {
  const project = destroyProject.value
  if (!project?.name) return
  const sequence = ++destroyPreviewSequence
  destroyLoading.value = true
  destroyError.value = ''
  try {
    const inventory = await compose.destroyPreview(project.name)
    if (sequence !== destroyPreviewSequence || !destroyVisible.value) return
    destroyInventory.value = inventory
    destroySelected.value = (inventory.delete || []).map(destroyResourceKey)
  } catch (error) {
    if (sequence !== destroyPreviewSequence || !destroyVisible.value) return
    destroyError.value = getErrorMessage(error)
  } finally {
    if (sequence === destroyPreviewSequence) destroyLoading.value = false
  }
}

async function confirmDestroy() {
  const project = destroyProject.value
  const fingerprint = destroyInventory.value?.fingerprint
  if (!project?.name || !fingerprint || destroySubmitting.value) return
  destroySubmitting.value = true
  startProjectProcessing(project, 'destroy')
  try {
    const res = await compose.destroyTask(project.name, fingerprint, destroySelected.value)
    if (!res?.taskId) throw new Error('未获取到任务 ID')
    startDeployTaskPolling(res.taskId, project.name, 'destroy', { projectIdentity: composeProjectIdentity(project) })
    destroyVisible.value = false
    resetDestroyDialog()
  } catch (error) {
    stopProjectProcessing(project)
    destroyError.value = getErrorMessage(error)
    destroyInventory.value = null
    uiStore.toastWarning('项目资源已变化或销毁任务提交失败，请重新核对清单')
  } finally {
    destroySubmitting.value = false
  }
}

function resetDestroyDialog() {
  destroyPreviewSequence += 1
  destroyLoading.value = false
  destroySubmitting.value = false
  destroyProject.value = null
  destroyInventory.value = null
  destroyError.value = ''
  destroySelected.value = []
}

function closeDestroyDialog() {
  destroyVisible.value = false
  resetDestroyDialog()
}

function destroyResourceLabel(kind) {
  return ({
    container: '容器',
    network: '项目网络',
    volume: '数据卷',
    image: '关联镜像',
    bind: '宿主机目录',
    project_directory: '项目目录',
    project_record: 'TRADIS 项目记录'
  })[kind] || '项目资源'
}

function destroyResourceKey(item) {
  return item?.key || `${item?.kind || 'resource'}:${item?.id || item?.name || ''}`
}

function destroyResourceIcon(kind) {
  return ({
    container: 'container',
    network: 'network',
    volume: 'database',
    image: 'image',
    bind: 'folder',
    project_directory: 'folder',
    project_record: 'file-text'
  })[kind] || 'box'
}

async function handleGitSync(project) {
  if (rejectComposeIdentityConflict(project)) return
  if (!project?.gitSource) return
  if ((project.containers?.length || 0) > 0) {
    uiStore.toastWarning('同步前请先点击“清除容器”。Git 同步会替换项目目录，不能在容器仍引用旧挂载目录时执行。')
    return
  }
  const confirmed = await confirmComposeAction({
    title: '同步 Git 项目',
    message: `确定从 Git 仓库同步项目 "${project.name}" 吗？同步会替换项目目录并覆盖其中的本地文件，请先做好备份；完成后需要手动启动。`,
    confirmText: '同步'
  })
  if (!confirmed) {
    return
  }
  if (isProjectProcessing(project)) return
  startProjectProcessing(project, 'git-sync')
  try {
    const result = await compose.syncGit(project.name)
    if (!result?.taskId) throw new Error('未获取到任务 ID')
    startDeployTaskPolling(result.taskId, project.name, 'git-sync', { projectIdentity: composeProjectIdentity(project) })
  } catch (error) {
    stopProjectProcessing(project)
    toastOperationError('Git 同步', error)
  }
}

function handleUpdate(project) {
  if (rejectComposeIdentityConflict(project)) return
  if (project.isSelf) {
    uiStore.toastWarning('自身项目不支持更新')
    return
  }
  if (isProjectProcessing(project)) return
  startProjectProcessing(project, 'update')
  updateProjectName.value = project.name
  updateProjectIdentity.value = composeProjectIdentity(project)
  showUpdateDialog.value = true
  // 对话框关闭时由 onUpdateDialogClose 移除处理状态
}

function openSafeUpdate (project) {
  if (rejectComposeIdentityConflict(project)) return
  if (project?.isSelf) {
    uiStore.toastWarning('自身项目不支持安全更新')
    return
  }
  if (!project?.name || isProjectProcessing(project)) return
  startProjectProcessing(project, 'safe-update')
  safeUpdateProjectName.value = project.name
  safeUpdateProjectIdentity.value = composeProjectIdentity(project)
  showSafeUpdateDialog.value = true
}

// 对话框操作
function openCreateDialog() {
  isEdit.value = false
  editFormData.value = { name: '', path: '', yaml: '', env: '' }
  showEditDialog.value = true
}

async function handleEdit(project) {
  if (rejectComposeIdentityConflict(project)) return
  isEdit.value = true
  try {
    const [yamlRes, envRes] = await Promise.all([
      compose.getYaml(project.name),
      compose.getEnv(project.name).catch(() => ({ content: '' }))
    ])
    editFormData.value = {
      name: project.name,
      path: project.path || '',
      yaml: yamlRes.content || '',
      env: envRes.content || ''
    }
    showEditDialog.value = true
  } catch (error) {
    toastOperationError('加载配置', error)
  }
}

function openComposeHistory(project) {
  if (!project?.name) return
  historyProjectName.value = project.name
  historyProjectIdentity.value = composeProjectIdentity(project)
  showHistoryDialog.value = true
}

async function onComposeHistoryRestored(payload) {
  const name = payload?.name || historyProjectName.value
  await fetchProjects({ force: true })
  const updated = findComposeProjectByIdentity(projectsList.value, historyProjectIdentity.value, name)
  if (updated) selectedProject.value = updated
}

function openAiDialog({ yaml, env }) {
  currentYamlForAi.value = yaml
  currentEnvForAi.value = env
  showAiDialog.value = true
}

function onAiGenerated(payload) {
  const draft = payload || {}
  if (Object.prototype.hasOwnProperty.call(draft, 'yaml')) {
    editFormData.value.yaml = String(draft.yaml || '')
  }
  if (Object.prototype.hasOwnProperty.call(draft, 'env')) {
    editFormData.value.env = String(draft.env || '')
  }
  editDialogRef.value?.applyAiDraft(draft)
}

function insertTemplate(type) {
  const templates = {
    nginx: `version: '3'
services:
  nginx:
    image: nginx:latest
    ports:
      - "80:80"
    volumes:
      - ./html:/usr/share/nginx/html
    environment:
      - TZ=Asia/Shanghai
    restart: always`,
    mysql: `version: '3'
services:
  mysql:
    image: mysql:8.0
    environment:
      MYSQL_ROOT_PASSWORD: rootpassword
      MYSQL_DATABASE: mydb
      TZ: Asia/Shanghai
    ports:
      - "3306:3306"
    volumes:
      - mysql_data:/var/lib/mysql
    restart: always

volumes:
  mysql_data:`,
    redis: `version: '3'
services:
  redis:
    image: redis:latest
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    command: redis-server --appendonly yes
    restart: always

volumes:
  redis_data:`,
    wordpress: `version: '3'
services:
  wordpress:
    image: wordpress:latest
    ports:
      - "8080:80"
    environment:
      WORDPRESS_DB_HOST: db
      WORDPRESS_DB_USER: wordpress
      WORDPRESS_DB_PASSWORD: wordpress
      WORDPRESS_DB_NAME: wordpress
      TZ: Asia/Shanghai
    volumes:
      - ./wordpress:/var/www/html
    depends_on:
      - db
    restart: always
  
  db:
    image: mysql:5.7
    environment:
      MYSQL_DATABASE: wordpress
      MYSQL_USER: wordpress
      MYSQL_PASSWORD: wordpress
      MYSQL_RANDOM_ROOT_PASSWORD: '1'
      TZ: Asia/Shanghai
    volumes:
      - db_data:/var/lib/mysql
    restart: always

volumes:
  db_data:`
  }
  
  if (templates[type]) {
    onAiGenerated({ yaml: templates[type] })
  }
}

function onProjectSaved() {
  fetchProjects({ force: true })
}

function onProjectDeployed() {
  fetchProjects({ force: true })
}

function saveDeployTaskState() {
  const tasks = Array.from(activeComposeTasks.value.entries()).map(([taskId, task]) => ({
    taskId,
    name: String(task?.name || ''),
    projectIdentity: String(task?.projectIdentity || ''),
    action: String(task?.action || ''),
    containerId: String(task?.containerId || ''),
    savedAt: Number(task?.savedAt || Date.now())
  }))
  if (tasks.length === 0) {
    localStorage.removeItem(DEPLOY_TASK_STATE_KEY)
    return
  }
  localStorage.setItem(DEPLOY_TASK_STATE_KEY, JSON.stringify(tasks))
}

function loadDeployTaskStates() {
  try {
    const raw = localStorage.getItem(DEPLOY_TASK_STATE_KEY)
    if (!raw) return []
    const state = JSON.parse(raw)
    const states = Array.isArray(state) ? state : [state]
    return states.filter(item => item?.taskId)
  } catch {
    return []
  }
}

function stopDeployTaskPolling(taskId = '', options = {}) {
  const normalizedTaskId = String(taskId || '')
  if (normalizedTaskId) {
    const task = activeComposeTasks.value.get(normalizedTaskId)
    activeComposeTasks.value.delete(normalizedTaskId)
    if (task?.containerId) {
      stopContainerProcessing(task.containerId)
    }
    const projectIdentity = task?.projectIdentity || task?.name
    if (projectIdentity && !hasActiveTaskForProject(projectIdentity)) {
      stopProjectProcessing(projectIdentity)
    }
  } else {
    for (const task of activeComposeTasks.value.values()) {
      if (task?.containerId) stopContainerProcessing(task.containerId)
      if (task?.projectIdentity || task?.name) stopProjectProcessing(task?.projectIdentity || task?.name)
    }
    activeComposeTasks.value.clear()
  }
  if (deployTaskPollTimer && activeComposeTasks.value.size === 0) {
    clearInterval(deployTaskPollTimer)
    deployTaskPollTimer = null
  }
  if (options.clearState) {
    saveDeployTaskState()
  }
}

function isTaskFinished(status) {
  const normalized = String(status || '').toLowerCase()
  return normalized === 'success' || normalized === 'completed' || normalized === 'error' || normalized === 'failed' || normalized === 'canceled'
}

async function pollDeployTask(taskId) {
  if (!taskId) return
  const trackedTask = activeComposeTasks.value.get(String(taskId))
  if (!trackedTask) return
  const { name = '', action = '', projectIdentity = '' } = trackedTask
  try {
    const task = await compose.getTask(taskId)
    if (!isTaskFinished(task?.status)) return
    stopDeployTaskPolling(taskId, { clearState: true })
    const status = String(task?.status || '').toLowerCase()
    const success = ['success', 'completed'].includes(status)
    const canceled = status === 'canceled'
    if (success) {
      await convergeComposeMutation({
        refresh: () => fetchProjects({ force: true }),
        resolveProject: () => findComposeProjectByIdentity(
          projectsList.value,
          projectIdentity,
          name
        ),
        action
      })
    } else {
      await fetchProjects({ force: true })
    }
    const actionText = getTaskActionText(task?.type)
    const message = success
      ? (name ? `${actionText}完成：${name}` : `${actionText}完成`)
      : canceled
        ? (name ? `${actionText}已中止：${name}` : `${actionText}已中止`)
        : (name ? `${actionText}失败：${name}` : `${actionText}失败`)
    if (success) {
      let warnings = []
      if (task?.type === 'compose_destroy' && task?.resultJson) {
        try {
          const result = JSON.parse(task.resultJson)
          warnings = Array.isArray(result?.warnings) ? result.warnings : []
        } catch {
          warnings = []
        }
      }
      if (warnings.length > 0) {
        uiStore.toastWarning(`${message}；${warnings.join('；')}`)
      } else {
        uiStore.toastSuccess(message)
      }
    } else if (canceled) {
      uiStore.toastInfo(message)
    } else {
      uiStore.toastError(task?.error ? `${message}（${task.error}）` : message)
    }
  } catch (error) {
    console.error('轮询部署任务失败:', error)
  }
}

function getTaskActionText(type) {
  switch (String(type || '')) {
    case 'compose_start':
      return '启动'
    case 'compose_stop':
      return '停止'
    case 'compose_kill':
      return '强制停止'
    case 'compose_restart':
      return '重启'
    case 'compose_down':
      return '清除'
    case 'compose_remove':
      return '销毁'
    case 'compose_destroy':
      return '销毁'
    case 'compose_build':
      return '构建'
    case 'compose_update':
      return '更新'
    case 'container_update':
      return '容器更新'
    case 'safe_update':
      return '安全更新'
    case 'compose_git_import':
      return 'Git 下载'
    case 'compose_git_sync':
      return 'Git 同步'
    default:
      return '部署'
  }
}

function startDeployTaskPolling(taskId, name = '', action = '', metadata = {}) {
  const normalizedTaskId = String(taskId || '')
  if (!normalizedTaskId) {
    return
  }
  let projectIdentity = String(metadata?.projectIdentity || '')
  if (!projectIdentity && action === 'update') projectIdentity = updateProjectIdentity.value
  if (!projectIdentity && action === 'safe-update') projectIdentity = safeUpdateProjectIdentity.value
  if (!projectIdentity && name) {
    const matches = projectsList.value.filter(project => String(project?.name || '') === String(name))
    projectIdentity = matches.length === 1 ? composeProjectIdentity(matches[0]) : String(name)
  }
  activeComposeTasks.value.set(normalizedTaskId, {
    name: String(name || ''),
    projectIdentity,
    action: String(action || ''),
    containerId: String(metadata?.containerId || ''),
    savedAt: Date.now()
  })
  if (projectIdentity) startProjectProcessing(projectIdentity, action || 'processing')
  saveDeployTaskState()
  pollDeployTask(normalizedTaskId)
  if (!deployTaskPollTimer) {
    deployTaskPollTimer = setInterval(() => {
      for (const activeTaskId of activeComposeTasks.value.keys()) {
        pollDeployTask(activeTaskId)
      }
    }, 3000)
  }
}

function onDeployTaskStarted(payload, action = '') {
  const name = payload?.name || ''
  const taskId = payload?.taskId || ''
  if (taskId) {
    startDeployTaskPolling(taskId, name, action)
  }
}

function onBackgroundDeploy(payload) {
  const name = typeof payload === 'string' ? payload : payload?.name
  const taskId = typeof payload === 'object' ? payload?.taskId : ''
  if (taskId) {
    startDeployTaskPolling(taskId, name, 'deploy')
  }
  uiStore.toastInfo(name ? `部署任务已后台运行：${name}` : '部署任务已后台运行')
}

async function recoverDeployTaskPolling() {
  const savedTasks = loadDeployTaskStates()
  for (const saved of savedTasks) {
    try {
      const task = await compose.getTask(saved.taskId)
      if (isTaskFinished(task?.status)) {
        await fetchProjects({ force: true })
      } else {
        startDeployTaskPolling(saved.taskId, saved.name || '', saved.action || '', {
          containerId: saved.containerId || '',
          projectIdentity: saved.projectIdentity || ''
        })
      }
    } catch {
      activeComposeTasks.value.delete(String(saved.taskId))
    }
  }
  saveDeployTaskState()
  if (activeComposeTasks.value.size > 0) return

  try {
    const tasks = await compose.listTasks({ statuses: 'pending,running', limit: 20 })
    for (const task of Array.isArray(tasks) ? tasks : []) {
      if (task?.id) {
        startDeployTaskPolling(task.id, '')
      }
    }
  } catch (error) {
    console.error('恢复部署任务轮询失败:', error)
  }
}

// 容器操作
async function handleContainerAction(action) {
  if (!selectedContainer.value) return
  
  const container = selectedContainer.value
  
  switch (action) {
    case 'terminal':
      openTerminalWithDrawer(container)
      break
    case 'logs':
      openLogsWithDrawer(container)
      break
    case 'start':
      await startContainer(container)
      break
    case 'stop':
      await stopContainer(container)
      break
    case 'restart':
      await restartContainer(container)
      break
    case 'update':
      await updateContainer(container)
      break
    case 'delete':
      await deleteContainer(container)
      break
  }
}

async function updateContainer(container) {
  if (container?.isSelf) {
    uiStore.toastWarning('容器化部署模式下，不支持操作自身容器')
    return
  }
  const name = getContainerName(container)
  const confirmed = await confirmComposeAction({
    type: 'warning',
    title: '更新容器',
    message: `将拉取 "${formatDockerImageReference(container?.Image) || '当前镜像'}" 的最新版本，并保留 "${name}" 的配置和挂载重新创建容器。`,
    confirmText: '后台更新'
  })
  if (!confirmed || isContainerProcessing(container.Id)) return

  startContainerProcessing(container.Id)
  try {
    const result = await containers.startUpdateTask(container.Id, { pull: true })
    if (!result?.taskId) throw new Error('未获取到任务 ID')
    startDeployTaskPolling(result.taskId, selectedProject.value?.name || '', 'container-update', {
      containerId: container.Id,
      projectIdentity: composeProjectIdentity(selectedProject.value)
    })
    uiStore.toastInfo(`容器 ${name} 已转入后台更新`)
  } catch (error) {
    stopContainerProcessing(container.Id)
    toastOperationError('更新容器', error)
  }
}

async function startContainer(container) {
  if (container?.isSelf) {
    uiStore.toastWarning('容器化部署模式下，不支持操作自身容器')
    return
  }
  startContainerProcessing(container.Id)
  try {
    await containers.start(container.Id)
    await refreshAfterContainerAction(container.Id)
  } catch (error) {
    toastOperationError('启动', error)
  } finally {
    stopContainerProcessing(container.Id)
  }
}

async function stopContainer(container) {
  if (container?.isSelf) {
    uiStore.toastWarning('容器化部署模式下，不支持操作自身容器')
    return
  }
  startContainerProcessing(container.Id)
  try {
    await containers.stop(container.Id)
    await refreshAfterContainerAction(container.Id)
  } catch (error) {
    toastOperationError('停止', error)
  } finally {
    stopContainerProcessing(container.Id)
  }
}

async function restartContainer(container) {
  if (container?.isSelf) {
    uiStore.toastWarning('容器化部署模式下，不支持操作自身容器')
    return
  }
  startContainerProcessing(container.Id)
  try {
    await containers.restart(container.Id)
    await refreshAfterContainerAction(container.Id)
  } catch (error) {
    toastOperationError('重启', error)
  } finally {
    stopContainerProcessing(container.Id)
  }
}

async function deleteContainer(container) {
  if (container?.isSelf) {
    uiStore.toastWarning('容器化部署模式下，不支持操作自身容器')
    return
  }
  const confirmed = await confirmComposeAction({
    type: 'danger',
    title: '删除容器',
    message: `确定删除容器 "${getContainerName(container)}" 吗？`,
    confirmText: '删除'
  })
  if (!confirmed) {
    return
  }
  startContainerProcessing(container.Id)
  try {
    await containers.remove(container.Id)
    await fetchProjects({ force: true })
    if (selectedProject.value) {
      const updated = projectsList.value.find(p => sameComposeProject(p, selectedProject.value))
      if (updated) {
        selectedProject.value = updated
        selectedContainer.value = updated.containers?.[0] || null
      }
    }
  } catch (error) {
    toastOperationError('删除', error)
  } finally {
    stopContainerProcessing(container.Id)
  }
}

async function refreshAfterContainerAction(containerId) {
  await fetchProjects({ force: true })
  if (selectedProject.value) {
    const updated = projectsList.value.find(p => sameComposeProject(p, selectedProject.value))
    if (updated) {
      selectedProject.value = updated
      const updatedContainer = updated.containers?.find(c => c.Id === containerId)
      if (updatedContainer) selectedContainer.value = updatedContainer
    }
  }
}

function getContainerName(container) {
  return (container.Names?.[0] || '').replace(/^\//, '') || container.name || container.Id?.substring(0, 12) || '未知'
}

// 项目级运行时间：以最早启动的运行容器作为项目启动时间（参考容器详情页的"已运行"）
function projectUptimeLabel(project) {
  const running = (project?.containers || []).filter(container => isRunning(container.State))
  if (!running.length) return '-'
  const created = running
    .map(container => Number(container.Created))
    .filter(value => Number.isFinite(value) && value > 0)
  if (created.length) {
    // Created 可能来自 docker 列表（秒）或详情补全后（毫秒），统一为毫秒
    const earliest = Math.min(...created)
    const startedMs = earliest < 1e12 ? earliest * 1000 : earliest
    return formatElapsed(Date.now() - startedMs)
  }
  // 无时间戳时回退到首个运行容器的运行时长文案
  return formatContainerUptimeCn(running[0])
}

function hasContainerUpdate(container) {
  return Boolean(container?.UpdateAvailable ?? container?.updateAvailable)
}

// 项目公开端口数量（按 PublicPort:PrivatePort 去重 IPv4/IPv6 重复，同容器详情页口径）
function dedupeProjectPorts(containers) {
  const seen = new Set()
  for (const container of containers || []) {
    for (const port of container.Ports || []) {
      if (!port.PublicPort) continue
      seen.add(`${port.PublicPort}:${port.PrivatePort}`)
    }
  }
  return seen.size
}

function projectPortsTitle(project) {
  const lines = []
  const seen = new Set()
  for (const container of project?.containers || []) {
    for (const port of container.Ports || []) {
      if (!port.PublicPort) continue
      const key = `${port.PublicPort}:${port.PrivatePort}`
      if (seen.has(key)) continue
      seen.add(key)
      lines.push(`${port.PublicPort}→${port.PrivatePort}/${port.Type || 'tcp'}`)
    }
  }
  return lines.join('\n')
}

// 终端和日志
function openTerminalWithDrawer(container) {
  currentContainer.value = container
  sessionStorage.setItem('composeDrawerPending', JSON.stringify({
    project: selectedProject.value?.name,
    projectIdentity: composeProjectIdentity(selectedProject.value),
    container: container?.Id,
    containerData: container,
    timestamp: Date.now()
  }))
  showTerminalDialog.value = true
  showDetailDrawer.value = false
}

function openLogsWithDrawer(container) {
  currentContainer.value = container
  currentLogSource.value = {
    type: 'container',
    id: container?.Id || '',
    label: getContainerName(container)
  }
  sessionStorage.setItem('composeDrawerPending', JSON.stringify({
    project: selectedProject.value?.name,
    projectIdentity: composeProjectIdentity(selectedProject.value),
    container: container?.Id,
    containerData: container,
    timestamp: Date.now()
  }))
  showLogsDialog.value = true
  showDetailDrawer.value = false
}

function openProjectLogs(project) {
  if (!project) return
  if (project.type === 'container') {
    const container = project.containers?.[0]
    if (!container) return
    currentLogSource.value = {
      type: 'container',
      id: container.Id || '',
      label: getContainerName(container)
    }
  } else {
    currentLogSource.value = {
      type: 'compose',
			id: isRemoteEnvironment.value ? remoteComposeOperationName(project) : (project.name || ''),
      label: project.name || 'compose'
    }
  }
  showLogsDialog.value = Boolean(currentLogSource.value.id)
}

function handleTerminal(project) {
  if (project.type !== 'container') return
  const container = project.containers?.[0]
  if (container) {
    currentContainer.value = container
    showTerminalDialog.value = true
  }
}

function handleTerminalStandalone(project) {
  const container = project.containers?.[0]
  if (container) {
    currentContainer.value = container
    showTerminalDialog.value = true
  }
}

function onTerminalClose() {
  isRestoringDrawer.value = true
  setTimeout(() => {
    restoreDrawer()
    setTimeout(() => isRestoringDrawer.value = false, 100)
  }, 300)
}

function onLogsClose() {
  isRestoringDrawer.value = true
  setTimeout(() => {
    restoreDrawer()
    setTimeout(() => isRestoringDrawer.value = false, 100)
  }, 300)
}

async function restoreDrawer() {
  try {
    const pending = sessionStorage.getItem('composeDrawerPending')
    if (pending) {
      const { project, projectIdentity, container, timestamp, containerData: savedContainerData } = JSON.parse(pending)
      if (Date.now() - timestamp < 5 * 60 * 1000) {
        if (projectsList.value.length === 0) {
          await fetchProjects()
        }
        const projectData = findComposeProjectByIdentity(projectsList.value, projectIdentity, project)
        if (projectData) {
          selectedProject.value = projectData
          if (container) {
            const containerData = projectData.containers?.find(c => c.Id === container || c.Id?.startsWith(container))
            if (containerData) {
              selectedContainer.value = containerData
            } else if (savedContainerData) {
              selectedContainer.value = savedContainerData
            }
          }
          showDetailDrawer.value = true
          if (selectedContainer.value) {
            void loadSelectedContainerDetail(selectedContainer.value)
          }
        } else if (savedContainerData) {
          selectedProject.value = { name: project, path: '-', containers: [] }
          selectedContainer.value = savedContainerData
          showDetailDrawer.value = true
          void loadSelectedContainerDetail(savedContainerData)
        }
      }
      sessionStorage.removeItem('composeDrawerPending')
    }
  } catch (e) {
    console.error('恢复抽屉状态失败:', e)
  }
}

// 点击外部处理
function handleClickOutside(e) {
  if (isRestoringDrawer.value) return
  if (!e.target.closest('.compose-card') && 
      !e.target.closest('.resource-table__row') &&
      !e.target.closest('tr') && 
      !e.target.closest('.compose-context-panel') &&
      !e.target.closest('.drawer-panel') &&
      !e.target.closest('.modal-overlay') &&
      !e.target.closest('.modal-panel')) {
    selectedProject.value = null
  }
}

usePageAction({
  create: openCreateDialog,
  'git-import': () => {
    showGitImportDialog.value = true
  }
})

// 生命周期
onMounted(() => {
  fetchProjects()
  if (!isRemoteEnvironment.value) fetchSystemInfo()
  startResourceStatsRefresh()
  recoverDeployTaskPolling()
  document.addEventListener('click', handleClickOutside)
})

watch(isRemoteEnvironment, () => {
  startResourceStatsRefresh()
  if (!isRemoteEnvironment.value) fetchSystemInfo()
})

onUnmounted(() => {
  composeViewActive = false
  selectAbortController?.abort()
  selectAbortController = null
  stopResourceStatsRefresh()
  if (deployTaskPollTimer) {
    clearInterval(deployTaskPollTimer)
    deployTaskPollTimer = null
  }
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.remark-field {
  display: flex;
  flex-direction: column;
  gap: 7px;
  color: var(--text-secondary);
  font-size: 0.8125rem;
  font-weight: 600;
}

.remark-field small {
  align-self: flex-end;
  color: var(--text-tertiary);
  font-size: 0.75rem;
  font-weight: 400;
}

.remark-input {
  width: 100%;
  box-sizing: border-box;
  padding: 10px 12px;
  border: 1px solid var(--border-default);
  border-radius: 8px;
  color: var(--text-primary);
  background: var(--bg-primary);
  font: inherit;
  font-weight: 400;
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out), box-shadow var(--motion-duration-quick) var(--motion-ease-out);
}

.remark-input:focus {
  outline: none;
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.destroy-dialog {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.destroy-spinner {
  animation: resource-refresh-spin 0.75s linear infinite;
}

@media (prefers-reduced-motion: reduce) {
  .destroy-spinner {
    animation: none;
  }
}

.destroy-dialog-state {
  min-height: 132px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: var(--text-secondary);
  font-size: 0.875rem;
}

.destroy-dialog-state.is-error {
  align-items: flex-start;
  justify-content: flex-start;
  padding: 14px;
  border: 1px solid var(--color-danger-200);
  border-radius: 8px;
  color: var(--color-danger-700);
  background: var(--color-danger-50);
}

.destroy-dialog-state.is-error > div {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.destroy-dialog-state.is-error span {
  color: var(--text-secondary);
  overflow-wrap: anywhere;
}

.destroy-warning {
  margin: 0;
  padding: 11px 12px;
  border-left: 3px solid var(--color-danger-500);
  color: var(--text-secondary);
  background: var(--color-danger-50);
  font-size: 0.8125rem;
  line-height: 1.6;
}

.destroy-resource-section {
  min-width: 0;
  border-top: 1px solid var(--border-subtle);
}

.destroy-resource-section header {
  height: 42px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: var(--text-primary);
  font-size: 0.8125rem;
  font-weight: 700;
}

.destroy-resource-section header b {
  min-width: 24px;
  height: 20px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0 6px;
  border-radius: 999px;
  color: var(--color-danger-700);
  background: var(--color-danger-100);
  font-size: 0.6875rem;
}

.destroy-resource-section.is-retained header b {
  color: var(--color-success-700);
  background: var(--color-success-100);
}

.destroy-resource-list {
  max-height: 230px;
  overflow-y: auto;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--bg-secondary);
}

.destroy-resource-row {
  min-height: 52px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
}

.destroy-resource-row.is-selectable {
  cursor: pointer;
}

.destroy-resource-selection {
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
  margin: 0;
  accent-color: var(--color-danger-600);
  cursor: pointer;
}

.destroy-resource-row + .destroy-resource-row {
  border-top: 1px solid var(--border-subtle);
}

.destroy-resource-icon {
  width: 30px;
  height: 30px;
  flex: 0 0 30px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 7px;
}

.destroy-resource-icon.is-delete {
  color: var(--color-danger-600);
  background: var(--color-danger-100);
}

.destroy-resource-icon.is-retained {
  color: var(--color-success-600);
  background: var(--color-success-100);
}

.destroy-resource-row > div {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.destroy-resource-row strong,
.destroy-resource-row small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.destroy-resource-row strong {
  color: var(--text-primary);
  font-size: 0.8125rem;
  font-weight: 600;
}

.destroy-resource-row small {
  color: var(--text-tertiary);
  font-size: 0.75rem;
}

.destroy-confirm-btn {
  color: #fff;
  background: var(--color-danger-600);
  border: 1px solid var(--color-danger-600);
}

.destroy-confirm-btn:not(:disabled):hover {
  background: var(--color-danger-700);
  border-color: var(--color-danger-700);
}

.content-scroll {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
  padding-bottom: 92px;
}

.compose-hero {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  max-width: 1680px;
  width: 100%;
  margin: 0 auto;
}

.hero-copy {
  min-width: 0;
}

.eyebrow {
  margin: 0 0 4px;
  color: var(--ops-muted);
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.hero-copy h1 {
  margin: 0;
  color: var(--ops-ink);
  font-size: clamp(1.65rem, 1.75vw, 2.25rem);
  line-height: 1.12;
  font-weight: 800;
  letter-spacing: 0;
}

.hero-copy p:last-child {
  margin: 6px 0 0;
  color: var(--ops-muted);
  font-size: 0.95rem;
}

.hero-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  flex-wrap: wrap;
}

.compose-rail {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 10px;
  max-width: 1680px;
  width: 100%;
  margin: 0 auto;
}

.rail-card {
  display: grid;
  grid-template-columns: 38px 1fr;
  grid-template-areas:
    "icon copy"
    "icon note";
  align-items: center;
  gap: 2px 10px;
  min-width: 0;
  padding: 12px;
  border: 1px solid var(--ops-line);
  border-radius: 12px;
  background: color-mix(in srgb, var(--ops-panel) 78%, transparent);
  box-shadow: 0 18px 48px color-mix(in srgb, var(--ops-ink) 7%, transparent);
  backdrop-filter: blur(12px);
}

.rail-icon {
  grid-area: icon;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  border-radius: 10px;
  color: var(--ops-blue);
  background: color-mix(in srgb, var(--ops-blue) 10%, var(--ops-panel));
}

.rail-card.success .rail-icon {
  color: var(--ops-green-strong);
  background: color-mix(in srgb, var(--ops-green) 14%, var(--ops-panel));
}

.rail-card.warning .rail-icon {
  color: var(--ops-amber);
  background: color-mix(in srgb, var(--ops-amber) 14%, var(--ops-panel));
}

.rail-card.muted .rail-icon {
  color: var(--ops-muted);
  background: color-mix(in srgb, var(--ops-muted) 13%, var(--ops-panel));
}

.rail-card.info .rail-icon {
  color: var(--ops-cyan);
  background: color-mix(in srgb, var(--ops-cyan) 12%, var(--ops-panel));
}

.rail-copy {
  grid-area: copy;
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}

.rail-copy strong {
  color: var(--ops-ink);
  font-size: 1.35rem;
  line-height: 1;
  font-weight: 800;
}

.rail-copy span,
.rail-card small {
  overflow: hidden;
  color: var(--ops-muted);
  font-size: 0.75rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rail-card small {
  grid-area: note;
}

/* 工具栏 */
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  max-width: 1680px;
  width: 100%;
  margin: 0 auto;
  flex-wrap: wrap;
}

.toolbar-left {
  display: grid;
  grid-template-columns: minmax(220px, 1fr) 190px minmax(240px, 1.15fr);
  align-items: center;
  gap: 12px;
  flex: 1;
  min-width: 0;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 0 0 auto;
  margin-left: auto;
}

.icon-btn.is-spinning svg {
  animation: resource-refresh-spin 0.75s linear infinite;
}

.sort-control {
  width: 190px;
  box-sizing: border-box;
  min-height: 38px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 8px 0 10px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--bg-elevated);
  color: var(--text-secondary);
}

.sort-control select {
  flex: 1;
  width: 0;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--text-primary);
  font: inherit;
  font-size: 0.8125rem;
  cursor: pointer;
}

.sort-control select option {
  background: var(--bg-elevated);
  color: var(--text-primary);
}

.sort-direction-btn {
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  cursor: pointer;
}

.sort-direction-btn:hover {
  color: var(--color-primary-600);
  background: var(--color-primary-100);
}

.sort-direction-btn.is-placeholder {
  visibility: hidden;
  pointer-events: none;
}

.compose-resource-board {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(280px, 300px);
  gap: 12px;
  align-items: stretch;
  max-width: 1680px;
  width: 100%;
  margin: 0 auto;
}

.compose-list-card,
.compose-context-panel {
  min-width: 0;
  min-height: 560px;
  border: 1px solid var(--ops-line);
  border-radius: 14px;
  background: color-mix(in srgb, var(--ops-surface) 90%, transparent);
  box-shadow: 0 18px 48px color-mix(in srgb, var(--ops-ink) 7%, transparent);
  backdrop-filter: blur(14px);
}

.compose-list-card {
  overflow: hidden;
}

.compose-list-card :deep(.card-grid-wrapper) {
  height: 100%;
  padding: 16px;
}

.compose-list-card :deep(.card-grid) {
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 16px;
  padding: 0;
}

.compose-list-card :deep(.compose-card) {
  min-height: 236px;
  aspect-ratio: auto;
  border-color: var(--ops-line);
  border-radius: 12px;
}

.compose-list-card :deep(.compose-card.is-selected) {
  border-color: color-mix(in srgb, var(--ops-green) 62%, var(--ops-line));
  box-shadow:
    inset 4px 0 0 var(--ops-green),
    0 18px 42px color-mix(in srgb, var(--ops-green) 14%, transparent);
  animation: compose-card-select var(--motion-duration-fast) var(--motion-ease-out);
}

.compose-list-card :deep(.resource-row) {
  transition:
    background var(--motion-duration-quick) var(--motion-ease-out),
    box-shadow var(--motion-duration-quick) var(--motion-ease-out);
}

.compose-list-card :deep(.resource-row:hover) {
  background: var(--ops-row-hover);
}

.compose-list-card :deep(.resource-row:active) {
  background: var(--ops-row-active);
}

.compose-list-card :deep(.resource-row.is-selected) {
  animation: compose-row-select var(--motion-duration-quick) var(--motion-ease-out);
  background: var(--ops-row-hover);
  box-shadow: inset 4px 0 0 var(--ops-green);
}

.compose-list-card :deep(.resource-row:focus-visible) {
  background: var(--ops-row-focus);
  outline: 2px solid color-mix(in srgb, var(--ops-green) 36%, transparent);
  outline-offset: -2px;
}

@keyframes compose-row-compress {
  0% {
    transform: none;
  }
  42% {
    transform: none;
  }
  100% {
    transform: none;
  }
}

@keyframes compose-row-select {
  0% {
    background: var(--ops-row-hover);
  }
  100% {
    background: var(--ops-row-hover);
  }
}

@keyframes compose-card-select {
  0% {
    transform: translateY(0) scale(0.985);
  }
  70% {
    transform: translateY(0) scale(1.006);
  }
  100% {
    transform: translateY(0) scale(1);
  }
}

@media (prefers-reduced-motion: reduce) {
  .compose-list-card :deep(.compose-card.is-selected),
  .compose-list-card :deep(.resource-row.is-selected) {
    animation: none;
  }
}

.compose-context-panel {
  position: sticky;
  top: 0;
  display: flex;
  flex-direction: column;
  align-self: start;
  gap: 16px;
  height: 100%;
  padding: 16px;
}

.context-header {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.resource-kind {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 34px;
  height: 34px;
  border-radius: 10px;
  color: var(--ops-green-strong);
  background: color-mix(in srgb, var(--ops-green) 14%, var(--ops-panel));
}

.context-header div {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.context-header strong {
  overflow: hidden;
  color: var(--ops-ink);
  font-size: 1.05rem;
  line-height: 1.1;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.context-header span {
  overflow: hidden;
  color: var(--ops-muted);
  font-size: 0.75rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.context-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.context-grid div {
  display: grid;
  gap: 4px;
  padding: 12px;
  border: 1px solid var(--ops-line);
  border-radius: 10px;
  background: color-mix(in srgb, var(--ops-panel) 72%, transparent);
}

.context-grid span,
.section-label {
  color: var(--ops-muted);
  font-size: 0.71875rem;
  font-weight: 800;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.context-grid strong {
  overflow: hidden;
  color: var(--ops-ink);
  font-size: 1rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.context-section {
  display: grid;
  gap: 8px;
}

.section-label {
  margin: 0;
}

.detail-lines {
  display: grid;
  gap: 6px;
}

.detail-lines span {
  display: grid;
  grid-template-columns: 48px minmax(0, 1fr);
  gap: 8px;
  min-height: 28px;
  align-items: center;
  padding: 0 9px;
  border: 1px solid var(--ops-line);
  border-radius: 8px;
  background: color-mix(in srgb, var(--ops-panel) 76%, transparent);
  color: var(--ops-ink);
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-size: 0.75rem;
}

.detail-lines b {
  color: var(--ops-muted);
  font-size: 0.6875rem;
}

.uptime-lines span {
  grid-template-columns: minmax(80px, 1fr) minmax(72px, auto);
  align-items: center;
  align-content: center;
  line-height: 1.5;
}

.uptime-lines b {
  align-self: center;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.uptime-cell {
  align-self: center;
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
  min-width: 0;
  line-height: 1.5;
}

.update-tag {
  flex: 0 0 auto;
  font-style: normal;
  display: inline-flex;
  align-items: center;
  padding: 1px 7px;
  border-radius: 999px;
  font-size: 0.6875rem;
  font-weight: 800;
  color: var(--ops-amber);
  background: var(--color-warning-100);
  border: 1px solid color-mix(in srgb, var(--ops-amber) 40%, var(--ops-line));
}

.log-stream-preview {
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  max-width: 100%;
  flex: 1 1 auto;
  min-height: 156px;
  padding: 12px;
  border: 1px solid var(--ops-line);
  border-radius: 12px;
  background:
    linear-gradient(90deg, color-mix(in srgb, var(--ops-ink) 5%, transparent) 1px, transparent 1px),
    linear-gradient(180deg, color-mix(in srgb, var(--ops-ink) 5%, transparent) 1px, transparent 1px),
    color-mix(in srgb, var(--ops-panel) 78%, transparent);
  background-size: 18px 18px;
  overflow: hidden;
}

.secondary-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 38px;
  padding: 0 14px;
  color: var(--text-secondary);
  background: var(--bg-secondary);
  border: 1px solid var(--border-default);
  border-radius: 10px;
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.secondary-btn:hover {
  color: var(--color-primary-600);
  background: var(--color-primary-50);
  border-color: var(--color-primary-300);
}

/* 分页 */
.pagination-wrapper {
  position: fixed;
  bottom: 0;
  left: 260px;
  right: 0;
  background: var(--bg-primary);
  border-top: 1px solid var(--border-subtle);
  padding: 12px 24px;
  z-index: 100;
}

.compose-rail {
  gap: 8px;
}

.workbench-toolbar {
  padding: 12px;
  border: 1px solid var(--ops-line);
  border-radius: 12px;
  background: color-mix(in srgb, var(--ops-surface) 90%, transparent);
  box-shadow: 0 18px 48px color-mix(in srgb, var(--ops-ink) 7%, transparent);
}

.rail-card {
  min-height: 64px;
  padding: 10px 12px;
}

.rail-icon {
  width: 34px;
  height: 34px;
  border-radius: 9px;
}

.rail-copy strong {
  font-size: 1.18rem;
}

.rail-copy span,
.rail-card small {
  font-size: 0.7rem;
}

@media (max-width: 1280px) {
  .compose-rail {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .compose-resource-board {
    grid-template-columns: 1fr;
  }

  .compose-context-panel {
    position: static;
    min-height: 360px;
  }
}

@media (max-width: 900px) {
  .compose-page {
    padding: 16px 16px 0;
  }

  .compose-hero,
  .hero-actions {
    align-items: stretch;
    flex-direction: column;
  }

  .hero-actions {
    flex-direction: row;
    justify-content: flex-start;
  }

  .compose-rail,
  .toolbar-left {
    grid-template-columns: 1fr;
  }

  .pagination-wrapper {
    left: 0 !important;
  }
}

/* DetailDrawer 头部样式 */
.drawer-header-compose {
  display: flex;
  flex-direction: column;
  gap: 6px;
  flex: 1;
  min-width: 0;
}

.drawer-header-main {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.drawer-header-label {
  font-size: 0.75rem;
  color: var(--text-tertiary);
  font-weight: 400;
  flex-shrink: 0;
}

.drawer-header-name {
  font-size: 1.5rem;
  font-weight: 700;
  color: var(--text-primary);
  margin: 0;
  line-height: 1.3;
}

.drawer-header-path {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 0.8125rem;
  color: var(--text-secondary);
}

.drawer-header-path svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
  flex-shrink: 0;
}

.path-value {
  font-family: 'JetBrains Mono', monospace;
  color: var(--text-tertiary);
}

/* 状态徽章 */
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: 9999px;
  font-size: 0.8125rem;
  font-weight: 600;
  background: var(--color-gray-100);
  color: var(--color-gray-600);
}

.status-badge.running {
  background: var(--color-success-100);
  color: var(--color-success-700);
}

.status-badge.stopped {
  background: var(--color-gray-100);
  color: var(--color-gray-600);
}

.status-badge.partial {
  background: var(--color-warning-100);
  color: var(--color-warning-700);
}

.status-badge .status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}

.status-dot.running {
  background: var(--color-success-500);
}

.status-dot.stopped {
  background: var(--color-gray-400);
}

.status-dot.partial {
  background: var(--color-warning-500);
}
</style>
