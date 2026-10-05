import { nextTick, onBeforeUnmount, onMounted, watch } from 'vue'
export function createTutorialAssetLoader({
  fetchAsset,
  createObjectURL = blob => window.URL.createObjectURL(blob),
  revokeObjectURL = url => window.URL.revokeObjectURL(url),
  Observer = window.IntersectionObserver
} = {}) {
  const objectURLs = new Map()
  const controllers = new Map()
  let observer = null

  const ensureObserver = () => {
    if (observer || typeof Observer !== 'function') return observer
    observer = new Observer((entries) => {
      entries.forEach((entry) => {
        if (!entry.isIntersecting) return
        observer?.unobserve(entry.target)
        void load(entry.target)
      })
    }, { rootMargin: '240px 0px' })
    return observer
  }

  const load = async (image) => {
    const key = String(image?.dataset?.tutorialAsset || '').trim()
    if (!key || image.dataset.assetState === 'loading') return
    controllers.get(image)?.abort()
    const controller = new AbortController()
    controllers.set(image, controller)
    image.dataset.assetState = 'loading'
    image.setAttribute('aria-busy', 'true')
    image.setAttribute('title', '图片加载中')
    try {
      const blob = await fetchAsset(key, { signal: controller.signal })
      if (controller.signal.aborted) return
      const previousURL = objectURLs.get(image)
      if (previousURL) revokeObjectURL(previousURL)
      const objectURL = createObjectURL(blob)
      objectURLs.set(image, objectURL)
      image.src = objectURL
      image.dataset.assetState = 'ready'
      image.removeAttribute('aria-busy')
      image.setAttribute('title', image.alt || '查看原图')
      image.setAttribute('tabindex', '0')
    } catch (error) {
      if (controller.signal.aborted) return
      image.removeAttribute('src')
      image.dataset.assetState = 'error'
      image.removeAttribute('aria-busy')
      image.setAttribute('role', 'button')
      image.setAttribute('tabindex', '0')
      image.setAttribute('title', '图片加载失败，点击重试')
    } finally {
      if (controllers.get(image) === controller) controllers.delete(image)
    }
  }

  const retry = async (image) => {
    if (!image) return
    image.dataset.assetState = ''
    await load(image)
  }

  const scan = (root) => {
    if (!root) return
    const currentObserver = ensureObserver()
    root.querySelectorAll('img[data-tutorial-asset]').forEach((image) => {
      image.dataset.assetState = 'pending'
      image.setAttribute('aria-busy', 'false')
      if (currentObserver) currentObserver.observe(image)
      else void load(image)
    })
  }

  const dispose = () => {
    observer?.disconnect()
    observer = null
    controllers.forEach(controller => controller.abort())
    controllers.clear()
    objectURLs.forEach(objectURL => revokeObjectURL(objectURL))
    objectURLs.clear()
  }

  return { scan, retry, dispose }
}

export function useTutorialAssets({
  root,
  contentKey,
  fetchAsset
}) {
  const loader = createTutorialAssetLoader({ fetchAsset })

  const refresh = async () => {
    loader.dispose()
    await nextTick()
    loader.scan(root.value)
  }

  onMounted(refresh)
  watch(contentKey, refresh, { flush: 'post' })
  onBeforeUnmount(() => loader.dispose())

  return {
    refreshTutorialAssets: refresh,
    retryTutorialAsset: loader.retry
  }
}
