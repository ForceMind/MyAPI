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

import { parseReleaseTag } from './semver.mjs'
import { checkWorkflowContracts } from './workflow-contract.mjs'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const workflowPath = path.join(root, '.github/workflows/release.yml')
const workflow = existsSync(workflowPath) ? readFileSync(workflowPath, 'utf8') : ''
const dockerWorkflowPath = path.join(root, '.github/workflows/docker-build.yml')
const dockerWorkflow = existsSync(dockerWorkflowPath) ? readFileSync(dockerWorkflowPath, 'utf8') : ''
const dockerSmokeWorkflowPath = path.join(root, '.github/workflows/docker-smoke.yml')
const dockerSmokeWorkflow = existsSync(dockerSmokeWorkflowPath)
  ? readFileSync(dockerSmokeWorkflowPath, 'utf8')
  : ''
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
const structuredContracts = checkWorkflowContracts({
  release: workflow,
  npm: npmWorkflow,
  docker: dockerWorkflow,
})
const checks = []

function record(name, ok, detail = '') {
  checks.push({ name, ok: Boolean(ok), detail })
}

record('release workflow exists', workflow.length > 0)
record('Docker workflow exists', dockerWorkflow.length > 0)
record('release workflow requires explicit publish gates', workflow.includes('environment: github-release') && workflow.includes("inputs.confirm == 'PUBLISH'") && workflow.includes("vars.MYAPI_ENABLE_RELEASE == 'true'"))
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
  'GitHub prereleases are marked prerelease and never made latest',
  workflow.includes("echo \"prerelease=true\" >> \"$GITHUB_OUTPUT\"") &&
    workflow.includes("prerelease: ${{ needs.prepare.outputs.prerelease == 'true' }}") &&
    workflow.includes("make_latest: ${{ needs.prepare.outputs.prerelease == 'true' && 'false' || 'true' }}"),
)
record(
  'NPM prereleases use beta and stable releases use latest',
  npmWorkflow.includes('echo "tag=beta" >> "$GITHUB_OUTPUT"') &&
    npmWorkflow.includes('echo "tag=latest" >> "$GITHUB_OUTPUT"') &&
    npmWorkflow.includes('npm publish --access public --provenance --tag "${{ steps.dist-tag.outputs.tag }}"'),
)
record(
  'GHCR prereleases do not update stable rolling tags',
  dockerWorkflow.includes('IS_PRERELEASE=false') &&
    dockerWorkflow.includes('promote_latest:') &&
    dockerWorkflow.includes("if: ${{ needs.create_manifests.result == 'success' && !contains(inputs.tag, '-')") &&
    dockerWorkflow.includes('echo "prerelease=${IS_PRERELEASE}" >> $GITHUB_OUTPUT') &&
    dockerWorkflow.includes("if: ${{ !contains(inputs.tag, '-') }}"),
)
record(
  'strict SemVer accepts beta.1 and rejects leading zero identifiers',
  parseReleaseTag('v0.2.0-beta.1') !== null &&
    ['v01.2.3', 'v1.02.3', 'v1.2.03', 'v1.2.3-alpha.01'].every(
      (tag) => parseReleaseTag(tag) === null,
    ),
)
for (const contract of structuredContracts) record(contract.name, contract.ok)
record(
  'GHCR workflow publishes both Full and LAN repositories',
  dockerWorkflow.includes('ghcr.io/forcemind/myapi') &&
    dockerWorkflow.includes('ghcr.io/forcemind/myapi-lan'),
)
record(
  'GHCR release tags reject overwrite',
  [dockerWorkflow, branchDockerWorkflow].every(
    (source) =>
      source.includes('docker buildx imagetools inspect') &&
      source.includes('immutable GHCR tag already exists') &&
      source.includes('timeout 20s') &&
      source.includes('inspect_status') &&
      source.includes('manifest unknown'),
  ),
)
record(
  'CLI and installer pull versioned GHCR images by default',
  cli.includes('ghcr.io/forcemind/myapi:v${packageMetadata.version}') &&
    installer.includes('ghcr.io/forcemind/myapi:v${distribution_version}') &&
    cli.includes("run('docker', composeArguments(paths, ['pull', 'my-api'])") &&
    installer.includes('docker compose --env-file "$env_file" -f "$script_dir/docker-compose.yml" pull my-api'),
)
record('release workflow requires VERSION without a v prefix', workflow.includes('if [[ "$FILE_VERSION" != "$TAG_VERSION" ]]'))
record(
  'release-state validation accepts SemVer prereleases and requires exact tag/version/SHA',
  readFileSync(path.join(root, 'tools/npm/check-release-state.mjs'), 'utf8').includes('is not valid SemVer') &&
    readFileSync(path.join(root, 'tools/npm/check-release-state.mjs'), 'utf8').includes('requested release tag ${requestedTag} does not match ${expectedTag}') &&
    readFileSync(path.join(root, 'tools/npm/check-release-state.mjs'), 'utf8').includes('HEAD must equal the commit tagged ${requestedTag}'),
)
record(
  'Docker workflow writes VERSION without the tag v prefix',
  dockerWorkflow.includes('EXPECTED_VERSION="${TAG#v}"') &&
    dockerWorkflow.includes("printf '%s\\n' \"$EXPECTED_VERSION\" > VERSION"),
)
record(
  'Docker smoke workflow builds locally without registry publishing',
  dockerSmokeWorkflow.includes('load: true') &&
    dockerSmokeWorkflow.includes('push: false') &&
    !dockerSmokeWorkflow.includes('docker/login-action') &&
    dockerSmokeWorkflow.includes('/api/status'),
)
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
  'release finalization is single-writer after all platform artifacts succeed',
  workflow.includes('needs: [prepare, linux, macos, windows]') &&
    workflow.includes('finalize:\n    name: Create MyAPI GitHub Release') &&
    (workflow.match(/softprops\/action-gh-release/g) || []).length === 1 &&
    ['myapi-release-linux', 'myapi-release-macos', 'myapi-release-windows'].every(
      (artifact) => workflow.includes(artifact),
    ),
)
record(
  'stable latest updates are globally serialized and monotonic',
  [workflow, npmWorkflow, dockerWorkflow].every((source) => source.includes('group: myapi-stable-latest-publication')) &&
  [workflow, npmWorkflow, dockerWorkflow].every((source) =>
    source.includes('can-advance-stable-latest'),
  ),
)
record(
  'GHCR stable latest compares the current container version before publication',
  dockerWorkflow.includes('package_type=container&per_page=100') &&
    dockerWorkflow.includes('latest-stable-container-tag') &&
    dockerWorkflow.includes('unsupported GHCR owner type') &&
    dockerWorkflow.includes('refusing to move GHCR latest'),
)
record(
  'GHCR promotes rolling tags once after every manifest succeeds',
  dockerWorkflow.includes('needs: [create_manifests]') &&
    !dockerWorkflow.includes('latest_tag=') &&
    dockerWorkflow.includes('latest-amd64') &&
    dockerWorkflow.includes('latest-arm64'),
)
record(
  'GHCR build identity uses the verified tag commit',
  [
    'ARG MYAPI_BUILD_ID=local',
    'VITE_BUILD_ID="${MYAPI_BUILD_ID}"',
    'MYAPI_BUILD_ID=${{ steps.version.outputs.sha }}',
    'VITE_BUILD_ID=${{ needs.prepare.outputs.sha }}',
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
