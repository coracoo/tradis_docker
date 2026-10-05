<template>
  <ResourceWorkbench>
    <TutorialGate
      :checking="licenseChecking"
      :entitled="entitled"
      v-bind="tutorialGateCopy"
    >
    <div class="tutorial-page" role="main" aria-label="教程中心">
    <aside class="tutorial-list" aria-label="教程列表">
      <div class="list-header">
        <div>
          <h1>教程中心</h1>
          <p>{{ manifestSummary }}</p>
        </div>
        <button class="icon-btn" title="刷新" @click="loadManifest(true)">
          <DynamicIcon name="refresh" :size="16" />
        </button>
      </div>

      <div class="search-row">
        <DynamicIcon name="search" :size="16" />
        <input v-model="query" type="search" placeholder="搜索教程" />
      </div>

      <div class="tutorial-nav-scroll">
        <div v-if="loading && articles.length === 0" class="state">
          <DynamicIcon name="loading" :size="24" />
          <span>加载中...</span>
        </div>
        <div v-else-if="filteredArticles.length === 0" class="state">
          <DynamicIcon name="book-open" :size="24" />
          <span>暂无教程</span>
        </div>
        <template v-else>
          <button
            v-for="article in filteredArticles"
            :key="article.slug"
            class="article-item"
            :class="{ active: article.slug === selectedSlug }"
            @click="selectArticle(article.slug)"
          >
            <span class="article-title">{{ article.title }}</span>
            <time v-if="article.updated_at" class="article-time" :datetime="article.updated_at">
              {{ formatDate(article.updated_at) }}
            </time>
          </button>
        </template>
      </div>
    </aside>

    <section class="tutorial-detail" aria-label="教程详情">
      <div v-if="detailLoading" class="detail-state">
        <DynamicIcon name="loading" :size="28" />
        <span>加载正文...</span>
      </div>
      <div v-else-if="!selectedArticle" class="detail-state">
        <DynamicIcon name="book-open" :size="32" />
        <span>选择一篇教程开始阅读</span>
      </div>
      <article v-else class="article-detail">
        <div class="article-toolbar">
          <button
            v-if="tutorialGoActionsEnabled && (summaryLoading || !activeSummary)"
            class="compact-action"
            :disabled="summaryLoading"
            @click="summarizeArticle"
          >
            <DynamicIcon :name="summaryLoading ? 'loading' : 'sparkles'" :size="14" />
            {{ summaryLoading ? '总结中...' : 'AI 总结' }}
          </button>
          <button
            v-else-if="tutorialGoActionsEnabled"
            class="compact-action summary-toggle"
            @click="toggleSummaryVisibility"
          >
            <DynamicIcon :name="summaryVisible ? 'eye-off' : 'eye'" :size="14" />
            {{ summaryVisible ? '隐藏总结' : '显示总结' }}
          </button>
          <button
            v-if="tutorialGoActionsEnabled && activeSummary && !summaryLoading"
            class="compact-action icon-only summary-refresh"
            title="重新生成 AI 总结"
            aria-label="重新生成 AI 总结"
            @click="summarizeArticle"
          >
            <DynamicIcon name="refresh" :size="14" />
          </button>
          <a
            v-if="selectedArticle.original_url"
            class="compact-action"
            :href="selectedArticle.original_url"
            target="_blank"
            rel="noopener noreferrer"
          >
            <DynamicIcon name="external-link" :size="14" />
            阅读原文
          </a>
          <button v-if="tutorialGoActionsEnabled" class="compact-action is-primary" @click="sendToAI">
            <DynamicIcon name="bot" :size="14" />
            AI 部署
          </button>
        </div>
        <section v-if="tutorialGoActionsEnabled && summaryVisible" class="ai-summary" aria-live="polite">
          <div class="ai-summary-label">
            <DynamicIcon name="sparkles" :size="15" />
            <span>AI 总结</span>
          </div>
          <div class="ai-summary-content" v-html="renderedAISummary"></div>
        </section>
        <div
          ref="markdownRoot"
          class="markdown-content"
          v-html="renderedContent"
          @click="handleMarkdownAction"
          @keydown="handleMarkdownKeydown"
        ></div>
      </article>
    </section>
    </div>
    <div
      v-if="previewImageURL"
      class="tutorial-image-preview"
      role="dialog"
      aria-modal="true"
      aria-label="教程图片预览"
      @click.self="previewImageURL = ''"
    >
      <button class="preview-close" type="button" title="关闭" @click="previewImageURL = ''">
        <DynamicIcon name="close" :size="18" />
      </button>
      <img :src="previewImageURL" alt="教程图片预览" />
    </div>
    </TutorialGate>
  </ResourceWorkbench>
</template>

<script setup>
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import ResourceWorkbench from '@/components/resource-workbench/ResourceWorkbench.vue'
import { getTutorialArticle, getTutorialManifest } from '@/api/tutorialsRead.js'
import { renderSafeMarkdown } from '@/utils/markdown.js'
import {
  TutorialGate,
  tutorialGateCopy,
  tutorialGoActionsEnabled,
  tutorialAccessDeniedMessage,
  loadTutorialAccess,
  checkTutorialGoFeature,
  openTutorialAgentRun,
  getCachedTutorialSummary,
  getTutorialAsset,
  getTutorialSummary
} from '@edition/tutorial-actions'
import { useToast } from '@/composables/useToast.js'
import { copyToClipboard } from '@/utils/helpers.js'
import { useTutorialAssets } from './useTutorialAssets.js'

const SUMMARY_VISIBILITY_KEY = 'tutorial_summary_visibility_v1'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const licenseChecking = ref(true)
const entitled = ref(false)
const loading = ref(false)
const detailLoading = ref(false)
const query = ref('')
const articles = ref([])
const selectedSlug = ref(String(route.params.slug || ''))
const selectedArticle = ref(null)
const manifestCursor = ref('')
const summaries = ref({})
const summaryLoadingSlug = ref('')
const summaryVisibility = ref(readSummaryVisibility())
const markdownRoot = ref(null)
const previewImageURL = ref('')

const filteredArticles = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return articles.value
  return articles.value.filter((item) => {
    return [item.title, item.summary, item.category, ...(item.tags || [])]
      .some(value => String(value || '').toLowerCase().includes(q))
  })
})

const manifestSummary = computed(() => {
  if (articles.value.length === 0) return tutorialGoActionsEnabled ? '官方部署文章与实战指南' : '公开部署文章与实战指南'
  return `${articles.value.length} 篇教程`
})

const renderedContent = computed(() => {
  return renderSafeMarkdown(selectedArticle.value?.content || '')
})

const tutorialContentKey = computed(() => {
  const article = selectedArticle.value
  return `${selectedSlug.value}:${article?.fingerprint || ''}:${article?.content?.length || 0}`
})

const { retryTutorialAsset } = useTutorialAssets({
  root: markdownRoot,
  contentKey: tutorialContentKey,
  fetchAsset: getTutorialAsset
})

const activeSummary = computed(() => {
  return summaries.value[selectedSlug.value] || ''
})

const summaryVisible = computed(() => {
  if (!activeSummary.value) return false
  return summaryVisibility.value[selectedSlug.value] !== false
})

const renderedAISummary = computed(() => {
  return renderSafeMarkdown(activeSummary.value)
})

const summaryLoading = computed(() => {
  return summaryLoadingSlug.value === selectedSlug.value
})

function formatDate(value) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleDateString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' })
}

function readSummaryVisibility() {
  try {
    const parsed = JSON.parse(localStorage.getItem(SUMMARY_VISIBILITY_KEY) || '{}')
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {}
  } catch {
    return {}
  }
}

function setSummaryVisibility(slug, visible) {
  if (!slug) return
  summaryVisibility.value = { ...summaryVisibility.value, [slug]: Boolean(visible) }
  localStorage.setItem(SUMMARY_VISIBILITY_KEY, JSON.stringify(summaryVisibility.value))
}

function toggleSummaryVisibility() {
  if (!activeSummary.value) return
  setSummaryVisibility(selectedSlug.value, !summaryVisible.value)
}

async function loadCachedSummary(slug) {
  if (!tutorialGoActionsEnabled || !slug || summaries.value[slug]) return
  try {
    const response = await getCachedTutorialSummary(slug)
    const summary = String(response?.summary || '').trim()
    if (summary) summaries.value = { ...summaries.value, [slug]: summary }
  } catch {
    // 缓存不存在、过期或许可不可用时保持正文阅读，不弹出错误。
  }
}

async function loadManifest(force = false) {
  loading.value = true
  try {
    const params = force ? { refresh: '1', _: Date.now().toString() } : {}
    const data = await getTutorialManifest(params)
    articles.value = Array.isArray(data?.articles) ? data.articles : []
    manifestCursor.value = data?.cursor || ''
    if (!selectedSlug.value && articles.value.length > 0) {
      await selectArticle(articles.value[0].slug)
    } else if (selectedSlug.value) {
      await selectArticle(selectedSlug.value, false)
    }
  } catch (error) {
    console.error('加载教程列表失败:', error)
  } finally {
    loading.value = false
  }
}

async function selectArticle(slug, updateRoute = true) {
  if (!slug) return
  if (selectedSlug.value !== slug) selectedArticle.value = null
  selectedSlug.value = slug
  if (updateRoute && route.params.slug !== slug) {
    router.push(`/tutorials/${encodeURIComponent(slug)}`)
  }
  detailLoading.value = true
  try {
    const detail = await getTutorialArticle(slug)
    const meta = articles.value.find(item => item.slug === slug) || {}
    if (selectedSlug.value === slug) selectedArticle.value = { ...meta, ...detail }
  } catch (error) {
    console.error('加载教程正文失败:', error)
    if (selectedSlug.value === slug) {
      selectedArticle.value = articles.value.find(item => item.slug === slug) || null
    }
  } finally {
    if (selectedSlug.value === slug) detailLoading.value = false
  }
  await loadCachedSummary(slug)
}

async function summarizeArticle() {
  const slug = String(selectedArticle.value?.slug || selectedSlug.value || '').trim()
  if (!slug || summaryLoadingSlug.value) return
  const entitled = await checkTutorialGoFeature().catch(() => false)
  if (!entitled) {
    toast.warning(tutorialAccessDeniedMessage)
    return
  }
  summaryLoadingSlug.value = slug
  try {
    const response = await getTutorialSummary(slug, {
      refresh: Boolean(summaries.value[slug])
    })
    const summary = String(response?.summary || '').trim()
    if (!summary) throw new Error('AI 未返回总结内容')
    summaries.value = { ...summaries.value, [slug]: summary }
    setSummaryVisibility(slug, true)
    if (response?.warning) toast.warning(String(response.warning))
  } catch (error) {
    toast.error(error?.message || 'AI 总结失败')
  } finally {
    if (summaryLoadingSlug.value === slug) summaryLoadingSlug.value = ''
  }
}

async function sendToAI() {
  if (!selectedArticle.value) return
  const entitled = await checkTutorialGoFeature().catch(() => false)
  if (!entitled) {
    toast.warning(tutorialAccessDeniedMessage)
    return
  }
  const slug = String(selectedArticle.value.slug || selectedSlug.value || '').trim()
  if (!slug) {
    toast.error('教程标识无效')
    return
  }
  openTutorialAgentRun({ ...selectedArticle.value, slug }, router)
}

async function decorateTutorialCodeBlocks() {
  await nextTick()
  markdownRoot.value?.querySelectorAll('pre').forEach((block) => {
    if (block.querySelector('.tutorial-code-copy')) return
    const button = document.createElement('button')
    button.type = 'button'
    button.className = 'tutorial-code-copy'
    button.dataset.tutorialAction = 'copy-code'
    button.textContent = '复制'
    block.appendChild(button)
  })
}

async function handleMarkdownAction(event) {
  const copyButton = event.target.closest?.('[data-tutorial-action="copy-code"]')
  if (copyButton) {
    const code = copyButton.closest('pre')?.querySelector('code')?.textContent || ''
    if (!code) return
    const ok = await copyToClipboard(code)
    if (ok) {
      copyButton.textContent = '已复制'
      window.setTimeout(() => { copyButton.textContent = '复制' }, 1200)
    } else {
      toast.error('复制失败')
    }
    return
  }
  const image = event.target.closest?.('img[data-tutorial-asset]')
  if (!image) return
  if (image.dataset.assetState === 'error') {
    await retryTutorialAsset(image)
    return
  }
  if (image.dataset.assetState === 'ready' && image.src) {
    previewImageURL.value = image.src
  }
}

function handleMarkdownKeydown(event) {
  if (event.key !== 'Enter' && event.key !== ' ') return
  const image = event.target.closest?.('img[data-tutorial-asset]')
  if (!image) return
  event.preventDefault()
  void handleMarkdownAction({ target: image })
}

onMounted(async () => {
  try {
    entitled.value = await loadTutorialAccess()
    if (entitled.value) {
      await loadManifest(false)
    }
  } catch (error) {
    toast.error(error?.message || '读取教程权限失败')
  } finally {
    licenseChecking.value = false
  }
})

watch(() => route.params.slug, (slug) => {
  const next = String(slug || '')
  if (entitled.value && next && next !== selectedSlug.value) {
    selectArticle(next, false)
  }
})

watch(tutorialContentKey, async () => {
  previewImageURL.value = ''
  await decorateTutorialCodeBlocks()
}, { flush: 'post' })
</script>

<style scoped>
.tutorial-page {
  box-sizing: border-box;
  display: grid;
  grid-template-columns: clamp(300px, 22vw, 340px) minmax(0, 1fr);
  gap: 14px;
  height: 100%;
  min-height: 0;
  padding-bottom: 22px;
  overflow: hidden;
  color: var(--resource-ink);
}

.tutorial-list,
.tutorial-detail {
  background: var(--resource-panel);
  border: 1px solid var(--resource-line);
  border-radius: 12px;
  box-shadow: 0 12px 30px color-mix(in srgb, var(--text-primary) 5%, transparent);
  min-height: 0;
}

.tutorial-list {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  overscroll-behavior: contain;
}

.list-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 15px 15px 12px;
}

.list-header h1 {
  margin: 0;
  font-size: 0.95rem;
  line-height: 1.25;
  font-weight: 680;
  color: var(--text-primary);
}

.list-header p,
.article-time {
  color: var(--text-secondary);
}

.list-header p {
  margin: 0.28rem 0 0;
  font-size: 0.6875rem;
  line-height: 1.35;
}

.icon-btn {
  width: 32px;
  height: 32px;
  border: 1px solid var(--resource-line);
  border-radius: 8px;
  background: var(--bg-elevated);
  color: var(--text-secondary);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background-color var(--motion-duration-quick) var(--motion-ease-out), border-color var(--motion-duration-quick) var(--motion-ease-out), color var(--motion-duration-quick) var(--motion-ease-out);
}

.icon-btn:hover {
  border-color: var(--resource-line-strong);
  background: var(--resource-row-hover);
  color: var(--text-primary);
}

.icon-btn:focus-visible,
.article-item:focus-visible,
.compact-action:focus-visible {
  outline: 3px solid color-mix(in srgb, var(--color-primary-500) 24%, transparent);
  outline-offset: 2px;
}

.search-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  height: 40px;
  padding: 0 12px;
  border: 1px solid var(--resource-line);
  border-radius: 8px;
  background: var(--bg-elevated);
  margin: 0 14px 10px;
  color: var(--text-tertiary);
  flex: 0 0 auto;
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out), box-shadow var(--motion-duration-quick) var(--motion-ease-out);
}

.search-row:focus-within {
  border-color: var(--color-primary-400);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary-500) 16%, transparent);
}

.search-row input {
  width: 100%;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--resource-muted);
  font-family: var(--font-sans, Inter, ui-sans-serif, system-ui, sans-serif);
  font-size: 0.8125rem;
  font-weight: 400;
}

.search-row input::placeholder {
  color: var(--text-tertiary);
}

.tutorial-nav-scroll {
  flex: 1 1 auto;
  min-height: 0;
  padding: 0 8px 10px;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-gutter: stable;
}

.article-item {
  position: relative;
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 11px 10px 13px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  text-align: left;
  cursor: pointer;
  transition: background-color var(--motion-duration-quick) var(--motion-ease-out), color var(--motion-duration-quick) var(--motion-ease-out);
}

.article-item + .article-item::before {
  content: '';
  position: absolute;
  top: 0;
  left: 11px;
  right: 11px;
  height: 1px;
  background: var(--resource-line);
  pointer-events: none;
}

.article-item:hover {
  background: var(--resource-row-hover);
}

.article-item.active {
  background: color-mix(in srgb, var(--color-primary-500) 7%, var(--bg-elevated));
  box-shadow: inset 3px 0 0 var(--color-primary-500);
}

.article-title {
  font-size: 0.8125rem;
  line-height: 1.5;
  font-weight: 600;
  color: var(--text-primary);
}

.article-time {
  font-size: 0.6875rem;
  line-height: 1.25;
}

.tutorial-detail {
  min-width: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-gutter: stable;
}

.article-detail {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  max-width: 920px;
  margin: 0 auto;
  padding: 0 30px 40px;
}

.article-toolbar {
  position: sticky;
  top: 0;
  z-index: 5;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.4rem;
  flex-wrap: wrap;
  min-height: 58px;
  margin-bottom: 1.15rem;
  padding: 12px 0;
  border-bottom: 1px solid var(--resource-line);
  background: color-mix(in srgb, var(--resource-panel) 96%, transparent);
}

.compact-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.32rem;
  height: 30px;
  padding: 0 0.62rem;
  border-radius: 7px;
  border: 1px solid var(--resource-line);
  background: var(--bg-elevated);
  color: var(--text-secondary);
  font-size: 0.75rem;
  font-weight: 560;
  text-decoration: none;
  white-space: nowrap;
  cursor: pointer;
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out), color var(--motion-duration-quick) var(--motion-ease-out), background var(--motion-duration-quick) var(--motion-ease-out);
}

.compact-action:hover:not(:disabled) {
  border-color: color-mix(in srgb, var(--color-primary-600) 42%, var(--border-subtle));
  color: var(--text-primary);
}

.compact-action:disabled {
  cursor: wait;
  opacity: 0.65;
}

.compact-action.is-primary {
  color: var(--text-inverse);
  background: var(--color-primary-600);
  border-color: var(--color-primary-600);
}

.compact-action.is-primary:hover {
  color: var(--text-inverse);
  background: var(--color-primary-700);
  border-color: var(--color-primary-700);
}

.compact-action.icon-only {
  width: 30px;
  padding-inline: 0;
  justify-content: center;
}

.ai-summary {
  padding: 0.05rem 0 1.15rem;
  margin-bottom: 1.35rem;
  border-bottom: 1px solid var(--border-subtle);
}

.ai-summary-label {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-bottom: 0.55rem;
  color: var(--color-primary-600);
  font-size: 0.75rem;
  font-weight: 700;
}

.ai-summary-content {
  padding-left: 1.15rem;
  border-left: 2px solid color-mix(in srgb, var(--color-primary-600) 58%, var(--border-subtle));
  color: var(--text-secondary);
  font-size: 0.85rem;
  line-height: 1.75;
}

.ai-summary-content :deep(p) {
  margin: 0.45rem 0;
}

.ai-summary-content :deep(ul),
.ai-summary-content :deep(ol) {
  margin: 0.5rem 0;
  padding-left: 1.25rem;
}

.ai-summary-content :deep(li) {
  margin: 0.2rem 0;
}

.ai-summary-content :deep(code) {
  padding: 0.1rem 0.3rem;
  border-radius: 4px;
  background: var(--bg-tertiary);
  color: var(--text-primary);
  font-family: var(--font-mono);
}

.markdown-content {
  min-width: 0;
  margin-top: 0;
  font-size: 0.925rem;
  line-height: 1.8;
  color: var(--text-primary);
  overflow-wrap: anywhere;
}

.markdown-content :deep(h1),
.markdown-content :deep(h2),
.markdown-content :deep(h3) {
  margin: 2rem 0 0.75rem;
  color: var(--text-primary);
  line-height: 1.4;
}

.markdown-content :deep(h1) {
  padding-bottom: 0.55rem;
  border-bottom: 1px solid var(--border-subtle);
  font-size: 1.35rem;
  font-weight: 750;
}

.markdown-content :deep(h2) {
  font-size: 1.18rem;
  font-weight: 720;
}

.markdown-content :deep(h3) {
  margin-top: 1.6rem;
  font-size: 1.02rem;
  font-weight: 700;
}

.markdown-content :deep(p) {
  margin: 0.85rem 0;
}

.markdown-content :deep(strong) {
  font-weight: 700;
}

.markdown-content :deep(ul),
.markdown-content :deep(ol) {
  margin: 0.8rem 0;
  padding-left: 1.55rem;
}

.markdown-content :deep(li) {
  margin: 0.3rem 0;
  padding-left: 0.15rem;
}

.markdown-content :deep(a) {
  color: var(--color-primary-600);
  text-decoration: underline;
  text-decoration-color: color-mix(in srgb, var(--color-primary-600) 45%, transparent);
  text-underline-offset: 3px;
}

.markdown-content :deep(a:hover) {
  text-decoration-color: currentColor;
}

.markdown-content :deep(blockquote) {
  margin: 1.1rem 0;
  padding: 0.7rem 0.9rem;
  border-left: 3px solid var(--color-primary-600);
  background: color-mix(in srgb, var(--color-primary-600) 7%, var(--bg-elevated));
  color: var(--text-secondary);
}

.markdown-content :deep(blockquote p) {
  margin: 0;
}

.markdown-content :deep(code) {
  background: var(--bg-tertiary);
  border-radius: 4px;
  padding: 0.15rem 0.35rem;
  font-family: var(--font-mono);
  font-size: 0.86em;
}

.markdown-content :deep(pre) {
  position: relative;
  box-sizing: border-box;
  max-width: 100%;
  overflow-x: auto;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  margin: 1.1rem 0;
  padding: 0.9rem 1rem;
  line-height: 1.65;
  scrollbar-gutter: stable;
}

.markdown-content :deep(.tutorial-code-copy) {
  position: sticky;
  top: 0;
  float: right;
  height: 26px;
  margin: -0.25rem -0.35rem 0.35rem 0.75rem;
  padding: 0 0.5rem;
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  background: var(--bg-elevated);
  color: var(--text-tertiary);
  font-size: 0.6875rem;
  cursor: pointer;
}

.markdown-content :deep(pre code) {
  padding: 0;
  background: transparent;
  font-size: 0.8125rem;
}

.markdown-content :deep(img) {
  display: block;
  max-width: 100%;
  height: auto;
  margin: 1rem auto;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  object-fit: contain;
  cursor: zoom-in;
}

.markdown-content :deep(img[data-asset-state='pending']),
.markdown-content :deep(img[data-asset-state='loading']) {
  width: 100%;
  min-height: 180px;
  background: var(--bg-tertiary);
}

.markdown-content :deep(img[data-asset-state='error']) {
  width: 100%;
  min-height: 120px;
  cursor: pointer;
  background: var(--bg-tertiary);
}

.tutorial-image-preview {
  position: fixed;
  inset: 0;
  z-index: 2100;
  display: grid;
  place-items: center;
  padding: 3rem;
  background: color-mix(in srgb, #000 78%, transparent);
}

.tutorial-image-preview > img {
  max-width: min(96vw, 1600px);
  max-height: 90vh;
  object-fit: contain;
}

.preview-close {
  position: fixed;
  top: 1rem;
  right: 1rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border: 1px solid color-mix(in srgb, #fff 28%, transparent);
  border-radius: 7px;
  background: color-mix(in srgb, #000 55%, transparent);
  color: #fff;
  cursor: pointer;
}

.markdown-content :deep(table) {
  display: block;
  width: max-content;
  max-width: 100%;
  margin: 1.1rem 0;
  overflow-x: auto;
  border-collapse: collapse;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
}

.markdown-content :deep(th),
.markdown-content :deep(td) {
  min-width: 8rem;
  padding: 0.55rem 0.7rem;
  border-right: 1px solid var(--border-subtle);
  border-bottom: 1px solid var(--border-subtle);
  text-align: left;
  vertical-align: top;
}

.markdown-content :deep(th) {
  background: var(--bg-tertiary);
  font-size: 0.8125rem;
  font-weight: 700;
}

.markdown-content :deep(tr:last-child td) {
  border-bottom: 0;
}

.markdown-content :deep(th:last-child),
.markdown-content :deep(td:last-child) {
  border-right: 0;
}

.markdown-content :deep(hr) {
  margin: 1.75rem 0;
  border: 0;
  border-top: 1px solid var(--border-subtle);
}

.markdown-content :deep(input[type='checkbox']) {
  margin-right: 0.45rem;
  accent-color: var(--color-primary-600);
}

.state,
.detail-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.6rem;
  color: var(--text-tertiary);
}

.state {
  min-height: 10rem;
}

.detail-state {
  min-height: 24rem;
}

@media (max-width: 900px) {
  .tutorial-page {
    grid-template-columns: 1fr;
    grid-template-rows: minmax(12rem, 42vh) minmax(0, 1fr);
  }

  .tutorial-list {
    max-height: none;
  }

  .article-toolbar {
    justify-content: flex-start;
  }
}

@media (max-width: 768px) {
  .tutorial-page {
    height: auto;
    min-height: 0;
    overflow: visible;
  }

  .tutorial-list {
    max-height: 42vh;
  }

  .tutorial-detail {
    overflow: visible;
  }

  .article-detail {
    padding: 0 16px 28px;
  }

  .article-toolbar {
    min-height: 54px;
    margin-bottom: 0.9rem;
    padding: 10px 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .icon-btn,
  .article-item,
  .compact-action {
    transition: none;
  }
}
</style>
