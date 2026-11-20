import assert from 'node:assert/strict'
import { execFile, spawn } from 'node:child_process'
import { createHash } from 'node:crypto'
import { chmod, chown, copyFile, link, lstat, mkdir, mkdtemp, readFile, readdir, rename, rm, symlink, unlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { test as nodeTest } from 'node:test'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'

const execute = promisify(execFile)
const test = process.platform === 'linux' ? nodeTest : nodeTest.skip
const helper = fileURLToPath(new URL('../../tools/operations/native_sqlite_transition.py', import.meta.url))
const secret = 'SYNTHETIC_NATIVE_ACCOUNTING_SECRET'
const sha256 = value => createHash('sha256').update(value).digest('hex')

// Linux POSIX ACL xattr format: version 2 followed by sorted 8-byte entries.
// A named numeric user does not require or refer to an actual passwd entry.
// Keep the access mask at zero for private files/directories, while retaining
// nonempty named-user permissions that cannot collapse to a mode-only ACL.
function posixAcl(namedUser, ownerPermissions, namedPermissions, maskPermissions = 0) {
  const entries = [[0x01, ownerPermissions, 0xffffffff], [0x02, namedPermissions, namedUser],
    [0x04, 0, 0xffffffff], [0x10, maskPermissions, 0xffffffff], [0x20, 0, 0xffffffff]]
  const bytes = Buffer.alloc(4 + entries.length * 8)
  bytes.writeUInt32LE(2, 0)
  for (const [index, [tag, permissions, id]] of entries.entries()) {
    bytes.writeUInt16LE(tag, 4 + index * 8)
    bytes.writeUInt16LE(permissions, 6 + index * 8)
    bytes.writeUInt32LE(id, 8 + index * 8)
  }
  return bytes.toString('hex')
}

async function aclFixture(operations) {
  const program = `import errno,json,os,sys
result=[]
for operation in json.loads(sys.argv[1]):
 for name,value in operation.get('set',{}).items(): os.setxattr(operation['path'],name,bytes.fromhex(value),follow_symlinks=False)
 row={}
 for name in dict.fromkeys(['system.posix_acl_access','system.posix_acl_default',*operation.get('set',{})]):
  try: row[name]=os.getxattr(operation['path'],name,follow_symlinks=False).hex()
  except OSError as error:
   if error.errno != errno.ENODATA: raise
   row[name]=None
 result.append(row)
print(json.dumps(result,separators=(',',':')))
`
  const { stdout } = await execute('python3', ['-I', '-B', '-c', program, JSON.stringify(operations)])
  assert.equal(stdout.includes(secret), false)
  return JSON.parse(stdout)
}

async function call(request, driver) {
  const args = driver ? ['-I', '-B', '-c', driver, helper] : ['-I', '-B', helper]
  const child = spawn('python3', args, { stdio: ['pipe', 'pipe', 'pipe'] })
  let stdout = '', stderr = ''
  child.stdout.on('data', chunk => { stdout += chunk })
  child.stderr.on('data', chunk => { stderr += chunk })
  const closed = new Promise((resolve, reject) => {
    const timer = setTimeout(() => { child.kill('SIGKILL'); reject(new Error('native SQLite helper timeout')) }, 30_000)
    child.once('error', error => { clearTimeout(timer); reject(error) })
    child.once('close', status => { clearTimeout(timer); resolve(status) })
  })
  child.stdin.end(JSON.stringify(request))
  const status = await closed
  assert.equal(stderr, '', 'do not emit exception text, paths or secrets')
  assert.equal(stdout.includes(secret), false)
  const value = JSON.parse(stdout)
  assert.equal(value.ok, status === 0)
  if (status !== 0) {
    assert.deepEqual(Object.keys(value).sort(), ['code', 'ok'])
    assert.match(value.code, /^[a-z_]{1,64}$/)
  }
  return { status, value }
}

async function contentTree(directory, relative = '.') {
  const filename = relative === '.' ? directory : path.join(directory, relative)
  const info = await lstat(filename)
  const rows = [{ path: relative, type: info.isDirectory() ? 'directory' : 'file',
    mode: info.mode & 0o7777, uid: info.uid, gid: info.gid,
    sha256: info.isDirectory() ? null : sha256(await readFile(filename)) }]
  if (info.isDirectory()) {
    for (const name of (await readdir(filename)).sort()) {
      rows.push(...await contentTree(directory, relative === '.' ? name : path.join(relative, name)))
    }
  }
  return rows
}

async function fixture(t, { userLayout = false } = {}) {
  const directory = await mkdtemp(path.join(tmpdir(), 'myapi-native-sqlite-'))
  t.after(() => rm(directory, { recursive: true, force: true }))
  await chmod(directory, 0o700)
  const data = path.join(directory, userLayout ? '.local/share/my-api/data' : 'data')
  const backups = path.join(directory, userLayout ? '.local/state/my-api/backups' : 'backups')
  for (const folder of [data, backups]) await mkdir(folder, { recursive: true, mode: 0o700 })
  const database = path.join(data, 'my-api.db')
  const config = path.join(directory, userLayout ? '.config/my-api/my-api.env' : 'runtime.env')
  const receipt = path.join(directory, userLayout ? '.local/state/my-api/state/installation.json' : 'installation.json')
  for (const folder of [path.dirname(config), path.dirname(receipt)]) await mkdir(folder, { recursive: true, mode: 0o700 })
  await writeFile(config, `SESSION_SECRET=${secret}\n`, { mode: 0o600 })
  await writeFile(receipt, JSON.stringify({ version: '1.2.0', privateFixture: secret }), { mode: 0o600 })
  await mkdir(path.join(data, 'uploads'), { mode: 0o750 })
  await writeFile(path.join(data, 'uploads', 'history'), 'synthetic saved upload', { mode: 0o640 })
  const program = `import os,sqlite3,sys
db=sqlite3.connect(sys.argv[1])
db.execute('PRAGMA journal_mode=WAL')
db.execute('PRAGMA wal_autocheckpoint=0')
db.execute('CREATE TABLE users(id INTEGER PRIMARY KEY,quota INTEGER,used_quota INTEGER,quota_version INTEGER,request_count INTEGER,username TEXT)')
db.execute('CREATE TABLE tokens(id INTEGER PRIMARY KEY,user_id INTEGER,remain_quota INTEGER,used_quota INTEGER,quota_version INTEGER,unlimited_quota INTEGER,key TEXT)')
db.execute('CREATE TABLE logs(id INTEGER PRIMARY KEY,type INTEGER,quota INTEGER,content TEXT,request_id TEXT)')
db.execute('CREATE TABLE quota_mutation_receipts(id INTEGER PRIMARY KEY,mutation_id TEXT,quota INTEGER)')
db.execute('INSERT INTO users VALUES(1,985,15,1,1,?)',(sys.argv[2],))
db.execute('INSERT INTO tokens VALUES(1,1,485,15,1,0,?)',(sys.argv[2],))
db.execute('INSERT INTO logs VALUES(1,2,15,?,?)',(sys.argv[2],sys.argv[2]))
db.execute('INSERT INTO quota_mutation_receipts VALUES(1,?,15)',(sys.argv[2],))
db.commit()
# Model stopped/crashed WAL state without close-time checkpointing it away.
os._exit(0)
`
  await execute('python3', ['-I', '-B', '-c', program, database, secret])
  assert.ok((await lstat(database + '-wal')).size > 0, 'fixture must carry committed transactions in a nonempty WAL')
  const info = await lstat(database, { bigint: true })
  const request = { data, database, config, receipt, device: String(info.dev), inode: String(info.ino),
    backup: path.join(backups, 'a'.repeat(64)) }
  return { directory, request,
    call(action, extra = {}, driver) { return call({ ...request, action, ...extra }, driver) },
    async sql(statements) {
      await execute('python3', ['-I', '-B', '-c', `import json,sqlite3,sys
db=sqlite3.connect(sys.argv[1])
for statement in json.loads(sys.argv[2]): db.execute(statement)
db.commit()
db.close()
`, database, JSON.stringify(statements)])
    } }
}

test('backs up all stopped data, nonempty WAL, private configuration and installation receipt without changing originals', async t => {
  const f = await fixture(t)
  const original = await contentTree(f.request.data)
  const config = await readFile(f.request.config), receipt = await readFile(f.request.receipt)
  assert.deepEqual((await f.call('preflight')).value, { ok: true, preflight: true })
  await assert.rejects(lstat(f.request.backup), { code: 'ENOENT' })
  assert.deepEqual(await contentTree(f.request.data), original)
  assert.deepEqual((await f.call('backup')).value, {
    ok: true, backup: f.request.backup, backupVerified: true, accountingVerified: true,
  })
  assert.deepEqual(await contentTree(path.join(f.request.backup, 'data')), original)
  assert.deepEqual(await contentTree(f.request.data), original, 'offline SQLite checks must never touch source WAL/SHM')
  assert.deepEqual(await readFile(path.join(f.request.backup, 'config')), config)
  assert.deepEqual(await readFile(path.join(f.request.backup, 'receipt')), receipt)
  assert.equal((await lstat(f.request.backup)).mode & 0o777, 0o700)
  for (const file of ['config', 'receipt', 'manifest.json', 'COMMITTED']) {
    assert.equal((await lstat(path.join(f.request.backup, file))).mode & 0o777, 0o600)
  }
  assert.equal((await f.call('check-stopped')).value.sourceUnchanged, true)
  assert.equal((await f.call('check-live')).value.historicalRowsVerified, true)
  assert.deepEqual(await readFile(f.request.config), config)
  assert.deepEqual(await readFile(f.request.receipt), receipt)
  assert.deepEqual((await f.call('backup')).value, { ok: false, code: 'backup_exists' })
})

test('accepts the exact B3c user layout while ignoring the adjacent current-release symlink', async t => {
  const f = await fixture(t, { userLayout: true })
  const release = path.join(f.directory, '.local/share/my-api/releases/1.2.0')
  await mkdir(release, { recursive: true, mode: 0o700 })
  await symlink(release, path.join(f.directory, '.local/share/my-api/current'))
  const original = await contentTree(f.request.data)
  assert.deepEqual((await f.call('preflight')).value, { ok: true, preflight: true })
  assert.deepEqual((await f.call('preflight')).value, { ok: true, preflight: true })
  assert.deepEqual(await contentTree(f.request.data), original)
  await assert.rejects(lstat(f.request.backup), { code: 'ENOENT' })
  assert.equal((await f.call('backup')).value.backupVerified, true)
  assert.equal((await f.call('check-stopped')).value.sourceUnchanged, true)
  assert.equal((await f.call('check-live')).value.historicalRowsVerified, true)
})

test('reads retained schema-1 native backups without attributing unrecorded ACLs', async t => {
  const f = await fixture(t)
  assert.equal((await f.call('backup')).value.backupVerified, true)
  const toLegacy = String.raw`
import hashlib, json, pathlib, sys
root = pathlib.Path(sys.argv[1])
manifest = json.loads((root / 'manifest.json').read_bytes())
assert manifest['schema'] == 2
assert all('posix_acls' not in row for row in manifest['source'])
for key in ('config', 'receipt'):
    assert manifest[key].pop('permissions')['posix_acls'] == {}
manifest['schema'] = 1
def canonical(value):
    return (json.dumps(value, sort_keys=True, ensure_ascii=False, separators=(',', ':')) + '\n').encode()
raw = canonical(manifest)
(root / 'manifest.json').write_bytes(raw)
(root / 'COMMITTED').write_bytes(canonical({'sha256': hashlib.sha256(raw).hexdigest()}))
`
  await execute('python3', ['-I', '-B', '-c', toLegacy, f.request.backup])
  assert.equal((await f.call('check-stopped')).value.sourceUnchanged, true)
  assert.equal((await f.call('check-live')).value.historicalRowsVerified, true)
  await aclFixture([{ path: path.join(f.request.backup, 'config'),
    set: { 'system.posix_acl_access': posixAcl(61016, 6, 4) } }])
  assert.deepEqual((await f.call('check-stopped')).value, { ok: false, code: 'backup_unverified' })
})

test('preserves real nonempty access and default ACLs, clearing unrelated inherited ACLs from copies', async t => {
  const f = await fixture(t, { userLayout: true })
  const access = 'system.posix_acl_access', defaults = 'system.posix_acl_default'
  const directoryAcl = posixAcl(61001, 7, 4)
  const defaultAcl = posixAcl(61002, 7, 5, 5)
  const parentDefault = posixAcl(62002, 7, 2, 2)
  const plainDirectory = path.join(f.request.data, 'no-acl')
  const plainFile = path.join(plainDirectory, 'plain')
  // Create these before installing the source defaults: absence is deliberate.
  await mkdir(plainDirectory, { mode: 0o700 })
  await writeFile(plainFile, 'synthetic no-ACL payload', { mode: 0o600 })
  const aclFiles = [f.request.database, f.request.database + '-wal',
    path.join(f.request.data, 'uploads/history'), f.request.config, f.request.receipt]
  const sourceDirectories = [f.request.data, path.join(f.request.data, 'uploads')]
  const installed = await aclFixture([
    ...sourceDirectories.map(filename => ({ path: filename, set: { [access]: directoryAcl, [defaults]: defaultAcl } })),
    ...aclFiles.map((filename, index) => ({ path: filename, set: { [access]: posixAcl(61100 + index, 6, 4) } })),
    { path: path.dirname(f.request.backup), set: { [defaults]: parentDefault } },
  ])
  for (let index = 0; index < sourceDirectories.length; index++) {
    assert.deepEqual(installed[index], { [access]: directoryAcl, [defaults]: defaultAcl })
  }
  for (let index = 0; index < aclFiles.length; index++) {
    assert.deepEqual(installed[sourceDirectories.length + index], { [access]: posixAcl(61100 + index, 6, 4), [defaults]: null })
  }
  assert.equal(installed.at(-1)[defaults], parentDefault)
  assert.notEqual(directoryAcl, defaultAcl)
  assert.notEqual(parentDefault, defaultAcl)
  const noAcl = { [access]: null, [defaults]: null }
  assert.deepEqual(await aclFixture([{ path: plainDirectory }, { path: plainFile }]), [noAcl, noAcl])
  const original = await contentTree(f.request.data)
  const sourceFiles = original.map(row => row.path === '.' ? f.request.data : path.join(f.request.data, row.path))
  const sourceAcl = await aclFixture([...sourceFiles, f.request.config, f.request.receipt].map(filename => ({ path: filename })))
  const config = await readFile(f.request.config), receipt = await readFile(f.request.receipt)
  const database = await readFile(f.request.database), wal = await readFile(f.request.database + '-wal')
  assert.ok(wal.length > 32)
  assert.deepEqual((await f.call('preflight')).value, { ok: true, preflight: true })
  assert.equal((await f.call('backup')).value.backupVerified, true)
  const backupData = path.join(f.request.backup, 'data')
  const copiedFiles = original.map(row => row.path === '.' ? backupData : path.join(backupData, row.path))
  const copiedAcl = await aclFixture([...copiedFiles, path.join(f.request.backup, 'config'),
    path.join(f.request.backup, 'receipt')].map(filename => ({ path: filename })))
  assert.deepEqual(copiedAcl, sourceAcl, 'ACL bytes and ACL absence must match each source, not the backup parent')
  assert.deepEqual(await contentTree(backupData), original)
  assert.deepEqual(await contentTree(f.request.data), original)
  assert.deepEqual(await readFile(path.join(backupData, 'my-api.db')), database)
  assert.deepEqual(await readFile(path.join(backupData, 'my-api.db-wal')), wal)
  assert.deepEqual(await readFile(path.join(f.request.backup, 'config')), config)
  assert.deepEqual(await readFile(path.join(f.request.backup, 'receipt')), receipt)
  assert.equal((await f.call('check-stopped')).value.sourceUnchanged, true)
  assert.equal((await f.call('check-live')).value.historicalRowsVerified, true)
  assert.deepEqual(await aclFixture([...copiedFiles, path.join(f.request.backup, 'config'),
    path.join(f.request.backup, 'receipt')].map(filename => ({ path: filename }))), copiedAcl,
  'both verification modes must leave copied access/default ACL bytes unchanged')
  assert.deepEqual(await aclFixture([...sourceFiles, f.request.config, f.request.receipt].map(filename => ({ path: filename }))), sourceAcl)
  assert.deepEqual(await readFile(f.request.database), database)
  assert.deepEqual(await readFile(f.request.database + '-wal'), wal)
  assert.deepEqual(await readFile(f.request.config), config)
  assert.deepEqual(await readFile(f.request.receipt), receipt)
})

test('rejects valid ACL changes to backed-up directories, configuration and receipt without repairing them', async t => {
  const access = 'system.posix_acl_access', defaults = 'system.posix_acl_default'
  for (const [relative, attribute, originalAcl, changedAcl] of [
    ['data', access, posixAcl(61001, 7, 4), posixAcl(61011, 7, 4)],
    ['data/uploads', defaults, posixAcl(61002, 7, 5, 5), posixAcl(61012, 7, 5, 5)],
    ['data/my-api.db-wal', access, posixAcl(61005, 6, 4), posixAcl(61015, 6, 4)],
    ['config', access, posixAcl(61003, 6, 4), posixAcl(61013, 6, 4)],
    ['receipt', access, posixAcl(61004, 6, 4), posixAcl(61014, 6, 4)],
  ]) {
    await t.test(`${relative}:${attribute}`, async t => {
      const f = await fixture(t)
      const source = relative === 'config' ? f.request.config : relative === 'receipt' ? f.request.receipt
        : relative === 'data' ? f.request.data : path.join(f.request.data, relative.slice('data/'.length))
      assert.equal((await aclFixture([{ path: source, set: { [attribute]: originalAcl } }]))[0][attribute], originalAcl)
      assert.equal((await f.call('backup')).value.backupVerified, true)
      const target = path.join(f.request.backup, relative)
      assert.equal((await aclFixture([{ path: target }]))[0][attribute], originalAcl)
      const bytesBefore = await contentTree(path.join(f.request.backup, 'data'))
      const configBefore = await readFile(path.join(f.request.backup, 'config'))
      const receiptBefore = await readFile(path.join(f.request.backup, 'receipt'))
      const modeBefore = (await lstat(target)).mode
      assert.equal((await aclFixture([{ path: target, set: { [attribute]: changedAcl } }]))[0][attribute], changedAcl)
      assert.equal((await lstat(target)).mode, modeBefore, 'tampering changes ACL identity without changing POSIX mode')
      assert.deepEqual(await contentTree(path.join(f.request.backup, 'data')), bytesBefore)
      for (const action of ['check-stopped', 'check-live']) {
        assert.deepEqual((await f.call(action)).value, { ok: false, code: 'backup_unverified' })
        assert.equal((await aclFixture([{ path: target }]))[0][attribute], changedAcl, 'verification must not repair ACLs')
      }
      assert.deepEqual(await readFile(path.join(f.request.backup, 'config')), configBefore)
      assert.deepEqual(await readFile(path.join(f.request.backup, 'receipt')), receiptBefore)
    })
  }
})

test('rejects changed source ACLs and actual unsupported xattrs while default Docker copies still refuse ACLs', async t => {
  const f = await fixture(t)
  const access = 'system.posix_acl_access', defaults = 'system.posix_acl_default'
  const acl = posixAcl(61001, 7, 4)
  assert.equal((await aclFixture([{ path: f.request.data, set: { [access]: acl } }]))[0][access], acl)
  assert.equal((await f.call('backup')).value.backupVerified, true)
  const changed = posixAcl(61002, 7, 4)
  assert.equal((await aclFixture([{ path: f.request.data, set: { [access]: changed } }]))[0][access], changed)
  assert.deepEqual((await f.call('check-stopped')).value, { ok: false, code: 'source_changed' })

  const unsupported = await fixture(t)
  const extra = Buffer.from('synthetic non-ACL metadata').toString('hex')
  const observed = await aclFixture([{ path: unsupported.request.data,
    set: { [access]: acl, [defaults]: posixAcl(61003, 7, 5, 5), 'user.myapi_synthetic_unsupported': extra } }])
  assert.equal(observed[0][access], acl)
  assert.equal(observed[0]['user.myapi_synthetic_unsupported'], extra)
  const original = await contentTree(unsupported.request.data)
  for (const action of ['preflight', 'backup']) {
    assert.deepEqual((await unsupported.call(action)).value, { ok: false, code: 'extended_metadata_unsupported' })
  }
  await assert.rejects(lstat(path.join(unsupported.request.backup, 'COMMITTED')), { code: 'ENOENT' })
  assert.deepEqual(await contentTree(unsupported.request.data), original)

  const target = path.join(f.directory, 'default-docker-copy')
  const program = `import json,runpy,sys
module=runpy.run_path(sys.argv[1],run_name='_default_bundle_acl_test')
result={}
for action in ('tree','copy'):
 try:
  if action=='tree': module['bundle_tree'](sys.argv[2],1024*1024,100)
  else: module['bundle_copy'](sys.argv[2],sys.argv[3],1024*1024,100)
 except module['BundleFailure'] as error: result[action]=str(error)
 else: result[action]='unexpectedly_accepted'
print(json.dumps(result))
`
  const { stdout } = await execute('python3', ['-I', '-B', '-c', program,
    fileURLToPath(new URL('../../tools/operations/backup_data.py', import.meta.url)), f.request.data, target])
  assert.deepEqual(JSON.parse(stdout), { tree: 'BUNDLE_EXTENDED_METADATA_UNSUPPORTED', copy: 'BUNDLE_EXTENDED_METADATA_UNSUPPORTED' })
  assert.deepEqual(await readdir(target), [])
})

test('distinguishes unsafe user paths and private metadata without weakening their rejection', async t => {
  const f = await fixture(t, { userLayout: true })
  const parent = path.dirname(f.request.backup)
  await chmod(parent, 0o750)
  assert.deepEqual((await f.call('preflight')).value, { ok: false, code: 'backup_parent_not_private' })
  await chmod(parent, 0o700)
  const alias = path.join(f.directory, 'config-alias')
  await symlink(f.request.config, alias)
  assert.deepEqual((await f.call('preflight', { config: alias })).value, { ok: false, code: 'unsafe_config_path' })
  const driver = `import os,runpy,sys
os.listxattr=lambda _: ['user.synthetic_metadata']
runpy.run_path(sys.argv[1],run_name='__main__')
`
  assert.deepEqual((await f.call('preflight', {}, driver)).value, { ok: false, code: 'extended_metadata_unsupported' })
  await assert.rejects(lstat(f.request.backup), { code: 'ENOENT' })
})

test('allows new users, keys, consumption and accounting rows while preserving historical values', async t => {
  const f = await fixture(t)
  assert.equal((await f.call('backup')).status, 0)
  await f.sql([
    "INSERT INTO users VALUES(2,100,0,0,0,'new-user')",
    "INSERT INTO tokens VALUES(2,2,100,0,0,0,'new-key')",
    "INSERT INTO logs VALUES(2,2,7,'new-consumption','new-request')",
    "INSERT INTO quota_mutation_receipts VALUES(2,'new-mutation',7)",
  ])
  assert.equal((await f.call('check-live')).value.historicalRowsVerified, true)
  assert.deepEqual((await f.call('check-stopped')).value, { ok: false, code: 'source_changed' })
})

for (const statement of [
  'UPDATE users SET quota=quota+1 WHERE id=1',
  'UPDATE tokens SET remain_quota=remain_quota+1 WHERE id=1',
  'UPDATE logs SET quota=quota+1 WHERE id=1',
  "UPDATE quota_mutation_receipts SET mutation_id='rewritten' WHERE id=1",
  'DELETE FROM logs WHERE id=1',
]) {
  test(`rejects changed historical accounting without restoring or replaying: ${statement}`, async t => {
    const f = await fixture(t)
    assert.equal((await f.call('backup')).status, 0)
    await f.sql([statement])
    const databaseBefore = await readFile(f.request.database)
    assert.deepEqual((await f.call('check-live')).value, { ok: false, code: 'accounting_changed' })
    assert.deepEqual(await readFile(f.request.database), databaseBefore, 'verification must not repair the source database')
  })
}

test('never substitutes an empty dataset when required tables or funding columns are missing', async t => {
  for (const statement of ['DROP TABLE logs', 'ALTER TABLE tokens RENAME COLUMN remain_quota TO unknown_quota']) {
    await t.test(statement, async t => {
      const f = await fixture(t)
      assert.equal((await f.call('backup')).status, 0)
      await f.sql([statement])
      assert.deepEqual((await f.call('check-live')).value, { ok: false, code: 'accounting_unknown' })
    })
  }
  const f = await fixture(t)
  await f.sql(['DROP TABLE logs'])
  assert.deepEqual((await f.call('backup')).value, { ok: false, code: 'accounting_unknown' })
  await assert.rejects(lstat(path.join(f.request.backup, 'COMMITTED')), { code: 'ENOENT' })
})

test('rejects missing, tampered or uncommitted backup payload and envelope', async t => {
  for (const file of ['COMMITTED', 'manifest.json', 'config', 'receipt', 'data/my-api.db-wal']) {
    await t.test(file, async t => {
      const f = await fixture(t)
      assert.equal((await f.call('backup')).status, 0)
      await unlink(path.join(f.request.backup, file))
      assert.notEqual((await f.call('check-stopped')).status, 0)
      assert.notEqual((await f.call('check-live')).status, 0)
    })
  }
  const f = await fixture(t)
  assert.equal((await f.call('backup')).status, 0)
  await writeFile(path.join(f.request.backup, 'data/uploads/history'), 'tampered')
  assert.deepEqual((await f.call('check-live')).value, { ok: false, code: 'backup_unverified' })
})

test('requires the original private configuration and installation receipt during both checks', async t => {
  for (const name of ['config', 'receipt']) {
    await t.test(name, async t => {
      const f = await fixture(t)
      assert.equal((await f.call('backup')).status, 0)
      await writeFile(f.request[name], 'changed')
      assert.deepEqual((await f.call('check-stopped')).value, { ok: false, code: 'source_changed' })
      assert.deepEqual((await f.call('check-live')).value, { ok: false, code: 'source_changed' })
    })
  }
})

test('refuses changed source files, replaced database inode, symlinks, hardlinks and overlapping paths', async t => {
  const f = await fixture(t)
  assert.equal((await f.call('backup')).status, 0)
  await writeFile(path.join(f.request.data, 'uploads/history'), 'changed after stopped backup')
  assert.deepEqual((await f.call('check-stopped')).value, { ok: false, code: 'source_changed' })
  const old = f.request.database + '.old'
  await rename(f.request.database, old)
  await copyFile(old, f.request.database)
  assert.deepEqual((await f.call('check-live')).value, { ok: false, code: 'source_changed' })
  const safe = await fixture(t)
  const alias = path.join(safe.directory, 'alias')
  await symlink(safe.request.data, alias)
  assert.notEqual((await safe.call('preflight', { data: alias, database: path.join(alias, 'my-api.db') })).status, 0)
  await symlink(safe.request.config, path.join(safe.directory, 'config-link'))
  assert.notEqual((await safe.call('preflight', { config: path.join(safe.directory, 'config-link') })).status, 0)
  assert.notEqual((await safe.call('preflight', { backup: path.join(safe.request.data, 'backup') })).status, 0)
  await link(path.join(safe.request.data, 'uploads/history'), path.join(safe.directory, 'hardlink'))
  assert.notEqual((await safe.call('preflight')).status, 0)
})

test('preflight rejects insufficient capacity and malformed requests without creating a backup', async t => {
  const f = await fixture(t)
  const driver = `import runpy,shutil,sys
shutil.disk_usage=lambda _: type('Usage',(),{'free':0})()
runpy.run_path(sys.argv[1],run_name='__main__')
`
  assert.deepEqual((await f.call('preflight', {}, driver)).value, { ok: false, code: 'no_space' })
  await assert.rejects(lstat(f.request.backup), { code: 'ENOENT' })
  for (const extra of [{ device: '01' }, { inode: '0' }, { unknown: true }, { action: 'restore' }]) {
    assert.notEqual((await call({ ...f.request, action: 'preflight', ...extra })).status, 0)
  }
})

test('retains an uncommitted backup if source bytes change during exclusive copying', async t => {
  const f = await fixture(t)
  const driver = `import json,runpy,sys
namespace=runpy.run_path(sys.argv[1],run_name='_native_test')
module=namespace['backup_data']
original=module.bundle_copy
def changing_copy(source,target,maximum,entries):
 original(source,target,maximum,entries)
 if not source.endswith('/data') or '/backups/' in source: return
 with open(source+'/uploads/history','wb') as writer: writer.write(b'changed-during-copy')
module.bundle_copy=changing_copy
try:
 result=namespace['main']()
except namespace['TransitionFailure'] as error:
 print(json.dumps({'ok':False,'code':str(error)}))
 sys.exit(2)
print(json.dumps(result))
`
  assert.deepEqual((await f.call('backup', {}, driver)).value, { ok: false, code: 'source_changed' })
  await assert.rejects(lstat(path.join(f.request.backup, 'COMMITTED')), { code: 'ENOENT' })
  assert.equal((await lstat(f.request.backup)).isDirectory(), true)
  assert.notEqual((await f.call('check-stopped')).status, 0)
})

test('blocks submitted or unknown work and pending financial delivery without replaying it', async t => {
  const f = await fixture(t)
  await f.sql([
    'CREATE TABLE tasks(id INTEGER PRIMARY KEY,status TEXT)',
    "INSERT INTO tasks VALUES(1,'SUCCESS')",
    'CREATE TABLE task_submission_operations(id INTEGER PRIMARY KEY,status TEXT)',
    "INSERT INTO task_submission_operations VALUES(1,'succeeded')",
    'CREATE TABLE task_billing_events(id INTEGER PRIMARY KEY,event_id TEXT,user_id INTEGER,token_id INTEGER,quota_delta INTEGER,payload TEXT,state TEXT,attempt_count INTEGER)',
    "INSERT INTO task_billing_events VALUES(1,'event',1,1,15,'synthetic-applied-payload','applied',0)",
    'CREATE TABLE task_billing_log_outboxes(id INTEGER PRIMARY KEY,billing_event_id TEXT,payload TEXT,state TEXT)',
    "INSERT INTO task_billing_log_outboxes VALUES(1,'event','synthetic-consume-payload','delivered')",
    'CREATE TABLE prompt_learning_runs(id INTEGER PRIMARY KEY,state TEXT)',
    "INSERT INTO prompt_learning_runs VALUES(1,'succeeded')",
    'CREATE TABLE quota_work_cursors(id INTEGER PRIMARY KEY,last_id INTEGER)',
    'INSERT INTO quota_work_cursors VALUES(1,10)',
  ])
  assert.deepEqual((await f.call('check-work')).value, { ok: true, workSettled: true })
  assert.equal((await f.call('backup')).status, 0)
  await f.sql(['UPDATE quota_work_cursors SET last_id=20', 'UPDATE task_billing_events SET attempt_count=1',
    "UPDATE users SET username='renamed-profile'"])
  assert.equal((await f.call('check-live')).value.historicalRowsVerified, true,
    'worker cursors, delivery attempts and user profile metadata are not historical money')
  const cases = [
    ['tasks', 'status', 'SUBMITTED', 'SUCCESS'],
    ['task_submission_operations', 'status', 'submission_unknown', 'succeeded'],
    ['task_billing_events', 'state', 'pending', 'applied'],
    ['task_billing_log_outboxes', 'state', 'retryable', 'delivered'],
    ['prompt_learning_runs', 'state', 'outcome_unknown', 'succeeded'],
  ]
  for (const [index, [table, column, pending, terminal]] of cases.entries()) {
    await f.sql([`UPDATE ${table} SET ${column}='${pending}'`])
    const databaseBefore = await readFile(f.request.database)
    assert.deepEqual((await f.call('check-work')).value, { ok: false, code: 'work_pending' })
    const backup = path.join(path.dirname(f.request.backup), `pending-${index}`)
    assert.deepEqual((await f.call('backup', { backup })).value, { ok: false, code: 'work_pending' })
    await assert.rejects(lstat(path.join(backup, 'COMMITTED')), { code: 'ENOENT' })
    assert.deepEqual(await readFile(f.request.database), databaseBefore)
    await f.sql([`UPDATE ${table} SET ${column}='${terminal}'`])
  }
})

test('does not invent optional modules, and rejects malformed present work schemas', async t => {
  const f = await fixture(t)
  assert.deepEqual((await f.call('check-work')).value, { ok: true, workSettled: true })
  await f.sql(['CREATE TABLE prompt_learning_runs(id INTEGER PRIMARY KEY,unknown TEXT)'])
  assert.deepEqual((await f.call('check-work')).value, { ok: false, code: 'work_unknown' })
})

test('root reads live SQLite as its service owner and never creates root-owned sidecars',
  { skip: process.getuid?.() !== 0 }, async t => {
    const f = await fixture(t)
    await chmod(f.directory, 0o711)
    for (const row of (await contentTree(f.request.data)).reverse()) {
      await chown(row.path === '.' ? f.request.data : path.join(f.request.data, row.path), 12345, 23456)
    }
    assert.equal((await f.call('backup')).status, 0)
    await unlink(f.request.database + '-shm')
    assert.equal((await f.call('check-live')).value.historicalRowsVerified, true)
    for (const suffix of ['', '-wal', '-shm']) {
      const info = await lstat(f.request.database + suffix).catch(error => {
        if (suffix && error.code === 'ENOENT') return null
        throw error
      })
      if (info) assert.equal(info.uid, 12345)
    }
    assert.equal((await lstat(f.request.config)).uid, 0)
    assert.equal((await lstat(f.request.receipt)).uid, 0)
  })
