export function hasAppStoreSourceChanges({ app = {}, composeText, sourceFiles } = {}) {
  const officialCompose = app.compose || ''
  const officialFiles = Array.isArray(app.source_files) ? app.source_files : []
  const effectiveCompose = composeText ?? officialCompose
  const effectiveFiles = Array.isArray(sourceFiles) ? sourceFiles : officialFiles
  return effectiveCompose !== officialCompose || JSON.stringify(effectiveFiles) !== JSON.stringify(officialFiles)
}

export function buildAppStoreDeployRequest({
  app = {},
  projectName = '',
  composeText,
  manifestDigest = '',
  baseManifestDigest = '',
  overrideManifestDigest = '',
  valuesByInputID = {},
  mappingOverlays = [],
  sourceFiles
} = {}) {
  const effectiveBaseDigest = baseManifestDigest || manifestDigest || app.manifest_digest || app.manifest?.manifest_digest || ''
  const request = {
    baseManifestDigest: effectiveBaseDigest,
    valuesByInputId: { ...valuesByInputID },
    mappingOverlays: mappingOverlays.map(item => ({ ...item }))
  }
  if (String(projectName || '').trim()) {
    request.projectName = String(projectName).trim()
  }

  const officialCompose = app.compose || ''
  const officialDotenv = app.dotenv || ''
  const officialFiles = Array.isArray(app.source_files) ? app.source_files : []
  const effectiveCompose = composeText ?? officialCompose
  const effectiveFiles = Array.isArray(sourceFiles) ? sourceFiles : officialFiles
  if (hasAppStoreSourceChanges({ app, composeText: effectiveCompose, sourceFiles: effectiveFiles })) {
    if (!effectiveBaseDigest) throw new Error('base manifest digest is required for an edited source')
    if (!overrideManifestDigest) throw new Error('override manifest digest is required for an edited source')
    request.overrideManifestDigest = overrideManifestDigest
    request.sourceOverride = {
      compose_path: 'compose.yaml',
      compose: effectiveCompose,
      dotenv: officialDotenv,
      files: effectiveFiles.map(file => ({ ...file }))
    }
  }
  return request
}
