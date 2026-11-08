import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { chmod, chown, lstat, mkdir, mkdtemp, readFile, readdir, readlink, rm, symlink, writeFile } from 'node:fs/promises'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'
import { BackupEngine } from './backup-engine.mjs'
import { backupPrivateDirectory, loadBackupConfig, loadConfig } from './config.mjs'

assert.equal(process.platform, 'linux', 'this contract must run in the explicit Linux root CI step')
assert.equal(process.arch, 'x64')
assert.equal(process.getuid(), 0, 'do not replace the root contract with a skipped nonroot run')
assert.ok(Number(process.versions.node.split('.')[0]) >= 22)

const repository = fileURLToPath(new URL('../../', import.meta.url))
const cli = path.join(repository, 'cli/myapi.mjs')
const lockHelper = fileURLToPath(new URL('./lock_guard.py', import.meta.url))
const secret = 'SYNTHETIC_ROOT_CONFIG_SECRET_DO_NOT_PRINT'

async function snapshot(root, relative = '.') {
  const absolute = relative === '.' ? root : path.join(root, relative)
  const info = await lstat(absolute)
  if (info.isSymbolicLink()) return [{ path: relative, type: 'symlink', target: await readlink(absolute) }]
  const result = [{ path: relative, uid: info.uid, gid: info.gid, mode: info.mode & 0o7777,
    type: info.isDirectory() ? 'directory' : 'file', hash: info.isDirectory() ? null : createHash('sha256').update(await readFile(absolute)).digest('hex') }]
  if (info.isDirectory()) {
    for (const name of (await readdir(absolute)).sort()) result.push(...await snapshot(root, relative === '.' ? name : path.join(relative, name)))
  }
  return result
}

async function fixture(t) {
  // /tmp's root-owned sticky ancestor is intentionally exercised. Every
  // descendant created or removed by this file belongs to this fresh root.
  const root = await mkdtemp('/tmp/myapi-backup-cli-root-')
  await chmod(root, 0o700)
  t.after(() => rm(root, { recursive: true, force: true }))
  for (const name of ['data', 'state', 'backups', 'restores', 'bridge', 'external']) await mkdir(path.join(root, name), { mode: 0o700 })
  const database = path.join(root, 'data/my-api.db')
  await writeFile(database, `synthetic-existing-accounting:${secret}`, { mode: 0o600 })
  const storage = { docker_socket: '/var/run/docker.sock', state_dir: path.join(root, 'state'), backups_dir: path.join(root, 'backups'),
    instances: [{ id: 'primary', container: 'myapi-b4-root-fixture', data_dir: path.join(root, 'data'), sqlite_file: database, health_url: 'http://127.0.0.1:18080/' }] }
  const daemon = { ...storage,
    listen: { host: '127.0.0.1', port: 9443, tls_cert_file: path.join(root, `missing-cert-${secret}`), tls_key_file: path.join(root, `missing-key-${secret}`) },
    unix_socket: path.join(root, 'bridge/operations.sock'), bridge_token_file: path.join(root, `missing-bridge-${secret}`), github_token_file: path.join(root, `missing-github-${secret}`) }
  const config = path.join(root, 'operations.json')
  await writeFile(config, JSON.stringify(storage), { mode: 0o600 })
  return { root, storage, daemon, config, database,
    async write(value) { await writeFile(config, JSON.stringify(value), { mode: 0o600 }); return config },
  }
}

function targetOnlyEngine(config) {
  const attempted = []
  const forbidden = new Proxy({}, { get(_object, method) { attempted.push(String(method)); assert.fail('target-only validation must not touch Docker') } })
  const engine = new BackupEngine(config, { store: { plans: new Map(), current: new Map(), frozen: new Set() }, docker: forbidden,
    bundle: () => assert.fail('target-only validation must not copy data'), writer: () => assert.fail('target-only validation must not scan or control processes') })
  return { engine, attempted }
}

test('the four storage keys load without daemon credentials while the daemon loader retains its full contract', async t => {
  const local = await fixture(t)
  const before = await snapshot(local.root)
  assert.deepEqual(await loadBackupConfig(local.config), local.storage)
  await assert.rejects(loadConfig(local.config), { code: 'INVALID_CONFIG' })
  assert.deepEqual(await snapshot(local.root), before, 'configuration validation must not create a listener, journal or credential')
  await local.write(local.daemon)
  const fullBefore = await snapshot(local.root)
  assert.deepEqual(await loadBackupConfig(local.config), local.storage, 'the same complete daemon schema projects only storage')
  for (const filename of [local.daemon.bridge_token_file, local.daemon.github_token_file, local.daemon.listen.tls_cert_file, local.daemon.listen.tls_key_file]) {
    await assert.rejects(lstat(filename), { code: 'ENOENT' })
  }
  await assert.rejects(loadConfig(local.config), { code: 'ENOENT' }, 'daemon loading still requires its protected credentials')
  assert.deepEqual(await snapshot(local.root), fullBefore)
})

test('root-private leaves reject nonroot or writable nonsticky ancestors without repairing permissions', async t => {
  const local = await fixture(t)
  await backupPrivateDirectory(path.join(local.root, 'restores'))
  for (const kind of ['nonroot-owner', 'writable-no-sticky']) {
    const ancestor = path.join(local.root, kind), leaf = path.join(ancestor, 'private')
    await mkdir(ancestor, { mode: 0o755 })
    await mkdir(leaf, { mode: 0o700 })
    if (kind === 'nonroot-owner') await chown(ancestor, 12345, 12345)
    else await chmod(ancestor, 0o777)
    const before = await snapshot(ancestor)
    await assert.rejects(backupPrivateDirectory(leaf), { code: 'UNSAFE_BACKUP_ANCESTOR' })
    await local.write({ ...local.storage, state_dir: leaf })
    await assert.rejects(loadBackupConfig(local.config), { code: 'UNSAFE_BACKUP_ANCESTOR' })
    assert.deepEqual(await snapshot(ancestor), before)
  }
})

test('storage registration rejects both directions of data, state and backup overlap', async t => {
  const local = await fixture(t)
  const nestedState = path.join(local.storage.state_dir, 'nested-data')
  const nestedBackup = path.join(local.storage.backups_dir, 'nested-data')
  const stateInData = path.join(local.storage.instances[0].data_dir, 'nested-state')
  const backupInData = path.join(local.storage.instances[0].data_dir, 'nested-backups')
  for (const directory of [nestedState, nestedBackup, stateInData, backupInData]) {
    await mkdir(directory, { mode: 0o700 })
    await writeFile(path.join(directory, 'my-api.db'), 'synthetic-existing-state', { mode: 0o600 })
  }
  const instanceAt = data => ({ ...local.storage.instances[0], data_dir: data, sqlite_file: path.join(data, 'my-api.db') })
  const invalid = [
    { ...local.storage, backups_dir: local.storage.state_dir },
    { ...local.storage, backups_dir: nestedState },
    { ...local.storage, state_dir: nestedBackup },
    { ...local.storage, instances: [instanceAt(nestedState)] },
    { ...local.storage, instances: [instanceAt(nestedBackup)] },
    { ...local.storage, state_dir: stateInData },
    { ...local.storage, backups_dir: backupInData },
  ]
  for (const value of invalid) {
    await local.write(value)
    const before = await snapshot(local.root)
    await assert.rejects(loadBackupConfig(local.config), error => ['INVALID_CONFIG', 'DATA_OVERLAP'].includes(error.code))
    assert.deepEqual(await snapshot(local.root), before)
  }
})

test('target-only validation accepts a new isolated path and rejects existing, overlapping and nonprivate targets', async t => {
  const local = await fixture(t)
  const { engine, attempted } = targetOnlyEngine(local.storage)
  const target = path.join(local.root, 'restores/new-data')
  await engine.assertRestoreTarget(target)
  await assert.rejects(lstat(target), { code: 'ENOENT' })
  const existing = path.join(local.root, 'restores/existing')
  await mkdir(existing, { mode: 0o700 })
  await writeFile(path.join(existing, 'new-accounting'), secret, { mode: 0o600 })
  const before = await snapshot(existing)
  await assert.rejects(engine.assertRestoreTarget(existing), { code: 'RESTORE_TARGET_EXISTS' })
  assert.deepEqual(await snapshot(existing), before)
  const existingFile = path.join(local.root, 'restores/existing.db')
  await writeFile(existingFile, secret, { mode: 0o600 })
  await assert.rejects(engine.assertRestoreTarget(existingFile), { code: 'RESTORE_TARGET_EXISTS' })
  assert.equal(await readFile(existingFile, 'utf8'), secret)
  for (const value of [local.storage.state_dir, local.storage.backups_dir, local.storage.instances[0].data_dir]) {
    await assert.rejects(engine.assertRestoreTarget(value), { code: 'RESTORE_TARGET_OVERLAP' })
    await assert.rejects(engine.assertRestoreTarget(path.join(value, 'new')), { code: 'RESTORE_TARGET_OVERLAP' })
  }
  await assert.rejects(engine.assertRestoreTarget(local.root), { code: 'UNSAFE_DIRECTORY' }, 'a sticky /tmp parent is not itself a private restore parent')
  const registered = path.join(local.root, 'registered')
  await mkdir(registered, { mode: 0o700 })
  for (const name of ['data', 'state', 'backups']) await mkdir(path.join(registered, name), { mode: 0o700 })
  await writeFile(path.join(registered, 'data/my-api.db'), 'synthetic-existing-state', { mode: 0o600 })
  const nested = { ...local.storage, state_dir: path.join(registered, 'state'), backups_dir: path.join(registered, 'backups'),
    instances: [{ ...local.storage.instances[0], data_dir: path.join(registered, 'data'), sqlite_file: path.join(registered, 'data/my-api.db') }] }
  await local.write(nested)
  const ancestor = targetOnlyEngine(await loadBackupConfig(local.config))
  await assert.rejects(ancestor.engine.assertRestoreTarget(registered), { code: 'RESTORE_TARGET_OVERLAP' }, 'a target above registered storage must be refused even with a private parent')
  assert.deepEqual(ancestor.attempted, [])
  const publicParent = path.join(local.root, 'not-private')
  await mkdir(publicParent, { mode: 0o755 })
  await assert.rejects(engine.assertRestoreTarget(path.join(publicParent, 'new')), { code: 'UNSAFE_DIRECTORY' })
  await assert.rejects(engine.assertRestoreTarget('relative'), { code: 'UNSAFE_RESTORE_TARGET' })
  assert.deepEqual(attempted, [], 'this is filesystem eligibility, never simulated Docker qualification')
})

test('external symlinks cannot replace a config, registered database or target parent', async t => {
  const local = await fixture(t)
  const outside = path.join(local.root, 'external')
  const marker = path.join(outside, 'secret.db')
  await writeFile(marker, secret, { mode: 0o600 })
  const before = await snapshot(outside)
  const configLink = path.join(local.root, 'config-link.json')
  await symlink(local.config, configLink)
  await assert.rejects(loadBackupConfig(configLink), { code: 'INVALID_CONFIG' })
  const databaseLink = path.join(local.root, 'data/linked.db')
  await symlink(marker, databaseLink)
  await local.write({ ...local.storage, instances: [{ ...local.storage.instances[0], sqlite_file: databaseLink }] })
  await assert.rejects(loadBackupConfig(local.config), { code: 'UNSAFE_PATH' })
  const parentLink = path.join(local.root, 'target-parent-link')
  await symlink(outside, parentLink)
  const { engine, attempted } = targetOnlyEngine(local.storage)
  await assert.rejects(engine.assertRestoreTarget(path.join(parentLink, 'new')), { code: 'UNSAFE_PATH' })
  const targetLink = path.join(local.root, 'restores/linked-target')
  await symlink(outside, targetLink)
  await assert.rejects(engine.assertRestoreTarget(targetLink), { code: 'RESTORE_TARGET_EXISTS' })
  assert.deepEqual(await snapshot(outside), before)
  assert.deepEqual(attempted, [])
})

test('real CLI rejects argument, confirmation and instance errors before Docker or copier calls without exposing configuration', async t => {
  const local = await fixture(t)
  await local.write(local.daemon)
  const log = path.join(local.root, 'forbidden-io.log'), wrapper = path.join(local.root, 'cli-boundary.mjs')
  await writeFile(log, '', { mode: 0o600 })
  // Deny actual API/process boundaries, not a fake Docker success. Only the
  // existing real daemon flock helper may execute for the rejection cases.
  await writeFile(wrapper, `import http from 'node:http'
import https from 'node:https'
import childProcess from 'node:child_process'
import { appendFileSync } from 'node:fs'
import { syncBuiltinESMExports } from 'node:module'
import { pathToFileURL } from 'node:url'
const block = () => { appendFileSync(process.env.ROOT_CONTRACT_IO_LOG, 'forbidden-io\\n'); throw new Error('ROOT_CONTRACT_FORBIDDEN_IO') }
http.request = block; http.get = block; https.request = block; https.get = block; globalThis.fetch = block
const spawn = childProcess.spawn
childProcess.spawn = (command, args, options) => command === '/usr/bin/python3' && args.length === 2 && args[0] === '-I' && args[1] === process.env.ROOT_CONTRACT_LOCK_HELPER ? spawn(command, args, options) : block()
for (const name of ['spawnSync','exec','execSync','execFile','execFileSync','fork']) childProcess[name] = block
syncBuiltinESMExports()
const [, , filename, ...args] = process.argv
process.argv = [process.execPath, filename, ...args]
await import(pathToFileURL(filename).href)
`, { mode: 0o600 })
  const base = ['--config', local.config, '--instance', 'primary']
  const restore = ['--backup', 'a'.repeat(64), '--target-dir', path.join(local.root, 'restores/proposed')]
  const cases = [
    { args: ['backup'], error: /--config is required/ },
    { args: ['backup', '--config', local.config], error: /--instance is required/ },
    { args: ['restore', ...base], error: /--backup is required/ },
    { args: ['restore', ...base, '--backup', 'a'.repeat(64)], error: /--target-dir is required/ },
    { args: ['backup', ...base, '--unknown'], error: /unknown option --unknown/ },
    { args: ['restore', ...base, ...restore, '--unknown'], error: /unknown option --unknown/ },
    { args: ['backup', ...base, '--confirm', 'invalid'], error: /PLAN_CONFIRMATION_INVALID/ },
    { args: ['restore', ...base, ...restore, '--confirm', 'invalid'], error: /PLAN_CONFIRMATION_INVALID/ },
    { args: ['backup', ...base, '--confirm', `${'1'.repeat(64)}:${'2'.repeat(64)}`], error: /PLAN_NOT_FOUND/ },
    { args: ['backup', '--config', local.config, '--instance', 'unknown', '--json'], error: /INSTANCE_NOT_FOUND/ },
    { args: ['restore', '--config', local.config, '--instance', 'unknown', ...restore, '--json'], error: /INSTANCE_NOT_FOUND/ },
  ]
  const sourceBefore = await snapshot(local.storage.instances[0].data_dir)
  const backupsBefore = await snapshot(local.storage.backups_dir)
  for (const entry of cases) {
    const result = spawnSync(process.execPath, [wrapper, cli, ...entry.args], { cwd: local.root,
      env: { ...process.env, ROOT_CONTRACT_IO_LOG: log, ROOT_CONTRACT_LOCK_HELPER: lockHelper },
      encoding: 'utf8', timeout: 15000, maxBuffer: 1024 * 1024 })
    assert.equal(result.error, undefined)
    assert.equal(result.signal, null)
    assert.equal(result.status, 1)
    assert.match(result.stderr, entry.error)
    assert.equal(`${result.stdout}${result.stderr}`.includes(secret), false)
    assert.equal(await readFile(log, 'utf8'), '', 'no Docker/API or backup/writer subprocess may be attempted')
    assert.deepEqual(await snapshot(local.storage.instances[0].data_dir), sourceBefore)
    assert.deepEqual(await snapshot(local.storage.backups_dir), backupsBefore)
  }
  await assert.rejects(lstat(path.join(local.root, 'restores/proposed')), { code: 'ENOENT' })
})
