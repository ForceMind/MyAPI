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

async function fixture(t) {
  const directory = await mkdtemp(path.join(tmpdir(), 'myapi-native-sqlite-'))
  t.after(() => rm(directory, { recursive: true, force: true }))
  await chmod(directory, 0o700)
  const data = path.join(directory, 'data'), backups = path.join(directory, 'backups')
  for (const folder of [data, backups]) await mkdir(folder, { mode: 0o700 })
  const database = path.join(data, 'my-api.db')
  const config = path.join(directory, 'runtime.env'), receipt = path.join(directory, 'installation.json')
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
