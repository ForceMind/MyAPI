#!/usr/bin/env node

/* Deterministic contract check for the redacted runtime probe. */
import { existsSync, readFileSync } from 'node:fs'
import path from 'node:path'

const root = path.resolve(import.meta.dirname, '../..')
const packageJson = JSON.parse(readFileSync(path.join(root, 'package.json'), 'utf8'))
const sourcePath = path.join(root, 'tools/runtime/auth-probe.mjs')
const testPath = path.join(root, 'tools/runtime/auth-probe.test.mjs')
const docsPath = path.join(root, 'docs/REAL_DEVICE_ACCEPTANCE.md')
const source = existsSync(sourcePath) ? readFileSync(sourcePath, 'utf8') : ''
const tests = existsSync(testPath) ? readFileSync(testPath, 'utf8') : ''
const docs = existsSync(docsPath) ? readFileSync(docsPath, 'utf8') : ''
const checks = []

function record(name, ok, detail = '') {
  checks.push({ name, ok: Boolean(ok), detail })
}

record('probe source exists', existsSync(sourcePath))
record('probe tests exist', existsSync(testPath))
record('probe exports a reusable function', source.includes('export async function runRuntimeProbe'))
record('probe checks server status', source.includes('/api/status'))
record('probe checks authenticated identity', source.includes("'/api/user/self'"))
record('probe checks quota changes', source.includes("'/api/channel/quota/changes?range=24h'"))
record('probe checks administrator logs', source.includes("'/api/log/?p=1&size=10'"))
record('probe has bounded timeout', source.includes('MAX_TIMEOUT_MS') && source.includes('AbortController'))
record('probe does not print credentials or response bodies', source.includes('never\n * prints') && source.includes('await response.arrayBuffer()'))
record('probe tests redaction and fail-closed behavior', tests.includes('without exposing tokens') && tests.includes('credentials are missing') && tests.includes('authentication failure'))
record('package includes runtime tools', Array.isArray(packageJson.files) && packageJson.files.includes('tools/runtime/'))
record('package exposes probe commands', packageJson.scripts?.['runtime:probe'] === 'node tools/runtime/auth-probe.mjs' && packageJson.scripts?.['runtime:probe:test'] === 'node --test tools/runtime/*.test.mjs')
record('acceptance guide documents env-only usage', docs.includes('MYAPI_PROBE_URL') && docs.includes('MYAPI_PROBE_PASSWORD') && docs.includes('不能代替手机浏览器'))

const failed = checks.filter((item) => !item.ok)
if (process.argv.includes('--json')) {
  console.log(JSON.stringify({ command: 'runtime:check', passed: failed.length === 0, checks }, null, 2))
} else {
  for (const item of checks) console.log(`${item.ok ? 'PASS' : 'FAIL'}  ${item.name}${item.detail ? ` (${item.detail})` : ''}`)
  console.log(`Runtime probe contract: ${failed.length === 0 ? 'PASS' : 'FAIL'} (${checks.length - failed.length}/${checks.length})`)
}
if (failed.length > 0) process.exitCode = 1
