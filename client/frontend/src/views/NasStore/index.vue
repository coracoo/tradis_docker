<template>
  <ResourceWorkbench>
    <div class="nas-store-page" role="main" aria-label="NAS 选购">
      <PromoBanner v-if="!loadError" :banners="banners" />

      <!-- 工具栏 -->
      <div class="nas-toolbar">
        <SearchInput
          v-model="searchQuery"
          placeholder="搜索品牌、型号或特性..."
          @search="onSearch"
        />
        <div class="sort-control">
          <DynamicIcon name="arrow-up-down" :size="16" />
          <select v-model="sortBy" class="sort-select">
            <option value="default">默认排序</option>
            <option value="price_asc">价格从低到高</option>
            <option value="price_desc">价格从高到低</option>
            <option value="bays_asc">盘位从少到多</option>
            <option value="bays_desc">盘位从多到少</option>
            <option value="name_asc">名称 A-Z</option>
          </select>
        </div>
        <NasFilterDropdown
          label="品牌"
          :count="selectedBrands.length"
          :open="activeFilterMenu === 'brand'"
          @update:open="setFilterMenu('brand', $event)"
        >
          <button
            v-for="brand in allBrands"
            :key="brand"
            role="menuitemcheckbox"
            :aria-checked="selectedBrands.includes(brand)"
            class="filter-chip"
            :class="{ active: selectedBrands.includes(brand) }"
            @click="toggleFilter(selectedBrands, brand)"
          >
            {{ brand }}
          </button>
        </NasFilterDropdown>
        <NasFilterDropdown
          label="盘位"
          :count="selectedBays.length"
          :open="activeFilterMenu === 'bay'"
          @update:open="setFilterMenu('bay', $event)"
        >
          <button
            v-for="bay in allDriveBays"
            :key="bay"
            role="menuitemcheckbox"
            :aria-checked="selectedBays.includes(bay)"
            class="filter-chip"
            :class="{ active: selectedBays.includes(bay) }"
            @click="toggleFilter(selectedBays, bay)"
          >
            {{ bay }} 盘位
          </button>
        </NasFilterDropdown>
        <NasFilterDropdown
          label="架构"
          :count="selectedArchitectures.length"
          :open="activeFilterMenu === 'architecture'"
          @update:open="setFilterMenu('architecture', $event)"
        >
          <button
            v-for="arch in allArchitectures"
            :key="arch"
            role="menuitemcheckbox"
            :aria-checked="selectedArchitectures.includes(arch)"
            class="filter-chip"
            :class="{ active: selectedArchitectures.includes(arch) }"
            @click="toggleFilter(selectedArchitectures, arch)"
          >
            {{ arch }}
          </button>
        </NasFilterDropdown>
        <NasFilterDropdown
          label="网络"
          :count="selectedNetworks.length"
          :open="activeFilterMenu === 'network'"
          @update:open="setFilterMenu('network', $event)"
        >
          <button
            v-for="net in allNetworks"
            :key="net"
            role="menuitemcheckbox"
            :aria-checked="selectedNetworks.includes(net)"
            class="filter-chip"
            :class="{ active: selectedNetworks.includes(net) }"
            @click="toggleFilter(selectedNetworks, net)"
          >
            {{ net }}
          </button>
        </NasFilterDropdown>
        <NasFilterDropdown
          label="价格"
          variant="price"
          :panel-width="300"
          :count="priceFilterCount"
          :open="activeFilterMenu === 'price'"
          @update:open="setFilterMenu('price', $event)"
        >
          <button
            v-for="preset in pricePresets"
            :key="preset.label"
            role="menuitemradio"
            :aria-checked="isPricePresetActive(preset)"
            class="filter-chip"
            :class="{ active: isPricePresetActive(preset) }"
            @click="applyPricePreset(preset)"
          >
            {{ preset.label }}
          </button>
          <div class="price-range">
            <input v-model.number="minPrice" type="number" min="0" placeholder="最低价" class="price-input" />
            <span class="price-separator">-</span>
            <input v-model.number="maxPrice" type="number" min="0" placeholder="最高价" class="price-input" />
          </div>
        </NasFilterDropdown>
        <NasFilterDropdown
          label="分类"
          :count="selectedCategory === 'all' ? 0 : 1"
          :open="activeFilterMenu === 'category'"
          @update:open="setFilterMenu('category', $event)"
        >
          <button
            v-for="cat in categories"
            :key="cat.id"
            role="menuitemradio"
            :aria-checked="selectedCategory === cat.id"
            class="filter-chip"
            :class="{ active: selectedCategory === cat.id }"
            @click="selectedCategory = cat.id"
          >
            {{ cat.name }}
          </button>
        </NasFilterDropdown>
        <button
          v-if="compareList.length > 0"
          class="compare-btn"
          @click="showCompare = true"
        >
          <DynamicIcon name="scale" :size="16" />
          对比 ({{ compareList.length }})
        </button>
        <button v-if="hasActiveFilters" class="filter-reset" @click="resetFilters">重置</button>
      </div>

    <!-- 专题与评测横条 -->
    <div v-if="topics.length > 0 || reviews.length > 0" class="nas-content-strip">
      <span class="content-strip-title">专题与评测</span>
      <div class="content-strip-scroll">
        <router-link
          v-for="topic in topics"
          :key="`topic-${stripTopicSlug(topic)}`"
          class="strip-card"
          :to="`/nas-store/topic/${stripTopicSlug(topic)}`"
        >
          <img
            v-if="stripTopicCover(topic)"
            class="strip-card-cover"
            :src="stripTopicCover(topic)"
            :alt="stripTopicTitle(topic)"
          />
          <span class="strip-card-title">{{ stripTopicTitle(topic) }}</span>
        </router-link>
        <router-link
          v-for="review in reviews"
          :key="`review-${stripReviewID(review)}`"
          class="strip-card"
          :to="`/nas-store/review/${stripReviewID(review)}`"
        >
          <img
            v-if="stripReviewCover(review)"
            class="strip-card-cover"
            :src="stripReviewCover(review)"
            :alt="stripReviewTitle(review)"
          />
          <span class="strip-card-title">{{ stripReviewTitle(review) }}</span>
        </router-link>
      </div>
    </div>

    <!-- 加载中 -->
    <div v-if="loading" class="nas-loading">
      <div v-for="i in 6" :key="i" class="nas-card skeleton">
        <div class="skeleton-logo"></div>
        <div class="skeleton-content">
          <div class="skeleton-title"></div>
          <div class="skeleton-line"></div>
          <div class="skeleton-line short"></div>
        </div>
      </div>
    </div>

    <div v-else-if="loadError" class="nas-empty">
      <DynamicIcon name="cloud-off" :size="48" />
      <p>NAS 数据暂不可用</p>
      <button type="button" class="nas-retry" @click="loadData">
        <DynamicIcon name="refresh" :size="16" />
        重试
      </button>
    </div>

    <!-- 空状态 -->
    <div v-else-if="filteredDevices.length === 0" class="nas-empty">
      <DynamicIcon name="search-x" :size="48" />
      <p>未找到符合条件的 NAS</p>
      <span>尝试切换分类或更换关键词</span>
    </div>

    <!-- 卡片网格 -->
    <div v-else class="nas-grid motion-reveal">
      <div
        v-for="device in filteredDevices"
        :key="device.id"
        class="nas-card"
        :class="{ 'is-diy': device.is_diy }"
      >
        <div class="nas-image">
          <div class="image-placeholder" aria-hidden="true">
            <DynamicIcon name="server" :size="28" />
          </div>
          <img
            v-if="device.image_url"
            :src="device.image_url"
            :alt="device.name"
            @error="$event.target.style.display = 'none'"
          />
        </div>

        <div class="nas-card-header">
          <span class="nas-brand">{{ device.brand }}</span>
          <span class="nas-category">{{ categoryName(device.category) }}</span>
        </div>

        <h3 class="nas-name" :title="device.name">{{ device.name }}</h3>

        <div class="nas-price-row">
          <div class="nas-price" :class="{ 'is-empty': !device.price_label }">
            {{ device.price_label || '暂无报价' }}
          </div>
          <span v-if="device.promotion_tags?.length" class="promotion-tag">
            {{ promotionTagName(device.promotion_tags[0]) }}
          </span>
        </div>

        <div class="nas-spec-line" :title="specSegments(device).join(' · ')">
          <span v-for="segment in specSegments(device)" :key="segment">{{ segment }}</span>
        </div>

        <div v-if="variantOptions(device).length > 1" class="variant-chips" aria-label="可选配置">
          <button
            v-for="variant in variantOptions(device)"
            :key="variant.id"
            type="button"
            class="variant-chip"
            :class="{ active: variant.id === device.selected_variant_id }"
            @click="selectDeviceVariant(device, variant.id)"
          >
            {{ variant.label }}
          </button>
        </div>

        <div v-if="device.suitable_for?.length" class="nas-suitable">
          <span>适合</span>
          <strong>{{ device.suitable_for.join(' · ') }}</strong>
        </div>

        <div class="nas-card-actions">
          <a
            v-if="purchaseUrl(device)"
            :href="purchaseUrl(device)"
            target="_blank"
            rel="noopener"
            class="purchase-btn"
          >
            <DynamicIcon name="shopping-cart" :size="15" />
            购买
          </a>
          <span v-else class="purchase-btn is-disabled">
            <DynamicIcon name="shopping-cart" :size="15" />
            暂无购买链接
          </span>
          <button
            class="compare-toggle"
            :class="{ active: isCompared(device) }"
            :disabled="!isCompared(device) && compareList.length >= 3"
            :title="isCompared(device) ? '取消对比' : compareList.length >= 3 ? '最多对比 3 款' : '加入对比'"
            :aria-label="isCompared(device) ? '取消对比' : '加入对比'"
            :aria-pressed="isCompared(device)"
            @click="toggleCompare(device)"
          >
            <DynamicIcon :name="isCompared(device) ? 'check-square' : 'square'" :size="16" />
          </button>
        </div>
      </div>
    </div>
    <!-- 对比抽屉 -->
    <Teleport to="body">
      <Transition name="drawer">
        <div
          v-if="showCompare"
          class="compare-overlay"
          :style="{ zIndex: compareOverlayZIndex }"
          role="dialog"
          aria-modal="true"
          aria-label="NAS 对比"
          @click.self="closeCompare"
        >
          <div ref="comparePanelRef" class="compare-panel" tabindex="-1">
            <div class="compare-header">
              <h2>
                <DynamicIcon name="scale" :size="20" />
                NAS 参数对比
              </h2>
              <button class="close-btn" aria-label="关闭 NAS 对比" @click="closeCompare">
                <DynamicIcon name="x" :size="20" />
              </button>
            </div>
            <div class="compare-body">
              <div class="compare-table-wrapper">
                <table class="compare-table">
                  <thead>
                    <tr>
                      <th class="row-label">参数</th>
                      <th v-for="d in compareDevices" :key="d.id" class="device-col">
                        <div class="device-header">
                          <div class="device-name">{{ d.name }}</div>
                          <div class="device-brand">{{ d.brand }}</div>
                          <button class="remove-btn" @click="toggleCompare(d)">
                            <DynamicIcon name="x" :size="14" />
                          </button>
                        </div>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      <td class="row-label">分类</td>
                      <td v-for="d in compareDevices" :key="d.id">{{ categoryName(d.category) }}</td>
                    </tr>
                    <tr>
                      <td class="row-label">价格</td>
                      <td v-for="d in compareDevices" :key="d.id" class="highlight">{{ d.price_label }}</td>
                    </tr>
                    <tr>
                      <td class="row-label">盘位</td>
                      <td v-for="d in compareDevices" :key="d.id">{{ d.drive_bays }} 盘位 / {{ d.max_capacity }}</td>
                    </tr>
                    <tr>
                      <td class="row-label">CPU</td>
                      <td v-for="d in compareDevices" :key="d.id">{{ displayCPU(d.cpu) }}</td>
                    </tr>
                    <tr>
                      <td class="row-label">内存</td>
                      <td v-for="d in compareDevices" :key="d.id">{{ d.ram }}</td>
                    </tr>
                    <tr>
                      <td class="row-label">网络</td>
                      <td v-for="d in compareDevices" :key="d.id">{{ d.network }}</td>
                    </tr>
                    <tr>
                      <td class="row-label">扩展</td>
                      <td v-for="d in compareDevices" :key="d.id">{{ d.expansion }}</td>
                    </tr>
                    <tr>
                      <td class="row-label">特性</td>
                      <td v-for="d in compareDevices" :key="d.id">
                        <div class="compare-tags">
                          <span v-for="(f, idx) in d.features" :key="idx" class="tag">{{ f }}</span>
                        </div>
                      </td>
                    </tr>
                    <tr>
                      <td class="row-label">适合</td>
                      <td v-for="d in compareDevices" :key="d.id">
                        <div class="compare-tags">
                          <span v-for="(s, idx) in d.suitable_for" :key="idx" class="tag">{{ s }}</span>
                        </div>
                      </td>
                    </tr>
                    <tr>
                      <td class="row-label">导购</td>
                      <td v-for="d in compareDevices" :key="d.id">
                        <div class="compare-links">
                          <a v-if="d.official_url" :href="d.official_url" target="_blank" rel="noopener">官网</a>
                          <a v-if="d.jd_search_url" :href="d.jd_search_url" target="_blank" rel="noopener">京东</a>
                          <a v-if="d.tb_search_url" :href="d.tb_search_url" target="_blank" rel="noopener">淘宝</a>
                        </div>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
    </div>
  </ResourceWorkbench>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import ResourceWorkbench from '@/components/resource-workbench/ResourceWorkbench.vue'
import {
  getNASBanners,
  getNASCategories,
  getNASDevices,
  getNASNetworkTiers,
  getNASPromotionTags,
  getNASReviews,
  getNASTopics
} from '@/api/nas.js'
import PromoBanner from './components/PromoBanner.vue'
import NasFilterDropdown from './components/NasFilterDropdown.vue'
import { useOverlayController } from '@/composables/useOverlayController.js'
import {
  DEFAULT_NAS_FILTER_CATALOG,
  deriveNASFilterCatalog,
  matchingNetworkTiers
} from './filterCatalog.js'
import {
  deviceMatchesVariantFilters,
  deviceWithVariant,
  normalizedNASVariants,
  selectVariantForFilters
} from './variantCatalog.js'

const loading = ref(true)
const loadError = ref(false)
const banners = ref([])
const defaultBanners = [
  { id: 'default_spring', Title: '2026 春季 NAS 推荐', Subtitle: '家庭与小型工作室主流机型选购指南', ImageURL: '', LinkURL: '/nas-store/topic/spring-sale', BgColor: 'var(--color-gray-900)', SortOrder: 1, IsActive: true },
  { id: 'default_diy', Title: 'DIY NAS 入门指南', Subtitle: '从零搭建你的第一台高性价比 DIY NAS', ImageURL: '', LinkURL: '/nas-store/topic/diy-guide', BgColor: 'var(--color-primary-900)', SortOrder: 2, IsActive: true },
  { id: 'default_review', Title: '群晖 DS923+ 深度评测', Subtitle: '万兆扩展 + ECC 内存，家庭进阶首选', ImageURL: '', LinkURL: '/nas-store/review/ds923_review', BgColor: 'var(--color-secondary-900)', SortOrder: 3, IsActive: true },
  { id: 'default_store', Title: '更多 NAS 设备对比', Subtitle: '群晖、威联通、绿联、极空间全品牌参数对比', ImageURL: '', LinkURL: '/nas-store', BgColor: 'var(--color-gray-800)', SortOrder: 5, IsActive: true },
]
const categories = ref([{ id: 'all', name: '全部', order: 0 }])
const promotionTags = ref([])
const devices = ref([])
const topics = ref([])
const reviews = ref([])
const networkTiers = ref([...DEFAULT_NAS_FILTER_CATALOG.networks])
const searchQuery = ref('')
const selectedCategory = ref('all')
const sortBy = ref('default')
const selectedBrands = ref([])
const selectedBays = ref([])
const selectedArchitectures = ref([])
const selectedNetworks = ref([])
const minPrice = ref('')
const maxPrice = ref('')
const activeFilterMenu = ref('')
const pricePresets = [
  { label: '不限', min: '', max: '' },
  { label: '2000 以下', min: '', max: 2000 },
  { label: '2000 - 4000', min: 2000, max: 4000 },
  { label: '4000 - 8000', min: 4000, max: 8000 },
  { label: '8000 以上', min: 8000, max: '' }
]
const compareList = ref([])
const selectedVariantIDs = ref({})
const showCompare = ref(false)
const comparePanelRef = ref(null)

function closeCompare() {
  showCompare.value = false
}

const { overlayZIndex: compareOverlayZIndex } = useOverlayController({
  visible: showCompare,
  containerRef: comparePanelRef,
  closeOnEscape: true,
  onClose: closeCompare
})

const filterCatalog = computed(() => deriveNASFilterCatalog(devices.value, networkTiers.value))
const allBrands = computed(() => filterCatalog.value.brands)
const allDriveBays = computed(() => filterCatalog.value.driveBays)
const allArchitectures = computed(() => filterCatalog.value.architectures)
const allNetworks = computed(() => filterCatalog.value.networks)
const priceFilterCount = computed(() => minPrice.value !== '' || maxPrice.value !== '' ? 1 : 0)
const hasActiveFilters = computed(() =>
  selectedCategory.value !== 'all' ||
  selectedBrands.value.length > 0 ||
  selectedBays.value.length > 0 ||
  selectedArchitectures.value.length > 0 ||
  selectedNetworks.value.length > 0 ||
  priceFilterCount.value > 0
)

const filteredDevices = computed(() => {
  let result = devices.value
  const q = searchQuery.value.trim().toLowerCase()
  if (selectedCategory.value !== 'all') {
    result = result.filter(d => d.category === selectedCategory.value)
  }
  if (q) {
    result = result.filter(d =>
      d.name.toLowerCase().includes(q) ||
      d.brand.toLowerCase().includes(q) ||
      (d.description && d.description.toLowerCase().includes(q)) ||
      (d.features || []).some(f => f.toLowerCase().includes(q)) ||
      (d.suitable_for || []).some(s => s.toLowerCase().includes(q)) ||
      normalizedNASVariants(d).some(variant =>
        [variant.label, variant.cpu, variant.ram].some(value => String(value || '').toLowerCase().includes(q))
      )
    )
  }
  if (selectedBrands.value.length > 0) {
    result = result.filter(d => selectedBrands.value.includes(d.brand))
  }
  if (selectedBays.value.length > 0) {
    result = result.filter(d => selectedBays.value.includes(d.drive_bays))
  }
  if (selectedNetworks.value.length > 0) {
    result = result.filter(d => matchingNetworkTiers(d.network, networkTiers.value)
      .some(tier => selectedNetworks.value.includes(tier)))
  }
  const min = minPrice.value === '' ? 0 : Number(minPrice.value)
  const max = maxPrice.value === '' ? Infinity : Number(maxPrice.value)
  const variantFilters = {
    architectures: selectedArchitectures.value,
    minPrice: minPrice.value,
    maxPrice: maxPrice.value
  }
  const hasVariantFilters = selectedArchitectures.value.length > 0 || min > 0 || max < Infinity
  if (hasVariantFilters) result = result.filter(d => deviceMatchesVariantFilters(d, variantFilters))

  result = result.map((device) => {
    const automatic = hasVariantFilters ? selectVariantForFilters(device, variantFilters) : null
    const selectedID = automatic?.id || selectedVariantIDs.value[device.id] || ''
    return deviceWithVariant(device, selectedID)
  })

  // 排序
  const sorted = [...result]
  switch (sortBy.value) {
    case 'price_asc':
      sorted.sort((a, b) => (a.price_cny || 0) - (b.price_cny || 0))
      break
    case 'price_desc':
      sorted.sort((a, b) => (b.price_cny || 0) - (a.price_cny || 0))
      break
    case 'bays_asc':
      sorted.sort((a, b) => (a.drive_bays || 0) - (b.drive_bays || 0))
      break
    case 'bays_desc':
      sorted.sort((a, b) => (b.drive_bays || 0) - (a.drive_bays || 0))
      break
    case 'name_asc':
      sorted.sort((a, b) => a.name.localeCompare(b.name, 'zh-CN'))
      break
    default:
      // 保持原始顺序
      break
  }
  return sorted
})

const compareDevices = computed(() => {
  return devices.value
    .filter(d => compareList.value.includes(d.id))
    .map(d => deviceWithVariant(d, selectedVariantIDs.value[d.id] || ''))
})

function variantOptions(device) {
  return normalizedNASVariants(device)
}

function selectDeviceVariant(device, variantID) {
  selectedVariantIDs.value = {
    ...selectedVariantIDs.value,
    [device.id]: variantID
  }
}

function categoryName(id) {
  const cat = categories.value.find(c => c.id === id)
  return cat ? cat.name : id
}

function displayCPU(cpu) {
  return cpu && cpu.trim() ? cpu.trim() : '-'
}

function specSegments(device) {
  return [
    `${device.drive_bays || '-'} 盘位`,
    displayCPU(device.cpu),
    device.ram || '-',
    device.network || '-'
  ]
}

function stripTopicSlug(topic) {
  return topic?.Slug || topic?.slug || topic?.ID || topic?.id || ''
}

function stripTopicTitle(topic) {
  return topic?.Title || topic?.title || ''
}

function stripTopicCover(topic) {
  return topic?.BannerImage || topic?.banner_image || ''
}

function stripReviewID(review) {
  return review?.ID || review?.id || ''
}

function stripReviewTitle(review) {
  return review?.Title || review?.title || ''
}

function stripReviewCover(review) {
  return review?.CoverImage || review?.cover_image || ''
}

function toggleFilter(list, value) {
  const idx = list.indexOf(value)
  if (idx >= 0) {
    list.splice(idx, 1)
  } else {
    list.push(value)
  }
}

function setFilterMenu(name, open) {
  activeFilterMenu.value = open ? name : ''
}

function applyPricePreset(preset) {
  minPrice.value = preset.min
  maxPrice.value = preset.max
}

function isPricePresetActive(preset) {
  return String(minPrice.value) === String(preset.min) &&
    String(maxPrice.value) === String(preset.max)
}

function resetFilters() {
  selectedCategory.value = 'all'
  selectedBrands.value = []
  selectedBays.value = []
  selectedArchitectures.value = []
  selectedNetworks.value = []
  minPrice.value = ''
  maxPrice.value = ''
  activeFilterMenu.value = ''
}

function promotionTagName(id) {
  const tag = promotionTags.value.find(t => t.id === id)
  return tag ? tag.name : id
}

function purchaseUrl(device) {
  return device.purchase_url || device.jd_search_url || device.tb_search_url || device.official_url || ''
}

function isCompared(device) {
  return compareList.value.includes(device.id)
}

function toggleCompare(device) {
  const idx = compareList.value.indexOf(device.id)
  if (idx >= 0) {
    compareList.value.splice(idx, 1)
  } else if (compareList.value.length < 3) {
    if (device.selected_variant_id) {
      selectedVariantIDs.value = {
        ...selectedVariantIDs.value,
        [device.id]: device.selected_variant_id
      }
    }
    compareList.value.push(device.id)
  }
  if (compareList.value.length === 0) {
    showCompare.value = false
  }
}

function onSearch() {
  // 搜索由 computed 自动处理
}

async function loadData() {
  loading.value = true
  loadError.value = false
  try {
    const [cats, devs, tags, bnrs, networkConfig, topicList, reviewList] = await Promise.all([
      getNASCategories().catch(() => []),
      getNASDevices(),
      getNASPromotionTags().catch(() => []),
      getNASBanners().catch(() => []),
      getNASNetworkTiers().catch(() => null),
      getNASTopics().catch(() => []),
      getNASReviews().catch(() => [])
    ])
    if (!Array.isArray(devs)) throw new Error('NAS device catalog is invalid')
    categories.value = [{ id: 'all', name: '全部', order: 0 }, ...(Array.isArray(cats) ? cats : [])]
    promotionTags.value = Array.isArray(tags) ? tags : []
    devices.value = Array.isArray(devs) ? devs : []
    topics.value = Array.isArray(topicList) ? topicList : []
    reviews.value = Array.isArray(reviewList) ? reviewList : []
    banners.value = Array.isArray(bnrs) && bnrs.length > 0 ? bnrs : defaultBanners
    const remoteTiers = Array.isArray(networkConfig) ? networkConfig : networkConfig?.tiers
    networkTiers.value = Array.isArray(remoteTiers) && remoteTiers.length > 0
      ? remoteTiers
      : [...DEFAULT_NAS_FILTER_CATALOG.networks]
  } catch (e) {
    loadError.value = true
    console.error('加载 NAS 数据失败:', e)
  } finally {
    loading.value = false
  }
}

onMounted(loadData)
</script>

<style scoped>
.nas-store-page {
  box-sizing: border-box;
  width: 100%;
  height: 100%;
  min-height: 0;
  padding: 0 0 22px;
  overflow: auto;
  background: transparent;
  color: var(--resource-ink);
  scrollbar-gutter: stable;
}

.compare-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 38px;
  padding: 0 14px;
  border-radius: 8px;
  border: 1px solid var(--color-primary-500);
  background: var(--color-primary-500);
  color: var(--text-inverse, var(--bg-elevated));
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  flex: 0 0 auto;
}

.compare-btn:hover {
  background: var(--color-primary-600);
}

.nas-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 14px;
  padding: 12px;
  overflow-x: auto;
  background: var(--resource-surface);
  border: 1px solid var(--resource-line);
  border-radius: 12px;
  box-shadow: 0 12px 34px color-mix(in srgb, var(--text-primary) 5%, transparent);
  isolation: isolate;
  scrollbar-width: none;
}

.nas-toolbar::-webkit-scrollbar {
  display: none;
}

.nas-toolbar :deep(.search-input-wrapper) {
  flex: 1 1 220px;
  min-width: 180px;
  max-width: 300px;
}

.sort-control {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex: 0 0 160px;
  width: 160px;
  min-height: 38px;
  padding: 0 9px;
  border-radius: 9px;
  border: 1px solid var(--resource-line);
  background: var(--bg-elevated);
  color: var(--text-secondary);
}

.filter-chip {
  flex: 0 0 auto;
  padding: 5px 12px;
  border-radius: 999px;
  border: 1px solid var(--border-subtle);
  background: var(--bg-secondary);
  color: var(--text-secondary);
  font-size: 0.8125rem;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.filter-chip:hover {
  border-color: var(--color-primary-300);
  color: var(--text-primary);
}

.filter-chip.active {
  border-color: var(--color-primary-500);
  background: var(--color-primary-500-10);
  color: var(--color-primary-600);
}

.price-range {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 0 0 auto;
  width: 100%;
  margin-left: 0;
}

.price-input {
  width: 72px;
  padding: 5px 8px;
  border-radius: 6px;
  border: 1px solid var(--border-subtle);
  background: var(--bg-secondary);
  color: var(--text-primary);
  font-size: 0.8125rem;
  outline: none;
}

.price-input:focus {
  border-color: var(--color-primary-500);
}

.price-separator {
  color: var(--text-tertiary);
}

.filter-reset {
  flex: 0 0 auto;
  padding: 6px 14px;
  border-radius: 6px;
  border: 1px solid var(--border-subtle);
  background: transparent;
  color: var(--text-secondary);
  font-size: 0.8125rem;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.filter-reset:hover {
  border-color: var(--color-danger-300);
  color: var(--color-danger-600);
  background: var(--color-danger-50);
}

.sort-select {
  border: none;
  background: transparent;
  color: var(--text-primary);
  min-width: 0;
  flex: 1;
  font-family: var(--font-sans, Inter, ui-sans-serif, system-ui, sans-serif);
  font-size: 0.8125rem;
  font-weight: 400;
  cursor: pointer;
  outline: none;
}

/* 专题与评测横条 */
.nas-content-strip {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 0 0 14px;
  padding: 10px 12px;
  background: var(--resource-surface);
  border: 1px solid var(--resource-line);
  border-radius: 12px;
}

.content-strip-title {
  flex: 0 0 auto;
  color: var(--text-tertiary);
  font-size: 0.8125rem;
  font-weight: 600;
  white-space: nowrap;
}

.content-strip-scroll {
  display: flex;
  gap: 8px;
  min-width: 0;
  overflow-x: auto;
  scrollbar-width: none;
}

.content-strip-scroll::-webkit-scrollbar {
  display: none;
}

.strip-card {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex: 0 0 auto;
  max-width: 240px;
  padding: 4px 10px;
  border: 1px solid var(--resource-line);
  border-radius: 8px;
  background: var(--bg-elevated);
  color: var(--text-secondary);
  font-size: 0.8125rem;
  text-decoration: none;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.strip-card:hover {
  border-color: var(--color-primary-300);
  color: var(--text-primary);
}

.strip-card-cover {
  flex: 0 0 auto;
  width: 36px;
  height: 36px;
  border-radius: 6px;
  background: var(--bg-tertiary);
  object-fit: cover;
}

.strip-card-title {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.nas-loading,
.nas-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}

.nas-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 80px 20px;
  color: var(--text-secondary);
  gap: 8px;
}

.nas-empty p {
  margin: 0;
  font-size: 1rem;
  color: var(--text-primary);
}

.nas-empty span {
  font-size: 0.875rem;
}

.nas-retry {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-top: 8px;
  padding: 8px 12px;
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  background: var(--bg-primary);
  color: var(--text-primary);
  cursor: pointer;
}

.nas-retry:hover { border-color: var(--color-primary-500); }

.nas-card {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 10px;
  padding: 14px;
  overflow: hidden;
  border: 1px solid var(--resource-line);
  border-radius: 10px;
  background: var(--resource-surface);
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out), box-shadow var(--motion-duration-quick) var(--motion-ease-out);
}

.nas-card:hover {
  border-color: var(--resource-line-strong);
  box-shadow: 0 10px 26px color-mix(in srgb, var(--text-primary) 7%, transparent);
}

.nas-card.is-diy {
  border-left: 3px solid var(--color-secondary-500);
}

.nas-image {
  position: relative;
  display: flex;
  width: 100%;
  height: 128px;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border: 1px solid var(--resource-line);
  border-radius: 8px;
  background: var(--bg-tertiary);
}

.nas-image img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  padding: 10px;
  background: var(--bg-tertiary);
  object-fit: contain;
}

.image-placeholder {
  display: flex;
  width: 100%;
  height: 100%;
  align-items: center;
  justify-content: center;
  color: var(--text-tertiary);
}

.nas-card-header {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.nas-brand {
  min-width: 0;
  overflow: hidden;
  color: var(--text-tertiary);
  font-size: 0.75rem;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.nas-category {
  overflow: hidden;
  color: var(--text-tertiary);
  font-size: 0.75rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.nas-brand + .nas-category::before {
  content: '·';
  margin-right: 10px;
  color: var(--resource-line-strong);
}

.nas-name {
  min-width: 0;
  margin: 0;
  overflow: hidden;
  color: var(--text-primary);
  font-size: 1rem;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.nas-price-row {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
}

.nas-price {
  flex: 0 0 auto;
  color: var(--text-primary);
  font-size: 1.25rem;
  font-weight: 700;
  white-space: nowrap;
}

.nas-price.is-empty {
  color: var(--text-tertiary);
  font-size: 0.875rem;
  font-weight: 500;
}

.promotion-tag {
  min-width: 0;
  overflow: hidden;
  padding: 3px 7px;
  border: 1px solid color-mix(in srgb, var(--color-warning-500) 34%, transparent);
  border-radius: 5px;
  background: color-mix(in srgb, var(--color-warning-500) 10%, transparent);
  color: var(--color-warning-700);
  font-size: 0.6875rem;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.nas-spec-line {
  display: flex;
  min-width: 0;
  gap: 0;
  overflow: hidden;
  color: var(--text-secondary);
  font-size: 0.75rem;
  line-height: 1.45;
  white-space: nowrap;
}

.nas-spec-line span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}

.nas-spec-line span:not(:last-child)::after {
  content: '·';
  margin: 0 6px;
  color: var(--text-tertiary);
}

.variant-chips {
  display: flex;
  gap: 6px;
  min-width: 0;
  overflow-x: auto;
  scrollbar-width: none;
}

.variant-chips::-webkit-scrollbar {
  display: none;
}

.variant-chip {
  flex: 0 0 auto;
  max-width: 180px;
  padding: 4px 8px;
  overflow: hidden;
  border: 1px solid var(--resource-line);
  border-radius: 6px;
  background: var(--bg-secondary);
  color: var(--text-secondary);
  font: inherit;
  font-size: 0.72rem;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
}

.variant-chip:hover,
.variant-chip.active {
  border-color: var(--color-primary-400);
  color: var(--color-primary-600);
}

.variant-chip.active {
  background: var(--color-primary-500-10);
}

.nas-suitable {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 7px;
  color: var(--text-tertiary);
  font-size: 0.72rem;
}

.nas-suitable > span {
  flex: 0 0 auto;
}

.nas-suitable strong {
  min-width: 0;
  overflow: hidden;
  color: var(--text-secondary);
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.nas-card-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: auto;
  padding-top: 10px;
  border-top: 1px solid var(--resource-line);
}

.purchase-btn {
  display: inline-flex;
  min-width: 88px;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 7px 12px;
  border: 1px solid var(--color-primary-500);
  border-radius: 6px;
  background: var(--color-primary-500);
  color: var(--text-inverse, var(--bg-elevated));
  font-size: 0.8125rem;
  font-weight: 500;
  text-decoration: none;
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out), background var(--motion-duration-quick) var(--motion-ease-out);
}

.purchase-btn:hover {
  border-color: var(--color-primary-600);
  background: var(--color-primary-600);
}

.purchase-btn.is-disabled {
  border-color: var(--border-subtle);
  background: var(--bg-secondary);
  color: var(--text-tertiary);
  cursor: not-allowed;
}

.compare-toggle {
  display: inline-flex;
  width: 32px;
  height: 32px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 1px solid var(--resource-line);
  border-radius: 7px;
  background: transparent;
  color: var(--text-tertiary);
  cursor: pointer;
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out), color var(--motion-duration-quick) var(--motion-ease-out), background var(--motion-duration-quick) var(--motion-ease-out);
}

.compare-toggle:hover:not(:disabled),
.compare-toggle.active {
  border-color: var(--color-primary-400);
  background: var(--color-primary-500-10);
  color: var(--color-primary-600);
}

.compare-toggle:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
/* 骨架屏 */
.nas-card.skeleton {
  gap: 12px;
}

.skeleton-logo {
  width: 48px;
  height: 48px;
  border-radius: 10px;
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}

.skeleton-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.skeleton-title {
  height: 16px;
  width: 60%;
  border-radius: 4px;
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}

.skeleton-line {
  height: 12px;
  width: 100%;
  border-radius: 4px;
  background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--border-subtle) 50%, var(--bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}

.skeleton-line.short {
  width: 40%;
}

/* 对比抽屉 */
.compare-overlay {
  position: fixed;
  inset: 0;
  background: var(--overlay-bg);
  backdrop-filter: blur(4px);
  display: flex;
  justify-content: flex-end;
}

.compare-panel {
  width: min(900px, 100%);
  height: 100%;
  background: var(--bg-elevated);
  border-left: 1px solid var(--border-subtle);
  display: flex;
  flex-direction: column;
  box-shadow: var(--shadow-xl);
  outline: none;
}

.compare-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 20px 24px;
  border-bottom: 1px solid var(--border-subtle);
}

.compare-header h2 {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  font-size: 1.125rem;
  color: var(--text-primary);
}

.close-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  border: none;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  cursor: pointer;
}

.close-btn:hover {
  background: var(--color-danger-100);
  color: var(--color-danger-600);
}

.close-btn:focus-visible,
.remove-btn:focus-visible {
  outline: 2px solid var(--color-primary-500);
  outline-offset: 2px;
}

.compare-body {
  flex: 1;
  overflow: auto;
  padding: 20px;
}

.compare-table-wrapper {
  overflow-x: auto;
}

.compare-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.875rem;
  min-width: 600px;
}

.compare-table th,
.compare-table td {
  padding: 12px;
  border: 1px solid var(--border-subtle);
  vertical-align: top;
  text-align: left;
}

.compare-table th {
  background: var(--bg-secondary);
  color: var(--text-primary);
  font-weight: 600;
}

.row-label {
  background: var(--bg-secondary);
  color: var(--text-secondary);
  font-weight: 500;
  white-space: nowrap;
  width: 120px;
}

.device-col {
  min-width: 180px;
}

.device-header {
  position: relative;
  padding-right: 24px;
}

.device-name {
  font-weight: 600;
  color: var(--text-primary);
}

.device-brand {
  font-size: 0.75rem;
  color: var(--text-tertiary);
}

.remove-btn {
  position: absolute;
  top: 0;
  right: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 4px;
  border: none;
  background: transparent;
  color: var(--text-tertiary);
  cursor: pointer;
}

.remove-btn:hover {
  background: var(--color-danger-50);
  color: var(--color-danger-600);
}

.compare-table td.highlight {
  color: var(--color-danger-600);
  font-weight: 600;
}

.compare-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.compare-tags .tag {
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--bg-secondary);
  color: var(--text-secondary);
  font-size: 0.75rem;
  border: 1px solid var(--border-subtle);
}

.compare-links {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.compare-links a {
  color: var(--color-primary-600);
  text-decoration: none;
  font-size: 0.8125rem;
}

.compare-links a:hover {
  text-decoration: underline;
}

.drawer-enter-active,
.drawer-leave-active {
  transition: opacity var(--motion-duration-fast) var(--motion-ease-out);
}

.drawer-enter-from,
.drawer-leave-to {
  opacity: 0;
}

.drawer-enter-active .compare-panel,
.drawer-leave-active .compare-panel {
  transition: transform var(--motion-duration-fast) var(--motion-ease-out);
}

.drawer-enter-from .compare-panel,
.drawer-leave-to .compare-panel {
  transform: translateX(100%);
}

@media (max-width: 768px) {
  .nas-store-page {
    height: auto;
    min-height: 100%;
    overflow: visible;
  }

  .nas-toolbar :deep(.search-input-wrapper) {
    flex: 0 0 220px;
    width: 220px;
    min-width: 220px;
    max-width: 220px;
  }

  .sort-control {
    flex: 0 0 160px;
    width: 160px;
    min-width: 160px;
  }

  .nas-grid,
  .nas-loading {
    grid-template-columns: 1fr;
  }

}

@media (max-width: 480px) {
  .nas-spec-line {
    flex-wrap: wrap;
    white-space: normal;
  }
}
</style>
