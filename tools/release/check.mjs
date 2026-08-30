#!/usr/bin/env node

/*
 * Deterministic release workflow contract check.
 * Copyright (C) 2026 ForceMind
 *
 * This check is intentionally dependency-free and never contacts GitHub,
 * Docker, a registry, or a release environment.
 */
import { existsSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const workflowPath = path.join(root, '.github/workflows/release.yml')
const workflow = existsSync(workflowPath) ? readFileSync(workflowPath, 'utf8') : ''
const dockerWorkflowPath = path.join(root, '.github/workflows/docker-build.yml')
const dockerWorkflow = existsSync(dockerWorkflowPath) ? readFileSync(dockerWorkflowPath, 'utf8') : ''
const checks = []

function record(name, ok, detail = '') {
  checks.push({ name, ok: Boolean(ok), detail })
}

record('release workflow exists', workflow.length > 0)
record('Docker workflow exists', dockerWorkflow.length > 0)
record('release workflow requires explicit publish gates', [
  "inputs.confirm == 'PUBLISH'",
  "vars.MYAPI_ENABLE_RELEASE == 'true'",
  'environment: github-release',
].every((fragment) => workflow.includes(fragment)))
record('release workflow protects the legacy v0.1.0 tag', workflow.includes('v0.1.0 is a protected legacy tag'))
record('release workflow requires VERSION without a v prefix', workflow.includes('if [[ "$FILE_VERSION" != "$TAG_VERSION" ]]'))
record('prepare and platform jobs have bounded timeouts', [
  'timeout-minutes: 10',
  'timeout-minutes: 45',
].every((fragment) => workflow.includes(fragment)) && (workflow.match(/timeout-minutes:/g) || []).length >= 4)

const buildLines = workflow
  .split('\n')
  .map((line) => line.trim())
  .filter((line) => line.includes('go build'))
record(
  'all release Go builds use bounded parallelism',
  buildLines.length >= 4 && buildLines.every((line) => line.includes('GOMAXPROCS=2') && line.includes('-p 2')),
)
record(
  'GHCR manifests are assembled from validated immutable digests',
  [
    'Upload immutable image digest',
    'Download immutable image digests',
    'Validate immutable image digests',
    ' =~ ^sha256:[0-9a-f]{64}$',
    'IMAGE_REPOSITORY}@${amd64}',
    'IMAGE_REPOSITORY}@${arm64}',
  ].every((fragment) => dockerWorkflow.includes(fragment)),
)

const failed = checks.filter((check) => !check.ok)
const jsonOutput = process.argv.includes('--json')
if (jsonOutput) {
  console.log(JSON.stringify({ command: 'release:check', passed: failed.length === 0, checks }, null, 2))
} else {
  for (const check of checks) console.log(`${check.ok ? 'PASS' : 'FAIL'}  ${check.name}${check.detail ? ` (${check.detail})` : ''}`)
  console.log(`Release workflow contract: ${failed.length === 0 ? 'PASS' : 'FAIL'} (${checks.length - failed.length}/${checks.length})`)
}
if (failed.length > 0) process.exitCode = 1
