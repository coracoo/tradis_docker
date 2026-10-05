let monacoPromise

export function loadMonacoEditor() {
  if (!monacoPromise) {
    monacoPromise = Promise.all([
      import('monaco-editor/esm/vs/editor/editor.api'),
      import('monaco-editor/esm/vs/editor/editor.worker?worker')
    ]).then(async ([monaco, workerModule]) => {
      const existing = globalThis.MonacoEnvironment || {}
      globalThis.MonacoEnvironment = {
        ...existing,
        getWorker: () => new workerModule.default()
      }
      await Promise.all([
        import('monaco-editor/esm/vs/basic-languages/yaml/yaml.contribution'),
        import('monaco-editor/esm/vs/basic-languages/ini/ini.contribution')
      ])
      return monaco
    })
  }
  return monacoPromise
}
