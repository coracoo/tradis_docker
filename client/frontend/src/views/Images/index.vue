<template>
  <ResourceWorkbench class="images-page images-workbench" role="main" aria-label="镜像管理" remote-context>
    <template #rail>
      <section class="resource-rail" aria-label="镜像概览">
        <article v-for="item in imageRail" :key="item.label" class="rail-card" :class="item.tone">
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

    <template #toolbar>
      <div
        class="toolbar workbench-toolbar images-toolbar"
        :class="{ 'images-toolbar--list': activeTab === 'images' }"
      >
      <div class="toolbar-left workbench-toolbar-left">
        <template v-if="activeTab === 'images'">
          <SearchInput
            v-model="searchQuery"
            placeholder="搜索镜像名称、标签、ID..."
            @search="handleSearch"
          />
          <div class="sort-control">
            <DynamicIcon name="arrow-up-down" :size="15" />
            <select v-model="sortField" aria-label="镜像排序字段">
              <option value="">默认排序</option>
              <option value="name">按名称</option>
              <option value="tags">按标签</option>
              <option value="status">按状态</option>
              <option value="update">按更新</option>
              <option value="size">按大小</option>
              <option value="created">按创建时间</option>
            </select>
            <button
              type="button"
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
        </template>
        <SegmentedTabs
          v-model="activeTab"
          class="images-tabs toolbar-tabs"
          :options="imageTabOptions"
          aria-label="镜像视图"
          compact
        />
        <button
          v-if="activeTab === 'images'"
          data-remote-write
          class="check-update-btn"
          :class="{ 'is-spinning': checkingUpdates }"
          title="检测更新"
          @click="checkUpdates"
        >
          <DynamicIcon name="cloud-download" :size="14" />
          <span>检测更新</span>
        </button>
        <span v-else class="cache-summary">
          共 {{ cacheCount }} 项，{{ cacheActiveCount }} 项使用中，占用 {{ cacheTotalSize }}
        </span>
      </div>
      <div v-if="activeTab === 'images'" class="images-filter-control">
        <SegmentedTabs
          v-model="imageFilters.update"
          :options="updateFilterOptions"
          aria-label="更新筛选"
          compact
        />
        <SegmentedTabs
          v-model="imageFilters.usage"
          :options="usageFilterOptions"
          aria-label="使用筛选"
          compact
        />
      </div>
      <template v-if="activeTab === 'images'">
        <div class="toolbar-right">
          <button
            v-if="viewMode !== 'grid' && checkedImages.length > 0"
            v-ripple
            data-remote-write
            class="secondary-btn batch-danger"
            type="button"
            :disabled="batchWorking"
            title="批量删除已勾选的镜像"
            @click="batchRemoveImages"
          >
            <DynamicIcon name="delete" :size="16" />
            批量删除（{{ checkedImages.length }}）
          </button>
          <ViewToggle v-model="viewMode" />
          <button
            v-ripple
            class="icon-btn resource-refresh-btn"
            :class="{ 'is-spinning': imagesBusy }"
            :disabled="imagesBusy"
            title="刷新镜像列表"
            @click="refreshImages"
          >
            <DynamicIcon class="resource-refresh-icon" name="refresh" :size="18" />
          </button>
          <button class="icon-btn" data-remote-write title="导入镜像" @click="showImportDialog = true">
            <DynamicIcon name="upload" :size="18" />
          </button>
          <div class="dropdown-wrapper">
            <button class="icon-btn" data-remote-write :class="{ 'is-active': showMoreMenu }" title="更多操作" @click.stop="toggleMoreMenu($event)">
              <DynamicIcon name="more-vertical" :size="18" />
            </button>
          </div>
          <button class="primary-btn" data-remote-write @click="showPullDialog = true">
            <DynamicIcon name="plus" :size="16" />
            拉取镜像
          </button>
        </div>
      </template>
      <template v-else>
        <div class="toolbar-right">
          <button class="icon-btn" title="刷新" @click="cachePanel?.loadData()">
            <DynamicIcon name="refresh" :size="18" />
          </button>
          <button class="tb-btn tb-btn-warning" data-remote-write @click="cachePanel?.pruneUnused()">
            <DynamicIcon name="trash" :size="14" />
            <span>清理悬空</span>
          </button>
          <button class="tb-btn tb-btn-danger" data-remote-write @click="cachePanel?.pruneAll()">
            <DynamicIcon name="trash" :size="14" />
            <span>清理全部未使用</span>
          </button>
        </div>
      </template>
      </div>
    </template>

    <AnchoredMenu
      :open="showMoreMenu"
      :anchor="toolbarMoreMenuAnchor"
      @update:open="handleMoreMenuOpenChange"
    >
      <button type="button" role="menuitem" @click="applyUpdates">
        <DynamicIcon name="cloud-download" :size="16" />
        一键更新
      </button>
      <button type="button" role="menuitem" @click="openSettings">
        <DynamicIcon name="settings" :size="16" />
        镜像加速
      </button>
      <div class="dropdown-divider" role="separator"></div>
      <button type="button" role="menuitem" class="danger" @click="pruneImages">
        <DynamicIcon name="delete" :size="16" />
        清理无用镜像
      </button>
    </AnchoredMenu>

    <ResourcePanel v-if="activeTab === 'build-cache'" class="build-cache-resource-panel">
      <BuildCachePanel ref="cachePanel" />
    </ResourcePanel>

    <ResourceBoard v-else>
      <ResourcePanel>
        <CardGrid
          v-if="viewMode === 'grid'"
          :items="paginatedImages"
          :loading="loading"
          :skeleton-count="12"
          :empty-props="emptyProps"
        >
          <template #default="{ items }">
            <ImageCard
              v-for="image in items"
              :key="image.Id"
              :image="image"
              :in-use-ids="inUseImageIds"
              :class="{ 'is-selected': selectedSummaryImage?.Id === image.Id }"
              @click="selectImage(image)"
              @export="handleExport(image)"
              @tag="handleTag(image)"
              @remove="handleRemove(image)"
              @update="handleUpdate(image)"
            />
          </template>
        </CardGrid>

        <ResourceTable
          v-else
          :columns="imageColumns"
          :rows="paginatedImages"
          table-key="images-v3"
          row-key="Id"
          :selected-key="selectedSummaryImage?.Id || ''"
          :loading="loading"
          aria-label="镜像列表"
          @select="selectImage"
          @sort="handleSort"
        >
          <template #header-check>
            <input
              class="image-batch-check"
              type="checkbox"
              :checked="allVisibleChecked"
              :indeterminate.prop="partiallyChecked"
              :disabled="paginatedImages.length === 0"
              title="全选/取消当前列表全部镜像"
              @click.stop
              @change="toggleImageCheckAll"
            />
          </template>

          <template #cell-check="{ row: image }">
            <input
              class="image-batch-check"
              type="checkbox"
              :checked="checkedImageIds.includes(image.Id)"
              title="勾选后可批量删除"
              @click.stop
              @change="toggleImageCheck(image.Id)"
            />
          </template>

          <template v-for="column in imageSortableColumns" :key="column.key" #[`header-${column.key}`]>
            {{ column.label }}
            <span class="sort-icon" :class="getSortIcon(column.key)">
              <DynamicIcon v-if="getSortIcon(column.key) === 'asc'" name="chevron-up" :size="14" />
              <DynamicIcon v-else-if="getSortIcon(column.key) === 'desc'" name="chevron-down" :size="14" />
              <DynamicIcon v-else name="arrow-up-down" :size="14" />
            </span>
          </template>

          <template #cell-name="{ row: image }">
            <button class="resource-name-button" type="button" @click.stop="selectImage(image)">
              <span class="resource-kind image">
                <DynamicIcon class="image-icon-small" name="image" :size="16" />
              </span>
              <span class="resource-name-copy">
                <span class="image-name-line">
                  <strong>{{ getImageName(image) }}</strong>
                  <span
                    v-if="getImageTagList(image).length"
                    class="image-inline-tags"
                    :title="getImageTags(image)"
                  >
                    <span
                      v-for="tag in getImageTagList(image).slice(0, 3)"
                      :key="tag"
                      class="image-inline-tag"
                    >
                      {{ tag }}
                    </span>
                    <span v-if="getImageTagList(image).length > 3" class="image-inline-tag more">
                      +{{ getImageTagList(image).length - 3 }}
                    </span>
                  </span>
                </span>
                <small>{{ shortImageId(image.Id) }}</small>
              </span>
            </button>
          </template>
          <template #cell-status="{ row: image }">
            <StatusBadge :status="inUseImageIds.has(image.Id) ? 'used' : 'unused'" variant="minimal" />
          </template>
          <template #cell-update="{ row: image }">
            <StatusBadge v-if="image.hasUpdate" status="update" label="可更新" variant="minimal" />
            <span v-else class="empty-cell">-</span>
          </template>
          <template #cell-size="{ row: image }">{{ formatBytes(image.Size) }}</template>
          <template #cell-created="{ row: image }">{{ formatTime(image.Created) }}</template>
          <template #cell-actions="{ row: image }">
            <div class="cell-actions" data-remote-write>
              <button class="table-btn success" title="运行" @click.stop="handleRun(image)">
                <DynamicIcon name="play" :size="14" />
              </button>
              <button v-if="image.hasUpdate" class="table-btn update" title="更新" @click.stop="handleUpdate(image)">
                <DynamicIcon name="cloud-download" :size="14" />
              </button>
              <button class="table-btn" title="导出" @click.stop="handleExport(image)">
                <DynamicIcon name="download" :size="14" />
              </button>
              <button class="table-btn" title="修改标签" @click.stop="handleTag(image)">
                <DynamicIcon name="tag" :size="14" />
              </button>
              <button class="table-btn danger" :disabled="inUseImageIds.has(image.Id)" title="删除" @click.stop="handleRemove(image)">
                <DynamicIcon name="delete" :size="14" />
              </button>
            </div>
          </template>
          <template #empty>
            <EmptyState v-bind="emptyProps" />
          </template>
        </ResourceTable>

        <template #footer>
          <Pagination
            v-model:current="currentPage"
            v-model:page-size="pageSize"
            :total="filteredImages.length"
          />
        </template>
      </ResourcePanel>

      <template #context>
        <ResourceContextPanel :motion-key="selectedSummaryImage?.Id || 'overview'" aria-label="镜像上下文">
        <div class="context-header">
          <span class="resource-kind image">
            <DynamicIcon name="image" :size="18" />
          </span>
          <div>
            <strong>{{ selectedSummaryImage ? getImageName(selectedSummaryImage) : '镜像概览' }}</strong>
            <span v-if="selectedSummaryImage">{{ getImageTags(selectedSummaryImage) }}</span>
            <span v-else>{{ filteredImages.length }} 个匹配镜像</span>
          </div>
        </div>

        <div v-if="selectedSummaryImage" class="context-actions">
          <button type="button" class="detail-action success" data-remote-write @click="handleRun(selectedSummaryImage)">
            <DynamicIcon name="play" :size="15" />
            运行
          </button>
          <button type="button" class="detail-action info" @click="openImageDetail(selectedSummaryImage)">
            <DynamicIcon name="eye" :size="15" />
            查看详情
          </button>
          <button type="button" class="detail-action info" data-remote-write @click="handleTag(selectedSummaryImage)">
            <DynamicIcon name="tag" :size="15" />
            标签
          </button>
          <button
            type="button"
            class="detail-action danger"
            data-remote-write
            :disabled="inUseImageIds.has(selectedSummaryImage.Id)"
            @click="handleRemove(selectedSummaryImage)"
          >
            <DynamicIcon name="delete" :size="15" />
            删除
          </button>
        </div>

        <div class="context-grid">
          <div>
            <span>状态</span>
            <strong>{{ selectedSummaryImage ? (inUseImageIds.has(selectedSummaryImage.Id) ? '使用中' : '未使用') : '全部' }}</strong>
          </div>
          <div>
            <span>更新</span>
            <strong>{{ selectedSummaryImage?.hasUpdate ? '可更新' : '-' }}</strong>
          </div>
          <div>
            <span>大小</span>
            <strong>{{ selectedSummaryImage ? formatBytes(selectedSummaryImage.Size) : imageRail[3]?.value }}</strong>
          </div>
          <div>
            <span>创建</span>
            <strong>{{ selectedSummaryImage ? formatTime(selectedSummaryImage.Created) : '-' }}</strong>
          </div>
        </div>

        <div v-if="selectedSummaryImage" class="context-section">
          <p class="section-label">标签 / ID</p>
          <div class="detail-lines">
            <span>
              <b>TAGS</b>
              <em>{{ getImageTags(selectedSummaryImage) }}</em>
            </span>
            <span>
              <b>ID</b>
              <em>{{ shortImageId(selectedSummaryImage.Id) }}</em>
            </span>
          </div>
        </div>
        <ResourceRelationList
          title="关联容器"
          :items="selectedImageContainerRelations"
          clickable
          empty-text="没有容器使用此镜像"
          @select="openContainerRelation"
        />
        </ResourceContextPanel>
      </template>
    </ResourceBoard>

    <!-- 拉取镜像对话框 -->
    <Modal
      v-model:visible="showPullDialog"
      :title="isUpdateMode ? '更新镜像' : '拉取镜像'"
      width="560px"
      :show-close="true"
      :close-on-esc="true"
    >
      <div class="form-group">
        <label class="form-label">镜像名称</label>
        <input v-model="pullForm.name" class="form-input" placeholder="例如: nginx:latest" :disabled="isUpdateMode" />
        <p class="form-hint">支持格式: nginx, nginx:latest, nginx:1.21</p>
      </div>
      <div class="form-group">
        <label class="form-label">镜像源</label>
        <select v-model="pullForm.registry" class="form-select" :disabled="isUpdateMode">
          <option value="">默认 (Docker Hub)</option>
          <option v-for="reg in registries" :key="reg.url" :value="reg.url">{{ reg.name }}</option>
        </select>
      </div>
      
      <!-- 拉取进度 -->
      <div v-if="pullProgress.show" class="pull-progress">
        <div class="progress-header">
          <span class="progress-status">{{ pullProgress.status }}</span>
          <span class="progress-percent">{{ pullProgress.progress }}%</span>
        </div>
        <div class="progress-bar">
          <div class="progress-fill" :style="{ width: pullProgress.progress + '%' }"></div>
        </div>
        <div v-if="pullProgress.details.length" class="progress-details">
          <div class="progress-detail-head" aria-hidden="true">
            <span>时间</span>
            <span>层</span>
            <span>状态</span>
          </div>
          <div v-for="detail in pullProgress.details.slice(0, 6)" :key="detail.id" class="progress-detail-item">
            <time class="detail-time">{{ detail.time || '--:--:--' }}</time>
            <span class="detail-id" :title="detail.id">{{ detail.id }}</span>
            <span class="detail-status">{{ detail.status }}</span>
          </div>
        </div>
      </div>
      
      <template #footer>
        <button class="btn btn-default" @click="showPullDialog = false">
          {{ pullProgress.show ? '后台运行并关闭' : '取消' }}
        </button>
        <button class="btn btn-primary" :disabled="!pullForm.name || pulling || pullProgress.show" @click="handlePullImage">
          <span v-if="pulling || pullProgress.show" class="btn-spinner"></span>
          {{ pullProgress.show ? '拉取中...' : (isUpdateMode ? '开始更新' : '开始拉取') }}
        </button>
      </template>
    </Modal>

    <!-- 导入镜像对话框 -->
    <Modal
      v-model:visible="showImportDialog"
      title="导入镜像"
      width="500px"
      :show-close="!importing"
      :close-on-esc="!importing"
    >
      <div class="upload-area" :class="{ 'is-dragover': isDragover }" @drop="handleDrop" @dragover.prevent="isDragover = true" @dragleave="isDragover = false">
        <input ref="fileInput" type="file" accept=".tar,.tar.gz" class="file-input" @change="handleFileSelect" />
        <div class="upload-content" @click="$refs.fileInput.click()">
          <DynamicIcon name="upload" :size="48" />
          <p class="upload-text">拖拽文件到此处，或 <span class="upload-link">点击上传</span></p>
          <p class="upload-hint">支持 .tar 和 .tar.gz 格式</p>
        </div>
      </div>
      <div v-if="importFile" class="file-info">
        <span class="file-name">{{ importFile.name }}</span>
        <span class="file-size">{{ formatBytes(importFile.size) }}</span>
      </div>
      <template #footer>
        <button class="btn btn-default" @click="showImportDialog = false">取消</button>
        <button class="btn btn-primary" :disabled="!importFile || importing" @click="importImage">
          <span v-if="importing" class="btn-spinner"></span>
          {{ importing ? '导入中...' : '开始导入' }}
        </button>
      </template>
    </Modal>

    <!-- 删除标签对话框 -->
    <Modal v-model:visible="showRemoveTagDialog" title="删除镜像标签" width="500px">
      <div class="remove-tag-dialog">
        <p class="dialog-hint">该镜像有多个标签，请选择要删除的标签（取消标签而不删除镜像）：</p>
        <div class="tag-list">
          <label 
            v-for="(tag, index) in removeTagForm.tags" 
            :key="index"
            class="tag-checkbox-item"
          >
            <input 
              v-model="tag.selected" 
              type="checkbox"
            />
            <span class="tag-name">{{ tag.name }}</span>
          </label>
        </div>
        <label class="select-all-label">
          <input 
            v-model="removeTagForm.allSelected" 
            type="checkbox"
            @change="toggleSelectAllTags"
          />
          <span>全选（强制删除整个镜像）</span>
        </label>
      </div>
      <template #footer>
        <button class="btn btn-default" @click="showRemoveTagDialog = false">取消</button>
        <button 
          class="btn btn-danger" 
          :disabled="removeTagsWorking || !removeTagForm.tags.some(t => t.selected)"
          @click="confirmRemoveTags"
        >
          确认删除
        </button>
      </template>
    </Modal>

    <!-- 更新镜像标签对话框（多 tag 单选） -->
    <Modal v-model:visible="showUpdateTagDialog" title="更新镜像标签" width="500px">
      <div class="remove-tag-dialog">
        <p class="dialog-hint">该镜像有多个标签，请选择要更新的标签（一次只更新一个，更新完成后镜像将拆分为两个）：</p>
        <div class="tag-list">
          <label
            v-for="(tag, index) in updateTagForm.tags"
            :key="index"
            class="tag-checkbox-item"
          >
            <input
              :checked="tag.selected"
              type="checkbox"
              @change="selectUpdateTag(index)"
            />
            <span class="tag-name" :class="{ 'has-update': tag.hasUpdate }">{{ tag.name }}</span>
            <span v-if="tag.hasUpdate" class="tag-update-badge">可更新</span>
          </label>
        </div>
      </div>
      <template #footer>
        <button class="btn btn-default" @click="showUpdateTagDialog = false">取消</button>
        <button
          class="btn btn-primary"
          :disabled="!updateTagForm.tags.some(t => t.selected)"
          @click="confirmUpdateTags"
        >
          开始更新
        </button>
      </template>
    </Modal>

    <!-- 修改标签对话框 -->
    <Modal v-model:visible="showTagDialog" title="修改标签" width="500px">
      <div class="form-group">
        <label class="form-label">仓库地址</label>
        <input v-model="tagForm.repo" class="form-input" placeholder="例如: nginx" />
      </div>
      <div class="form-group">
        <label class="form-label">标签</label>
        <input v-model="tagForm.tag" class="form-input" placeholder="例如: latest" />
      </div>
      <div class="form-group">
        <label class="checkbox-label">
          <input v-model="tagForm.removeOld" type="checkbox" />
          <span>同时删除原标签</span>
        </label>
        <p v-if="tagForm.removeOld && tagForm.oldRepoTag" class="form-hint">
          将删除: {{ tagForm.oldRepoTag }}
        </p>
      </div>
      <template #footer>
        <button class="btn btn-default" @click="showTagDialog = false">取消</button>
        <button class="btn btn-primary" :disabled="!tagForm.repo || !tagForm.tag" @click="saveTag">保存</button>
      </template>
    </Modal>

    <!-- 镜像详情对话框 -->
    <Modal v-model:visible="showDetailDialog" title="镜像详情" width="760px">
      <div v-if="detailLoading" class="detail-loading">加载中...</div>
      <div v-else-if="imageDetail" class="image-detail">
        <div class="detail-section">
          <h4>基础信息</h4>
          <div class="detail-grid">
            <span>名称</span>
            <strong>{{ detailDisplayName }}</strong>
            <span>ID</span>
            <code>{{ shortImageId(imageDetail.image?.Id || imageDetail.image?.ID) }}</code>
            <span>大小</span>
            <strong>{{ formatBytes(imageDetail.image?.Size || 0) }}</strong>
            <span>创建时间</span>
            <strong>{{ formatTimeFromValue(imageDetail.image?.Created) }}</strong>
            <span>Digest</span>
            <code>{{ firstDigest }}</code>
          </div>
        </div>

        <div class="detail-section">
          <h4>标签</h4>
          <div class="detail-tags">
            <span v-for="tag in detailTags" :key="tag" class="detail-tag">{{ tag }}</span>
            <span v-if="detailTags.length === 0" class="muted">无标签</span>
          </div>
        </div>

        <div class="detail-section">
          <h4>关联容器</h4>
          <div v-if="detailContainers.length" class="related-list">
            <div v-for="container in detailContainers" :key="container.id" class="related-item">
              <strong>{{ container.name }}</strong>
              <span>{{ container.state }} · {{ container.image }}</span>
            </div>
          </div>
          <p v-else class="muted">当前没有容器使用该镜像</p>
        </div>

        <div class="detail-section">
          <h4>构建历史</h4>
          <div v-if="imageHistory.length" class="history-list">
            <div v-for="item in imageHistory.slice(0, 8)" :key="`${item.ID}-${item.Created}`" class="history-item">
              <code>{{ shortImageId(item.ID) }}</code>
              <span>{{ formatBytes(item.Size || 0) }}</span>
              <span class="history-command" :title="item.CreatedBy">{{ item.CreatedBy || '-' }}</span>
            </div>
          </div>
          <p v-else class="muted">暂无历史记录</p>
        </div>
      </div>
      <template #footer>
        <button class="btn btn-default" @click="showDetailDialog = false">关闭</button>
      </template>
    </Modal>

    <!-- Docker 设置（镜像加速） -->
    <DockerSettings v-model="settingsVisible" />
  </ResourceWorkbench>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useDockerResourcesStore } from '@/stores/dockerResources.js'
import { useEnvironmentContext } from '@/composables/useEnvironmentContext.js'
import { useUiStore } from '@/stores/ui.js'
import { images } from '@edition/api'
import { currentEnvironmentId } from '@edition/current-environment'
import { formatBytes, formatDockerImageReference, formatTimeTwoLines } from '@/utils/format.js'
import { useViewMode } from '@/composables/useViewMode.js'
import { useSort } from '@/composables/useSort.js'
// 组件
import SearchInput from '@/components/ui/SearchInput.vue'
import SegmentedTabs from '@/components/ui/SegmentedTabs.vue'
import ViewToggle from '@/components/ui/ViewToggle.vue'
import CardGrid from '@/components/data-display/CardGrid.vue'
import ImageCard from '@/components/data-display/ImageCard.vue'
import Pagination from '@/components/ui/Pagination.vue'
import DockerSettings from './components/DockerSettings.vue'
import Modal from '@/components/feedback/Modal.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import AnchoredMenu from '@/components/ui/AnchoredMenu.vue'
import BuildCachePanel from './components/BuildCachePanel.vue'
import ResourceWorkbench from '@/components/resource-workbench/ResourceWorkbench.vue'
import ResourceBoard from '@/components/resource-workbench/ResourceBoard.vue'
import ResourcePanel from '@/components/resource-workbench/ResourcePanel.vue'
import ResourceContextPanel from '@/components/resource-workbench/ResourceContextPanel.vue'
import ResourceTable from '@/components/resource-workbench/ResourceTable.vue'
import ResourceRelationList from '@/components/resource-workbench/ResourceRelationList.vue'
import { usePageAction } from '@/composables/usePageAction.js'
import { upsertPullProgressDetail } from './pullProgress.js'
import { matchesBooleanFilter } from '@/utils/resourceFilters.js'
import { notifyBatchResult } from '@/utils/batchFeedback.js'

const dockerResources = useDockerResourcesStore()
const { isRemoteEnvironment } = useEnvironmentContext()
const uiStore = useUiStore()
const router = useRouter()

// localStorage key for pull state persistence
const PULL_STATE_KEY = 'tradis_pull_state'

// 保存拉取状态到 localStorage
function savePullState(state) {
  localStorage.setItem(PULL_STATE_KEY, JSON.stringify({
    ...state,
    savedAt: Date.now()
  }))
}

// 从 localStorage 加载拉取状态
function loadPullState() {
  try {
    const saved = localStorage.getItem(PULL_STATE_KEY)
    if (!saved) return null
    const state = JSON.parse(saved)
    // 检查是否过期（超过 1 小时视为无效）
    if (state.savedAt && Date.now() - state.savedAt > 3600000) {
      localStorage.removeItem(PULL_STATE_KEY)
      return null
    }
    return state
  } catch {
    return null
  }
}

// 清除 localStorage 中的拉取状态
function clearPullState() {
  localStorage.removeItem(PULL_STATE_KEY)
}

// 发送浏览器通知
async function sendNotification(title, body) {
  if (!('Notification' in window)) return
  if (Notification.permission === 'granted') {
    new Notification(title, { body, icon: '/tradis-logo.png' })
  } else if (Notification.permission !== 'denied') {
    const permission = await Notification.requestPermission()
    if (permission === 'granted') {
      new Notification(title, { body, icon: '/tradis-logo.png' })
    }
  }
}

// 状态
const loading = computed(() => {
  const resource = dockerResources.resources.images
  return resource.loading || resource.refreshing
})
const imagesBusy = computed(() => loading.value)
const imagesList = computed(() => dockerResources.imageList)
const containerList = computed(() => dockerResources.imageContainerList)

function normalizedDockerId(value) {
  return String(value || '').replace(/^sha256:/, '')
}

function containerDisplayName(container) {
  return String(container?.Names?.[0] || container?.Name || container?.Id || '')
    .replace(/^\//, '')
}

const selectedImageContainerRelations = computed(() => {
  const image = selectedSummaryImage.value
  if (!image) return []
  const imageId = normalizedDockerId(image.Id)
  const imageTags = new Set((image.RepoTags || []).map(formatDockerImageReference))

  return containerList.value
    .filter((container) => {
      const containerImageId = normalizedDockerId(container.ImageID || container.ImageId)
      return (imageId && containerImageId === imageId)
        || imageTags.has(formatDockerImageReference(container.Image))
    })
    .map(container => ({
      id: container.Id,
      name: containerDisplayName(container) || '未命名容器',
      meta: `${String(container.Id || '').slice(0, 12) || '-'} · ${container.State || 'unknown'}`
    }))
})
const searchQuery = ref('')
const imageFilters = ref({ update: 'all', usage: 'all' })
const updateFilterOptions = [
  { value: 'all', label: '全部' },
  { value: 'yes', label: '可更新' },
  { value: 'no', label: '不可更新' }
]
const usageFilterOptions = [
  { value: 'all', label: '全部' },
  { value: 'yes', label: '已使用' },
  { value: 'no', label: '未使用' }
]
const activeTab = ref('images')
const imageTabOptions = computed(() => isRemoteEnvironment.value
  ? [{ value: 'images', label: '镜像列表' }]
  : [
      { value: 'images', label: '镜像列表' },
      { value: 'build-cache', label: '构建缓存' }
    ])
const cachePanel = ref(null)
const cacheCount = computed(() => cachePanel.value?.buildCache?.length ?? 0)
const cacheActiveCount = computed(() => cachePanel.value?.activeCount ?? 0)
const cacheTotalSize = computed(() => formatBytes(cachePanel.value?.totalSize ?? 0))
const { viewMode } = useViewMode('images_view_mode', 'table')
const currentPage = ref(1)
const pageSize = ref(20)

// 排序
const { sortState, handleSort, setSort, sortData, getSortIcon } = useSort('sort_images')
const imageColumns = [
  { key: 'check', label: '', cellClass: 'cell-check', width: 40, minWidth: 36 },
  { key: 'name', label: '名称', sortable: true, cellClass: 'cell-name', width: 320, minWidth: 240, flex: true },
  { key: 'status', label: '状态', sortable: true, width: 84, minWidth: 76 },
  { key: 'update', label: '更新', sortable: true, width: 84, minWidth: 76 },
  { key: 'size', label: '大小', sortable: true, width: 82, minWidth: 72 },
  { key: 'created', label: '创建时间', sortable: true, width: 112, minWidth: 96 },
  { key: 'actions', label: '操作', cellClass: 'cell-actions-column', width: 190, minWidth: 164 }
]
const imageSortableColumns = imageColumns.filter(column => column.sortable)
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
const checkingUpdates = ref(false)
const showMoreMenu = ref(false)
const toolbarMoreMenuAnchor = ref(null)
const updating = ref(false)

// 批量删除：仅表格视图提供勾选，勾选键为镜像 ID。
const checkedImageIds = ref([])
const batchWorking = ref(false)
// 批量目标取当前过滤结果中被勾选的镜像，保证计数与用户可见范围一致。
const checkedImages = computed(() =>
  filteredImages.value.filter(image => checkedImageIds.value.includes(image.Id))
)
const allVisibleChecked = computed(() =>
  paginatedImages.value.length > 0 &&
  paginatedImages.value.every(image => checkedImageIds.value.includes(image.Id))
)
const partiallyChecked = computed(() =>
  !allVisibleChecked.value &&
  paginatedImages.value.some(image => checkedImageIds.value.includes(image.Id))
)

function toggleImageCheck(id) {
  const next = new Set(checkedImageIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  checkedImageIds.value = [...next]
}

// 表头全选：勾选/清空当前页条目，不影响其它页已勾选的镜像。
function toggleImageCheckAll() {
  const visible = paginatedImages.value.map(image => image.Id)
  const visibleSet = new Set(visible)
  const allChecked = visible.length > 0 && visible.every(id => checkedImageIds.value.includes(id))
  checkedImageIds.value = allChecked
    ? checkedImageIds.value.filter(id => !visibleSet.has(id))
    : [...new Set([...checkedImageIds.value, ...visible])]
}

// 刷新后剔除已不存在的勾选，避免对已删除镜像执行批量操作。
function pruneCheckedImages() {
  const existing = new Set(imagesList.value.map(image => image.Id))
  const next = checkedImageIds.value.filter(id => existing.has(id))
  if (next.length !== checkedImageIds.value.length) checkedImageIds.value = next
}

// 批量删除：确认后串行逐项按镜像 ID 调用单镜像删除接口（不带 force，多标签镜像会被后端拒绝），
// 逐条隔离失败并汇总；正在被容器使用的镜像由后端报错计入失败，前端不预检。
async function batchRemoveImages() {
  const targets = checkedImages.value
  if (batchWorking.value || targets.length === 0) return
  const environmentId = currentEnvironmentId()
  batchWorking.value = true
  try {
    const confirmed = await uiStore.confirm({
      type: 'danger',
      title: '批量删除镜像',
      message: `确定删除勾选的 ${targets.length} 个镜像吗？将逐条删除，单个失败后继续删除其余镜像；多标签镜像需先删除多余标签，否则会计入失败；正在被容器使用的镜像会删除失败并计入失败数。`,
      confirmText: '删除'
    })
    if (!confirmed) return
    let succeeded = 0
    const failures = []
    for (const image of targets) {
      try {
        await images.remove(image.Id, '', { environmentId })
        succeeded += 1
      } catch (error) {
        failures.push({ name: getImageName(image), reason: error.message || '' })
      }
    }
    checkedImageIds.value = []
    await fetchImages({ force: true })
    notifyBatchResult(uiStore, { label: '批量删除', succeeded, failures })
  } finally {
    batchWorking.value = false
  }
}

// 更新状态映射
const updateStatusMap = ref({})

// 对话框状态
const showPullDialog = ref(false)
const showImportDialog = ref(false)

// 监听拉取弹窗打开，检查并进行中的拉取
watch(showPullDialog, async (newVal) => {
  if (newVal) {
    // 弹窗打开时，检查是否有进行中的拉取
    const savedState = loadPullState()
    if (savedState && !savedState.completed && !savedState.error) {
      // 有进行中的拉取，恢复 UI 状态
      pullProgress.value = {
        show: true,
        status: savedState.status || '拉取中...',
        progress: savedState.progress || 0,
        details: savedState.details || []
      }
      pullForm.value = {
        name: savedState.name || '',
        registry: savedState.registry || ''
      }
      isUpdateMode.value = savedState.isUpdate || false

      // 检查 SSE 是否还在连接，不在则重连。新任务模型按 taskId 恢复，不会重复拉取。
      if (!pullEventSource || pullEventSource.readyState === EventSource.CLOSED) {
        if (savedState.taskId) {
          connectPullTaskEvents(savedState.taskId)
        }
      }
    }
  }
})

function handlePullPayload(data, eventTime = '') {
  if (data.type === 'done') {
    completePullTask('success')
    return
  }

  if (data.error) {
    failPullTask(data.errorDetail?.message || data.error || '未知错误')
    return
  }

  if (data.status) {
    pullProgress.value.status = data.status
  }

  if (data.progressDetail && data.progressDetail.current && data.progressDetail.total) {
    const current = Number(data.progressDetail.current || 0)
    const total = Number(data.progressDetail.total || 0)
    if (total > 0) {
      pullProgress.value.progress = Math.max(0, Math.min(100, Math.round((current / total) * 100)))
    }
  }

  if (data.id) {
    pullProgress.value.details = upsertPullProgressDetail(
      pullProgress.value.details,
      data,
      eventTime
    )
  }

  savePullState({
    taskId: currentPullTaskId.value,
    name: pullForm.value.name,
    registry: pullForm.value.registry,
    isUpdate: isUpdateMode.value,
    status: pullProgress.value.status,
    progress: pullProgress.value.progress,
    details: pullProgress.value.details
  })
}

function completePullTask(status = 'success') {
  pullProgress.value.status = status === 'success' ? '拉取完成' : '拉取结束'
  pullProgress.value.progress = 100

  savePullState({
    taskId: currentPullTaskId.value,
    name: pullForm.value.name,
    registry: pullForm.value.registry,
    isUpdate: isUpdateMode.value,
    status: pullProgress.value.status,
    progress: 100,
    details: pullProgress.value.details,
    completed: true
  })

  sendNotification('镜像拉取完成', `镜像 ${pullForm.value.name} 已成功拉取`)

  if (isUpdateMode.value && pullForm.value.name) {
    images.clearUpdate({ repoTag: pullForm.value.name })
      .finally(() => {
        fetchUpdateStatus()
      })
  }

  setTimeout(() => {
    showPullDialog.value = false
    pullProgress.value.show = false
    pullForm.value = { name: '', registry: '' }
    isUpdateMode.value = false
    currentPullTaskId.value = ''
    clearPullState()
    fetchImages({ force: true })
    fetchUpdateStatus()
  }, 500)

  if (pullEventSource) {
    pullEventSource.close()
    pullEventSource = null
  }
}

function failPullTask(errorMsg) {
  pullProgress.value.status = '拉取失败: ' + errorMsg

  savePullState({
    taskId: currentPullTaskId.value,
    name: pullForm.value.name,
    registry: pullForm.value.registry,
    isUpdate: isUpdateMode.value,
    status: '拉取失败',
    progress: 0,
    details: pullProgress.value.details,
    error: errorMsg
  })

  sendNotification('镜像拉取失败', `镜像 ${pullForm.value.name} 拉取失败: ${errorMsg}`)

  if (pullEventSource) {
    pullEventSource.close()
    pullEventSource = null
  }

  if (pullProgress.value.show) {
    uiStore.toastError('拉取失败: ' + errorMsg)
  }
}

function connectPullTaskEvents(taskId) {
  if (!taskId) return
  currentPullTaskId.value = String(taskId)
  const url = images.getPullTaskEventsUrl(taskId)

  if (pullEventSource) {
    pullEventSource.close()
  }

  pullEventSource = new EventSource(url)
  setupPullEventSourceHandlers()
}

// 单独提取 SSE 事件处理，便于复用
function setupPullEventSourceHandlers() {
  if (!pullEventSource) return

  pullEventSource.onmessage = (event) => {
    let data = null
    try {
      const raw = String(event?.data || '').trim()
      if (!raw) return
      data = JSON.parse(raw)
    } catch (e) {
      return
    }

    if (data.type === 'result') {
      const status = String(data.status || '').toLowerCase()
      if (status === 'success' || status === 'completed') {
        completePullTask(status)
      } else {
        failPullTask(data.error || '任务失败')
      }
      return
    }

    if (data.type === 'progress') {
      try {
        handlePullPayload(JSON.parse(data.message || '{}'), data.time)
      } catch {
        // ignore malformed progress line
      }
      return
    }

    if (data.type === 'error') {
      failPullTask(data.message || '未知错误')
      return
    }

    if (data.type === 'info' || data.type === 'success') {
      pullProgress.value.status = data.message || pullProgress.value.status
    }
  }

  pullEventSource.onerror = () => {
    if (pullProgress.value.show) {
      pullProgress.value.status = '连接中断，正在重连...'
    }
    savePullState({
      taskId: currentPullTaskId.value,
      name: pullForm.value.name,
      registry: pullForm.value.registry,
      isUpdate: isUpdateMode.value,
      status: '连接中断',
      progress: pullProgress.value.progress,
      details: pullProgress.value.details,
      error: 'SSE连接中断'
    })
  }
}
const showTagDialog = ref(false)
const showRemoveTagDialog = ref(false)
const showUpdateTagDialog = ref(false)
const settingsVisible = ref(false)
const showDetailDialog = ref(false)
const detailLoading = ref(false)
const imageDetail = ref(null)
const imageHistory = ref([])
const selectedImageId = ref('')

// 删除标签表单
const removeTagForm = ref({
  imageId: '',
  tags: [],
  allSelected: false
})
const removeTagsWorking = ref(false)

// 更新标签表单（多 tag 单选：选要更新的那个 repo:tag）
const updateTagForm = ref({
  imageId: '',
  tags: []   // [{ name, selected, hasUpdate }]
})

// 表单数据
const pullForm = ref({ name: '', registry: '' })
const tagForm = ref({ id: '', repo: '', tag: '', removeOld: false, oldRepoTag: '' })
const registries = ref([])
const isUpdateMode = ref(false)
const currentPullTaskId = ref('')

// 拉取进度
const pullProgress = ref({
  show: false,
  status: '',
  progress: 0,
  details: []
})

// SSE EventSource
let pullEventSource = null

// 导入文件
const importFile = ref(null)
const isDragover = ref(false)
const importing = ref(false)
const pulling = ref(false)

// 空状态配置
const emptyProps = {
  title: '暂无镜像',
  description: '点击"拉取镜像"按钮添加第一个镜像',
  icon: 'image'
}

const detailTags = computed(() => imageDetail.value?.image?.RepoTags?.filter(tag => tag && tag !== '<none>:<none>') || [])
const detailContainers = computed(() => imageDetail.value?.containers || [])
const detailDisplayName = computed(() => {
  const firstTag = detailTags.value[0]
  if (firstTag) return parseImageRef(firstTag).name
  return '<none>'
})
const firstDigest = computed(() => imageDetail.value?.image?.RepoDigests?.[0] || '-')
const selectedSummaryImage = computed(() => {
  if (selectedImageId.value) {
    const found = imagesWithUpdateStatus.value.find(image => image.Id === selectedImageId.value)
    if (found) return found
  }
  return paginatedImages.value[0] || null
})

// 计算属性
const inUseImageIds = computed(() => {
  const ids = new Set()
  containerList.value.forEach(container => {
    if (container.ImageID) {
      ids.add(container.ImageID)
    }
  })
  return ids
})

// 带更新状态的镜像列表
const imagesWithUpdateStatus = computed(() => {
  return imagesList.value.map(img => {
    // 后端 runImageUpdateCheck 是 per-tag 检测的（每个 repo:tag 独立比对 digest），
    // updateStatusMap 的 key 是完整 repo:tag。同一 image id 上不同 tag 的更新状态独立，
    // 因此 hasUpdate 应聚合判定：任一 tag 命中即标记，并暴露 updatingTags 给 UI/操作。
    const tags = (img.RepoTags || []).filter(t => t && t !== '<none>:<none>')
    const updatingTags = tags.filter(t => updateStatusMap.value[t] === true)
    return {
      ...img,
      hasUpdate: updatingTags.length > 0,
      updatingTags
    }
  })
})

const imageRail = computed(() => {
  const list = imagesWithUpdateStatus.value
  const used = list.filter(image => inUseImageIds.value.has(image.Id)).length
  const updates = list.filter(image => image.hasUpdate).length
  const totalSize = list.reduce((sum, image) => sum + Number(image.Size || 0), 0)
  return [
    {
      label: '镜像总数',
      value: list.length,
      note: `${filteredImages.value.length} 个匹配`,
      icon: 'image',
      tone: 'neutral'
    },
    {
      label: '使用中',
      value: used,
      note: '被容器引用',
      icon: 'container',
      tone: 'success'
    },
    {
      label: '可更新',
      value: updates,
      note: checkingUpdates.value ? '检测中' : '远端状态',
      icon: 'cloud-download',
      tone: updates > 0 ? 'warning' : 'muted'
    },
    {
      label: '总占用',
      value: formatBytes(totalSize),
      note: activeTab.value === 'build-cache' ? '含缓存视图' : '本地镜像',
      icon: 'hard-drive',
      tone: 'info'
    }
  ]
})

const filteredImages = computed(() => {
  let list = imagesWithUpdateStatus.value.filter(image => (
    matchesBooleanFilter(image.hasUpdate, imageFilters.value.update)
    && matchesBooleanFilter(inUseImageIds.value.has(image.Id), imageFilters.value.usage)
  ))
  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    list = list.filter(img => {
      const name = getImageName(img).toLowerCase()
      const tags = (img.RepoTags || []).join(' ').toLowerCase()
      const id = (img.Id || '').toLowerCase()
      return name.includes(query) || tags.includes(query) || id.includes(query)
    })
  }
  // 应用排序
  return sortData(list, (item, prop) => {
    switch (prop) {
      case 'name':
        return getImageName(item)
      case 'tags':
        return getImageTags(item)
      case 'status':
        return inUseImageIds.value.has(item.Id) ? 'used' : 'unused'
      case 'update':
        return item.hasUpdate ? 1 : 0
      case 'size':
        return item.Size || 0
      case 'created':
        return item.Created || 0
      default:
        return ''
    }
  })
})

watch(imageFilters, () => {
  currentPage.value = 1
}, { deep: true })

function openContainerRelation(item) {
  if (!item?.id && !item?.name) return
  router.push({
    name: 'Containers',
    query: { focus: item.id || '', focusName: item.name || '' }
  })
}

// 分页后的镜像列表
const paginatedImages = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  const end = start + pageSize.value
  return filteredImages.value.slice(start, end)
})

// 批量删除或筛选导致总页数收缩时，把当前页钳制到最后一个非空页。
watch(() => Math.max(1, Math.ceil(filteredImages.value.length / pageSize.value)), pages => {
  if (currentPage.value > pages) currentPage.value = pages
})

// 方法
function parseImageRef(ref) {
  const value = String(ref || '')
  const lastSlash = value.lastIndexOf('/')
  const lastColon = value.lastIndexOf(':')
  if (lastColon > lastSlash) {
    return {
      name: value.slice(0, lastColon) || '<none>',
      tag: value.slice(lastColon + 1) || 'latest'
    }
  }
  return {
    name: value || '<none>',
    tag: 'latest'
  }
}

function getImageName(image) {
  if (image.RepoTags && image.RepoTags.length > 0) {
    return parseImageRef(image.RepoTags[0]).name
  }
  return '<none>'
}

function getImageTags(image) {
  const tags = getImageTagList(image)
  return tags.length ? tags.join(', ') : '-'
}

function getImageTagList(image) {
  if (!Array.isArray(image?.RepoTags)) return []
  return [...new Set(image.RepoTags.map(tag => parseImageRef(tag).tag).filter(Boolean))]
}

function formatTime(timestamp) {
  if (!timestamp) return '-'
  const date = new Date(timestamp * 1000)
  const { date: d, time: t } = formatTimeTwoLines(date)
  return `${d} ${t}`
}

function formatTimeFromValue(value) {
  if (!value) return '-'
  const date = typeof value === 'number' ? new Date(value * 1000) : new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  const { date: d, time: t } = formatTimeTwoLines(date)
  return `${d} ${t}`
}

function shortImageId(id) {
  if (!id) return '-'
  return String(id).replace('sha256:', '').slice(0, 12)
}

async function fetchImages(options = {}) {
  try {
    await dockerResources.loadImages(options)
    pruneCheckedImages()
  } catch (error) {
    console.error('获取镜像列表失败:', error)
  }
}

async function refreshImages() {
  if (imagesBusy.value) return
  const requests = [fetchImages({ force: true })]
  if (!isRemoteEnvironment.value) requests.push(fetchUpdateStatus())
  await Promise.allSettled(requests)
}

// 获取镜像更新状态
async function fetchUpdateStatus() {
  try {
    const res = await images.getUpdateStatus()
    const data = res || {}
    const updates = Array.isArray(data.updates) ? data.updates : []
    const map = {}
    updates.forEach(item => {
      if (item && item.repoTag) {
        map[item.repoTag] = true
      }
    })
    updateStatusMap.value = map
  } catch (error) {
    console.error('获取更新状态失败:', error)
  }
}

async function fetchRegistries() {
  try {
    const data = await images.getProxy()
    registries.value = Object.values(data.registries || {})
  } catch (error) {
    console.error('获取镜像源失败:', error)
  }
}

function handleSearch() {
  currentPage.value = 1
}

async function checkUpdates() {
  checkingUpdates.value = true
  try {
    const res = await images.checkUpdates({ force: true })
    const data = res || {}
    const msg = `检测完成：远端错误 ${data.remoteErrors || 0}，跳过退避 ${data.skippedBackoff || 0}，跳过不可用 ${data.skippedUnavailable || 0}`
    uiStore.toastInfo(msg)
    await fetchUpdateStatus()
    await fetchImages({ force: true })
  } catch (error) {
    console.error('检测更新失败:', error)
    uiStore.toastError('检测更新失败: ' + (error.message || '未知错误'))
  } finally {
    checkingUpdates.value = false
  }
}

function toggleMoreMenu(event) {
  if (showMoreMenu.value) {
    closeMoreMenu()
    return
  }
  toolbarMoreMenuAnchor.value = event?.currentTarget || null
  showMoreMenu.value = Boolean(toolbarMoreMenuAnchor.value)
}

function closeMoreMenu() {
  showMoreMenu.value = false
  toolbarMoreMenuAnchor.value = null
}

function handleMoreMenuOpenChange(open) {
  if (!open) closeMoreMenu()
}

function openSettings() {
  closeMoreMenu()
  settingsVisible.value = true
}

// 开始拉取镜像（后台任务 + SSE）
async function handlePullImage() {
  if (!pullForm.value.name) {
    uiStore.toastWarning('请输入镜像名称')
    return
  }
  if (pulling.value) return

  // 重置进度
  pulling.value = true
  pullProgress.value = {
    show: true,
    status: '准备拉取...',
    progress: 0,
    details: []
  }

  // 保存初始状态到 localStorage
  savePullState({
    name: pullForm.value.name,
    registry: pullForm.value.registry,
    isUpdate: isUpdateMode.value,
    status: '准备拉取...',
    progress: 0,
    details: []
  })

  try {
    const res = await images.startPullTask({
      name: pullForm.value.name,
      registry: pullForm.value.registry,
      cleanupOldImage: isUpdateMode.value
    })
    const taskId = res?.taskId
    if (!taskId) {
      throw new Error('未获取到任务ID')
    }
    currentPullTaskId.value = String(taskId)
    savePullState({
      taskId: currentPullTaskId.value,
      name: pullForm.value.name,
      registry: pullForm.value.registry,
      isUpdate: isUpdateMode.value,
      status: '任务已提交',
      progress: 0,
      details: []
    })
    connectPullTaskEvents(currentPullTaskId.value)
  } catch (error) {
    failPullTask(error.message || '任务提交失败')
  } finally {
    pulling.value = false
  }
}

// 单镜像更新
// 运行镜像（跳转到创建容器）
function handleRun(image) {
  const name = getImageName(image)
  const tag = parseImageRef(image.RepoTags?.[0]).tag
  // 存储镜像信息并跳转
  localStorage.setItem('run_image', JSON.stringify({
    image: `${name}:${tag}`,
    from: 'images'
  }))
  // 跳转到容器页面或显示提示
  uiStore.toastInfo(`即将运行镜像: ${name}:${tag}，功能开发中...`)
}

async function handleUpdate(image) {
  const validTags = (image.RepoTags || []).filter(t => t && t !== '<none>:<none>')
  if (validTags.length === 0) {
    uiStore.toastWarning('该镜像没有有效标签，无法更新')
    return
  }

  // 检查是否在使用中
  if (inUseImageIds.value.has(image.Id)) {
    const confirmed = await uiStore.confirm({
      type: 'warning',
      title: '更新使用中的镜像',
      message: '该镜像正在被容器使用。更新镜像不会自动重启容器，您需要稍后手动重建容器以应用新镜像。是否继续？',
      confirmText: '继续'
    })
    if (!confirmed) {
      return
    }
  }

  // 单标签直接更新，无需选择
  if (validTags.length === 1) {
    startImageUpdate(validTags[0])
    return
  }

  // 多标签：弹框让用户选择要更新的那个 tag。
  // updatingTags 来自后端 per-tag 检测，默认选中真正有更新的 tag。
  const updatingSet = new Set(image.updatingTags || [])
  updateTagForm.value = {
    imageId: image.Id,
    tags: validTags.map(name => ({
      name,
      selected: updatingSet.size > 0 ? updatingSet.has(name) : false,
      hasUpdate: updatingSet.has(name)
    }))
  }
  // 有更新 tag 时默认选中第一个可更新的；都没有则不预选（由用户挑）
  if (updatingSet.size > 0) {
    const firstUpdating = updateTagForm.value.tags.find(t => t.hasUpdate)
    if (firstUpdating) firstUpdating.selected = true
  }
  showUpdateTagDialog.value = true
}

// 单选互斥：选中某个 tag 时清除其它
function selectUpdateTag(index) {
  const tags = updateTagForm.value.tags
  // 取反当前项的选中状态（允许点击已选中项取消，但至少要保持能 disabled 按钮）
  const willSelect = !tags[index].selected
  tags.forEach((t, i) => { t.selected = (i === index) ? willSelect : false })
}

// 确认更新：取选中的那个 tag 走拉取流程
function confirmUpdateTags() {
  const selected = updateTagForm.value.tags.find(t => t.selected)
  if (!selected) return
  showUpdateTagDialog.value = false
  updateTagForm.value = { imageId: '', tags: [] }
  startImageUpdate(selected.name)
}

// 实际执行拉取（复用 pull dialog 流程）
function startImageUpdate(repoTag) {
  isUpdateMode.value = true
  pullForm.value = {
    name: repoTag,
    registry: ''
  }
  showPullDialog.value = true

  // 自动开始拉取
  setTimeout(() => {
    handlePullImage()
  }, 100)
}

function handleDrop(e) {
  e.preventDefault()
  isDragover.value = false
  const files = e.dataTransfer.files
  if (files.length > 0) {
    importFile.value = files[0]
  }
}

function handleFileSelect(e) {
  const files = e.target.files
  if (files.length > 0) {
    importFile.value = files[0]
  }
}

async function importImage() {
  if (!importFile.value) return
  importing.value = true
  try {
    const formData = new FormData()
    formData.append('file', importFile.value)
    await images.import(formData)
    showImportDialog.value = false
    importFile.value = null
    await fetchImages({ force: true })
  } catch (error) {
    console.error('导入镜像失败:', error)
    uiStore.toastError('导入失败: ' + (error.message || '未知错误'))
  } finally {
    importing.value = false
  }
}

async function openImageDetail(image) {
  showDetailDialog.value = true
  detailLoading.value = true
  imageDetail.value = null
  imageHistory.value = []
  try {
    const [detailResult, historyResult] = await Promise.allSettled([
      images.detail(image.Id),
			isRemoteEnvironment.value ? Promise.resolve({ history: [] }) : images.history(image.Id)
    ])
    if (detailResult.status === 'rejected') throw detailResult.reason
    imageDetail.value = detailResult.value || null
    imageHistory.value = historyResult.status === 'fulfilled' && Array.isArray(historyResult.value?.history)
      ? historyResult.value.history
      : []
  } catch (error) {
    console.error('获取镜像详情失败:', error)
    uiStore.toastError('获取镜像详情失败: ' + (error.message || '未知错误'))
  } finally {
    detailLoading.value = false
  }
}

function selectImage(image) {
  selectedImageId.value = image?.Id || ''
}

function handleExport(image) {
  const url = images.getExportUrl(image.Id)
  window.open(url, '_blank')
}

function handleTag(image) {
  const name = getImageName(image)
  const tag = parseImageRef(image.RepoTags?.[0]).tag
  const oldRepoTag = image.RepoTags?.[0] || ''
  tagForm.value = { id: image.Id, repo: name, tag, removeOld: false, oldRepoTag }
  showTagDialog.value = true
}

// 全选/取消全选标签
function toggleSelectAllTags() {
  const allSelected = removeTagForm.value.allSelected
  removeTagForm.value.tags.forEach(tag => {
    tag.selected = allSelected
  })
}

// 确认删除选中的标签
async function confirmRemoveTags() {
  const selectedTags = removeTagForm.value.tags.filter(t => t.selected)
  const allTags = removeTagForm.value.tags
  if (removeTagsWorking.value || selectedTags.length === 0) return
  const { imageId, environmentId } = removeTagForm.value
  removeTagsWorking.value = true

  try {
    if (selectedTags.length === allTags.length) {
      // 全选 = 删除整个 image (rmi)。当 image 被多个 repo 引用时，
      // Docker 会以 "referenced in multiple repositories / must be forced" 拒绝，
      // 此时自动加 force 重试。
      try {
        await images.remove(imageId, '', { environmentId })
      } catch (err) {
        const msg = (err && (err.message || err.error)) || ''
        const needsForce = msg.includes('referenced in multiple repositories')
          || msg.includes('must be forced')
          || msg.includes('image is being used')
          || msg.includes('image is in use')
        if (needsForce) {
          await images.remove(imageId, '', { force: true, environmentId })
        } else {
          throw err
        }
      }
    } else {
      // 部分选中 = 逐个 untag（仅解除 repo:tag 引用，不删 image id）
      for (const tag of selectedTags) {
        await images.remove(imageId, tag.name, { environmentId })
      }
    }

    showRemoveTagDialog.value = false
    removeTagForm.value = { imageId: '', tags: [], allSelected: false }
    await fetchImages({ force: true })
  } catch (error) {
    console.error('删除标签失败:', error)
    uiStore.toastError('删除失败: ' + (error.message || '未知错误'))
    // 即使中途失败也刷新，让 UI 反映已成功删除的 tag
    await fetchImages({ force: true })
  } finally {
    removeTagsWorking.value = false
  }
}

async function saveTag() {
  try {
    // 打新标签
    await images.tag({
      id: tagForm.value.id,
      repo: tagForm.value.repo,
      tag: tagForm.value.tag
    })
    
    // 如果需要删除原标签
    if (tagForm.value.removeOld && tagForm.value.oldRepoTag) {
      await images.remove(tagForm.value.id, tagForm.value.oldRepoTag)
    }
    
    showTagDialog.value = false
    await fetchImages({ force: true })
  } catch (error) {
    console.error('修改标签失败:', error)
    uiStore.toastError('修改标签失败: ' + (error.message || '未知错误'))
  }
}

// 删除镜像（多标签时选择删除）
async function handleRemove(image) {
  const environmentId = currentEnvironmentId()
  if (inUseImageIds.value.has(image.Id)) {
    uiStore.toastWarning('该镜像正在使用中，无法删除')
    return
  }
  
  // 获取镜像的所有标签
  const repoTags = image.RepoTags || []
  const validTags = repoTags.filter(tag => tag && tag !== '<none>:<none>')
  
  // 如果有多个标签，让用户选择删除哪些
  if (validTags.length > 1) {
    removeTagForm.value = {
      imageId: image.Id,
      environmentId,
      tags: validTags.map(tag => ({ name: tag, selected: false })),
      allSelected: false
    }
    showRemoveTagDialog.value = true
    return
  }
  
  // 只有一个标签或无标签，直接删除
  const tagToRemove = validTags.length === 1 ? validTags[0] : ''
  const confirmed = await uiStore.confirm({
    type: 'danger',
    title: '删除镜像',
    message: `确定删除镜像 "${tagToRemove || image.Id.slice(0, 12)}" 吗？`,
    confirmText: '删除'
  })
  if (!confirmed) {
    return
  }
  
  try {
    await images.remove(image.Id, tagToRemove, { environmentId })
    await fetchImages({ force: true })
  } catch (error) {
    console.error('删除镜像失败:', error)
    uiStore.toastError('删除失败: ' + (error.message || '未知错误'))
  }
}

async function applyUpdates() {
  closeMoreMenu()
  if (updating.value) return
  
  updating.value = true
  try {
    const res = await images.applyUpdates()
    const data = res || {}
    const total = data.total || 0
    const attempted = data.attempted || 0
    const success = data.success || 0
    const failed = data.failed || 0
    const skippedUsed = data.skippedUsed || 0
    
    let msg = `更新完成：成功 ${success}/${attempted}/${total}`
    if (skippedUsed > 0) {
      msg += `，跳过使用中 ${skippedUsed} 个`
    }
    if (failed > 0) {
      msg += `，失败 ${failed} 个`
    }
    uiStore.toastInfo(msg)
    
    if (failed > 0 && Array.isArray(data.failedTags) && data.failedTags.length) {
      const list = data.failedTags.slice(0, 5).join('、')
      const more = data.failedTags.length > 5 ? ` 等 ${data.failedTags.length} 个` : ''
      uiStore.toastWarning(`以下镜像更新失败：${list}${more}`)
    }
    
    await fetchUpdateStatus()
    await fetchImages({ force: true })
  } catch (error) {
    console.error('更新镜像失败:', error)
    uiStore.toastError('更新失败: ' + (error.message || '未知错误'))
  } finally {
    updating.value = false
  }
}

async function pruneImages() {
  closeMoreMenu()
  const confirmed = await uiStore.confirm({
    type: 'danger',
    title: '清理无用镜像',
    message: '确定清理所有无用的镜像吗？此操作不可恢复。',
    confirmText: '清理'
  })
  if (!confirmed) {
    return
  }
  try {
    await images.prune()
    await fetchImages({ force: true })
  } catch (error) {
    console.error('清理镜像失败:', error)
    uiStore.toastError('清理失败: ' + (error.message || '未知错误'))
  }
}

usePageAction({
  pull: () => {
    showPullDialog.value = true
  },
  import: () => {
    showImportDialog.value = true
  },
  'check-updates': checkUpdates
})

onMounted(() => {
  fetchImages()
  if (!isRemoteEnvironment.value) {
    fetchRegistries()
    fetchUpdateStatus()
  }
  const savedState = loadPullState()
  if (savedState?.taskId && !savedState.completed && !savedState.error) {
    pullProgress.value = {
      show: true,
      status: savedState.status || '恢复镜像拉取任务...',
      progress: savedState.progress || 0,
      details: savedState.details || []
    }
    pullForm.value = {
      name: savedState.name || '',
      registry: savedState.registry || ''
    }
    isUpdateMode.value = savedState.isUpdate || false
    connectPullTaskEvents(savedState.taskId)
  }
})

watch(isRemoteEnvironment, (remote) => {
  if (remote) {
    activeTab.value = 'images'
    updateStatusMap.value = {}
    return
  }
  fetchRegistries()
  fetchUpdateStatus()
})

onUnmounted(() => {
  if (pullEventSource) {
    pullEventSource.close()
  }
})
</script>

<style scoped>
.image-batch-check {
  width: 15px;
  height: 15px;
  cursor: pointer;
  accent-color: var(--color-primary-600, #2563eb);
}

.batch-danger {
  color: var(--color-danger-600);
}

.content-scroll {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
  padding-bottom: 24px;
  max-width: 1680px;
  width: 100%;
  margin: 0 auto;
}

.image-resource-board {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(280px, 300px);
  gap: 12px;
  align-items: start;
}

.image-list-card,
.resource-context-panel {
  min-width: 0;
  border: 1px solid var(--ops-line);
  border-radius: 14px;
  background: color-mix(in srgb, var(--ops-surface) 90%, transparent);
  box-shadow: 0 18px 48px color-mix(in srgb, var(--ops-ink) 7%, transparent);
  backdrop-filter: blur(14px);
}

.image-list-card {
  overflow: hidden;
  min-height: 560px;
}

.resource-context-panel {
  position: sticky;
  top: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-height: 560px;
  padding: 16px;
}

.context-header {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.context-header > div {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.context-header strong,
.context-header span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.context-header strong {
  color: var(--ops-ink);
  font-size: 0.98rem;
  font-weight: 800;
}

.context-header span {
  color: var(--ops-muted);
  font-size: 0.75rem;
}

.resource-kind.image {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 34px;
  height: 34px;
  border-radius: 10px;
  color: var(--ops-green-strong);
  background: var(--ops-green-soft);
}

.context-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.context-grid > div,
.context-section {
  min-width: 0;
  padding: 12px;
  border: 1px solid var(--ops-line);
  border-radius: 10px;
  background: color-mix(in srgb, var(--ops-panel) 70%, transparent);
}

.context-grid span,
.section-label {
  display: block;
  margin: 0 0 6px;
  color: var(--ops-muted);
  font-size: 0.7rem;
  font-weight: 800;
}

.context-grid strong {
  display: block;
  overflow: hidden;
  color: var(--ops-ink);
  font-size: 0.95rem;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-lines {
  display: grid;
  gap: 8px;
}

.detail-lines span {
  display: grid;
  grid-template-columns: 44px minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.detail-lines b {
  color: var(--ops-muted);
  font-size: 0.68rem;
}

.detail-lines em {
  overflow: hidden;
  color: var(--ops-ink);
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.72rem;
  font-style: normal;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.images-hero,
.resource-rail,
.toolbar,
.images-tabs {
  max-width: 1680px;
  width: 100%;
  margin-left: auto;
  margin-right: auto;
}

.images-hero {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.hero-copy {
  min-width: 0;
}

.eyebrow {
  margin: 0 0 6px;
  color: var(--ops-muted);
  font-size: 0.75rem;
  font-weight: 800;
  letter-spacing: 0.08em;
}

.hero-copy h1 {
  margin: 0;
  color: var(--ops-ink);
  font-size: 1.55rem;
  line-height: 1.15;
  font-weight: 900;
  letter-spacing: 0;
}

.hero-copy p:last-child {
  margin: 7px 0 0;
  color: var(--ops-muted);
  font-size: 0.875rem;
}

.resource-rail {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 14px;
}

.rail-card {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 10px;
  min-height: 70px;
  padding: 12px;
  border: 1px solid var(--ops-line);
  border-radius: 10px;
  background: color-mix(in srgb, var(--ops-panel) 78%, transparent);
  box-shadow: 0 18px 48px color-mix(in srgb, var(--ops-ink) 7%, transparent);
}

.rail-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border-radius: 9px;
}

.rail-card.neutral .rail-icon { color: var(--ops-blue); background: var(--ops-blue-soft); }
.rail-card.success .rail-icon { color: var(--ops-green-strong); background: var(--ops-green-soft); }
.rail-card.warning .rail-icon { color: var(--ops-amber); background: var(--ops-amber-soft); }
.rail-card.info .rail-icon { color: var(--ops-cyan); background: var(--ops-cyan-soft); }
.rail-card.muted .rail-icon { color: var(--ops-muted); background: var(--ops-muted-soft); }

.rail-copy {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.rail-copy strong {
  color: var(--ops-ink);
  font-size: 1.05rem;
  font-weight: 900;
  line-height: 1.1;
}

.rail-copy span,
.rail-card small {
  color: var(--ops-muted);
  font-size: 0.72rem;
  font-weight: 700;
}

/* 工具栏 */
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
  flex-wrap: wrap;
  padding: 12px;
  border: 1px solid var(--ops-line);
  border-radius: 12px;
  background: color-mix(in srgb, var(--ops-surface) 90%, transparent);
  box-shadow: 0 18px 48px color-mix(in srgb, var(--ops-ink) 7%, transparent);
}

.toolbar-left {
  display: grid;
  grid-template-columns: minmax(220px, 1fr) 190px auto;
  align-items: center;
  gap: 12px;
  flex: 1 1 560px;
  min-width: 0;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 0 1 auto;
  min-width: 0;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.sort-control {
  width: 190px;
  box-sizing: border-box;
  min-height: 38px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 8px 0 10px;
  border: 1px solid var(--ops-line);
  border-radius: 10px;
  background: color-mix(in srgb, var(--ops-panel) 82%, transparent);
  color: var(--ops-muted);
}

.sort-control select {
  flex: 1;
  width: 0;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--ops-ink);
  font: inherit;
  font-size: 0.8125rem;
  font-weight: 700;
  cursor: pointer;
}

.sort-control select option {
  background: var(--ops-panel);
  color: var(--ops-ink);
}

.sort-direction-btn {
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 7px;
  background: var(--ops-muted-soft);
  color: var(--ops-muted);
  cursor: pointer;
}

.sort-direction-btn:hover {
  color: var(--ops-green-strong);
  background: var(--ops-green-soft);
}

.sort-direction-btn.is-placeholder {
  visibility: hidden;
  pointer-events: none;
}

.images-tabs {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex: 0 0 auto;
  margin-bottom: 0;
  padding: 4px;
  border: 1px solid var(--ops-line);
  border-radius: 10px;
  background: color-mix(in srgb, var(--ops-panel) 82%, transparent);
}

.toolbar-tabs {
  height: 38px;
  width: auto;
  max-width: none;
  margin: 0;
  white-space: nowrap;
}

.images-toolbar .toolbar-right {
  flex-wrap: nowrap;
}

@media (max-width: 1500px) {
  .images-toolbar {
    align-items: stretch;
    flex-direction: column;
  }

  .images-toolbar .toolbar-left {
    flex-wrap: wrap;
    width: 100%;
  }

  .images-toolbar .toolbar-right {
    justify-content: flex-end;
    width: 100%;
  }
}

.tab-btn {
  height: 28px;
  padding: 0 10px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--ops-muted);
  font-size: 0.78rem;
  font-weight: 800;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.tab-btn:hover {
  color: var(--ops-ink);
  background: color-mix(in srgb, var(--ops-green) 10%, var(--ops-panel));
}

.tab-btn.active {
  color: var(--ops-green-strong);
  background: var(--ops-green-soft);
  font-weight: 800;
}

.icon-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  border-radius: 10px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.icon-btn:hover,
.icon-btn.is-active {
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

/* 下拉菜单 */
.dropdown-wrapper {
  position: relative;
}

.dropdown-menu {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: 10px;
  box-shadow: var(--shadow-lg);
  padding: 6px;
  min-width: 170px;
  z-index: 100;
}

.dropdown-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 12px;
  border-radius: 8px;
  font-size: 0.8125rem;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  white-space: nowrap;
}

.dropdown-item:hover {
  background: var(--bg-tertiary);
  color: var(--text-primary);
}

.dropdown-item.danger {
  color: var(--color-danger-600);
}

.dropdown-item.danger:hover {
  background: var(--color-danger-100);
}

.dropdown-item svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
  flex-shrink: 0;
}

.dropdown-divider {
  height: 1px;
  background: var(--border-subtle);
  margin: 6px 0;
}

/* 列表视图 */
.list-view {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  overflow-x: auto;
  overflow-y: hidden;
}

.data-table thead {
  position: sticky;
  top: 0;
  z-index: 1;
}

.list-loading {
  padding: 16px;
}

.list-skeleton {
  display: flex;
  gap: 16px;
  padding: 12px 0;
  border-bottom: 1px solid var(--border-subtle);
}

.skeleton-cell {
  height: 16px;
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
  border-radius: 4px;
}

.list-empty {
  padding: 48px 0;
}

.data-table {
  width: 100%;
  min-width: 1120px;
  border-collapse: collapse;
  font-size: 0.875rem;
  table-layout: fixed;
}

.col-name { width: 23%; }
.col-tags { width: 15%; }
.col-status { width: 10%; }
.col-update { width: 10%; }
.col-size { width: 10%; }
.col-created { width: 14%; }
.col-actions { width: 18%; }

.data-table th {
  text-align: left;
  padding: 12px 16px;
  font-weight: 600;
  color: var(--text-secondary);
  background: var(--bg-elevated);
  border-bottom: 1px solid var(--border-subtle);
  white-space: nowrap;
}

html.dark .data-table th {
  background: var(--bg-secondary);
}

.data-table th.sortable {
  cursor: pointer;
  user-select: none;
}

.data-table th.sortable:hover {
  color: var(--text-primary);
  background: var(--bg-secondary);
}

.data-table tr:hover td {
  background: var(--ops-row-hover);
}

.data-table tbody tr {
  cursor: pointer;
}

.data-table tbody tr.selected td {
  background: var(--ops-row-hover);
}

.data-table tbody tr.selected {
  box-shadow: inset 4px 0 0 var(--ops-green);
}

.image-list-card :deep(.image-card.is-selected) {
  border-color: color-mix(in srgb, var(--ops-green) 62%, var(--ops-line));
  box-shadow:
    inset 4px 0 0 var(--ops-green),
    0 18px 42px color-mix(in srgb, var(--ops-green) 14%, transparent);
}

.sort-icon {
  display: inline-flex;
  align-items: center;
  margin-left: 4px;
  color: var(--text-tertiary);
  vertical-align: middle;
}

.sort-icon.asc,
.sort-icon.desc {
  color: var(--color-primary-500);
}

.data-table td {
  padding: 12px 16px;
  border-bottom: 1px solid var(--border-subtle);
  color: var(--text-primary);
  vertical-align: middle;
}

.data-table tr:last-child td {
  border-bottom: none;
}

.cell-name {
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.link-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--text-primary);
  font: inherit;
  cursor: pointer;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.link-btn span {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.link-btn:hover span {
  color: inherit;
}

.image-icon-small {
  color: var(--color-primary-500);
  flex-shrink: 0;
}

.image-name-line {
  display: flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
}

.image-name-line strong {
  flex: 0 1 auto;
  min-width: 0;
}

.image-inline-tags {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
}

.image-inline-tag {
  flex: 0 0 auto;
  max-width: 92px;
  padding: 2px 6px;
  overflow: hidden;
  border: 1px solid color-mix(in srgb, var(--color-primary-500) 30%, var(--border-subtle));
  border-radius: 6px;
  background: color-mix(in srgb, var(--color-primary-500) 10%, var(--bg-elevated));
  color: var(--color-primary-700);
  font-size: 0.6875rem;
  font-weight: 600;
  line-height: 1.15;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.image-inline-tag.more {
  max-width: none;
  color: var(--text-secondary);
  border-color: var(--border-subtle);
  background: var(--bg-tertiary);
}

.cell-size {
  font-family: 'JetBrains Mono', monospace;
  color: var(--text-secondary);
}

.cell-time {
  color: var(--text-tertiary);
  white-space: nowrap;
}

.empty-cell {
  color: var(--text-tertiary);
}

.cell-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 100%;
  min-width: 176px;
  white-space: nowrap;
}

.table-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: var(--bg-tertiary);
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.table-btn:hover {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

/* 运行 - 绿色 */
.table-btn.success {
  background: var(--color-success-100);
  color: var(--color-success-600);
}

.table-btn.success svg {
  stroke: var(--color-success-600);
}

.table-btn.success:hover:not(:disabled) {
  background: var(--color-success-200);
  color: var(--color-success-700);
}

.table-btn.success:hover:not(:disabled) svg {
  stroke: var(--color-success-700);
}

/* 更新 - 紫色 */
.table-btn.update {
  background: var(--color-accent-100);
  color: var(--color-accent-600);
}

.table-btn.update svg {
  stroke: var(--color-accent-600);
}

.table-btn.update:hover:not(:disabled) {
  background: var(--color-accent-200);
  color: var(--color-accent-700);
}

.table-btn.update:hover:not(:disabled) svg {
  stroke: var(--color-accent-700);
}

/* 删除 - 粉红色 */
.table-btn.danger {
  background: var(--color-danger-100);
  color: var(--color-danger-600);
}

.table-btn.danger svg {
  stroke: var(--color-danger-600);
}

.table-btn.danger:hover:not(:disabled) {
  background: var(--color-danger-200);
  color: var(--color-danger-700);
}

.table-btn.danger:hover:not(:disabled) svg {
  stroke: var(--color-danger-700);
}

.table-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.table-btn svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
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


/* 表单 */
.form-group {
  margin-bottom: 18px;
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

.form-hint {
  font-size: 0.75rem;
  color: var(--text-tertiary);
  margin: 6px 0 0;
}

/* 检测更新按钮 - 黄色底色红色文字 */
.check-update-btn {
  min-height: 34px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 0 10px;
  border: 1px solid color-mix(in srgb, var(--ops-amber) 28%, var(--ops-line));
  border-radius: 8px;
  background: color-mix(in srgb, var(--ops-amber) 10%, var(--ops-panel));
  color: var(--ops-amber);
  font-size: 0.76rem;
  font-weight: 800;
  white-space: nowrap;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.check-update-btn:hover {
  background: color-mix(in srgb, var(--ops-amber) 16%, var(--ops-panel));
}

.check-update-btn svg {
  stroke: currentColor;
}

.check-update-btn.is-spinning svg {
  animation: spin 1s linear infinite;
}

.detail-loading {
  padding: 32px;
  text-align: center;
  color: var(--text-secondary);
}

.image-detail {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.detail-section {
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  padding: 14px;
  background: var(--bg-secondary);
}

.detail-section h4 {
  margin: 0 0 12px;
  color: var(--text-primary);
  font-size: 0.9375rem;
}

.detail-grid {
  display: grid;
  grid-template-columns: 96px minmax(0, 1fr);
  gap: 10px 14px;
  font-size: 0.875rem;
}

.detail-grid span,
.muted {
  color: var(--text-tertiary);
}

.detail-grid strong,
.detail-grid code {
  min-width: 0;
  color: var(--text-primary);
  overflow-wrap: anywhere;
}

.detail-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.detail-tag {
  padding: 4px 8px;
  border-radius: 6px;
  background: var(--bg-tertiary);
  color: var(--text-primary);
  font-size: 0.8125rem;
}

.related-list,
.history-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.related-item,
.history-item {
  display: grid;
  grid-template-columns: 140px minmax(0, 1fr);
  gap: 10px;
  align-items: center;
  font-size: 0.8125rem;
}

.related-item span,
.history-command {
  min-width: 0;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.history-item {
  grid-template-columns: 110px 84px minmax(0, 1fr);
}

.history-item code {
  color: var(--text-primary);
}

/* 删除标签对话框 */
.remove-tag-dialog {
  padding: 8px 0;
}

.dialog-hint {
  font-size: 0.875rem;
  color: var(--text-secondary);
  margin-bottom: 16px;
  line-height: 1.5;
}

.tag-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 300px;
  overflow-y: auto;
  padding: 12px;
  background: var(--bg-secondary);
  border-radius: 8px;
  margin-bottom: 16px;
}

.tag-checkbox-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  background: var(--bg-elevated);
  border-radius: 6px;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.tag-checkbox-item:hover {
  background: var(--bg-tertiary);
}

.tag-checkbox-item input[type="checkbox"] {
  width: 16px;
  height: 16px;
  cursor: pointer;
  accent-color: var(--color-primary-500);
}

.tag-name {
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.875rem;
  color: var(--text-primary);
  flex: 1;
}

.tag-name.has-update {
  color: var(--color-primary-600, var(--color-primary));
  font-weight: 600;
}

.tag-update-badge {
  font-size: 0.75rem;
  padding: 2px 8px;
  border-radius: 10px;
  background: var(--color-primary-50, rgba(59, 130, 246, 0.1));
  color: var(--color-primary-600, var(--color-primary));
  white-space: nowrap;
}

.select-all-label {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px;
  background: var(--color-danger-50);
  border-radius: 8px;
  cursor: pointer;
  font-size: 0.875rem;
  color: var(--color-danger-700);
}

.select-all-label input[type="checkbox"] {
  width: 16px;
  height: 16px;
  cursor: pointer;
  accent-color: var(--color-danger-500);
}

.btn-danger {
  background: var(--color-danger-500);
  color: white;
}

.btn-danger:hover:not(:disabled) {
  background: var(--color-danger-600);
}

.btn-danger:disabled {
  background: var(--bg-tertiary);
  color: var(--text-tertiary);
  cursor: not-allowed;
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
  cursor: pointer;
}

/* 上传区域 */
.upload-area {
  border: 2px dashed var(--border-subtle);
  border-radius: 12px;
  padding: 40px;
  text-align: center;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  cursor: pointer;
}

.upload-area.is-dragover {
  border-color: var(--color-primary-500);
  background: var(--color-primary-500-5);
}

.file-input {
  display: none;
}

.upload-content {
  color: var(--text-secondary);
}

.upload-content svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 1.5;
  stroke-linecap: round;
  stroke-linejoin: round;
  margin-bottom: 12px;
}

.upload-text {
  font-size: 0.9375rem;
  margin: 0 0 6px;
}

.upload-link {
  color: var(--color-primary-500);
  cursor: pointer;
}

.upload-hint {
  font-size: 0.75rem;
  color: var(--text-tertiary);
  margin: 0;
}

.file-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 14px;
  background: var(--bg-tertiary);
  border-radius: 8px;
  margin-top: 14px;
}

.file-name {
  font-size: 0.8125rem;
  color: var(--text-primary);
}

.file-size {
  font-size: 0.75rem;
  color: var(--text-secondary);
}

/* 按钮 */
.btn {
  height: 40px;
  padding: 0 18px;
  border-radius: 8px;
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  border: none;
}

.btn-default {
  background: var(--bg-tertiary);
  color: var(--text-primary);
}

.btn-default:hover {
  background: var(--border-default);
}

.btn-primary {
  background: linear-gradient(135deg, var(--color-primary-500), var(--color-primary-600));
  color: var(--text-inverse);
}

.btn-primary:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px var(--color-primary-500-30);
}

.btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-spinner {
  display: inline-block;
  width: 14px;
  height: 14px;
  border: 2px solid var(--white-30);
  border-top-color: var(--text-inverse);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  margin-right: 6px;
  vertical-align: middle;
}

/* 拉取进度条 */
.pull-progress {
  margin-top: 16px;
  padding: 16px;
  background: var(--bg-tertiary);
  border-radius: 10px;
  border: 1px solid var(--border-subtle);
}

.progress-header {
  display: grid;
  grid-template-columns: minmax(0, 1fr) max-content;
  align-items: start;
  gap: 12px;
  margin-bottom: 8px;
}

.progress-status {
  min-width: 0;
  font-size: 0.8125rem;
  line-height: 1.45;
  color: var(--text-secondary);
  overflow-wrap: anywhere;
}

.progress-percent {
  font-size: 0.8125rem;
  font-weight: 600;
  color: var(--color-primary-600);
}

.progress-bar {
  height: 6px;
  background: var(--border-subtle);
  border-radius: 3px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, var(--color-primary-500), var(--color-primary-600));
  border-radius: 3px;
  transition: width var(--motion-duration-fast) var(--motion-ease-out);
}

.progress-details {
  margin-top: 12px;
  max-height: 164px;
  overflow-y: auto;
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.6875rem;
}

.progress-detail-head,
.progress-detail-item {
  display: grid;
  grid-template-columns: 64px minmax(72px, 92px) minmax(0, 1fr);
  align-items: start;
  gap: 10px;
}

.progress-detail-head {
  position: sticky;
  top: 0;
  z-index: 1;
  padding: 3px 6px 5px;
  color: var(--text-tertiary);
  background: var(--bg-tertiary);
  border-bottom: 1px solid var(--border-subtle);
  font-family: var(--font-sans, Inter, sans-serif);
  font-size: 0.625rem;
  font-weight: 600;
}

.progress-detail-item {
  min-height: 25px;
  padding: 5px 6px;
  color: var(--text-tertiary);
  border-bottom: 1px solid color-mix(in srgb, var(--border-subtle) 66%, transparent);
}

.detail-time,
.detail-id {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-time {
  color: var(--text-tertiary);
  font-variant-numeric: tabular-nums;
}

.detail-id {
  color: var(--color-primary-600);
}

.detail-status {
  min-width: 0;
  color: var(--text-secondary);
  line-height: 1.35;
  overflow-wrap: anywhere;
}

/* 构建缓存工具栏 */
.cache-summary {
  grid-column: span 3;
  min-height: 38px;
  display: inline-flex;
  align-items: center;
  font-size: 0.875rem;
  color: var(--text-secondary);
}

.tb-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 14px;
  height: 38px;
  border-radius: 10px;
  font-size: 0.8125rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  border: none;
}

.tb-btn-warning {
  background: transparent;
  border: 1px solid var(--color-warning-300);
  color: var(--color-warning-600);
}

.tb-btn-warning:hover {
  background: var(--color-warning-100);
}

.tb-btn-danger {
  background: var(--color-danger-500);
  color: #fff;
}

.tb-btn-danger:hover {
  background: var(--color-danger-600);
}

.detail-id {
  color: var(--color-primary-500);
  width: 60px;
  flex-shrink: 0;
}

.detail-status {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 768px) {
  .toolbar {
    flex-direction: column;
    align-items: stretch;
  }
  
  .toolbar-left,
  .toolbar-right {
    width: 100%;
  }
  
  .toolbar-right {
    justify-content: flex-end;
  }
}

/* Compose/容器工作台基准覆盖 */
.images-page {
  padding: 22px 24px 0;
  gap: 14px;
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

.content-scroll {
  padding-bottom: 92px;
}

.images-hero {
  align-items: flex-start;
  gap: 24px;
  margin-bottom: 0;
}

.eyebrow {
  margin: 0 0 4px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.hero-copy h1 {
  font-size: clamp(1.65rem, 1.75vw, 2.25rem);
  line-height: 1.12;
  font-weight: 800;
}

.hero-copy p:last-child {
  margin: 6px 0 0;
  font-size: 0.95rem;
}

.resource-rail {
  gap: 10px;
  margin-bottom: 0;
}

.rail-card {
  grid-template-columns: 38px 1fr;
  grid-template-areas:
    "icon copy"
    "icon note";
  gap: 2px 10px;
  min-height: 64px;
  border-radius: 12px;
  backdrop-filter: blur(12px);
}

.rail-icon {
  grid-area: icon;
  width: 38px;
  height: 38px;
  border-radius: 10px;
}

.rail-copy {
  grid-area: copy;
  flex-direction: row;
  align-items: baseline;
  gap: 8px;
}

.rail-copy strong {
  font-size: 1.35rem;
  line-height: 1;
  font-weight: 800;
}

.rail-copy span,
.rail-card small {
  overflow: hidden;
  font-size: 0.75rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rail-card small {
  grid-area: note;
}

.toolbar {
  margin-bottom: 0;
}

.list-view {
  border-color: var(--ops-line);
  border-radius: 14px;
  background: color-mix(in srgb, var(--ops-surface) 90%, transparent);
  box-shadow: 0 18px 48px color-mix(in srgb, var(--ops-ink) 7%, transparent);
}

.data-table {
  min-width: 0;
  font-size: 0.875rem;
}

.data-table th {
  position: relative;
  padding: 12px 14px;
  border-bottom-color: var(--ops-line);
  background: color-mix(in srgb, var(--ops-bg) 88%, transparent);
  color: var(--ops-muted);
  font-size: 0.72rem;
  font-weight: 800;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.data-table th:not(:last-child)::after {
  content: "";
  position: absolute;
  top: 9px;
  right: -5px;
  bottom: 9px;
  width: 10px;
  cursor: col-resize;
}

.data-table th:not(:last-child):hover::after {
  right: -1px;
  width: 1px;
  background: var(--ops-strong-line);
}

html.dark .data-table th {
  background: color-mix(in srgb, var(--ops-surface) 90%, transparent);
}

.data-table th.sortable:hover {
  color: var(--ops-ink);
  background: color-mix(in srgb, var(--ops-panel) 80%, transparent);
}

.data-table td {
  height: 68px;
  padding: 12px 14px;
  border-bottom-color: var(--ops-line);
  color: var(--ops-ink);
}

.data-table tr:hover td {
  background: var(--ops-row-hover);
}

.cell-size,
.cell-time,
.empty-cell {
  color: var(--ops-muted);
}

.link-btn {
  color: var(--ops-ink);
}

.image-icon-small {
  color: var(--ops-green-strong);
}

.table-btn {
  width: 32px;
  height: 32px;
  border-radius: 8px;
}

/* 紧凑资源页头：全局 Header 已经承担页面定位 */
.images-hero {
  display: none;
}

.resource-rail {
  gap: 8px;
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

/* 按可用内容宽度稳定分行，避免侧边栏展开时工具栏产生横向滚动。 */
.images-filter-control {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.images-toolbar.images-toolbar--list {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  grid-template-areas:
    "primary primary"
    "filters actions";
  align-items: center;
}

.images-toolbar.images-toolbar--list > .workbench-toolbar-left {
  grid-area: primary;
  display: grid;
  grid-template-columns: minmax(220px, 1fr) 190px auto auto;
  align-items: center;
  gap: 12px;
  width: 100%;
}

.images-toolbar.images-toolbar--list > .images-filter-control {
  grid-area: filters;
  justify-self: start;
  min-width: 0;
}

.images-toolbar.images-toolbar--list > .toolbar-right {
  grid-area: actions;
  justify-self: end;
  width: auto;
  flex-wrap: nowrap;
}

@media (max-width: 1200px) {
  .images-toolbar.images-toolbar--list > .workbench-toolbar-left {
    grid-template-columns: auto auto minmax(0, 1fr) 190px;
  }

  .images-toolbar.images-toolbar--list > .workbench-toolbar-left > .search-input-wrapper {
    grid-column: 1 / 4;
  }

  .images-toolbar.images-toolbar--list > .workbench-toolbar-left > .sort-control {
    grid-column: 4;
  }

  .images-toolbar.images-toolbar--list > .workbench-toolbar-left > .toolbar-tabs {
    grid-row: 2;
    grid-column: 1;
    justify-self: start;
  }

  .images-toolbar.images-toolbar--list > .workbench-toolbar-left > .check-update-btn {
    grid-row: 2;
    grid-column: 2;
    justify-self: start;
  }
}

@media (max-width: 900px) {
  .images-toolbar.images-toolbar--list {
    grid-template-columns: minmax(0, 1fr);
    grid-template-areas:
      "primary"
      "filters"
      "actions";
  }

  .images-toolbar.images-toolbar--list > .toolbar-right {
    justify-self: stretch;
    justify-content: flex-end;
    width: 100%;
  }
}

@media (max-width: 620px) {
  .images-toolbar.images-toolbar--list > .workbench-toolbar-left {
    grid-template-columns: minmax(0, 1fr);
  }

  .images-toolbar.images-toolbar--list > .workbench-toolbar-left > .search-input-wrapper,
  .images-toolbar.images-toolbar--list > .workbench-toolbar-left > .sort-control,
  .images-toolbar.images-toolbar--list > .workbench-toolbar-left > .toolbar-tabs,
  .images-toolbar.images-toolbar--list > .workbench-toolbar-left > .check-update-btn {
    grid-row: auto;
    grid-column: 1;
    justify-self: start;
  }

  .images-toolbar.images-toolbar--list > .toolbar-right {
    flex-wrap: wrap;
  }
}

@container (min-width: 1480px) {
  .images-toolbar.images-toolbar--list {
    display: flex;
    align-items: center;
    flex-flow: row nowrap;
    gap: 10px;
  }

  .images-toolbar.images-toolbar--list > .workbench-toolbar-left {
    display: flex;
    align-items: center;
    flex: 1 1 auto;
    gap: 10px;
    width: auto;
    min-width: 0;
  }

  .images-toolbar.images-toolbar--list > .images-filter-control,
  .images-toolbar.images-toolbar--list > .toolbar-right {
    flex: 0 0 auto;
  }

  .images-toolbar.images-toolbar--list > .toolbar-right {
    display: flex;
    align-items: center;
    flex-wrap: nowrap;
    width: auto;
  }
}

</style>
