import assert from 'node:assert/strict'
import { spawn, execFile } from 'node:child_process'
import { createHash } from 'node:crypto'
import { chmod, link, lstat, mkdir, mkdtemp, readFile, readdir, readlink, rm, symlink, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { createInterface } from 'node:readline'
import test from 'node:test'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'

assert.equal(process.platform, 'linux')
assert.equal(process.arch, 'x64')
assert.equal(process.getuid(), 0, 'prune contracts require the explicit sudo root Actions step, never skip')

const execute = promisify(execFile)
const helper = fileURLToPath(new URL('./backup_data.py', import.meta.url))
const secret = 'SYNTHETIC_PRUNE_SECRET_MUST_NOT_APPEAR'
const id = '1'.repeat(64)
const otherId = '2'.repeat(64)
const hash = value => createHash('sha256').update(value).digest('hex')

async function snapshot(root, relative = '.') {
  const absolute = relative === '.' ? root : path.join(root, relative)
  const info = await lstat(absolute)
  if (info.isSymbolicLink()) return [{ path: relative, type: 'symlink', target: await readlink(absolute) }]
  const rows = [{ path: relative, type: info.isDirectory() ? 'directory' : 'file', uid: info.uid, gid: info.gid,
    mode: info.mode & 0o7777, size: info.isDirectory() ? null : info.size, sha256: info.isDirectory() ? null : hash(await readFile(absolute)) }]
  if (info.isDirectory()) for (const name of (await readdir(absolute)).sort()) rows.push(...await snapshot(root, relative === '.' ? name : path.join(relative, name)))
  return rows
}

async function call(input, driver, extraArgs = []) {
  const args = driver ? ['-I', '-c', driver, helper, JSON.stringify(input), ...extraArgs] : ['-I', helper]
  const child = spawn('/usr/bin/python3', args, { stdio: ['pipe', 'pipe', 'pipe'] })
  let stdout = '', stderr = ''
  child.stdout.on('data', value => { stdout += value })
  child.stderr.on('data', value => { stderr += value })
  const closed = new Promise((resolve, reject) => {
    const timeout = setTimeout(() => { child.kill('SIGKILL'); reject(new Error('prune helper timeout')) }, 60000)
    child.once('error', error => { clearTimeout(timeout); reject(error) })
    child.once('close', status => { clearTimeout(timeout); resolve(status) })
  })
  child.stdin.end(driver ? '' : JSON.stringify(input))
  const status = await closed
  assert.equal(stderr, '', 'no traceback or private source information may be printed')
  assert.equal(stdout.includes(secret), false)
  assert.equal(stdout.includes(input.data_dir), false)
  return { status, value: stdout ? JSON.parse(stdout) : null }
}

async function rootIdentity(directory) {
  const info = await lstat(directory, { bigint: true })
  return { dev: String(info.dev), ino: String(info.ino), uid: Number(info.uid), gid: Number(info.gid), mode: Number(info.mode & 0o7777n).toString(8).padStart(4, '0') }
}

async function fixture(t) {
  const root = await mkdtemp('/tmp/myapi-prune-root-')
  await chmod(root, 0o700)
  t.after(() => rm(root, { recursive: true, force: true }))
  for (const name of ['data', 'backups', 'state']) await mkdir(path.join(root, name), { mode: 0o700 })
  const data = path.join(root, 'data'), database = path.join(data, 'my-api.db')
  await writeFile(path.join(data, 'private-config'), secret, { mode: 0o600 })
  await writeFile(path.join(root, 'state/existing-journal'), 'existing operator state', { mode: 0o600 })
  await execute('/usr/bin/python3', ['-I', '-c', 'import sqlite3,sys;db=sqlite3.connect(sys.argv[1]);db.execute("CREATE TABLE ledger(id INTEGER PRIMARY KEY,value TEXT)");db.execute("INSERT INTO ledger VALUES(1,\"old committed state\")");db.commit();db.close()', database])
  const common = { data_dir: data, sqlite_file: database, backups_dir: path.join(root, 'backups'), state_dir: path.join(root, 'state'),
    bundle_id: id, identity_sha256: 'a'.repeat(64), max_bytes: 16 * 1024 * 1024, max_entries: 10000 }
  async function backup(bundleId) {
    const request = { ...common, bundle_id: bundleId }
    const prepared = await call({ action: 'bundle_prepare', ...request })
    assert.equal(prepared.status, 0)
    const committed = await call({ action: 'bundle_commit', ...request, expected_manifest_sha256: prepared.value.manifest_sha256, expected_tree_sha256: prepared.value.tree_sha256 })
    assert.equal(committed.status, 0)
    return { request, receipt: committed.value, directory: path.join(common.backups_dir, bundleId) }
  }
  const target = await backup(id)
  return { root, data, database, common, target, backup,
    async request(extra = {}) { return { action: 'bundle_prune', ...target.request, expected_manifest_sha256: target.receipt.manifest_sha256,
      expected_root: await rootIdentity(target.directory), ...extra } },
  }
}

async function consumer(t, directory, kind) {
  const filename = path.join(directory, 'data/my-api.db')
  const program = `import ctypes,os,sys
root,filename,kind=sys.argv[1:]
descriptor=None;address=None;length=None;lib=None
if kind=='ro-fd':descriptor=os.open(filename,os.O_RDONLY)
elif kind=='directory-fd':descriptor=os.open(root,os.O_RDONLY|os.O_DIRECTORY)
elif kind=='cwd':os.chdir(root)
elif kind=='root':os.chroot(root);os.chdir('/')
elif kind=='private-mmap':
 descriptor=os.open(filename,os.O_RDONLY);length=os.fstat(descriptor).st_size
 lib=ctypes.CDLL(None,use_errno=True);lib.mmap.restype=ctypes.c_void_p
 lib.mmap.argtypes=[ctypes.c_void_p,ctypes.c_size_t,ctypes.c_int,ctypes.c_int,ctypes.c_int,ctypes.c_long]
 address=lib.mmap(None,length,1,2,descriptor,0)
 if address==ctypes.c_void_p(-1).value:raise OSError('fixture mmap failed')
 os.close(descriptor);descriptor=None
else:raise Exception('unknown consumer')
print('ready',flush=True)
sys.stdin.read()
if descriptor is not None:os.close(descriptor)
if address is not None:
 lib.munmap.argtypes=[ctypes.c_void_p,ctypes.c_size_t];lib.munmap(address,length)
`
  const child = spawn('/usr/bin/python3', ['-I', '-u', '-c', program, directory, filename, kind], { stdio: ['pipe', 'pipe', 'pipe'] })
  child.stderr.resume()
  const lines = createInterface({ input: child.stdout })
  const exited = new Promise(resolve => child.once('close', resolve))
  t.after(async () => { child.stdin.end(); const timer = setTimeout(() => child.kill('SIGKILL'), 5000); await exited; clearTimeout(timer); lines.close() })
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('consumer handshake timeout')), 10000)
    lines.once('line', value => { clearTimeout(timer); assert.equal(value, 'ready'); resolve() })
    child.once('error', reject)
  })
  if (kind === 'private-mmap') {
    const descriptors = await readdir(`/proc/${child.pid}/fd`)
    for (const descriptor of descriptors) assert.notEqual(await readlink(`/proc/${child.pid}/fd/${descriptor}`), filename, 'mapping coverage must not be satisfied by a leftover archive file descriptor')
  }
  return child
}

test('one verified unused bundle is deleted while source, other backup, unknown directories and state stay unchanged', async t => {
  const local = await fixture(t)
  const other = await local.backup(otherId)
  const unknown = path.join(local.common.backups_dir, 'unknown-legacy')
  await mkdir(unknown, { mode: 0o700 })
  await writeFile(path.join(unknown, 'bare-snapshot'), secret, { mode: 0o600 })
  const before = { source: await snapshot(local.data), other: await snapshot(other.directory), unknown: await snapshot(unknown), state: await snapshot(local.common.state_dir) }
  const result = await call(await local.request())
  assert.equal(result.status, 0)
  assert.equal(result.value.deleted, true)
  assert.equal(result.value.bundle_id, id)
  assert.equal(result.value.manifest_sha256, local.target.receipt.manifest_sha256)
  assert.equal(result.value.logical_bytes, local.target.receipt.total_bytes)
  assert.equal(result.value.verified_before_delete, true)
  assert.equal(result.value.archive_scan_observed, true)
  assert.equal(result.value.deletion_authorized, false)
  assert.equal(result.value.external_mutual_exclusion_verified, false)
  await assert.rejects(lstat(local.target.directory), { code: 'ENOENT' })
  assert.deepEqual(await snapshot(local.data), before.source)
  assert.deepEqual(await snapshot(other.directory), before.other)
  assert.deepEqual(await snapshot(unknown), before.unknown)
  assert.deepEqual(await snapshot(local.common.state_dir), before.state)
})

test('root inode, trusted digest and COMMITTED mismatches reject before any archive deletion', async t => {
  for (const kind of ['inode', 'digest', 'commit']) {
    const local = await fixture(t)
    const request = await local.request()
    if (kind === 'inode') request.expected_root.ino = String(BigInt(request.expected_root.ino) + 1n)
    if (kind === 'digest') request.expected_manifest_sha256 = 'b'.repeat(64)
    if (kind === 'commit') await writeFile(path.join(local.target.directory, 'COMMITTED'), '{}')
    const before = await snapshot(local.target.directory)
    const result = await call(request)
    assert.notEqual(result.status, 0)
    assert.equal(result.value.deleted, undefined)
    assert.deepEqual(await snapshot(local.target.directory), before)
  }
})

test('unexpected entries, symlinks and hardlinks are not followed or removed', async t => {
  for (const kind of ['extra', 'symlink', 'hardlink']) {
    const local = await fixture(t)
    if (kind === 'extra') await writeFile(path.join(local.target.directory, 'unknown'), secret)
    if (kind === 'symlink') await symlink(local.database, path.join(local.target.directory, 'data/linked'))
    if (kind === 'hardlink') await link(local.database, path.join(local.target.directory, 'data/linked'))
    const before = await snapshot(local.target.directory), source = await snapshot(local.data)
    const result = await call(await local.request())
    assert.notEqual(result.status, 0)
    assert.deepEqual(await snapshot(local.target.directory), before)
    assert.deepEqual(await snapshot(local.data), source)
  }
})

test('a real readonly FD, directory FD, cwd/root reference or closed-FD private mmap blocks deletion', async t => {
  for (const kind of ['ro-fd', 'directory-fd', 'cwd', 'root', 'private-mmap']) {
    const local = await fixture(t)
    await consumer(t, local.target.directory, kind)
    const before = await snapshot(local.target.directory)
    const result = await call(await local.request())
    assert.notEqual(result.status, 0, kind)
    assert.match(result.value.error, /^BUNDLE_ARCHIVE_/)
    assert.deepEqual(await snapshot(local.target.directory), before)
  }
})

test('fsync or unlink failures after revoking COMMITTED leave an invalid partial that cannot be retried', async t => {
  for (const kind of ['fsync', 'unlink']) {
    const local = await fixture(t)
    const request = await local.request()
    const driver = `import io,json,os,runpy,sys
space=runpy.run_path(sys.argv[1]);request=json.loads(sys.argv[2]);root=os.path.join(request['backups_dir'],request['bundle_id']);kind=sys.argv[3]
original_sync=os.fsync;original_unlink=os.unlink
def sync(fd):
 if kind=='fsync' and not os.path.lexists(root+'/COMMITTED'):raise OSError('synthetic durability failure')
 return original_sync(fd)
def unlink(name,*args,**kwargs):
 if kind=='unlink' and name=='my-api.db' and not os.path.lexists(root+'/COMMITTED'):raise OSError('synthetic unlink failure')
 return original_unlink(name,*args,**kwargs)
os.fsync=sync;os.unlink=unlink;sys.stdin=io.StringIO(json.dumps(request));space['main']()
`
    // This child injects OS-boundary failure only; it does not replace the
    // real fixed archive scanner or manufacture an observed qualification.
    const failed = await call(request, driver, [kind])
    assert.notEqual(failed.status, 0)
    assert.equal(failed.value.deleted, undefined)
    await assert.rejects(lstat(path.join(local.target.directory, 'COMMITTED')), { code: 'ENOENT' })
    const partial = await snapshot(local.target.directory)
    const retried = await call(request)
    assert.notEqual(retried.status, 0)
    assert.deepEqual(await snapshot(local.target.directory), partial)
  }
})

test('natural source SQLite writes and its open connection do not turn into archive usage or source rollback', async t => {
  const local = await fixture(t)
  const program = 'import sqlite3,sys;db=sqlite3.connect(sys.argv[1]);db.execute("INSERT INTO ledger VALUES(2,\"new live source state\")");db.commit();print("ready",flush=True);sys.stdin.read();db.close()'
  const child = spawn('/usr/bin/python3', ['-I', '-u', '-c', program, local.database], { stdio: ['pipe', 'pipe', 'pipe'] })
  child.stderr.resume()
  const lines = createInterface({ input: child.stdout })
  const exited = new Promise(resolve => child.once('close', resolve))
  t.after(async () => { child.stdin.end(); const timer = setTimeout(() => child.kill('SIGKILL'), 5000); await exited; clearTimeout(timer); lines.close() })
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('source writer handshake timeout')), 10000)
    lines.once('line', line => { clearTimeout(timer); assert.equal(line, 'ready'); resolve() })
  })
  const source = await snapshot(local.data)
  const result = await call(await local.request())
  assert.equal(result.status, 0)
  assert.equal(result.value.deleted, true)
  assert.equal(child.exitCode, null, 'pruning must not stop the live source writer')
  assert.deepEqual(await snapshot(local.data), source)
})

test('unknown bundles and bypass fields remain untouched and cannot be elevated into prune qualification', async t => {
  const local = await fixture(t)
  const unknownId = '3'.repeat(64), unknown = path.join(local.common.backups_dir, unknownId)
  await mkdir(unknown, { mode: 0o700 })
  await writeFile(path.join(unknown, 'bare.sqlite'), secret)
  const request = await local.request({ bundle_id: unknownId, expected_root: await rootIdentity(unknown) })
  const before = await snapshot(unknown)
  assert.notEqual((await call(request)).status, 0)
  assert.deepEqual(await snapshot(unknown), before)
  const original = await snapshot(local.target.directory)
  for (const field of ['verified', 'script', 'proc_root', 'ignore_in_use']) {
    assert.notEqual((await call(await local.request({ [field]: true }))).status, 0)
    assert.deepEqual(await snapshot(local.target.directory), original)
  }
})
