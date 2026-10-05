import { defineComponent, h } from 'vue'

export const TutorialGate = defineComponent({
  name: 'CommunityTutorialGate',
  setup(_, { slots }) { return () => h('div', slots.default?.()) }
})
export const tutorialGateCopy = {}
export const tutorialGoActionsEnabled = false
export const tutorialAccessDeniedMessage = ''
export const loadTutorialAccess = async () => true
export const checkTutorialGoFeature = async () => false
export const getCachedTutorialSummary = async () => ({})
export const getTutorialSummary = async () => ({})
export const getTutorialAsset = async () => { throw new Error('RSS 教程不使用官方图片缓存') }
export const openTutorialAgentRun = () => {}
