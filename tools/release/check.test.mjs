/*
 * Release workflow contract regression tests.
 * Copyright (C) 2026 ForceMind
 */
import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import test from 'node:test'

test('release workflow protects prerelease publication channels', () => {
  const result = spawnSync(process.execPath, ['tools/release/check.mjs', '--json'], {
    encoding: 'utf8',
  })

  assert.equal(result.status, 0, result.stderr || result.stdout)
  const report = JSON.parse(result.stdout)
  assert.equal(report.passed, true)
  for (const name of [
    'GitHub prereleases are marked prerelease and never made latest',
    'NPM prereleases use beta and stable releases use latest',
    'GHCR prereleases do not update stable rolling tags',
    'strict SemVer accepts beta.1 and rejects leading zero identifiers',
    'GHCR publisher has explicit environment and release gate',
    'release finalization is single-writer after all platform artifacts succeed',
    'stable latest updates are globally serialized and monotonic',
    'GHCR stable latest compares the current container version before publication',
    'GHCR promotes rolling tags once after every manifest succeeds',
    'GHCR build identity uses the verified tag commit',
    'release-state validation accepts SemVer prereleases and requires exact tag/version/SHA',
  ]) {
    assert.equal(report.checks.find((check) => check.name === name)?.ok, true)
  }
})
