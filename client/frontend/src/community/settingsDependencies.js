export const RemoteNodesSettings = null
export const LicenseSettingsPanel = null
export const GitHubAppSettingsPanel = null
export const EditionSettingsFields = defineAsyncComponent(() => import('./CommunitySettingsFields.vue'))
export const editionSettingsCopy = Object.freeze({
  aiEnabledHint: '开启后为导航识别和 Compose 生成提供统一模型能力',
  aiKeyHint: '导航识别与 Compose 生成共用此 Base URL 和 API Key',
  aiModelLabel: 'Compose 生成模型',
  aiUtilityHint: '用于导航识别；留空时使用 Compose 生成模型',
  aiBehaviorSubtitle: '导航、Compose 定制',
  aiComposePromptHint: '场景：仅用于 Compose 页面中的 AI 生成/补全'
})
export function createEditionSettingsFields() { return { tutorialRSSURL: '' } }
export function readEditionSettingsFields(settings = {}) { return { tutorialRSSURL: settings.tutorialRSSURL || '' } }
export function getEditionServerFields(fields) { return { tutorialRSSURL: fields.tutorialRSSURL } }
export function getEditionAiFields() { return {} }
export async function loadEditionLicenseStatus() { return null }

export async function loadFullSettingsDependencies() {
  const aiModule = await import('@/api/aiFree.js')
  return { aiApi: aiModule.default, testAI: aiModule.testAI }
}
import { defineAsyncComponent } from 'vue'
