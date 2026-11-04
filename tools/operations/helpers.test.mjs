import test from 'node:test'
import assert from 'node:assert/strict'
import { spawn, execFile } from 'node:child_process'
import { promisify } from 'node:util'
import { mkdtemp, mkdir, writeFile, rm, symlink, readFile, stat } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { validateManagementKey, validateActor } from './auth.mjs'
import { python } from './python.mjs'
import { digest } from './safety.mjs'

const run = promisify(execFile)
const secret = 'myapi_mgmt_' + 'a'.repeat(64)

async function directory(t) { const root = await mkdtemp(path.join(os.tmpdir(), 'myapi-helpers-')); t.after(() => rm(root, { recursive: true, force: true })); return root }

test('real SQLite WAL revocation, expiry, scopes, Root role and target restrictions are observed without caches', async t => {
  const root = await directory(t), sqlite_file = path.join(root, 'my-api.db')
  const script = `import sqlite3,json,sys,time\ndb=sqlite3.connect(sys.argv[1])\ndb.execute('PRAGMA journal_mode=WAL')\ndb.execute('PRAGMA wal_autocheckpoint=0')\ndb.execute('CREATE TABLE users(id INTEGER PRIMARY KEY,role INTEGER,status INTEGER,deleted_at TEXT)')\ndb.execute('CREATE TABLE management_keys(id INTEGER PRIMARY KEY,owner_id INTEGER,secret_hash TEXT,scopes_json TEXT,channel_ids_json TEXT,token_ids_json TEXT,revoked_at INTEGER,expires_at INTEGER)')\ndb.execute('INSERT INTO users VALUES(1,100,1,NULL)')\ndb.execute('INSERT INTO management_keys VALUES(7,1,?, ?, ?, ?, 0, ?)',(sys.argv[2],json.dumps(['operations.read','operations.prepare','operations.execute']),'[]','[]',int(time.time())+600))\ndb.commit()\nprint('ready',flush=True)\nfor line in sys.stdin:\n value=json.loads(line)\n db.execute(value['sql'],value.get('args',[]))\n db.commit()\n print('done',flush=True)\n`
  const child = spawn('/usr/bin/python3', ['-I', '-c', script, sqlite_file, digest(secret)], { stdio: ['pipe', 'pipe', 'pipe'] })
  child.stderr.resume(); t.after(() => child.kill('SIGKILL'))
  let output = ''; const waiting = []
  child.stdout.on('data', chunk => { output += chunk; while (output.includes('\n')) { const index = output.indexOf('\n'); const line = output.slice(0, index); output = output.slice(index + 1); const resolve = waiting.shift(); if (resolve) resolve(line) } })
  const next = () => new Promise((resolve, reject) => { const timer = setTimeout(() => reject(new Error('fixture timeout')), 10000); waiting.push(value => { clearTimeout(timer); resolve(value) }) })
  assert.equal(await next(), 'ready')
  async function update(sql, args = []) { const response = next(); child.stdin.write(JSON.stringify({ sql, args }) + '\n'); assert.equal(await response, 'done') }
  const instance = { sqlite_file }
  assert.deepEqual(await validateManagementKey(instance, { secret, scope: 'operations.read' }), { key_id: 7, owner_id: 1 })
  await update('UPDATE management_keys SET revoked_at = 1 WHERE id = 7')
  await assert.rejects(validateManagementKey(instance, { secret, scope: 'operations.read' }), { code: 'UNAUTHORIZED' })
  await update('UPDATE management_keys SET revoked_at = 0, channel_ids_json = ? WHERE id = 7', ['[42]'])
  await assert.rejects(validateManagementKey(instance, { secret, scope: 'operations.execute' }), { code: 'UNAUTHORIZED' })
  await update('UPDATE management_keys SET channel_ids_json = ?, token_ids_json = ? WHERE id = 7', ['[]', '[2]'])
  await assert.rejects(validateManagementKey(instance, { secret, scope: 'operations.execute' }), { code: 'UNAUTHORIZED' })
  await update('UPDATE management_keys SET token_ids_json = ?, scopes_json = ? WHERE id = 7', ['[]', '["operations.prepare"]'])
  await assert.rejects(validateManagementKey(instance, { key_id: 7, owner_id: 1, scope: 'operations.execute' }), { code: 'UNAUTHORIZED' })
  await update('UPDATE users SET role = 10 WHERE id = 1')
  await assert.rejects(validateActor(instance, 1), { code: 'UNAUTHORIZED' })
  await update('UPDATE users SET role = 100, status = 2 WHERE id = 1')
  await assert.rejects(validateActor(instance, 1), { code: 'UNAUTHORIZED' })
  await update('UPDATE users SET status = 1 WHERE id = 1')
  await update('UPDATE management_keys SET expires_at = 1 WHERE id = 7')
  await assert.rejects(validateManagementKey(instance, { secret, scope: 'operations.prepare' }), { code: 'UNAUTHORIZED' })
})

test('fixed ZIP extraction rejects traversal, duplicates, symlink and oversized members', async t => {
  for (const mode of ['good', 'traversal', 'duplicate', 'symlink', 'large']) {
    const root = await directory(t)
    const script = `import sys,zipfile,stat\nmode=sys.argv[2]\nwith zipfile.ZipFile(sys.argv[1]+'/artifact.zip','w') as z:\n z.writestr('images.tar',b'tar')\n if mode=='symlink':\n  x=zipfile.ZipInfo('manifest.json');x.external_attr=(stat.S_IFLNK|0o777)<<16;z.writestr(x,b'/etc/passwd')\n else:z.writestr('../manifest.json' if mode=='traversal' else 'manifest.json',b'x'*20000 if mode=='large' else b'{}')\n if mode=='duplicate':z.writestr('manifest.json',b'{}')\n`
    await run('/usr/bin/python3', ['-I', '-c', script, root, mode])
    if (mode === 'good') { assert.deepEqual(await python('extract_bundle.py', { directory: root, max_bytes: 1024 * 1024 }), { extracted: true }); assert.equal((await readFile(path.join(root, 'images.tar'))).toString(), 'tar') }
    else await assert.rejects(python('extract_bundle.py', { directory: root, max_bytes: 1024 * 1024 }))
  }
})

test('backup retains DB/WAL/SHM bytes, modes and ownership and rejects links without restoring', async t => {
  const root = await directory(t), data = path.join(root, 'data'), destination = path.join(root, 'copy')
  await mkdir(data, { mode: 0o700 })
  for (const suffix of ['', '-wal', '-shm']) await writeFile(path.join(data, 'my-api.db' + suffix), 'live-' + suffix, { mode: 0o640 })
  const result = await python('backup_data.py', { action: 'backup', data_dir: data, sqlite_file: path.join(data, 'my-api.db'), destination, max_bytes: 1024 * 1024 })
  assert.deepEqual(result.before, result.after); assert.deepEqual(result.before, result.copied)
  for (const suffix of ['', '-wal', '-shm']) {
    assert.equal((await readFile(path.join(destination, 'my-api.db' + suffix))).toString(), 'live-' + suffix)
    const original = await stat(path.join(data, 'my-api.db' + suffix)), copy = await stat(path.join(destination, 'my-api.db' + suffix))
    assert.equal(copy.uid, original.uid); assert.equal(copy.gid, original.gid); assert.equal(copy.mode, original.mode)
  }
  await symlink('/etc/passwd', path.join(data, 'external'))
  await assert.rejects(python('backup_data.py', { action: 'backup', data_dir: data, sqlite_file: path.join(data, 'my-api.db'), destination: path.join(root, 'unsafe'), max_bytes: 1024 * 1024 }))
  assert.equal((await readFile(path.join(data, 'my-api.db'))).toString(), 'live-')
})

test('OS daemon lock has one owner and reuses stable inode after orderly release', async t => {
  const { claimDaemon } = await import('./daemon.mjs')
  const root = await directory(t), config = { state_dir: root }
  const first = await claimDaemon(config)
  t.after(() => first.release())
  const initial = await stat(path.join(root, 'daemon.lock'))
  await assert.rejects(claimDaemon(config))
  await first.release()
  const second = await claimDaemon(config)
  t.after(() => second.release())
  const current = await stat(path.join(root, 'daemon.lock'))
  assert.equal(current.ino, initial.ino); assert.equal(current.dev, initial.dev)
  await second.release()
})

test('live summary reads committed WAL consistently while a writer holds an uncommitted transaction', async t => {
  const root = await directory(t), database = path.join(root, 'my-api.db')
  const script = `import sqlite3,sys\ndb=sqlite3.connect(sys.argv[1])\ndb.execute('PRAGMA journal_mode=WAL')\ndb.execute('PRAGMA wal_autocheckpoint=0')\nfor table in ('users','tokens','logs'):\n db.execute('CREATE TABLE '+table+' (id INTEGER PRIMARY KEY)')\n db.execute('INSERT INTO '+table+' VALUES (1)')\ndb.commit()\nprint('ready',flush=True)\nfor line in sys.stdin:\n action=line.strip()\n if action=='begin':\n  db.execute('BEGIN IMMEDIATE')\n  for table in ('users','tokens','logs'):db.execute('INSERT INTO '+table+' VALUES (2)')\n elif action=='commit':db.commit()\n else:raise Exception('unknown fixture command')\n print('done',flush=True)\n`
  const child = spawn('/usr/bin/python3', ['-I', '-c', script, database], { stdio: ['pipe', 'pipe', 'pipe'] })
  child.stderr.resume(); t.after(() => child.kill('SIGKILL'))
  let output = ''; const waiting = []
  child.stdout.on('data', chunk => { output += chunk; while (output.includes('\n')) { const end = output.indexOf('\n'); const value = output.slice(0, end); output = output.slice(end + 1); waiting.shift()?.(value) } })
  const next = () => new Promise((resolve, reject) => { const timer = setTimeout(() => reject(new Error('WAL fixture timeout')), 10000); waiting.push(value => { clearTimeout(timer); resolve(value) }) })
  const send = async action => { const response = next(); child.stdin.write(action + '\n'); assert.equal(await response, 'done') }
  assert.equal(await next(), 'ready')
  assert.ok((await stat(database + '-wal')).size > 0)
  await send('begin')
  const request = { action: 'live_summary', data_dir: root, sqlite_file: database }
  assert.deepEqual(await python('backup_data.py', request), { state: 'observed', counts: { users: 1, tokens: 1, logs: 1 } })
  await send('commit')
  assert.deepEqual(await python('backup_data.py', request), { state: 'observed', counts: { users: 2, tokens: 2, logs: 2 } })
})

test('live summary distinguishes unavailable sampling from confirmed SQLite corruption without fabricating zero counts', async t => {
  const root = await directory(t), database = path.join(root, 'my-api.db')
  await run('/usr/bin/python3', ['-I', '-c', "import sqlite3,sys;db=sqlite3.connect(sys.argv[1]);db.execute('CREATE TABLE users(id INTEGER PRIMARY KEY)');db.commit();db.close()", database])
  // Missing diagnostic tables makes the observation unavailable, rather than
  // inventing empty token/log counts or marking a structurally valid DB corrupt.
  const missing = await python('backup_data.py', { action: 'live_summary', data_dir: root, sqlite_file: database })
  assert.equal(missing.state, 'unknown'); assert.equal(Object.hasOwn(missing, 'counts'), false)
  await writeFile(database, 'not a SQLite database')
  const corrupt = await python('backup_data.py', { action: 'live_summary', data_dir: root, sqlite_file: database })
  assert.equal(corrupt.state, 'corrupt'); assert.equal(Object.hasOwn(corrupt, 'counts'), false)
})

test('live summary reports an exclusive SQLite reader lock as unknown, not corruption', async t => {
  const root = await directory(t), database = path.join(root, 'my-api.db')
  const script = `import sqlite3,sys\ndb=sqlite3.connect(sys.argv[1])\nfor table in ('users','tokens','logs'):db.execute('CREATE TABLE '+table+'(id INTEGER PRIMARY KEY)')\ndb.commit()\ndb.execute('BEGIN EXCLUSIVE')\nprint('locked',flush=True)\nsys.stdin.read()\ndb.rollback()\ndb.close()\n`
  const child = spawn('/usr/bin/python3', ['-I', '-c', script, database], { stdio: ['pipe', 'pipe', 'pipe'] })
  child.stderr.resume(); t.after(() => child.kill('SIGKILL'))
  const ready = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('exclusive lock fixture timeout')), 10000)
    child.stdout.once('data', chunk => { clearTimeout(timer); resolve(String(chunk).trim()) })
  })
  assert.equal(ready, 'locked')
  const result = await python('backup_data.py', { action: 'live_summary', data_dir: root, sqlite_file: database })
  assert.deepEqual(result, { state: 'unknown', code: 'SQLITE_OBSERVATION_UNAVAILABLE' })
})
