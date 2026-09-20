import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import test from 'node:test'

import {
  canAdvanceStableLatest,
  compareSemVer,
  latestStableTag,
  flattenPages,
  stableLatestAction,
  parseReleaseTag,
  parseSemVer,
} from './semver.mjs'

test('strict SemVer accepts beta.1 and rejects leading zero components', () => {
  assert.deepEqual(parseReleaseTag('v0.2.0-beta.1')?.prerelease, ['beta', '1'])
  for (const value of ['01.2.3', '1.02.3', '1.2.03', '1.2.3-alpha.01']) {
    assert.equal(parseSemVer(value), null)
  }
})

test('stable latest only advances to a newer stable version', () => {
  assert.equal(canAdvanceStableLatest('0.2.0', '0.1.9'), true)
  assert.equal(canAdvanceStableLatest('0.1.9', '0.2.0'), false)
  assert.equal(canAdvanceStableLatest('0.2.0-beta.1', '0.1.9'), false)
  assert.equal(compareSemVer('0.2.0-beta.1', '0.2.0-beta.2'), -1)
  assert.equal(latestStableTag(['v0.2.0-beta.1', 'v0.1.9', 'v0.2.0']), 'v0.2.0')
  assert.deepEqual(flattenPages([[{ tag_name: 'v0.1.0' }], [{ tag_name: 'v0.2.0' }]]).map((item) => item.tag_name), ['v0.1.0', 'v0.2.0'])
  assert.equal(
    compareSemVer('9007199254740993.0.0', '9007199254740992.0.0'),
    1
  )
  assert.equal(stableLatestAction('0.2.0', '0.2.0'), 'idempotent')
  assert.equal(stableLatestAction('0.2.0', '0.3.0'), 'reject')
})

test('workflow CLI validates tags and latest advancement with the same rules', () => {
  const run = (...args) =>
    spawnSync(process.execPath, ['tools/release/semver.mjs', ...args], {
      encoding: 'utf8',
    }).status

  assert.equal(run('validate-tag', 'v0.2.0-beta.1'), 0)
  assert.equal(run('validate-tag', 'v1.2.3-alpha.01'), 1)
  assert.equal(run('can-advance-stable-latest', '0.2.0', '0.1.9'), 0)
  assert.equal(run('can-advance-stable-latest', '0.1.9', '0.2.0'), 1)
})
