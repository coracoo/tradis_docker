<template>
  <div v-if="section === 'service'" class="form-group">
    <label class="form-label" for="community-tutorial-rss">教程 RSS 地址</label>
    <input
      id="community-tutorial-rss"
      v-model="fields.tutorialRSSURL"
      class="form-input"
      type="url"
      placeholder="https://example.com/feed.xml"
    />
    <p class="form-hint">自定义 HTTP(S) RSS 地址；留空不加载教程，与应用商店内容源独立。</p>
  </div>
</template>

<script setup>
defineProps({ section: { type: String, required: true } })
const fields = defineModel('fields', { type: Object, required: true })
</script>

<style scoped>
.form-group { margin-bottom: 16px; }
.form-label { display: block; margin-bottom: 6px; color: var(--text-primary); font-size: 0.8125rem; font-weight: 500; }
.form-input { width: 100%; padding: 10px 12px; border: 1px solid var(--border-subtle); border-radius: 8px; background: var(--bg-primary); color: var(--text-primary); font-size: 0.875rem; }
.form-input:focus { outline: none; border-color: var(--color-primary-500); box-shadow: 0 0 0 3px var(--ring-primary); }
.form-hint { margin: 6px 0 0; color: var(--text-tertiary); font-size: 0.75rem; }
</style>
