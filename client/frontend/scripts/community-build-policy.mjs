import { readdir, readFile } from 'node:fs/promises'
import path from 'node:path'

export const COMMUNITY_BUILD_FORBIDDEN_TOKENS = Object.freeze([
  '/ai/agent',
  '/nas-store',
  '/nas/list',
  '/api/nas/',
  '/api/license',
  '/api/environments',
  '/api/self-update',
  '/settings/version-status',
  '/settings/version-check',
  '/ai-agent',
  '/github-apps',
  '/tutorials/cache',
  '/tutorials/assets',
  '/protection',
  '/remote-agents',
  'official.tradis',
  'api.coracoo',
  'X-TRADIS-Environment',
  'X-Tradis-Remote',
  'X-Tradis-Stale',
  'X-Tradis-Captured-At',
  'tradis_current_environment',
  'deploy_count',
  'tradis:remote-response',
  'githubAppSearchToken',
  'tutorialCacheLimitMB',
  'agentSafetyMode',
  'aiAgentPrompt',
  'TRADIS-XXXX',
  'afdian.com/a/cherry4nas',
  'Agent 主模型'
])

const TEXT_BUILD_EXTENSIONS = new Set(['.css', '.html', '.js', '.json', '.map', '.svg', '.txt'])

async function listBuildTextFiles(directory, rootDirectory = directory) {
  const entries = await readdir(directory, { withFileTypes: true })
  const files = []

  for (const entry of entries) {
    const entryPath = path.join(directory, entry.name)
    if (entry.isDirectory()) {
      files.push(...await listBuildTextFiles(entryPath, rootDirectory))
      continue
    }
    if (entry.isFile() && TEXT_BUILD_EXTENSIONS.has(path.extname(entry.name).toLowerCase())) {
      files.push({
        absolutePath: entryPath,
        relativePath: path.relative(rootDirectory, entryPath).split(path.sep).join('/')
      })
    }
  }

  return files
}

export async function scanCommunityBuild(directory, tokens = COMMUNITY_BUILD_FORBIDDEN_TOKENS) {
  const files = await listBuildTextFiles(directory)
  const violations = []

  for (const file of files) {
    const content = await readFile(file.absolutePath, 'utf8')
    if (content.includes('\0')) continue

    for (const token of tokens) {
      if (content.includes(token)) {
        violations.push({ file: file.relativePath, token })
      }
    }
  }

  return violations
}

export function formatCommunityBuildViolations(violations) {
  return violations.map(({ file, token }) => `- ${file}: ${token}`).join('\n')
}
