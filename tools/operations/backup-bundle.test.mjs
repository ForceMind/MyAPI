import assert from 'node:assert/strict'
import { spawn, execFile } from 'node:child_process'
import { createHash } from 'node:crypto'
import { chmod, chown, link, lstat, mkdir, mkdtemp, readFile, readdir, readlink, rm, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { createInterface } from 'node:readline'
import test from 'node:test'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'

const execute = promisify(execFile)
const helper = fileURLToPath(new URL('./backup_data.py', import.meta.url))
const secret = 'SYNTHETIC_BUNDLE_SECRET_MUST_NOT_APPEAR'
const identity = 'a'.repeat(64)
const digest = bytes => createHash('sha256').update(bytes).digest('hex')

async function call(request, { driver } = {}) {
  const args = driver ? ['-I', '-c', driver, helper, JSON.stringify(request)] : ['-I', helper]
  const child = spawn('/usr/bin/python3', args, { stdio: ['pipe', 'pipe', 'pipe'] })
  let stdout = '', stderr = ''
  child.stdout.on('data', value => { stdout += value })
  child.stderr.on('data', value => { stderr += value })
  const closed = new Promise((resolve, reject) => {
    const timer = setTimeout(() => { child.kill('SIGKILL'); reject(new Error('bundle helper timeout')) }, 30000)
    child.once('error', error => { clearTimeout(timer); reject(error) })
    child.once('close', code => { clearTimeout(timer); resolve(code) })
  })
  child.stdin.end(driver ? '' : JSON.stringify(request))
  const status = await closed
  assert.equal(stdout.includes(secret), false)
  assert.equal(stderr.includes(secret), false)
  assert.equal(stderr, '', 'helper failures must not print paths, data or tracebacks')
  return { status, stdout, value: stdout ? JSON.parse(stdout) : null }
}

async function tree(root, relative = '.') {
  const absolute = relative === '.' ? root : path.join(root, relative)
  const stat = await lstat(absolute)
  if (stat.isSymbolicLink()) return [{ path: relative, type: 'symlink', target: await readlink(absolute) }]
  const rows = [{ path: relative, type: stat.isDirectory() ? 'directory' : 'file', uid: stat.uid, gid: stat.gid,
    mode: stat.mode & 0o7777, size: stat.isDirectory() ? null : stat.size,
    sha256: stat.isDirectory() ? null : digest(await readFile(absolute)) }]
  if (stat.isDirectory()) {
    for (const name of (await readdir(absolute)).sort()) rows.push(...await tree(root, relative === '.' ? name : path.join(relative, name)))
  }
  return rows.sort((left, right) => left.path.localeCompare(right.path))
}

async function fixture(t, { wal = true } = {}) {
  const root = await mkdtemp(path.join(tmpdir(), 'myapi-bundle-contract-'))
  await chmod(root, 0o700)
  const data = path.join(root, 'data'), backups = path.join(root, 'backups'), state = path.join(root, 'state')
  for (const directory of [data, backups, state]) await mkdir(directory, { mode: 0o700 })
  await chmod(data, 0o750)
  await mkdir(path.join(data, 'config'), { mode: 0o700 })
  await writeFile(path.join(data, 'config/runtime.json'), JSON.stringify({ private: secret }), { mode: 0o640 })
  const database = path.join(data, 'my-api.db')
  const program = `import sqlite3,sys
db=sqlite3.connect(sys.argv[1])
if sys.argv[2]=='wal':
 db.execute('PRAGMA journal_mode=WAL')
 db.execute('PRAGMA wal_autocheckpoint=0')
for table in ('users','tokens','logs'):
 db.execute('CREATE TABLE '+table+'(id INTEGER PRIMARY KEY)')
 db.execute('INSERT INTO '+table+' VALUES (1)')
db.execute('CREATE TABLE ledger(id INTEGER PRIMARY KEY,balance INTEGER,used INTEGER,receipt TEXT UNIQUE)')
db.execute('INSERT INTO ledger VALUES(1,985,15,"synthetic-applied-operation")')
db.commit()
print('ready',flush=True)
sys.stdin.read()
db.close()
`
  const child = spawn('/usr/bin/python3', ['-I', '-u', '-c', program, database, wal ? 'wal' : 'plain'], { stdio: ['pipe', 'pipe', 'pipe'] })
  const lines = createInterface({ input: child.stdout })
  child.stderr.resume()
  const exited = new Promise(resolve => child.once('close', resolve))
  t.after(async () => {
    child.stdin.end()
    const timer = setTimeout(() => child.kill('SIGKILL'), 5000)
    await exited
    clearTimeout(timer)
    lines.close()
    await rm(root, { recursive: true, force: true })
  })
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('SQLite fixture handshake timeout')), 10000)
    lines.once('line', line => { clearTimeout(timer); assert.equal(line, 'ready'); resolve() })
    child.once('error', reject)
  })
  if (wal) assert.ok((await lstat(database + '-wal')).size > 0, 'the committed fixture must still live in WAL')
  const common = { data_dir: data, sqlite_file: database, backups_dir: backups, state_dir: state,
    bundle_id: 'bundle-first', identity_sha256: identity, max_bytes: 16 * 1024 * 1024, max_entries: 10000 }
  return { root, data, database, backups, state, common, bundle: path.join(backups, common.bundle_id),
    scan() { return call({ action: 'stopped_scan', data_dir: data, sqlite_file: database, max_bytes: common.max_bytes, max_entries: common.max_entries }) },
    prepare() { return call({ action: 'bundle_prepare', ...common }) },
    commit(prepared) { return call({ action: 'bundle_commit', ...common, expected_manifest_sha256: prepared.manifest_sha256, expected_tree_sha256: prepared.tree_sha256 }) },
    verify(prepared, extra = {}) { return call({ action: 'bundle_verify', ...common, expected_manifest_sha256: prepared.manifest_sha256, ...extra }) },
    restore(prepared, destination) { return call({ action: 'bundle_restore', ...common, expected_manifest_sha256: prepared.manifest_sha256, destination }) },
  }
}

test('committed WAL bytes, full metadata and accounting survive prepare, explicit commit, verify and isolated restore', async t => {
  const local = await fixture(t)
  // The normal test owns its source. The root CI run exercises a genuinely
  // different source owner, not a copy normalized to the helper user.
  if (process.getuid() === 0) {
    for (const row of (await tree(local.data)).reverse()) await chown(row.path === '.' ? local.data : path.join(local.data, row.path), 12345, 23456)
    assert.notEqual((await lstat(local.data)).uid, process.getuid())
  }
  const original = await tree(local.data)
  const scan = await local.scan()
  assert.equal(scan.status, 0)
  assert.equal(scan.value.source_quiescence_verified, false)
  const prepared = await local.prepare()
  assert.equal(prepared.status, 0, prepared.stdout)
  assert.equal(prepared.value.prepared, true)
  assert.equal(prepared.value.committed, false)
  assert.equal(prepared.value.integrity_verified, true)
  assert.equal(prepared.value.tree_sha256, scan.value.tree_sha256)
  assert.equal(prepared.value.file_count + prepared.value.directory_count, original.length)
  assert.equal(prepared.value.total_bytes, original.reduce((sum, row) => sum + (row.size ?? 0), 0))
  assert.equal((await lstat(local.bundle)).mode & 0o777, 0o700)
  assert.equal((await lstat(path.join(local.bundle, 'manifest.json'))).mode & 0o777, 0o600)
  assert.deepEqual(await tree(path.join(local.bundle, 'data')), original)
  assert.deepEqual(await tree(local.data), original, 'SQLite validation must not open or checkpoint the source')
  assert.notEqual((await local.verify(prepared.value)).status, 0, 'a complete-looking stage is not committed')
  const committed = await local.commit(prepared.value)
  assert.equal(committed.status, 0, committed.stdout)
  assert.equal(committed.value.committed, true)
  assert.equal(committed.value.source_quiescence_verified, false)
  const frozen = await tree(local.bundle)
  assert.equal((await local.verify(prepared.value)).status, 0)
  const restored = path.join(local.root, 'restored')
  const result = await local.restore(prepared.value, restored)
  assert.equal(result.status, 0, result.stdout)
  assert.equal(result.value.restored, true)
  assert.deepEqual(await tree(restored), original)
  assert.deepEqual(await tree(local.bundle), frozen, 'validation must not rebuild the bundle WAL/SHM')
  assert.deepEqual(await tree(local.data), original)
  const query = await execute('/usr/bin/python3', ['-I', '-c', 'import json,sqlite3,sys;from urllib.parse import quote;db=sqlite3.connect("file:"+quote(sys.argv[1],safe="/")+"?mode=ro",uri=True);db.execute("PRAGMA query_only=ON");print(json.dumps(db.execute("SELECT balance,used,receipt FROM ledger").fetchall()));db.close()', path.join(restored, 'my-api.db')])
  assert.deepEqual(JSON.parse(query.stdout), [[985, 15, 'synthetic-applied-operation']])
})

test('legacy summary, live_summary and backup keep their original result contracts', async t => {
  const local = await fixture(t)
  const summary = await call({ action: 'summary', data_dir: local.data, sqlite_file: local.database })
  assert.equal(summary.status, 0)
  assert.deepEqual(Object.keys(summary.value).sort(), ['-shm', '-wal', 'db'])
  const live = await call({ action: 'live_summary', data_dir: local.data, sqlite_file: local.database })
  assert.deepEqual(live.value, { state: 'observed', counts: { users: 1, tokens: 1, logs: 1 } })
  const copied = await call({ action: 'backup', data_dir: local.data, sqlite_file: local.database, destination: path.join(local.root, 'legacy-copy'), max_bytes: 16 * 1024 * 1024 })
  assert.equal(copied.status, 0)
  assert.deepEqual(copied.value.before, copied.value.after)
  assert.deepEqual(copied.value.before, copied.value.copied)
})

test('a changed registered source cannot promote a prepared bundle', async t => {
  const local = await fixture(t)
  const prepared = await local.prepare()
  assert.equal(prepared.status, 0)
  await writeFile(path.join(local.data, 'new-accounting-marker'), 'new committed source state')
  const before = await tree(local.data)
  const result = await local.commit(prepared.value)
  assert.notEqual(result.status, 0)
  assert.equal(result.value.error, 'BUNDLE_SOURCE_CHANGED')
  assert.equal((await readdir(local.bundle)).includes('COMMITTED'), false)
  assert.deepEqual(await tree(local.data), before)
})

test('a source mutation at a copy durability boundary retains an uncommitted partial', async t => {
  const local = await fixture(t)
  const driver = `import io,json,os,runpy,sys
space=runpy.run_path(sys.argv[1]); request=json.loads(sys.argv[2]); original=os.fsync; changed=False
def sync(fd):
 global changed
 original(fd)
 if not changed:
  changed=True
  with open(os.path.join(request['data_dir'],'config/runtime.json'),'ab') as stream:stream.write(b' changed')
os.fsync=sync
sys.stdin=io.StringIO(json.dumps(request))
space['main']()
`
  const result = await call({ action: 'bundle_prepare', ...local.common }, { driver })
  assert.notEqual(result.status, 0)
  assert.equal(result.value.error, 'BUNDLE_SOURCE_CHANGED')
  assert.equal((await readdir(local.bundle)).includes('COMMITTED'), false)
})

test('corruption and unsupported manifest schema never become valid bundles', async t => {
  const corrupt = await fixture(t, { wal: false })
  await writeFile(corrupt.database, 'not a SQLite database')
  const before = await tree(corrupt.data)
  const failed = await corrupt.prepare()
  assert.notEqual(failed.status, 0)
  assert.equal(failed.value.integrity_verified, undefined)
  assert.deepEqual(await tree(corrupt.data), before)
  assert.equal((await readdir(corrupt.bundle)).includes('COMMITTED'), false)
  const local = await fixture(t)
  const prepared = await local.prepare()
  assert.equal(prepared.status, 0)
  const filename = path.join(local.bundle, 'manifest.json')
  const value = JSON.parse(await readFile(filename, 'utf8'))
  value.schema_version = 99
  const bytes = Buffer.from(JSON.stringify(value))
  await writeFile(filename, bytes)
  const result = await local.commit({ ...prepared.value, manifest_sha256: digest(bytes) })
  assert.notEqual(result.status, 0)
  assert.equal(result.value.error, 'BUNDLE_SCHEMA_INVALID')
})

test('payload bytes, extra files, metadata and identity are all part of verification', async t => {
  for (const mutation of ['bytes', 'extra', 'mode', 'identity']) {
    const local = await fixture(t)
    const prepared = await local.prepare()
    assert.equal(prepared.status, 0)
    assert.equal((await local.commit(prepared.value)).status, 0)
    if (mutation === 'bytes') await writeFile(path.join(local.bundle, 'data/config/runtime.json'), 'changed')
    if (mutation === 'extra') await writeFile(path.join(local.bundle, 'unlisted'), 'extra')
    if (mutation === 'mode') await chmod(path.join(local.bundle, 'data/config/runtime.json'), 0o600)
    const result = await local.verify(prepared.value, mutation === 'identity' ? { identity_sha256: 'b'.repeat(64) } : {})
    assert.notEqual(result.status, 0, mutation)
    const target = path.join(local.root, 'must-not-be-created')
    if (mutation !== 'identity') {
      assert.notEqual((await local.restore(prepared.value, target)).status, 0)
      await assert.rejects(lstat(target), { code: 'ENOENT' })
    }
  }
})

test('traversal, links, special files and same-device submount declarations fail closed', async t => {
  for (const kind of ['symlink', 'hardlink', 'fifo', 'submount']) {
    const local = await fixture(t)
    if (kind === 'symlink') await symlink(path.join(local.data, 'config/runtime.json'), path.join(local.data, 'linked'))
    if (kind === 'hardlink') await link(path.join(local.data, 'config/runtime.json'), path.join(local.data, 'linked'))
    if (kind === 'fifo') await execute('/usr/bin/python3', ['-I', '-c', 'import os,sys;os.mkfifo(sys.argv[1])', path.join(local.data, 'pipe')])
    const driver = kind === 'submount' ? `import builtins,io,json,runpy,sys
space=runpy.run_path(sys.argv[1]); request=json.loads(sys.argv[2]); original=builtins.open
def opened(filename,*args,**kwargs):
 if filename=='/proc/self/mountinfo':return io.StringIO('1 0 0:1 / '+request['data_dir']+'/config rw - ext4 fixture rw\\n')
 return original(filename,*args,**kwargs)
builtins.open=opened;sys.stdin=io.StringIO(json.dumps(request));space['main']()
` : undefined
    const result = await call({ action: 'bundle_prepare', ...local.common }, { driver })
    assert.notEqual(result.status, 0, kind)
  }
  const local = await fixture(t)
  const prepared = await local.prepare()
  const filename = path.join(local.bundle, 'manifest.json')
  const value = JSON.parse(await readFile(filename, 'utf8'))
  value.entries[1].path = '../outside'
  const bytes = Buffer.from(JSON.stringify(value))
  await writeFile(filename, bytes)
  assert.notEqual((await local.commit({ ...prepared.value, manifest_sha256: digest(bytes) })).status, 0)
})

test('ACL and xattr metadata outside manifest v1 is refused instead of being silently dropped', async t => {
  for (const kind of ['xattr', 'acl']) {
    const local = await fixture(t)
    const filename = path.join(local.data, 'config/runtime.json')
    const program = kind === 'xattr'
      ? 'import os,sys;os.setxattr(sys.argv[1],"user.myapi_bundle",b"synthetic-xattr-must-stay")'
      : 'import os,struct,sys;entries=[(1,6,4294967295),(2,4,os.getuid()+1),(4,4,4294967295),(16,4,4294967295),(32,0,4294967295)];value=struct.pack("<I",2)+b"".join(struct.pack("<HHI",*entry) for entry in entries);os.setxattr(sys.argv[1],"system.posix_acl_access",value)'
    await execute('/usr/bin/python3', ['-I', '-c', program, filename])
    const before = await tree(local.data)
    const result = await local.prepare()
    assert.notEqual(result.status, 0)
    assert.equal(result.value.error, 'BUNDLE_EXTENDED_METADATA_UNSUPPORTED')
    assert.deepEqual(await tree(local.data), before)
    const attribute = kind === 'xattr' ? 'user.myapi_bundle' : 'system.posix_acl_access'
    const observed = await execute('/usr/bin/python3', ['-I', '-c', 'import os,sys;print(len(os.getxattr(sys.argv[1],sys.argv[2])))', filename, attribute])
    assert.ok(Number(observed.stdout.trim()) > 0)
  }
})

test('integrity scratch is private, same-parent and removed before a bundle becomes prepared', async t => {
  const local = await fixture(t)
  const driver = `import io,json,os,runpy,sys,tempfile
space=runpy.run_path(sys.argv[1]);request=json.loads(sys.argv[2]);original=tempfile.mkdtemp;created=[]
def temporary(*args,**kwargs):
 expected=os.path.join(request['backups_dir'],request['bundle_id'])
 if kwargs.get('dir')!=expected:raise Exception('unexpected validation filesystem')
 result=original(*args,**kwargs);created.append(result)
 if (os.stat(result).st_mode&511)!=448:raise Exception('scratch not private')
 return result
tempfile.mkdtemp=temporary;sys.stdin=io.StringIO(json.dumps(request));space['main']()
if not created or any(os.path.lexists(item) for item in created):raise Exception('validation scratch retained')
`
  const before = await tree(local.data)
  const result = await call({ action: 'bundle_prepare', ...local.common }, { driver })
  assert.equal(result.status, 0, result.stdout)
  assert.deepEqual((await readdir(local.bundle)).sort(), ['data', 'manifest.json'])
  assert.deepEqual(await tree(local.data), before)
})

test('existing or overlapping restore targets remain unchanged and an exclusive stage cannot be reused', async t => {
  const local = await fixture(t)
  const prepared = await local.prepare()
  assert.equal(prepared.status, 0)
  assert.notEqual((await local.prepare()).status, 0)
  assert.equal((await local.commit(prepared.value)).status, 0)
  assert.notEqual((await local.commit(prepared.value)).status, 0, 'commit is not an automatic replay path')
  const target = path.join(local.root, 'existing')
  await mkdir(target, { mode: 0o700 })
  await writeFile(path.join(target, 'new-accounting'), 'preserve this new state')
  const before = await tree(target)
  assert.notEqual((await local.restore(prepared.value, target)).status, 0)
  assert.deepEqual(await tree(target), before)
  for (const destination of [local.data, path.join(local.data, 'nested'), path.join(local.backups, 'nested'), path.join(local.state, 'nested')]) {
    assert.notEqual((await local.restore(prepared.value, destination)).status, 0)
  }
})

test('explicit caps and exact request fields cannot be silently clamped or treated as unknown zero', async t => {
  const local = await fixture(t)
  for (const changes of [{ max_bytes: 1 }, { max_bytes: 16 * 1024 ** 3 + 1 }, { max_entries: 1 }, { max_entries: 10001 },
    { identity_sha256: null }, { bundle_id: '../escape' }, { verified: true }]) {
    const result = await call({ action: 'bundle_prepare', ...local.common, ...changes })
    assert.notEqual(result.status, 0)
    assert.equal(result.value.prepared, undefined)
  }
})
