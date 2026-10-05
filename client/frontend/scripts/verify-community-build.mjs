#!/usr/bin/env node
import path from 'node:path'

import { formatCommunityBuildViolations, scanCommunityBuild } from './community-build-policy.mjs'

const buildDirectory = path.resolve(process.cwd(), process.argv[2] || 'dist')

try {
  const violations = await scanCommunityBuild(buildDirectory)
  if (violations.length > 0) {
    throw new Error(`Community build contains excluded protocols or routes:\n${formatCommunityBuildViolations(violations)}`)
  }
  console.log(`Community build policy passed: ${buildDirectory}`)
} catch (error) {
  console.error(error instanceof Error ? error.message : error)
  process.exitCode = 1
}
