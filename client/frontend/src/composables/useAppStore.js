import { ref, computed, watch } from 'vue'
import {
  getAppDetail,
  getApps,
  getAppsMeta,
  getAppsMetaStatus,
  getStatus,
  normalizeAppStoreStatus
} from '../api/appstore.js'
import { appStoreDeploymentMetrics } from '@edition/appstore-deployment-metrics'

const CACHE_KEY = 'appstore_apps_cache'
const CACHE_TIME_KEY = 'appstore_cache_time'
const CACHE_META_KEY = 'appstore_apps_meta'
const CACHE_FRESH_DURATION = 10 * 60 * 1000
const CACHE_STALE_DURATION = 6 * 60 * 60 * 1000

// 分类中文映射
const categoryMap = {
  'media': '媒体',
  'development': '开发',
  'tools': '工具',
  'productivity': '生产力',
  'database': '数据库',
  'storage': '存储',
  'network': '网络',
  'monitoring': '监控',
  'security': '安全',
  'automation': '自动化',
  'ai': 'AI工具',
  'multimedia': '多媒体',
  'other': '其他',
  'others': '其他',
  'utilities': '工具',
  'dev': '开发',
  'web': '网站',
  'blog': '博客',
  'cms': '内容管理',
  'cloud': '云存储',
  'communication': '通讯',
  'chat': '聊天',
  'game': '游戏',
  'finance': '金融',
  'shopping': '购物',
  'social': '社交',
  'learning': '学习',
  'reading': '阅读',
  'music': '音乐',
  'video': '视频',
  'photo': '图片',
  'photos': '图像',
  'download': '下载',
  'proxy': '代理',
  'vpn': 'VPN',
  'dns': 'DNS',
  'mail': '邮件',
  'git': '版本控制',
  'ci': '持续集成',
  'office': '办公',
  'note': '笔记',
  'wiki': '知识库',
  'home': '智能家居',
  'iot': '物联网',
  'platform': '平台',
  'entertainment': '娱乐',
  'file': '文件',
  'knowledge': '知识库',
  'hobby': '爱好'
}

export function useAppStore() {
  // 状态
  const loading = ref(false)
  const apps = ref([])
  const appStoreStatus = ref(normalizeAppStoreStatus())
  const statusLoading = ref(false)
  const appStoreMetaStatus = ref({
    state: 'unknown',
    summary: '尚未获取模板版本信息',
    inSync: false,
    needsRefresh: false,
    local: null,
    remote: null
  })
  const metaStatusLoading = ref(false)
  const searchQuery = ref('')
  const searchQueryDebounced = ref('')
  const selectedCategory = ref('all')
  const selectedApp = ref(null)
  const detailLoading = ref(false)
  const lastFetchAt = ref(0)
  let searchTimeout = null
  let fetchAppsPromise = null

  // 分页状态
  const currentPage = ref(1)
  const pageSize = ref(40)
  const hasMore = ref(true)
  const loadingMore = ref(false)

  // 排序状态
  const currentSort = ref({ key: 'created_at', order: 'desc' })
  const sortValue = ref('created_at:desc')

  // 排序下拉选项
  const sortOptionsList = [
    { value: 'created_at:desc', label: '最新发布' },
    { value: 'created_at:asc', label: '最早发布' },
    ...appStoreDeploymentMetrics.sortOptions
  ]

  // 当前排序标签
  const sortLabel = computed(() => {
    const opt = sortOptionsList.find(o => o.value === sortValue.value)
    return opt?.label || '排序'
  })

  function getSourceOrder(app) {
    const explicitOrder = Number(app?.sort_order)
    if (Number.isFinite(explicitOrder) && explicitOrder > 0) {
      return explicitOrder
    }
    const value = Number(app?.__sourceOrder)
    return Number.isFinite(value) ? value : Number.MAX_SAFE_INTEGER
  }

  // 分类列表
  const categories = computed(() => {
    const cats = [{ id: 'all', name: '全部', count: apps.value.length }]
    const catMap = new Map()
    
    apps.value.forEach(app => {
      const cat = app.category || '其他'
      catMap.set(cat, (catMap.get(cat) || 0) + 1)
    })
    
    catMap.forEach((count, name) => {
      cats.push({ id: name, name: getCategoryCN(name), count })
    })
    
    return cats
  })

  // 筛选和排序后的应用（不分页）
  const sortedApps = computed(() => {
    let result = apps.value
    
    // 分类筛选
    if (selectedCategory.value !== 'all') {
      result = result.filter(app => app.category === selectedCategory.value)
    }
    
    // 搜索筛选（使用防抖后的值）
    if (searchQueryDebounced.value.trim()) {
      const query = searchQueryDebounced.value.toLowerCase()
      result = result.filter(app =>
        app.name.toLowerCase().includes(query) ||
        (app.description && app.description.toLowerCase().includes(query))
      )
    }
    
    // 排序
    result = [...result].sort((a, b) => {
      let compareResult = 0
      let preserveDirection = false
      
      if (currentSort.value.key === 'created_at') {
        const timeA = Date.parse(a?.created_at || '')
        const timeB = Date.parse(b?.created_at || '')
        const hasTimeA = Number.isFinite(timeA)
        const hasTimeB = Number.isFinite(timeB)

        if (hasTimeA && hasTimeB) {
          compareResult = timeB - timeA
        } else {
          compareResult = getSourceOrder(a) - getSourceOrder(b)
          preserveDirection = true
        }
      } else if (currentSort.value.key === 'deployment_count') {
        const countA = a.deployment_count || 0
        const countB = b.deployment_count || 0
        compareResult = countB - countA
      }
      
      // 如果主排序字段相同，按上游返回顺序作为后备，避免退化为首字母排序
      if (compareResult === 0) {
        compareResult = getSourceOrder(a) - getSourceOrder(b)
        preserveDirection = true
      }
      
      return preserveDirection || currentSort.value.order !== 'asc'
        ? compareResult
        : -compareResult
    })
    
    return result
  })

  // 分页后的应用
  const filteredApps = computed(() => {
    const end = currentPage.value * pageSize.value
    return sortedApps.value.slice(0, end)
  })

  // 空状态消息
  const emptyMessage = computed(() => {
    if (searchQuery.value) {
      return `未找到与 "${searchQuery.value}" 相关的应用`
    }
    if (selectedCategory.value !== 'all') {
      return `该分类暂无应用`
    }
    return '暂无应用数据'
  })

  // 获取分类中文名
  function getCategoryCN(category) {
    if (!category) return '其他'
    return categoryMap[category.toLowerCase()] || category
  }

  // 从缓存加载
  function loadFromCache() {
    try {
      const cached = localStorage.getItem(CACHE_KEY)
      const cacheTime = localStorage.getItem(CACHE_TIME_KEY)
      
      if (cached && cacheTime) {
        const now = Date.now()
        const time = parseInt(cacheTime)
        const age = now - time
        const parsed = JSON.parse(cached)
        
        if (age < CACHE_STALE_DURATION) {
          apps.value = Array.isArray(parsed)
            ? parsed.map((app, index) => ({ ...app, __sourceOrder: index }))
            : []
          lastFetchAt.value = time
          return {
            hit: true,
            isFresh: age < CACHE_FRESH_DURATION,
            isStale: age >= CACHE_FRESH_DURATION,
            age
          }
        }
      }
    } catch (e) {
      console.error('读取缓存失败:', e)
    }
    return {
      hit: false,
      isFresh: false,
      isStale: false,
      age: 0
    }
  }

  // 保存到缓存
  function saveToCache() {
    try {
      const now = Date.now()
      localStorage.setItem(CACHE_KEY, JSON.stringify(apps.value))
      localStorage.setItem(CACHE_TIME_KEY, now.toString())
      lastFetchAt.value = now
    } catch (e) {
      console.error('保存缓存失败:', e)
    }
  }

  function touchCacheTime() {
    try {
      const now = Date.now()
      localStorage.setItem(CACHE_TIME_KEY, now.toString())
      lastFetchAt.value = now
    } catch (e) {
      console.error('更新缓存时间失败:', e)
    }
  }

  function loadCacheMeta() {
    try {
      const raw = localStorage.getItem(CACHE_META_KEY)
      if (!raw) return null
      return JSON.parse(raw)
    } catch (e) {
      console.error('读取元数据缓存失败:', e)
      return null
    }
  }

  function saveCacheMeta(meta) {
    try {
      if (!meta?.versionHash) return
      localStorage.setItem(CACHE_META_KEY, JSON.stringify(meta))
    } catch (e) {
      console.error('保存元数据缓存失败:', e)
    }
  }

  async function refreshDeployCounts() {
    if (!appStoreDeploymentMetrics.enabled) return null
    try {
      const counts = await appStoreDeploymentMetrics.fetch()
      apps.value = appStoreDeploymentMetrics.apply(apps.value, counts)
      saveToCache()
      return counts
    } catch (error) {
      console.error('刷新官方下载量失败:', error)
      apps.value = appStoreDeploymentMetrics.clear(apps.value)
      return null
    }
  }

  function clearCache() {
    try {
      localStorage.removeItem(CACHE_KEY)
      localStorage.removeItem(CACHE_TIME_KEY)
      localStorage.removeItem(CACHE_META_KEY)
    } catch (e) {
      console.error('清理缓存失败:', e)
    }
  }

  async function fetchAppsMeta(options = {}) {
    const forceRefresh = !!options.force
    const params = forceRefresh ? { refresh: '1', _: Date.now().toString() } : {}
    const data = await getAppsMeta(params)
    return data
      ? {
          versionHash: data.version_hash || '',
          generatedAt: data.generated_at || '',
          totalCount: data.total_count || 0,
          source: data.source || ''
        }
      : null
  }

  async function fetchStatus() {
    statusLoading.value = true
    try {
      appStoreStatus.value = await getStatus()
      return appStoreStatus.value
    } catch (error) {
      console.error('获取应用商店状态失败:', error)
      appStoreStatus.value = normalizeAppStoreStatus({
        state: 'offline',
        summary: '无法获取应用商店状态，请检查后端服务与配置'
      })
      return appStoreStatus.value
    } finally {
      statusLoading.value = false
    }
  }

  async function fetchMetaStatus(options = {}) {
    metaStatusLoading.value = true
    try {
      const params = options.force ? { refresh: '1', _: Date.now().toString() } : {}
      const data = await getAppsMetaStatus(params)
      appStoreMetaStatus.value = data
        ? {
            state: data.state || 'unknown',
            summary: data.summary || '尚未获取模板版本信息',
            inSync: !!data.in_sync,
            needsRefresh: !!data.needs_refresh,
            local: data.local || null,
            remote: data.remote || null
          }
        : {
            state: 'unknown',
            summary: '尚未获取模板版本信息',
            inSync: false,
            needsRefresh: false,
            local: null,
            remote: null
          }
      return appStoreMetaStatus.value
    } catch (error) {
      console.error('获取模板版本状态失败:', error)
      appStoreMetaStatus.value = {
        state: 'unknown',
        summary: '获取模板版本状态失败',
        inSync: false,
        needsRefresh: false,
        local: null,
        remote: null
      }
      return appStoreMetaStatus.value
    } finally {
      metaStatusLoading.value = false
    }
  }

  // 获取应用列表
  async function fetchApps(options = {}) {
    const forceRefresh = !!options.force
    const cacheState = forceRefresh
      ? { hit: false, isFresh: false, isStale: false, age: 0 }
      : loadFromCache()
    const localMeta = forceRefresh ? null : loadCacheMeta()

    if (forceRefresh) {
      clearCache()
    }

    if (!cacheState.hit) {
      loading.value = true
    }

    if (!forceRefresh && fetchAppsPromise) {
      return fetchAppsPromise
    }

    fetchAppsPromise = (async () => {
      try {
        let remoteMeta = null
        if (!forceRefresh && cacheState.hit) {
          try {
            remoteMeta = await fetchAppsMeta()
            if (remoteMeta?.versionHash && remoteMeta.versionHash === localMeta?.versionHash) {
              await refreshDeployCounts()
              touchCacheTime()
              return apps.value
            }
          } catch (metaError) {
            console.error('获取应用列表元数据失败:', metaError)
          }
        }

        const params = forceRefresh ? { refresh: '1', _: Date.now().toString() } : {}
        const requestOptions = forceRefresh
          ? {
              headers: {
                'Cache-Control': 'no-cache',
                Pragma: 'no-cache'
              }
            }
          : {}
        const data = await getApps(params, requestOptions)
        apps.value = Array.isArray(data)
          ? data.map((app, index) => ({ ...app, __sourceOrder: index }))
          : []
        currentPage.value = 1
        hasMore.value = apps.value.length > pageSize.value
        saveToCache()
        if (!remoteMeta) {
          try {
            remoteMeta = await fetchAppsMeta({ force: forceRefresh })
          } catch (metaError) {
            console.error('刷新应用列表元数据失败:', metaError)
          }
        }
        if (remoteMeta) {
          saveCacheMeta(remoteMeta)
        }
        return apps.value
      } catch (error) {
        console.error('获取应用列表失败:', error)
        if (!cacheState.hit) {
          throw error
        }
        return apps.value
      } finally {
        loading.value = false
        fetchAppsPromise = null
      }
    })()

    return fetchAppsPromise
  }

  // 获取应用详情
  async function fetchAppDetail(appId) {
    detailLoading.value = true
    try {
      const detail = await getAppDetail(appId).catch(() => null)
      return detail
    } catch (error) {
      console.error('获取应用详情失败:', error)
      return null
    } finally {
      detailLoading.value = false
    }
  }

  // 打开应用详情
  async function openAppDetail(app) {
    selectedApp.value = app
    const detail = await fetchAppDetail(app.id)
    // 合并详情数据
    if (detail) {
      selectedApp.value = { ...app, ...detail }
    }
    return selectedApp.value
  }

  // 重置分页
  function resetPagination() {
    currentPage.value = 1
    hasMore.value = sortedApps.value.length > pageSize.value
  }

  // 加载更多
  async function loadMore() {
    if (loadingMore.value || !hasMore.value) return
    
    loadingMore.value = true
    
    // 模拟异步加载
    await new Promise(resolve => setTimeout(resolve, 300))
    
    currentPage.value++
    
    // 检查是否还有更多
    if (filteredApps.value.length >= sortedApps.value.length) {
      hasMore.value = false
    }
    
    loadingMore.value = false
  }

  // 排序变更处理
  function handleSortChange(value) {
    sortValue.value = value
    const [key, order] = value.split(':')
    currentSort.value = { key, order }
    resetPagination()
  }

  // 搜索处理（防抖）
  function handleSearch(query, debounceMs = 300) {
    searchQuery.value = query
    
    // 清除之前的定时器
    if (searchTimeout) {
      clearTimeout(searchTimeout)
    }
    
    // 防抖处理
    searchTimeout = setTimeout(() => {
      searchQueryDebounced.value = query
      resetPagination()
    }, debounceMs)
  }

  // 监听分类变化，重置分页
  watch(selectedCategory, () => {
    resetPagination()
  })

  // 工具函数
  function truncateDescription(desc, maxLength = 60) {
    if (!desc) return '暂无描述'
    return desc.length > maxLength ? desc.substring(0, maxLength) + '...' : desc
  }

  function formatDeployCount(count) {
    if (count >= 1000) {
      return (count / 1000).toFixed(1) + 'k'
    }
    return count
  }

  return {
    // 状态
    loading,
    apps,
    appStoreStatus,
    statusLoading,
    appStoreMetaStatus,
    metaStatusLoading,
    searchQuery,
    searchQueryDebounced,
    selectedCategory,
    selectedApp,
    detailLoading,
    lastFetchAt,
    // 分页
    currentPage,
    pageSize,
    hasMore,
    loadingMore,
    // 排序
    currentSort,
    sortValue,
    sortOptionsList,
    sortLabel,
    // 计算属性
    categories,
    sortedApps,
    filteredApps,
    emptyMessage,
    // 方法
    getCategoryCN,
    fetchStatus,
    fetchMetaStatus,
    fetchApps,
    clearCache,
    fetchAppDetail,
    openAppDetail,
    resetPagination,
    loadMore,
    handleSortChange,
    handleSearch,
    truncateDescription,
    formatDeployCount
  }
}
