import { afterEach, describe, expect, it } from 'vitest'
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'

import { scanCommunityBuild } from '../../../scripts/community-build-policy.mjs'

const temporaryDirectories = []

async function createBuildFixture(files) {
  const directory = await mkdtemp(path.join(tmpdir(), 'tradis-community-build-'))
  temporaryDirectories.push(directory)

  await Promise.all(Object.entries(files).map(async ([relativePath, content]) => {
    const filePath = path.join(directory, relativePath)
    await mkdir(path.dirname(filePath), { recursive: true })
    await writeFile(filePath, content)
  }))

  return directory
}

afterEach(async () => {
  await Promise.all(temporaryDirectories.splice(0).map(directory => rm(directory, { recursive: true, force: true })))
})

describe('scanCommunityBuild', () => {
  it('accepts a local-only build artifact', async () => {
    const directory = await createBuildFixture({
      'index.html': '<div id="app"></div>',
      'assets/app.js': 'fetch("/api/containers")',
      'assets/app.css': '.app { color: var(--text-primary); }'
    })

    await expect(scanCommunityBuild(directory)).resolves.toEqual([])
  })

  it('reports each forbidden protocol marker with its artifact path', async () => {
    const directory = await createBuildFixture({
      'assets/app.js': 'fetch("/api/github-apps/search"); headers.set("X-TRADIS-Environment", "remote")'
    })

    await expect(scanCommunityBuild(directory)).resolves.toEqual([
      expect.objectContaining({ file: 'assets/app.js', token: '/github-apps' }),
      expect.objectContaining({ file: 'assets/app.js', token: 'X-TRADIS-Environment' })
    ])
  })

  it('allows free AI while rejecting Agent and official version checks', async () => {
    const directory = await createBuildFixture({
      'assets/settings.js': 'post("/ai/test"); post("/ai/compose/generate"); post("/ai/agent/runs"); get("/settings/version-status"); post("/settings/version-check")'
    })

    await expect(scanCommunityBuild(directory)).resolves.toEqual([
      expect.objectContaining({ file: 'assets/settings.js', token: '/ai/agent' }),
      expect.objectContaining({ file: 'assets/settings.js', token: '/settings/version-status' }),
      expect.objectContaining({ file: 'assets/settings.js', token: '/settings/version-check' })
    ])
  })

  it('rejects Full-only settings fields and license purchase copy', async () => {
    const directory = await createBuildFixture({
      'assets/settings.js': 'githubAppSearchToken tutorialCacheLimitMB agentSafetyMode aiAgentPrompt TRADIS-XXXX afdian.com/a/cherry4nas Agent 主模型'
    })

    const violations = await scanCommunityBuild(directory)
    expect(violations.map(({ token }) => token)).toEqual(expect.arrayContaining([
      'githubAppSearchToken', 'tutorialCacheLimitMB',
      'agentSafetyMode', 'aiAgentPrompt', 'TRADIS-XXXX',
      'afdian.com/a/cherry4nas', 'Agent 主模型'
    ]))
  })
})
