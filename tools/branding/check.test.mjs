/*
 * MyAPI brand audit contract tests.
 * Copyright (C) 2026 ForceMind
 */
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { test } from 'node:test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '../..'
)
const checker = path.join(repositoryRoot, 'tools/branding/check.mjs')

function runAudit(...args) {
  return execFileSync(process.execPath, [checker, ...args], {
    cwd: repositoryRoot,
    encoding: 'utf8',
  })
}

test('brand audit passes and emits machine-readable JSON', () => {
  const result = JSON.parse(runAudit('--json'))

  assert.equal(result.command, 'brand:check')
  assert.equal(result.passed, true)
  assert.equal(result.blocking_count, 0)
  assert.ok(result.tracked_files > 100)
  assert.ok(Array.isArray(result.findings))
  assert.ok(
    result.findings.some(
      (finding) =>
        finding.file === 'NOTICE' &&
        finding.category === 'legal' &&
        finding.rule === 'QuantumNous' &&
        finding.blocking === false
    )
  )
})

test('human output exposes only classification metadata', () => {
  const output = runAudit()

  assert.match(output, /Brand audit: PASS \(/)
  assert.doesNotMatch(output, /API_KEY|JWT|SESSION_SECRET|Bearer\s/i)
})
