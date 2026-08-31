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
const branchDockerWorkflowPath = path.join(root, '.github/workflows/docker-image-branch.yml')
const branchDockerWorkflow = existsSync(branchDockerWorkflowPath)
  ? readFileSync(branchDockerWorkflowPath, 'utf8')
  : ''
const dockerfilePath = path.join(root, 'Dockerfile')
const dockerfile = existsSync(dockerfilePath) ? readFileSync(dockerfilePath, 'utf8') : ''
const electronWorkflowPath = path.join(root, '.github/workflows/electron-build.yml')
const electronWorkflow = existsSync(electronWorkflowPath)
  ? readFileSync(electronWorkflowPath, 'utf8')
  : ''
const npmWorkflowPath = path.join(root, '.github/workflows/npm-publish.yml')
const npmWorkflow = existsSync(npmWorkflowPath)
  ? readFileSync(npmWorkflowPath, 'utf8')
  : ''
const cliPath = path.join(root, 'cli/myapi.mjs')
const cli = existsSync(cliPath) ? readFileSync(cliPath, 'utf8') : ''
const installerPath = path.join(root, 'deploy/install.sh')
const installer = existsSync(installerPath) ? readFileSync(installerPath, 'utf8') : ''
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
record(
  'release workflow protects existing legacy tags',
  /\$RELEASE_TAG.*v0\.1\.0.*v0\.1\.1/.test(workflow) && workflow.includes('protected legacy tag'),
)
record(
  'Docker workflow protects existing legacy tags',
  /\$TAG.*v0\.1\.0.*v0\.1\.1/.test(dockerWorkflow) && dockerWorkflow.includes('protected legacy tag'),
)
record(
  'NPM workflow protects existing legacy tags',
  /\$RELEASE_TAG.*v0\.1\.0.*v0\.1\.1/.test(npmWorkflow) && npmWorkflow.includes('protected legacy tag'),
)
record(
  'GHCR workflow automatically runs for semantic version tags',
  /push:\s*\n\s*tags:\s*\n\s*- ['"]v\*\.\*\.\*['"]/.test(dockerWorkflow),
)
record(
  'GHCR workflow publishes both Full and LAN repositories',
  dockerWorkflow.includes('ghcr.io/forcemind/myapi') &&
    dockerWorkflow.includes('ghcr.io/forcemind/myapi-lan'),
)
record(
  'CLI and installer pull versioned GHCR images by default',
  cli.includes('ghcr.io/forcemind/myapi:v${packageMetadata.version}') &&
    installer.includes('ghcr.io/forcemind/myapi:v${distribution_version}') &&
    cli.includes("run('docker', composeArguments(paths, ['pull', 'my-api'])") &&
    installer.includes('docker compose --env-file "$env_file" -f "$script_dir/docker-compose.yml" pull my-api'),
)
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
record(
  'manual branch GHCR manifests are assembled from validated immutable digests',
  [
    'Upload immutable image digest',
    'Download immutable image digests',
    'Validate immutable image digests',
    ' =~ ^sha256:[0-9a-f]{64}$',
    'IMAGE_REPOSITORY}@${amd64}',
    'IMAGE_REPOSITORY}@${arm64}',
  ].every((fragment) => branchDockerWorkflow.includes(fragment)),
)
record(
  'frontend build identity is injected from an immutable commit',
  [
    'ARG MYAPI_BUILD_ID=local',
    'VITE_BUILD_ID="${MYAPI_BUILD_ID}"',
    'MYAPI_BUILD_ID=${{ github.sha }}',
    'VITE_BUILD_ID=${{ needs.prepare.outputs.sha }}',
    'VITE_BUILD_ID=${{ github.sha }}',
  ].every(
    (fragment) =>
      dockerfile.includes(fragment) ||
      dockerWorkflow.includes(fragment) ||
      workflow.includes(fragment) ||
      electronWorkflow.includes(fragment),
  ),
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
