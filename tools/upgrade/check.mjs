#!/usr/bin/env node

/*
 * Deterministic upgrade/recovery contract check.
 *
 * This check is deliberately dependency-free and read-only. It does not
 * invoke Docker, contact GHCR, read deploy/.env, create a backup, or touch a
 * running service. It only verifies that the documented safety gates and the
 * CLI tests remain present in a release source tree.
 */
import { existsSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const checks = []
const jsonOutput = process.argv.includes('--json')

function record(name, ok, detail = '') {
  checks.push({ name, ok: Boolean(ok), detail })
}

function read(relative) {
  const file = path.join(repositoryRoot, relative)
  record(`file: ${relative}`, existsSync(file))
  return existsSync(file) ? readFileSync(file, 'utf8') : ''
}

function includes(text, fragments) {
  return fragments.every((fragment) => text.includes(fragment))
}

const cli = read('cli/myapi.mjs')
const tests = read('cli/test/myapi.test.mjs')
const docs = read('docs/UPGRADE_REHEARSAL.md')
const installer = read('deploy/install.sh')

record(
  'CLI exposes an explicit, read-only dry-run mode',
  includes(cli, ['--dry-run', 'printUpgradeDryRun', 'dockerOperations: []', 'writes: []']),
)
record(
  'dry-run resolves a version-pinned GHCR image by edition',
  includes(cli, ['imageRepositoryForEdition', 'ghcr.io/forcemind/myapi', 'ghcr.io/forcemind/myapi-lan', 'imageSource: \'ghcr-pull\'']),
)
record(
  'upgrade validates runtime resource guardrails before writes',
  includes(cli, ['validateRuntimeConfiguration(targetValues)', 'MYAPI_CPU_LIMIT', 'MYAPI_MEMORY_LIMIT', 'MYAPI_BUILD_PARALLELISM']),
)
record(
  'upgrade verifies an optional signature before changing deployment state',
  includes(cli, ['if (plan.verifySignature) verifyImageSignature(image, values)', 'MYAPI_COSIGN_CERTIFICATE_IDENTITY']),
)
record(
  'upgrade stores a private environment backup',
  includes(cli, ['mkdirSync(backupDir, { recursive: true, mode: 0o700 })', "writeFileSync(backupPath, originalContents, { mode: 0o600 })", 'chmodSync(backupPath, 0o600)']),
)
record(
  'upgrade waits for a healthy target before success',
  includes(cli, ["['up', '-d', '--force-recreate', '--wait', '--wait-timeout', '120']", 'previous image restored and health-checked']),
)
record(
  'upgrade restores the previous environment and reports rollback failure',
  includes(cli, ['writeFileSync(paths.envFile, originalContents, { mode: 0o600 })', 'automatic rollback failed']),
)

record(
  'installer validates compose configuration before pull/build',
  includes(installer, ['docker compose', 'config --quiet', 'MYAPI_CPU_LIMIT', 'MYAPI_MEMORY_LIMIT']),
)
record(
  'local image builds inherit CPU and memory guardrails',
  includes(installer, ['docker build', '--cpus', '--memory', '--memory-swap']),
)
record(
  'installer waits for health and does not remove volumes',
  includes(installer, ['--force-recreate', '--wait', '--wait-timeout', '120']) &&
    !/docker compose[^\n]*down[^\n]*-v/.test(installer),
)

record(
  'upgrade guide separates dry-run from database restore',
  includes(docs, ['--dry-run --json', '不会写入环境文件', '不等同于数据库恢复测试', '副本', '生产操作']),
)
record(
  'upgrade guide documents health, backup, rollback and resource checks',
  includes(docs, ['健康检查', '备份', '回滚', '资源限制', '0600']),
)
record(
  'CLI regression tests cover dry-run, resource rejection and rollback',
  includes(tests, [
    "upgrade dry-run validates a copy without writing files or invoking Docker",
    'upgrade dry-run fails closed on invalid runtime configuration',
    'upgrade restores the environment and reruns the old deployment after a pull failure',
    'up rejects unsafe Docker resource limits before invoking Compose',
  ]),
)

const failed = checks.filter((check) => !check.ok)
if (jsonOutput) {
  console.log(JSON.stringify({ command: 'upgrade:check', passed: failed.length === 0, checks }, null, 2))
} else {
  for (const check of checks) {
    console.log(`${check.ok ? 'PASS' : 'FAIL'}  ${check.name}${check.detail ? ` (${check.detail})` : ''}`)
  }
  console.log(`Upgrade/recovery contract: ${failed.length === 0 ? 'PASS' : 'FAIL'} (${checks.length - failed.length}/${checks.length})`)
}
if (failed.length > 0) process.exitCode = 1
