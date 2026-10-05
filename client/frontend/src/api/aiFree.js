import { get, post } from '../utils/request.js'

export const enrichNavigation = (data) => post('/ai/navigation/enrich', data)
export const enrichNavigationByTitle = (data) => post('/ai/navigation/enrich-by-title', data)
export const enrichNavigationById = (data) => post('/ai/navigation/enrich-by-id', data)
export const generateCompose = (data) => post('/ai/compose/generate', data)
export const getAILogs = (params = {}) => get('/ai/logs', params)
export const listModels = (data = {}) => post('/ai/models', data)
export const testAI = (data) => post('/ai/test', data)

export default { enrichNavigation, enrichNavigationByTitle, enrichNavigationById, generateCompose, getLogs: getAILogs, listModels, testAI }
