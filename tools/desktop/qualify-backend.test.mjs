import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { mkdtemp, readFile, readdir, rm, symlink, writeFile } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'
import { binaryDigest, childEnvironment, createOwnedFixture, qualificationScope, removeOwnedFixture } from './qualify-backend.mjs'

const env = {
  MYAPI_DESKTOP_BACKEND_QUALIFICATION: '1', GITHUB_ACTIONS: 'true', GITHUB_EVENT_NAME: 'workflow_dispatch',
  GITHUB_REF: 'refs/heads/dev', GITHUB_REPOSITORY: 'ForceMind/MyAPI', GITHUB_SHA: '1'.repeat(40),
  GITHUB_RUN_ID: '123', GITHUB_RUN_ATTEMPT: '1', MYAPI_DESKTOP_BINARY_SHA256: '2'.repeat(64), RUNNER_TEMP: os.tmpdir(),
}

test('qualification accepts only the two native targets and exact manual dev identity without caller arguments', () => {
  for (const [platform, arch] of [['darwin', 'arm64'], ['win32', 'x64']]) {
    const result = qualificationScope(env, [], platform, arch)
    assert.equal(result.source_sha, env.GITHUB_SHA)
    assert.equal(result.binary_sha256, env.MYAPI_DESKTOP_BINARY_SHA256)
    assert.equal(result.platform, platform)
    assert.equal(result.architecture, arch)
  }
  const rejected = [
    { GITHUB_ACTIONS: 'false' }, { GITHUB_EVENT_NAME: 'push' }, { GITHUB_REF: 'refs/heads/main' },
    { GITHUB_REPOSITORY: 'unrelated/project' }, { GITHUB_SHA: 'short' }, { GITHUB_RUN_ID: '0' },
    { GITHUB_RUN_ATTEMPT: '-1' }, { MYAPI_DESKTOP_BACKEND_QUALIFICATION: '0' },
    { MYAPI_DESKTOP_BINARY_SHA256: '' }, { RUNNER_TEMP: 'relative' },
  ]
  for (const change of rejected) assert.throws(() => qualificationScope({ ...env, ...change }, [], 'darwin', 'arm64'), /^Error: DESKTOP_(?:SCOPE|IDENTITY)_REJECTED$/)
  assert.throws(() => qualificationScope(env, ['--binary', '/somewhere'], 'darwin', 'arm64'), /DESKTOP_SCOPE_REJECTED/)
  for (const [platform, arch] of [['linux', 'x64'], ['darwin', 'x64'], ['win32', 'arm64']]) {
    assert.throws(() => qualificationScope(env, [], platform, arch), /DESKTOP_PLATFORM_REJECTED/)
  }
})

test('the child environment does not inherit provider credentials, existing data locations or development mode', () => {
  const selected = childEnvironment({ PATH: '/tool-path', SystemRoot: 'C:\\Windows', HOME: '/personal',
    SESSION_SECRET: 'private', SQL_DSN: 'private-database', SQLITE_PATH: '/existing.db',
    MYAPI_BIND_ADDRESS: '0.0.0.0', MYAPI_EDITION: 'full', NODE_ENV: 'development', HTTP_PROXY: 'private-proxy',
    OPENAI_API_KEY: 'private-key', AWS_SHARED_CREDENTIALS_FILE: '/private-credentials', NODE_OPTIONS: '--inspect' })
  assert.deepEqual(selected, { PATH: '/tool-path', SystemRoot: 'C:\\Windows' })
})

test('a bound binary is read only and requires its exact hash and a regular file', async t => {
  const root = await mkdtemp(path.join(os.tmpdir(), 'desktop-binary-contract-'))
  t.after(() => rm(root, { recursive: true, force: true }))
  const filename = path.join(root, 'synthetic executable 非ASCII')
  const bytes = Buffer.from('synthetic data; this contract never executes it')
  const expected = createHash('sha256').update(bytes).digest('hex')
  await writeFile(filename, bytes)
  assert.equal(await binaryDigest(filename, expected), expected)
  assert.deepEqual(await readFile(filename), bytes)
  await assert.rejects(binaryDigest(filename, '0'.repeat(64)), /DESKTOP_BINARY_CHANGED/)
  await assert.rejects(binaryDigest(root, expected), /DESKTOP_BINARY_TYPE_REJECTED/)
  await assert.rejects(binaryDigest(path.join(root, 'missing'), expected), { code: 'ENOENT' })
  // Windows junctions are directory links and do not require developer-mode
  // permission to create a symbolic file link; neither is a regular binary.
  const alias = path.join(root, 'alias')
  await symlink(process.platform === 'win32' ? root : filename, alias, process.platform === 'win32' ? 'junction' : 'file')
  await assert.rejects(binaryDigest(alias, expected), /DESKTOP_BINARY_TYPE_REJECTED/)
})

test('fixture cleanup requires its exact private ownership record and never removes a neighbouring directory', async t => {
  const parent = await mkdtemp(path.join(os.tmpdir(), 'desktop-owned-contract-'))
  t.after(() => rm(parent, { recursive: true, force: true }))
  const neighbour = path.join(parent, 'keep-me')
  await writeFile(neighbour, 'neighbour')
  const scope = qualificationScope({ ...env, RUNNER_TEMP: parent }, [], 'darwin', 'arm64')
  const fixture = await createOwnedFixture(parent, scope)
  const marker = path.join(fixture.root, 'ownership.json')
  const original = await readFile(marker)
  await writeFile(marker, JSON.stringify({ ...fixture.ownership, run_id: '456' }))
  await assert.rejects(removeOwnedFixture(fixture), /DESKTOP_CLEANUP_OWNERSHIP_UNKNOWN/)
  assert.deepEqual(await readFile(neighbour, 'utf8'), 'neighbour')
  await writeFile(marker, original)
  await removeOwnedFixture(fixture)
  assert.deepEqual(await readdir(parent), ['keep-me'])
})
