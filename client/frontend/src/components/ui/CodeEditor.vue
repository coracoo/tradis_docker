<template>
  <div class="code-editor" :style="{ height }">
    <textarea
      v-if="fallback"
      class="code-editor__fallback"
      :value="modelValue"
      :aria-label="ariaLabel"
      spellcheck="false"
      @input="emit('update:modelValue', $event.target.value)"
    ></textarea>
    <div v-else ref="editorHost" class="code-editor__host" :aria-label="ariaLabel"></div>
  </div>
</template>

<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { loadMonacoEditor } from '@/utils/monacoEnvironment.js'

const props = defineProps({
  modelValue: { type: String, default: '' },
  language: { type: String, default: 'yaml' },
  ariaLabel: { type: String, default: '代码编辑器' },
  height: { type: String, default: '430px' }
})

const emit = defineEmits(['update:modelValue'])
const editorHost = ref(null)
const fallback = ref(false)
let editor
let monaco
let model
let modelSubscription
let themeObserver

function editorTheme() {
  return document.documentElement.classList.contains('dark') ? 'vs-dark' : 'vs'
}

function createEditorOptions() {
  return {
    value: props.modelValue,
    language: props.language,
    theme: editorTheme(),
    automaticLayout: true,
    lineNumbers: 'on',
    lineNumbersMinChars: 3,
    minimap: { enabled: false },
    folding: true,
    glyphMargin: false,
    renderLineHighlight: 'line',
    renderWhitespace: 'boundary',
    guides: { indentation: true, highlightActiveIndentation: true },
    scrollBeyondLastLine: false,
    wordWrap: 'off',
    tabSize: 2,
    insertSpaces: true,
    detectIndentation: true,
    formatOnPaste: true,
    fontFamily: "'JetBrains Mono', 'SFMono-Regular', Consolas, monospace",
    fontSize: 13,
    lineHeight: 21,
    padding: { top: 10, bottom: 10 },
    overviewRulerBorder: false,
    hideCursorInOverviewRuler: true,
    scrollbar: { verticalScrollbarSize: 8, horizontalScrollbarSize: 8 }
  }
}

onMounted(async () => {
  try {
    monaco = await loadMonacoEditor()
    await nextTick()
    if (!editorHost.value) return
    editor = monaco.editor.create(editorHost.value, createEditorOptions())
    model = editor.getModel()
    modelSubscription = editor.onDidChangeModelContent(() => {
      const value = editor.getValue()
      if (value !== props.modelValue) emit('update:modelValue', value)
    })
    themeObserver = new MutationObserver(() => {
      monaco?.editor.setTheme(editorTheme())
    })
    themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
  } catch (error) {
    console.error('代码编辑器加载失败，已切换基础编辑模式:', error)
    fallback.value = true
  }
})

watch(() => props.modelValue, value => {
  if (editor && editor.getValue() !== value) editor.setValue(value || '')
})

watch(() => props.language, language => {
  const model = editor?.getModel()
  if (model && monaco) monaco.editor.setModelLanguage(model, language)
})

onBeforeUnmount(() => {
  themeObserver?.disconnect()
  modelSubscription?.dispose()
  editor?.dispose()
  model?.dispose?.()
})
</script>

<style scoped>
.code-editor {
  min-height: 180px;
  overflow: hidden;
  border: 1px solid var(--border-default);
  border-radius: 8px;
  background: var(--bg-primary);
}

.code-editor:focus-within {
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.code-editor__host,
.code-editor__fallback {
  width: 100%;
  height: 100%;
}

.code-editor__fallback {
  display: block;
  resize: none;
  padding: 10px 12px;
  border: 0;
  outline: 0;
  background: var(--bg-primary);
  color: var(--text-primary);
  font-family: var(--font-mono, 'JetBrains Mono', 'SFMono-Regular', Consolas, monospace);
  font-size: 0.8125rem;
  line-height: 1.6;
  tab-size: 2;
}
</style>
