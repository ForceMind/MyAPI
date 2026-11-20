import assert from 'node:assert/strict'
import { execFileSync, spawn } from 'node:child_process'
import { createHash, generateKeyPairSync, randomBytes, sign, verify } from 'node:crypto'
import { createReadStream } from 'node:fs'
import { appendFile, copyFile, cp, lstat, mkdir, mkdtemp, readFile, readdir, readlink, realpath, rename, rm, rmdir, stat, truncate, unlink, writeFile } from 'node:fs/promises'
import { createServer } from 'node:net'
import { createServer as createHttpServer } from 'node:http'
import { userInfo } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { installNative } from '../../cli/lib/native-install.mjs'
import { verifyNativePackage, verifyNativeTransitionPackages } from '../../cli/lib/native-package-verification.mjs'
import { executeNativeTransition, planNativeTransition, readNativeTransitionStatus } from '../../cli/lib/native-transition.mjs'
import { executeNativeUninstall, planNativeUninstall } from '../../cli/lib/native-uninstall.mjs'

assert.equal(process.env.GITHUB_ACTIONS, 'true')
assert.equal(process.env.GITHUB_REPOSITORY, 'ForceMind/MyAPI')
assert.equal(process.env.GITHUB_EVENT_NAME, 'workflow_dispatch')
assert.equal(process.env.GITHUB_REF, 'refs/heads/dev')
const [scope, binary, targetBinary, requestedEdition = scope === 'system' ? 'full' : 'lite', executionMode = 'upgrade'] = process.argv.slice(2)
assert.ok(['system', 'user'].includes(scope))
assert.ok(path.isAbsolute(binary))
assert.ok(path.isAbsolute(targetBinary))
assert.ok(['full', 'lite'].includes(requestedEdition))
assert.ok(scope !== 'user' || requestedEdition === 'lite')
assert.ok(['upgrade', 'interrupt'].includes(executionMode))
assert.ok(executionMode !== 'interrupt' || scope === 'system')
const uid = process.getuid()
assert.equal(uid === 0, scope === 'system')
const manager = args => execFileSync('systemctl', [...(scope === 'user' ? ['--user'] : []), ...args], {
  encoding: 'utf8', timeout: 30_000,
})
const shellEntry = fileURLToPath(new URL('../../deploy/install-native.sh', import.meta.url))
const plan = execFileSync('bash', [shellEntry, 'plan', '--scope', scope], { encoding: 'utf8' })
const paths = Object.fromEntries([...plan.matchAll(/^(binary|config|state|data|unit): (\/.+)$/gm)].map(match => [match[1], match[2]]))
assert.equal(Object.keys(paths).length, 5)
if (scope === 'user') {
  const { homedir, username } = userInfo()
  assert.equal(username, 'myapi-ci-personal')
  assert.ok(Object.values(paths).every(filename => filename.startsWith(homedir + '/')))
}
for (const filename of Object.values(paths)) await assert.rejects(stat(filename), { code: 'ENOENT' })

const version = (await readFile(new URL('../../VERSION', import.meta.url), 'utf8')).trim()
// A source-bound CI fixture with a newer version, first inspected read-only,
// then started only in the explicitly authorized upgrade qualification.
const [major, minor, patch] = version.match(/^(\d+)\.(\d+)\.(\d+)/).slice(1)
const targetVersion = `${major}.${minor}.${BigInt(patch) + 1n}`
const edition = requestedEdition
const port = scope === 'system' ? '21951' : '21952'
const hash = createHash('sha256')
for await (const chunk of createReadStream(binary)) hash.update(chunk)
const sha256 = hash.digest('hex')
const scratch = await mkdtemp('/tmp/myapi-native-qualification-')
let fakeUpstream
let fakeUpstreamURL
let fakeUpstreamRequests = 0
const accountingFixture = String.raw`
import json, os, pathlib, sqlite3, stat, sys
from urllib.parse import urlsplit
request = json.load(sys.stdin)
assert os.environ.get('GITHUB_ACTIONS') == 'true'
database = request['database']
assert os.path.isabs(database) and os.path.realpath(database) == database
info = os.lstat(database)
assert stat.S_ISREG(info.st_mode) and info.st_nlink == 1
action = request['action']
assert action in ('seed', 'charge', 'enable-wal', 'charge-new-version', 'snapshot')
db = sqlite3.connect(pathlib.Path(database).as_uri() + ('?mode=ro' if action == 'snapshot' else '?mode=rw'), uri=True, timeout=10)
db.row_factory = sqlite3.Row
try:
    if action == 'seed':
        db.execute('BEGIN IMMEDIATE')
        assert db.execute('SELECT count(*) FROM users WHERE id=900001').fetchone()[0] == 0
        assert db.execute('SELECT count(*) FROM tokens WHERE id=900001').fetchone()[0] == 0
        # Disabled synthetic identities; no login or provider request is made.
        db.execute('INSERT INTO users (id,username,password,display_name,role,status,quota,quota_version,used_quota,request_count,aff_code,setting) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)', (900001,'native-fixture','!synthetic-no-login','Native fixture',1,2,100012,2,11,1,'native-fixture','{}'))
        db.execute('INSERT INTO tokens (id,user_id,key,status,name,created_time,accessed_time,expired_time,remain_quota,used_quota,quota_version,credential_version,unlimited_quota,model_limits_enabled,model_limits,allow_ips,"group",access_profile_id,cross_group_retry,auto_groups) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)', (900001,900001,request['token_secret'],2,'native-synthetic',1700000000,0,-1,89989,11,1,1,0,0,'','','default','standard',0,'[]'))
        db.execute('INSERT INTO admin_quota_adjustments (admin_user_id,target_user_id,request_id,operation,delta,target_quota,applied_quota_before,applied_quota_after,status,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)', (900001,900001,'native-synthetic-adjustment','add',23,100023,100000,100023,'succeeded',1700000000,1700000000))
        db.execute('INSERT INTO logs (user_id,created_at,type,content,username,token_name,model_name,quota,prompt_tokens,completion_tokens,use_time,is_stream,channel_id,token_id,"group",ip,request_id,upstream_request_id,other) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)', (900001,1700000001,2,'synthetic fixture; no provider request','native-fixture','native-synthetic','synthetic-no-provider',11,0,0,0,0,0,900001,'default','127.0.0.1','native-synthetic-baseline','','{"synthetic":true}'))
        db.commit()
    elif action == 'charge':
        db.execute('BEGIN IMMEDIATE')
        assert db.execute('SELECT count(*) FROM logs WHERE request_id=?', ('native-synthetic-after-plan',)).fetchone()[0] == 0
        assert db.execute('UPDATE users SET quota=quota-17,used_quota=used_quota+17,request_count=request_count+1,quota_version=quota_version+1 WHERE id=900001 AND quota=100012 AND used_quota=11').rowcount == 1
        assert db.execute('UPDATE tokens SET remain_quota=remain_quota-17,used_quota=used_quota+17,quota_version=quota_version+1 WHERE id=900001 AND remain_quota=89989 AND used_quota=11').rowcount == 1
        db.execute('INSERT INTO logs (user_id,created_at,type,content,username,token_name,model_name,quota,prompt_tokens,completion_tokens,use_time,is_stream,channel_id,token_id,"group",ip,request_id,upstream_request_id,other) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)', (900001,1700000002,2,'synthetic committed after plan; no provider request','native-fixture','native-synthetic','synthetic-no-provider',17,0,0,0,0,0,900001,'default','127.0.0.1','native-synthetic-after-plan','','{"synthetic":true,"units":17}'))
        db.commit()
    elif action == 'enable-wal':
        # Only called by the owned CI fixture after confirmed real stop, before
        # the product creates its first upgrade backup. No business row changes.
        assert db.execute('PRAGMA journal_mode=WAL').fetchone()[0] == 'wal'
    elif action == 'charge-new-version':
        assert db.execute('PRAGMA journal_mode').fetchone()[0] == 'wal'
        db.execute('PRAGMA wal_autocheckpoint=0')
        db.execute('BEGIN IMMEDIATE')
        upstream = urlsplit(request['upstream_url'])
        assert upstream.scheme == 'http' and upstream.hostname == '127.0.0.1' and 1024 <= upstream.port <= 65535
        assert not upstream.username and not upstream.password and not upstream.query and not upstream.fragment and upstream.path == ''
        assert db.execute('SELECT count(*) FROM channels WHERE id=900001').fetchone()[0] == 0
        db.execute('INSERT INTO channels (id,type,key,status,name,created_time,base_url,models,"group",used_quota,channel_info,settings) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)', (900001,1,'synthetic-local-only-key',1,'native-loopback-fixture',1700000003,request['upstream_url'],'synthetic-no-provider','default',0,'{}','{}'))
        db.execute('INSERT INTO abilities ("group",model,channel_id,enabled,priority,weight) VALUES (?,?,?,?,?,?)', ('default','synthetic-no-provider',900001,1,0,1))
        assert db.execute('SELECT count(*) FROM logs WHERE request_id=?', ('native-synthetic-new-version-19',)).fetchone()[0] == 0
        assert db.execute('UPDATE users SET quota=quota-19,used_quota=used_quota+19,request_count=request_count+1,quota_version=quota_version+1 WHERE id=900001 AND quota=99995 AND used_quota=28').rowcount == 1
        assert db.execute('UPDATE tokens SET remain_quota=remain_quota-19,used_quota=used_quota+19,quota_version=quota_version+1 WHERE id=900001 AND remain_quota=89972 AND used_quota=28').rowcount == 1
        db.execute('INSERT INTO logs (user_id,created_at,type,content,username,token_name,model_name,quota,prompt_tokens,completion_tokens,use_time,is_stream,channel_id,token_id,"group",ip,request_id,upstream_request_id,other) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)', (900001,1700000003,2,'synthetic committed on newer version; no provider request','native-fixture','native-synthetic','synthetic-no-provider',19,0,0,0,0,900001,900001,'default','127.0.0.1','native-synthetic-new-version-19','','{"synthetic":true,"units":19}'))
        db.commit()
    else:
        db.execute('PRAGMA query_only=ON')
    def rows(statement):
        return [dict(row) for row in db.execute(statement)]
    value = {
        'users': rows('SELECT id,username,status,quota,quota_version,used_quota,request_count,self_use_no_balance,usage_policy_revision FROM users WHERE id=900001'),
        'tokens': rows('SELECT id,user_id,name,status,remain_quota,used_quota,quota_version,credential_version,unlimited_quota,model_limits_enabled,model_limits,"group",expired_time FROM tokens WHERE id=900001'),
        'logs': rows('SELECT id,user_id,request_id,token_id,channel_id,quota,prompt_tokens,completion_tokens,type,content,other FROM logs WHERE user_id=900001 ORDER BY id'),
        'adjustments': rows('SELECT * FROM admin_quota_adjustments WHERE target_user_id=900001 ORDER BY id'),
        'options': rows('SELECT "key",value FROM options ORDER BY "key"'),
        'channels': rows('SELECT id,type,status,base_url,models,used_quota FROM channels WHERE id=900001'),
        'abilities': rows('SELECT "group",model,channel_id,enabled,priority,weight FROM abilities WHERE channel_id=900001'),
    }
    if action == 'enable-wal': value['journal_mode'] = db.execute('PRAGMA journal_mode').fetchone()[0]
    assert len(value['users']) == 1 and len(value['tokens']) == 1 and len(value['adjustments']) == 1
    print(json.dumps(value,separators=(',',':')))
finally:
    db.close()
`
try {
  const manifest = path.join(scratch, 'manifest.json')
  const bytes = Buffer.from(JSON.stringify({
    schema_version: 1,
    product: { name: 'My API', version, source_sha: process.env.GITHUB_SHA, build_revision: process.env.GITHUB_SHA },
    bootstrap: { min_version: version, max_version: version },
    compatibility: { config_schema: '1.0.0', data_schema: '1.0.0', supported_from: [version], rollback: 'required_manual' },
    artifacts: [{
      id: 'my-api-test', edition, installation_shape: scope === 'system' ? 'server-native' : 'personal-native',
      os: 'linux', arch: 'amd64', kind: 'binary', sha256_or_digest: sha256, signature: 'qualification-fixture',
      location: `https://github.com/ForceMind/MyAPI/releases/download/v${version}/my-api-test`,
      sbom_or_provenance: `https://github.com/ForceMind/MyAPI/releases/download/v${version}/provenance.json`,
    }],
  }))
  const key = generateKeyPairSync('ed25519')
  const signature = sign(null, bytes, key.privateKey)
  await writeFile(manifest, bytes)
  const inputs = { scope, edition, port, tag: `v${version}`, manifest, package: binary }
  const verifyFixture = staged => {
    assert.notEqual(staged.package, binary)
    return verifyNativePackage(staged, { arch: 'amd64', bootstrapVersion: version, verifySignature(raw) {
      if (!verify(null, raw, key.publicKey, signature)) {
        throw Object.assign(new Error('Fixture signature rejected'), { code: 'signature_rejected' })
      }
    } })
  }

  await mkdir(path.dirname(paths.config), { recursive: true, mode: 0o700 })
  const sentinel = Buffer.from('existing config bytes')
  await writeFile(paths.config, sentinel, { flag: 'wx', mode: 0o600 })
  await assert.rejects(installNative(inputs, verifyFixture), { code: 'installation_conflict' })
  assert.deepEqual(await readFile(paths.config), sentinel)
  await unlink(paths.config)
  await rmdir(path.dirname(paths.config))

  const listener = createServer()
  await new Promise((resolve, reject) => {
    listener.once('error', reject)
    listener.listen(Number(port), '127.0.0.1', resolve)
  })
  try {
    await assert.rejects(installNative(inputs, verifyFixture), { code: 'port_conflict' })
  } finally {
    await new Promise(resolve => listener.close(resolve))
  }
  await writeFile(manifest, Buffer.concat([bytes, Buffer.from(' ')]))
  await assert.rejects(installNative(inputs, verifyFixture), { code: 'signature_rejected' })
  for (const filename of Object.values(paths)) await assert.rejects(stat(filename), { code: 'ENOENT' })
  await writeFile(manifest, bytes)

  const foreignSettings = ['SQL_DSN=qualification-invalid-database', 'LOG_SQL_DSN=qualification-invalid-log-database',
    'REDIS_CONN_STRING=qualification-invalid-redis']
  if (scope === 'user') manager(['set-environment', ...foreignSettings])
  let installed
  try {
    installed = await installNative(inputs, verifyFixture)
  } finally {
    if (scope === 'user') manager(['unset-environment', ...foreignSettings.map(setting => setting.split('=')[0])])
  }
  assert.equal(installed.status, 'healthy')
  assert.equal(installed.sha256, sha256)
  const receiptBytes = await readFile(path.join(paths.state, 'installation.json'))
  const receipt = JSON.parse(receiptBytes)
  assert.equal(receipt.sha256, sha256)
  assert.equal(receipt.sourceSha, process.env.GITHUB_SHA)
  assert.equal(receipt.edition, edition)
  assert.ok((await stat(path.join(paths.data, 'my-api.db'))).isFile())
  assert.equal((await stat(paths.config)).mode & 0o777, 0o600)
  const config = await readFile(paths.config, 'utf8')
  assert.match(config, /^SESSION_SECRET=[a-f0-9]{96}$/m)
  const unitBefore = await readFile(paths.unit)
  const pidBefore = manager(['show', 'my-api.service', '--property=MainPID', '--value'])
  await assert.rejects(installNative(inputs, verifyFixture), { code: 'installation_conflict' })
  assert.equal(await readFile(paths.config, 'utf8'), config)
  assert.deepEqual(await readFile(paths.unit), unitBefore)
  assert.equal(manager(['show', 'my-api.service', '--property=MainPID', '--value']), pidBefore)

  const currentBundle = path.join(scratch, 'current-bundle.json')
  const targetManifest = path.join(scratch, 'target-manifest.json')
  const targetBundle = path.join(scratch, 'target-bundle.json')
  await writeFile(currentBundle, JSON.stringify({ signature: signature.toString('base64') }))
  const targetHash = createHash('sha256')
  for await (const chunk of createReadStream(targetBinary)) targetHash.update(chunk)
  const targetSha256 = targetHash.digest('hex')
  assert.notEqual(targetSha256, sha256, 'The newer source-built fixture must be a distinct binary')
  const targetDocument = JSON.parse(bytes)
  targetDocument.schema_version = 2
  targetDocument.product.version = targetVersion
  targetDocument.compatibility.supported_from = [version]
  targetDocument.compatibility.rollback = 'code_only'
  targetDocument.native_transitions = [{
    from: { version, source_sha: receipt.sourceSha, artifact_id: receipt.artifact, sha256 },
    to_artifact_id: receipt.artifact, storage: 'sqlite', allow_code_rollback: true,
  }]
  const targetArtifact = targetDocument.artifacts[0]
  targetArtifact.sha256_or_digest = targetSha256
  targetArtifact.location = `https://github.com/ForceMind/MyAPI/releases/download/v${targetVersion}/my-api-test`
  targetArtifact.sbom_or_provenance = `https://github.com/ForceMind/MyAPI/releases/download/v${targetVersion}/provenance.json`
  const targetKey = generateKeyPairSync('ed25519')
  async function writeTarget(document) {
    const raw = Buffer.from(JSON.stringify(document))
    await writeFile(targetManifest, raw)
    await writeFile(targetBundle, JSON.stringify({ signature: sign(null, raw, targetKey.privateKey).toString('base64') }))
  }
  await writeTarget(targetDocument)
  const transitionInputs = {
    scope, edition, tag: `v${targetVersion}`, manifest: targetManifest, bundle: targetBundle, package: targetBinary,
    'current-manifest': manifest, 'current-bundle': currentBundle,
    // These test-only paths cannot be consumed by the production pinned
    // verifier. Only the explicitly injected Ed25519 fixture verifier is used.
    cosign: '/qualification-fixture/not-production-cosign', root: '/qualification-fixture/not-production-root',
  }
  const authenticatedTags = []
  const verifyTransitionFixture = (requested, identity, currentBinary, operation) =>
    verifyNativeTransitionPackages(requested, identity, currentBinary, operation, {
      arch: 'amd64', bootstrapVersion: version,
      async verifySignature(raw, signedInputs, _arch, tag) {
        const trustedKey = tag === `v${version}` ? key.publicKey : targetKey.publicKey
        const bundle = JSON.parse(await readFile(signedInputs.bundle, 'utf8'))
        if (!verify(null, raw, trustedKey, Buffer.from(bundle.signature, 'base64'))) {
          throw Object.assign(new Error('Fixture signature rejected'), { code: 'signature_rejected' })
        }
        authenticatedTags.push(tag)
      },
    })

  async function qualificationPlan(stage) {
    console.log(`MYAPI_NATIVE_PLAN_STAGE scope=${scope} stage=${stage} result=started`)
    try {
      const result = await planNativeTransition(transitionInputs, verifyTransitionFixture)
      console.log(`MYAPI_NATIVE_PLAN_STAGE scope=${scope} stage=${stage} result=accepted`)
      return result
    } catch (error) {
      console.error(`MYAPI_NATIVE_PLAN_STAGE scope=${scope} stage=${stage} result=rejected code=${/^[a-z_]+$/.test(error.code ?? '') ? error.code : 'unknown'}`)
      throw error
    }
  }

  const database = path.join(paths.data, 'my-api.db')
  // A personal service can inherit these two ACLs even on a mode-0700 data
  // directory. Keep any inherited ACL. Supply a masked, fixture-only ACL
  // when the disposable runner's new home has none.
  function personalDataACLs(directory, prepare = false) {
    const inspect = String.raw`
import hashlib, json, os, stat, struct, sys
directory, operation = sys.argv[1:]
assert os.environ['GITHUB_ACTIONS'] == 'true' and os.geteuid() != 0
assert os.path.realpath(directory) == directory and os.path.commonpath([os.environ['HOME'], directory]) == os.environ['HOME']
fd = os.open(directory, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
try:
    info = os.fstat(fd)
    assert info.st_uid == os.geteuid() and stat.S_IMODE(info.st_mode) == 0o700
    checks = {}
    for index, name in enumerate(('system.posix_acl_access', 'system.posix_acl_default')):
        if operation == 'prepare' and name not in os.listxattr(fd):
            undefined = 0xffffffff
            entries = ((1, 7, undefined), (2, 0, 80001 + index), (4, 0, undefined), (16, 0, undefined), (32, 0, undefined))
            acl = struct.pack('<I', 2) + b''.join(struct.pack('<HHI', *entry) for entry in entries)
            os.setxattr(fd, name, acl, os.XATTR_CREATE)
        checks[name] = hashlib.sha256(os.getxattr(fd, name)).hexdigest()
    print(json.dumps(checks))
finally:
    os.close(fd)
`
    return JSON.parse(execFileSync('/usr/bin/python3', ['-I', '-B', '-c', inspect, directory, prepare ? 'prepare' : 'inspect'], { encoding: 'utf8' }))
  }
  const personalPermissions = scope === 'user' ? personalDataACLs(paths.data, true) : null
  const databaseIdentity = await stat(database, { bigint: true })
  const pid = Number(pidBefore.trim())
  assert.ok(Number.isSafeInteger(pid) && pid > 0)
  const processBinaryBefore = await stat(`/proc/${pid}/exe`, { bigint: true })
  const currentBinaryBefore = await realpath(paths.binary)
  const descriptors = await readdir(`/proc/${pid}/fd`)
  const openedDatabase = await Promise.all(descriptors.map(async descriptor => {
    try {
      const entry = await stat(`/proc/${pid}/fd/${descriptor}`, { bigint: true })
      return entry.dev === databaseIdentity.dev && entry.ino === databaseIdentity.ino
    } catch (error) {
      if (error.code !== 'ENOENT') throw error
      return false
    }
  }))
  assert.ok(openedDatabase.some(Boolean), 'The running service must hold this actual SQLite inode')

  // Hold a write reservation without writing. This prevents periodic backend
  // workers from changing SQLite/WAL bytes while the read-only plan is checked.
  // It does not freeze or restart the running service and permits SQLite reads.
  const databaseGuard = spawn('python3', ['-u', '-c', [
    'import json, os, sqlite3, sys',
    'database = sys.argv[1]',
    'connection = sqlite3.connect("file:" + database + "?mode=rw", uri=True, timeout=10)',
    'connection.execute("BEGIN IMMEDIATE")',
    'identity = os.stat(database)',
    'print(json.dumps({"database": connection.execute("PRAGMA database_list").fetchone()[2], "inode": str(identity.st_ino), "device": str(identity.st_dev)}), flush=True)',
    'try:',
    '    sys.stdin.readline()',
    'finally:',
    '    connection.rollback()',
    '    connection.close()',
  ].join('\n'), database], { stdio: ['pipe', 'pipe', 'pipe'], timeout: 120_000 })
  let guardError = ''
  databaseGuard.stderr.on('data', chunk => { guardError = (guardError + chunk).slice(-4096) })
  const guardExited = new Promise((resolve, reject) => {
    databaseGuard.once('error', reject)
    databaseGuard.once('exit', (code, signal) => resolve({ code, signal }))
  })
  try {
    const guard = await Promise.race([
      new Promise((resolve, reject) => {
        let output = ''
        databaseGuard.stdout.on('data', chunk => {
          output += chunk
          if (output.includes('\n')) {
            try { resolve(JSON.parse(output.trim())) } catch (error) { reject(error) }
          }
        })
      }),
      guardExited.then(result => { throw new Error(`SQLite reservation failed: ${JSON.stringify(result)} ${guardError}`) }),
    ])
    assert.equal(guard.database, database)
    assert.equal(guard.inode, String(databaseIdentity.ino))
    assert.equal(guard.device, String(databaseIdentity.dev))

    const allPaths = Object.fromEntries([...plan.matchAll(/^(\w+): (\/.+)$/gm)].map(match => [match[1], match[2]]))
    const persistentRoots = [...new Set([path.dirname(allPaths.releases), path.dirname(paths.config), paths.data,
      paths.state, allPaths.backups, paths.unit, scratch])]
    // Logs and disposable cache are autonomous worker output. SQLite shared-
    // memory read-lock slots can change during inspection. Their exclusion is
    // explicit; the database,
    // WAL/journal, config, receipts, releases, backups and unit stay byte-exact.
    const excluded = new Set([allPaths.logs, allPaths.cache, `${database}-shm`])
    async function persistentSnapshot() {
      const entries = {}
      async function visit(filename) {
        if (excluded.has(filename)) return
        let info
        try { info = await lstat(filename, { bigint: true }) } catch (error) {
          if (error.code !== 'ENOENT') throw error
          entries[filename] = null
          return
        }
        const metadata = [String(info.dev), String(info.ino), String(info.mode), String(info.uid), String(info.gid)]
        if (info.isSymbolicLink()) entries[filename] = [...metadata, 'symlink', await readlink(filename)]
        else if (info.isDirectory()) {
          entries[filename] = [...metadata, 'directory']
          for (const name of (await readdir(filename)).sort()) await visit(path.join(filename, name))
        } else {
          assert.ok(info.isFile(), 'Only regular files and explicit installation links are expected')
          const digest = createHash('sha256')
          for await (const chunk of createReadStream(filename)) digest.update(chunk)
          entries[filename] = [...metadata, 'file', String(info.size), digest.digest('hex')]
        }
      }
      for (const root of persistentRoots) await visit(root)
      return entries
    }
    async function readOnlyCheck(action) {
      const before = await persistentSnapshot()
      try { return await action() } finally {
        assert.deepEqual(await persistentSnapshot(), before, 'Planning must not alter persistent installation or fixture bytes')
        assert.equal(manager(['show', 'my-api.service', '--property=MainPID', '--value']), pidBefore)
        assert.equal(manager(['show', 'my-api.service', '--property=ActiveState', '--value']).trim(), 'active')
        assert.equal(await realpath(paths.binary), currentBinaryBefore)
        const executable = await stat(`/proc/${pid}/exe`, { bigint: true })
        assert.equal(executable.dev, processBinaryBefore.dev)
        assert.equal(executable.ino, processBinaryBefore.ino)
      }
    }
    const qualified = await readOnlyCheck(() => qualificationPlan('initial-plan'))
    assert.equal(qualified.eligible, true)
    assert.equal(qualified.operation, 'upgrade')
    assert.equal(qualified.scope, scope)
    assert.equal(qualified.edition, edition)
    assert.equal(qualified.current.version, version)
    assert.equal(qualified.current.sha256, sha256)
    assert.equal(qualified.target.version, targetVersion)
    assert.equal(qualified.target.sha256, targetSha256)
    assert.equal(qualified.database, database)
    assert.equal(qualified.service.name, 'my-api.service')
    assert.equal(qualified.service.pid, pid)
    assert.ok(Array.isArray(qualified.steps) && qualified.steps.length > 0)
    assert.deepEqual(new Set(authenticatedTags), new Set([`v${version}`, `v${targetVersion}`]))

    // The installed command goes through the pinned verifier, even with the
    // very same fixture that the isolated integration seam accepted.
    const testRoot = path.join(scratch, 'untrusted-root.json')
    await writeFile(testRoot, '{}')
    await readOnlyCheck(() => {
      const cliArgs = Object.entries({ ...transitionInputs, root: testRoot }).flatMap(([key, value]) => ['--' + key, value])
      assert.throws(() => execFileSync('bash', [shellEntry, 'upgrade-plan', ...cliArgs],
        { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }), error => {
        assert.equal(error.status, 2)
        const refusal = JSON.parse(error.stderr)
        assert.equal(refusal.eligible, false)
        assert.equal(refusal.code, 'trust_root_mismatch')
        return true
      })
    })

    await writeFile(manifest, Buffer.concat([bytes, Buffer.from(' ')]))
    try {
      await readOnlyCheck(() => assert.rejects(planNativeTransition(transitionInputs, verifyTransitionFixture), { code: 'signature_rejected' }))
    } finally { await writeFile(manifest, bytes) }

    const wrongDigest = structuredClone(targetDocument)
    wrongDigest.artifacts[0].sha256_or_digest = '0'.repeat(64)
    await writeTarget(wrongDigest)
    try {
      await readOnlyCheck(() => assert.rejects(planNativeTransition(transitionInputs, verifyTransitionFixture), { code: 'digest_mismatch' }))
    } finally { await writeTarget(targetDocument) }

    const wrongDeclaration = structuredClone(targetDocument)
    wrongDeclaration.native_transitions[0].from.sha256 = '0'.repeat(64)
    await writeTarget(wrongDeclaration)
    try {
      await readOnlyCheck(() => assert.rejects(planNativeTransition(transitionInputs, verifyTransitionFixture), { code: 'native_transition_missing' }))
    } finally { await writeTarget(targetDocument) }

    const receiptPath = path.join(paths.state, 'installation.json')
    const originalReceipt = await readFile(receiptPath)
    const incomplete = JSON.parse(originalReceipt)
    delete incomplete.artifact
    await writeFile(receiptPath, JSON.stringify(incomplete))
    try {
      await readOnlyCheck(() => assert.rejects(planNativeTransition(transitionInputs, verifyTransitionFixture), { code: 'invalid_installation_record' }))
    } finally { await writeFile(receiptPath, originalReceipt) }

    await writeFile(receiptPath, JSON.stringify({ ...receipt, artifact: 'other-managed-release' }))
    try {
      await readOnlyCheck(() => assert.rejects(planNativeTransition(transitionInputs, verifyTransitionFixture), { code: 'native_service_mismatch' }))
    } finally { await writeFile(receiptPath, originalReceipt) }

    // The installed service keeps its original inode open. A config pointing
    // elsewhere must be rejected without trying to connect to that database.
    const otherDatabase = path.join(scratch, 'other-database.db')
    await writeFile(otherDatabase, await readFile(database), { flag: 'wx', mode: 0o600 })
    assert.notEqual((await stat(otherDatabase, { bigint: true })).ino, databaseIdentity.ino)
    await writeFile(paths.config, config.replace(/^SQL_DSN=$/m, `SQL_DSN=${otherDatabase}`))
    try {
      await readOnlyCheck(() => assert.rejects(planNativeTransition(transitionInputs, verifyTransitionFixture), { code: 'native_sqlite_mismatch' }))
    } finally { await writeFile(paths.config, config) }

    const displacedDatabase = `${database}.qualification-held`
    await rename(database, displacedDatabase)
    try {
      await readOnlyCheck(() => assert.rejects(planNativeTransition(transitionInputs, verifyTransitionFixture), { code: 'native_sqlite_mismatch' }))
    } finally { await rename(displacedDatabase, database) }

    // A complete pre-existing schema-1 first-install record still works after
    // all refusal cases; this batch neither enriches it nor starts the target.
    const requalified = await readOnlyCheck(() => qualificationPlan('requalification'))
    assert.equal(requalified.current.version, version)
    assert.deepEqual(await readFile(receiptPath), originalReceipt)
    assert.equal(await readFile(paths.config, 'utf8'), config)
    assert.deepEqual(await readFile(paths.unit), unitBefore)
    const currentStatus = await readOnlyCheck(async () => {
      const response = await fetch(`http://127.0.0.1:${port}/api/status`, {
        redirect: 'error', signal: AbortSignal.timeout(2_000),
      })
      assert.equal(response.ok, true)
      return response.json()
    })
    assert.equal(currentStatus.success, true)
    assert.equal(currentStatus.data.version, version)
    assert.equal(currentStatus.data.product_edition, edition)
    console.log(`MYAPI_NATIVE_TRANSITION_PLAN_PASS scope=${scope} edition=${edition} current_schema=1 target_schema=2 dual_signature=true actual_sqlite_inode=true readonly=true switched=false target_started=false fixture=local-ed25519-source-build excluded=autonomous-logs,cache,sqlite-shm sqlite_write_reservation=true`)
  } finally {
    databaseGuard.stdin.end('\n')
    const result = await guardExited
    assert.deepEqual(result, { code: 0, signal: null }, guardError)
  }

  const databaseOwner = await stat(database)
  const accounting = (action, filename = database) => JSON.parse(execFileSync('python3', ['-I', '-B', '-c', accountingFixture], {
    encoding: 'utf8', timeout: 30_000,
    input: JSON.stringify({ action, database: filename, ...(action === 'seed' ? { token_secret: randomBytes(32).toString('hex') } : {}),
      ...(action === 'charge-new-version' ? { upstream_url: fakeUpstreamURL } : {}) }),
    ...(scope === 'system' && (action !== 'snapshot' || filename === database) ? { uid: databaseOwner.uid, gid: databaseOwner.gid } : {}),
  }))
  const seededAccounting = accounting('seed')
  assert.equal(seededAccounting.users[0].quota, 100012)
  assert.equal(seededAccounting.users[0].used_quota, 11)
  assert.equal(seededAccounting.tokens[0].remain_quota, 89989)
  assert.equal(seededAccounting.logs.length, 1)
  assert.equal(seededAccounting.adjustments[0].delta, 23)

  const candidateBinary = path.join(scratch, 'execution-candidate')
  await copyFile(targetBinary, candidateBinary)
  const executionInputs = { ...transitionInputs, package: candidateBinary }
  const executionPaths = Object.fromEntries([...plan.matchAll(/^(\w+): (\/.+)$/gm)].map(match => [match[1], match[2]]))
  const stateFile = path.join(paths.state, 'native-transition.json')
  async function executionSnapshot({ includeAutonomous = false } = {}) {
    const files = {}
    async function visit(filename) {
      // The user release root also contains live data. Verify synthetic
      // accounting below; exclude autonomous task rows and SQLite lock bytes
      // rather than take an unsafe byte snapshot of a running database.
      if (filename === paths.data || (!includeAutonomous && [executionPaths.logs, executionPaths.cache].includes(filename))) return
      let info
      try { info = await lstat(filename, { bigint: true }) } catch (error) {
        if (error.code !== 'ENOENT') throw error
        files[filename] = null
        return
      }
      const metadata = [String(info.dev), String(info.ino), String(info.mode), String(info.uid), String(info.gid)]
      if (info.isDirectory()) {
        files[filename] = [...metadata, 'directory']
        for (const name of (await readdir(filename)).sort()) await visit(path.join(filename, name))
      } else if (info.isSymbolicLink()) files[filename] = [...metadata, 'symlink', await readlink(filename)]
      else {
        assert.ok(info.isFile())
        const digest = createHash('sha256')
        for await (const chunk of createReadStream(filename)) digest.update(chunk)
        files[filename] = [...metadata, String(info.size), digest.digest('hex')]
      }
    }
    for (const filename of [path.dirname(executionPaths.releases), paths.config, paths.unit, paths.state, executionPaths.backups]) await visit(filename)
    if (includeAutonomous) {
      await visit(executionPaths.logs)
      await visit(executionPaths.cache)
    }
    return { files, accounting: accounting('snapshot'),
      pid: manager(['show', 'my-api.service', '--property=MainPID', '--value']),
      active: manager(['show', 'my-api.service', '--property=ActiveState', '--value']),
      activeBinary: await realpath(paths.binary) }
  }
  async function unchangedExecution(action) {
    const before = await executionSnapshot()
    try { return await action() } finally {
      assert.deepEqual(await executionSnapshot(), before, 'A refused or repeated confirmation must not stop, restart, back up or change accounting')
    }
  }
  const approval = await planNativeTransition(executionInputs, verifyTransitionFixture)
  assert.match(approval.confirmation, /^[a-f0-9]{64}$/)
  await unchangedExecution(() => assert.rejects(executeNativeTransition(executionInputs, verifyTransitionFixture), { code: 'native_confirmation_required' }))
  await writeFile(paths.config, config + '# qualification changed after confirmation\n')
  try {
    await unchangedExecution(() => assert.rejects(executeNativeTransition({ ...executionInputs, confirm: approval.confirmation }, verifyTransitionFixture), { code: 'native_confirmation_stale' }))
  } finally { await writeFile(paths.config, config) }

  const targetBytes = await readFile(targetManifest)
  await appendFile(targetManifest, ' ')
  try {
    await unchangedExecution(() => assert.rejects(executeNativeTransition({ ...executionInputs, confirm: approval.confirmation }, verifyTransitionFixture), { code: 'signature_rejected' }))
  } finally { await writeFile(targetManifest, targetBytes) }
  const candidateSize = (await stat(candidateBinary)).size
  await appendFile(candidateBinary, 'qualification digest changed after plan')
  try {
    await unchangedExecution(() => assert.rejects(executeNativeTransition({ ...executionInputs, confirm: approval.confirmation }, verifyTransitionFixture), { code: 'digest_mismatch' }))
  } finally { await truncate(candidateBinary, candidateSize) }

  const lockDirectory = path.join(scope === 'system' ? '/run' : `/run/user/${uid}`, 'myapi-native-install')
  await mkdir(lockDirectory, { mode: 0o700 })
  try {
    await unchangedExecution(() => assert.rejects(executeNativeTransition({ ...executionInputs, confirm: approval.confirmation }, verifyTransitionFixture), { code: 'installation_busy' }))
  } finally { await rmdir(lockDirectory) }

  const executionPlan = await planNativeTransition(executionInputs, verifyTransitionFixture)
  assert.match(executionPlan.confirmation, /^[a-f0-9]{64}$/)
  const latestAccounting = accounting('charge')
  assert.equal(latestAccounting.users[0].quota, 99995)
  assert.equal(latestAccounting.users[0].used_quota, 28)
  assert.equal(latestAccounting.tokens[0].remain_quota, 89972)
  assert.equal(latestAccounting.tokens[0].used_quota, 28)
  assert.equal(latestAccounting.logs.length, 2)
  assert.equal(latestAccounting.logs[1].request_id, 'native-synthetic-after-plan')
  assert.deepEqual(latestAccounting.adjustments, seededAccounting.adjustments)
  const confirmedInputs = { ...executionInputs, confirm: executionPlan.confirmation }
  if (executionMode === 'interrupt') {
    const interruptPayload = path.join(scratch, 'interrupt-input.json')
    await writeFile(interruptPayload, JSON.stringify({ inputs: confirmedInputs, version,
      currentKey: key.publicKey.export({ type: 'spki', format: 'pem' }),
      targetKey: targetKey.publicKey.export({ type: 'spki', format: 'pem' }) }), { flag: 'wx', mode: 0o600 })
    const interruptProgram = String.raw`
      import { readFile } from 'node:fs/promises';
      import { verify } from 'node:crypto';
      const { executeNativeTransition } = await import(process.argv[1]);
      const { verifyNativeTransitionPackages } = await import(process.argv[2]);
      const fixture = JSON.parse(await readFile(process.argv[3], 'utf8'));
      const authenticated = (inputs, installed, currentBinary, operation) => verifyNativeTransitionPackages(inputs, installed, currentBinary, operation, {
        arch: 'amd64', bootstrapVersion: fixture.version,
        async verifySignature(raw, inputs, arch, tag) {
          const bundle = JSON.parse(await readFile(inputs.bundle, 'utf8'));
          const key = tag === 'v' + fixture.version ? fixture.currentKey : fixture.targetKey;
          if (!verify(null, raw, key, Buffer.from(bundle.signature, 'base64'))) throw Object.assign(new Error('Fixture signature rejected'), { code: 'signature_rejected' });
        }
      });
      await executeNativeTransition(fixture.inputs, authenticated, { onPhase: async phase => {
        if (phase === 'stopped') process.exit(73);
      }});
      process.exit(74);
    `
    assert.throws(() => execFileSync(process.execPath, ['--input-type=module', '--eval', interruptProgram,
      new URL('../../cli/lib/native-transition.mjs', import.meta.url).href,
      new URL('../../cli/lib/native-package-verification.mjs', import.meta.url).href, interruptPayload], {
      encoding: 'utf8', timeout: 120_000, stdio: ['ignore', 'pipe', 'pipe'],
    }), error => {
      assert.equal(error.status, 73, error.stderr)
      return true
    })
    const interrupted = await readNativeTransitionStatus(scope)
    assert.equal(interrupted.phase, 'stopped')
    assert.equal(interrupted.service.state, 'inactive')
    assert.equal(Number(interrupted.service.pid), 0)
    assert.equal(interrupted.activeBinary, currentBinaryBefore)
    const interruptedDatabase = await stat(database, { bigint: true })
    assert.equal(interruptedDatabase.dev, databaseIdentity.dev)
    assert.equal(interruptedDatabase.ino, databaseIdentity.ino)
    assert.deepEqual(accounting('snapshot'), latestAccounting)
    assert.equal(await readFile(paths.config, 'utf8'), config)
    assert.deepEqual(await readFile(paths.unit), unitBefore)
    await unchangedExecution(async () => {
      assert.deepEqual(await readNativeTransitionStatus(scope), interrupted)
      assert.deepEqual(JSON.parse(execFileSync('bash', [shellEntry, 'transition-status', '--scope', scope], { encoding: 'utf8' })), interrupted)
      await assert.rejects(executeNativeTransition(confirmedInputs, verifyTransitionFixture), { code: 'native_transition_pending' })
      await assert.rejects(planNativeUninstall({ scope }), { code: 'native_transition_pending' })
    })
    assert.equal(JSON.parse(await readFile(stateFile, 'utf8')).phase, 'stopped')
    console.log(`MYAPI_NATIVE_TRANSITION_INTERRUPT_PASS scope=${scope} edition=${edition} real_service=true persisted_phase=stopped child_exit=73 replay_refused=true restarted=false accounting_preserved=true manual_state_preserved=true`)
  } else {
    let fixtureWAL = false
    const upgraded = await executeNativeTransition(confirmedInputs, verifyTransitionFixture, { onPhase: async phase => {
      if (phase !== 'stopped') return
      assert.equal(manager(['show', 'my-api.service', '--property=MainPID', '--value']).trim(), '0')
      assert.equal(manager(['show', 'my-api.service', '--property=ActiveState', '--value']).trim(), 'inactive')
      const enabled = accounting('enable-wal')
      assert.equal(enabled.journal_mode, 'wal')
      delete enabled.journal_mode
      assert.deepEqual(enabled, latestAccounting, 'WAL preparation must not change business rows or switches')
      assert.equal(await readFile(paths.config, 'utf8'), config)
      assert.deepEqual(await readFile(paths.unit), unitBefore)
      fixtureWAL = true
    } })
    assert.equal(fixtureWAL, true)
    assert.equal(upgraded.status, 'healthy')
    assert.equal(upgraded.operation, 'upgrade')
    assert.equal(upgraded.current.version, version)
    assert.equal(upgraded.target.version, targetVersion)
    assert.equal(upgraded.database, database)
    assert.equal(upgraded.accountingVerified, true)
    assert.ok(path.isAbsolute(upgraded.backup))
    const databaseAfter = await stat(database, { bigint: true })
    assert.equal(databaseAfter.dev, databaseIdentity.dev)
    assert.equal(databaseAfter.ino, databaseIdentity.ino)
    assert.deepEqual(accounting('snapshot'), latestAccounting)
    assert.equal(await readFile(paths.config, 'utf8'), config)
    assert.deepEqual(await readFile(paths.unit), unitBefore)
    const currentStatus = await readNativeTransitionStatus(scope)
    assert.equal(currentStatus.phase, 'committed')
    assert.equal(currentStatus.service.state, 'active')
    const newPid = Number(manager(['show', 'my-api.service', '--property=MainPID', '--value']).trim())
    assert.ok(newPid > 0 && newPid !== pid)
    assert.equal(currentStatus.service.pid, newPid)
    assert.equal(currentStatus.activeBinary, await realpath(paths.binary))
    assert.notEqual(currentStatus.activeBinary, currentBinaryBefore)
    const targetExecutable = await stat(`/proc/${newPid}/exe`, { bigint: true })
    const installedExecutable = await stat(paths.binary, { bigint: true })
    assert.equal(targetExecutable.dev, installedExecutable.dev)
    assert.equal(targetExecutable.ino, installedExecutable.ino)
    const statusResponse = await fetch(`http://127.0.0.1:${port}/api/status`, { redirect: 'error', signal: AbortSignal.timeout(2_000) })
    assert.equal(statusResponse.ok, true)
    const statusBody = await statusResponse.json()
    assert.equal(statusBody.success, true)
    assert.equal(statusBody.data.version, targetVersion)
    assert.equal(statusBody.data.product_edition, edition)
    const newReceipt = JSON.parse(await readFile(path.join(paths.state, 'installation.json'), 'utf8'))
    assert.equal(newReceipt.version, targetVersion)
    assert.equal(newReceipt.sha256, targetSha256)
    // Read only a disposable copy: SQLite lock/SHM coordination must not alter
    // the committed backup tree while proving the after-plan record is there.
    const backupInspection = path.join(scratch, 'backup-inspection')
    await cp(path.join(upgraded.backup, 'data'), backupInspection, { recursive: true, errorOnExist: true, force: false })
    assert.deepEqual(accounting('snapshot', path.join(backupInspection, path.relative(paths.data, database))), latestAccounting)
    assert.deepEqual(await readFile(path.join(upgraded.backup, 'config')), Buffer.from(config))
    assert.deepEqual(await readFile(path.join(upgraded.backup, 'receipt')), receiptBytes)
    if (personalPermissions) {
      assert.deepEqual(personalDataACLs(paths.data), personalPermissions)
      assert.deepEqual(personalDataACLs(path.join(upgraded.backup, 'data')), personalPermissions)
      console.log('MYAPI_NATIVE_ACL_PASS scope=user access_acl=true default_acl=true live_unchanged=true backup_matches=true')
    }
    await unchangedExecution(async () => {
      assert.equal((await readNativeTransitionStatus(scope)).phase, 'committed')
      await assert.rejects(executeNativeTransition(confirmedInputs, verifyTransitionFixture), { code: 'native_confirmation_consumed' })
    })
    console.log(`MYAPI_NATIVE_WAL_FIXTURE_PASS scope=${scope} edition=${edition} source=stopped-fixture journal_mode=wal configuration_preserved=true`)
    console.log(`MYAPI_NATIVE_TRANSITION_EXECUTE_PASS scope=${scope} edition=${edition} same_database=true accounting_preserved=true after_plan_record_backed_up=true real_target_healthy=true repeated_confirmation_refused=true signature_change_refused=true digest_change_refused=true active_lock_refused=true configuration_preserved=true`)

    // This distinct charge is committed while the actual newer program runs.
    // A code-only rollback must retain it, including its WAL-backed history.
    fakeUpstream = createHttpServer((request, response) => {
      fakeUpstreamRequests++
      request.resume()
      response.writeHead(200, { 'content-type': 'application/json' })
      response.end(JSON.stringify({ id: 'synthetic-native-no-provider', choices: [], usage: { total_tokens: 0 } }))
    })
    await new Promise((resolve, reject) => {
      fakeUpstream.once('error', reject)
      fakeUpstream.listen(0, '127.0.0.1', resolve)
    })
    const upstreamAddress = fakeUpstream.address()
    assert.equal(upstreamAddress.address, '127.0.0.1')
    fakeUpstreamURL = `http://127.0.0.1:${upstreamAddress.port}`
    // One owned loopback probe establishes that this is a live counter.
    const probe = await fetch(fakeUpstreamURL + '/__qualification_probe__', { redirect: 'error', signal: AbortSignal.timeout(2_000) })
    assert.equal(probe.ok, true)
    await probe.arrayBuffer()
    const upstreamBefore = fakeUpstreamRequests
    assert.equal(upstreamBefore, 1)
    const newerAccounting = accounting('charge-new-version')
    assert.equal(newerAccounting.users[0].quota, 99976)
    assert.equal(newerAccounting.users[0].used_quota, 47)
    assert.equal(newerAccounting.tokens[0].remain_quota, 89953)
    assert.equal(newerAccounting.tokens[0].used_quota, 47)
    assert.equal(newerAccounting.logs.length, 3)
    assert.equal(newerAccounting.logs[2].request_id, 'native-synthetic-new-version-19')
    assert.equal(newerAccounting.logs[2].channel_id, 900001)
    assert.equal(newerAccounting.channels[0].base_url, fakeUpstreamURL)
    assert.equal(newerAccounting.abilities[0].channel_id, 900001)
    assert.deepEqual(newerAccounting.adjustments, latestAccounting.adjustments)
    assert.deepEqual(newerAccounting.options, latestAccounting.options)
    assert.ok((await stat(database + '-wal')).size > 32, 'the newer-version charge must have a real nonempty WAL')
    assert.equal(manager(['show', 'my-api.service', '--property=MainPID', '--value']).trim(), String(newPid))
    const rollbackInputs = { ...transitionInputs, tag: `v${version}`, manifest, bundle: currentBundle, package: binary,
      'current-manifest': targetManifest, 'current-bundle': targetBundle }
    const rollbackOptions = { operation: 'rollback' }

    async function stablePlanCheck(action) {
      // Prevent autonomous worker commits while comparing the real database
      // and WAL bytes around a plan. The service remains active throughout.
      const guard = spawn('python3', ['-I', '-B', '-u', '-c', [
        'import pathlib,sqlite3,sys',
        'db=sqlite3.connect(pathlib.Path(sys.argv[1]).as_uri()+"?mode=rw",uri=True,timeout=10)',
        'db.execute("BEGIN IMMEDIATE")',
        'print("ready",flush=True)',
        'try: sys.stdin.readline()',
        'finally: db.rollback(); db.close()',
      ].join('\n'), database], { stdio: ['pipe', 'pipe', 'pipe'], timeout: 120_000,
        ...(scope === 'system' ? { uid: databaseOwner.uid, gid: databaseOwner.gid } : {}) })
      guard.stdin.on('error', () => {})
      guard.stderr.resume()
      const exited = new Promise((resolve, reject) => {
        guard.once('error', reject)
        guard.once('exit', (code, signal) => resolve({ code, signal }))
      })
      try {
        await Promise.race([
          new Promise(resolve => {
            let output = ''
            guard.stdout.on('data', chunk => {
              output += chunk
              if (output === 'ready\n') resolve()
            })
          }),
          exited.then(() => { throw new Error('The rollback fixture could not reserve SQLite without writing') }),
        ])
        const beforeDatabase = await readFile(database), beforeWal = await readFile(database + '-wal')
        const result = await unchangedExecution(action)
        assert.deepEqual(await readFile(database), beforeDatabase, 'planning must not write the current database')
        assert.deepEqual(await readFile(database + '-wal'), beforeWal, 'planning must not rewrite or checkpoint current WAL history')
        return result
      } finally {
        guard.stdin.end('\n')
        assert.deepEqual(await exited, { code: 0, signal: null })
      }
    }

    const rollbackPreview = await stablePlanCheck(() => planNativeTransition(rollbackInputs, verifyTransitionFixture, rollbackOptions))
    assert.equal(rollbackPreview.operation, 'rollback')
    assert.equal(rollbackPreview.current.version, targetVersion)
    assert.equal(rollbackPreview.target.version, version)
    assert.equal(rollbackPreview.database, database)
    assert.equal(rollbackPreview.service.pid, newPid)
    assert.equal(rollbackPreview.dataPolicy, 'keep_current_sqlite')
    assert.equal(rollbackPreview.reusesInstalledBinary, true)
    assert.equal(fakeUpstreamRequests, upstreamBefore)

    for (const missing of ['policy', 'edge']) {
      const forbidden = structuredClone(targetDocument)
      if (missing === 'policy') forbidden.compatibility.rollback = 'required_manual'
      else forbidden.native_transitions[0].allow_code_rollback = false
      await writeTarget(forbidden)
      try {
        await unchangedExecution(() => assert.rejects(
          planNativeTransition(rollbackInputs, verifyTransitionFixture, rollbackOptions), { code: 'native_rollback_forbidden' }))
      } finally { await writeTarget(targetDocument) }
    }
    const incompatible = structuredClone(targetDocument)
    incompatible.compatibility.data_schema = '2.0.0'
    await writeTarget(incompatible)
    try {
      await unchangedExecution(() => assert.rejects(
        planNativeTransition(rollbackInputs, verifyTransitionFixture, rollbackOptions), { code: 'native_schema_mismatch' }))
    } finally { await writeTarget(targetDocument) }

    const upgradedReceiptBytes = await readFile(path.join(paths.state, 'installation.json'))
    const unowned = JSON.parse(upgradedReceiptBytes)
    unowned.ownedReleases = unowned.ownedReleases.filter(release => release.version !== version)
    await writeFile(path.join(paths.state, 'installation.json'), JSON.stringify(unowned) + '\n')
    try {
      await unchangedExecution(() => assert.rejects(
        planNativeTransition(rollbackInputs, verifyTransitionFixture, rollbackOptions), { code: 'native_release_conflict' }))
    } finally { await writeFile(path.join(paths.state, 'installation.json'), upgradedReceiptBytes) }
    const unexpectedReleaseFile = path.join(path.dirname(currentBinaryBefore), 'qualification-unowned-entry')
    await writeFile(unexpectedReleaseFile, 'synthetic unowned release entry', { flag: 'wx', mode: 0o600 })
    try {
      await unchangedExecution(() => assert.rejects(
        planNativeTransition(rollbackInputs, verifyTransitionFixture, rollbackOptions), { code: 'native_release_conflict' }))
    } finally { await unlink(unexpectedReleaseFile) }
    const retainedSize = (await stat(currentBinaryBefore)).size
    await appendFile(currentBinaryBefore, 'synthetic retained binary modification')
    try {
      await unchangedExecution(() => assert.rejects(
        planNativeTransition(rollbackInputs, verifyTransitionFixture, rollbackOptions), { code: 'digest_mismatch' }))
    } finally { await truncate(currentBinaryBefore, retainedSize) }

    // Re-plan after restoring the deliberate refusal fixtures; the confirmed
    // plan binds the retained file's latest inode/ctime and both signatures.
    await unchangedExecution(() => assert.rejects(
      executeNativeTransition({ ...rollbackInputs, confirm: rollbackPreview.confirmation }, verifyTransitionFixture, rollbackOptions),
      { code: 'native_confirmation_stale' }))
    const rollbackPlan = await stablePlanCheck(() => planNativeTransition(rollbackInputs, verifyTransitionFixture, rollbackOptions))
    assert.match(rollbackPlan.confirmation, /^[a-f0-9]{64}$/)
    assert.equal(rollbackPlan.dataPolicy, 'keep_current_sqlite')
    assert.equal(rollbackPlan.reusesInstalledBinary, true)
    const rollbackConfirmed = { ...rollbackInputs, confirm: rollbackPlan.confirmation }
    const rolledBack = await executeNativeTransition(rollbackConfirmed, verifyTransitionFixture, rollbackOptions)
    assert.equal(rolledBack.status, 'healthy')
    assert.equal(rolledBack.operation, 'rollback')
    assert.equal(rolledBack.current.version, targetVersion)
    assert.equal(rolledBack.target.version, version)
    assert.equal(rolledBack.database, database)
    assert.equal(rolledBack.accountingVerified, true)
    assert.ok(path.isAbsolute(rolledBack.backup))
    assert.notEqual(rolledBack.backup, upgraded.backup)
    const rollbackDatabase = await stat(database, { bigint: true })
    assert.equal(rollbackDatabase.dev, databaseIdentity.dev)
    assert.equal(rollbackDatabase.ino, databaseIdentity.ino)
    assert.deepEqual(accounting('snapshot'), newerAccounting)
    assert.equal(await readFile(paths.config, 'utf8'), config)
    assert.deepEqual(await readFile(paths.unit), unitBefore)
    const rollbackStatus = await readNativeTransitionStatus(scope)
    assert.equal(rollbackStatus.phase, 'committed')
    assert.equal(rollbackStatus.operation, 'rollback')
    assert.equal(rollbackStatus.service.state, 'active')
    assert.equal(rollbackStatus.activeBinary, currentBinaryBefore)
    const rollbackPid = Number(manager(['show', 'my-api.service', '--property=MainPID', '--value']).trim())
    assert.ok(rollbackPid > 0 && rollbackPid !== newPid && rollbackPid !== pid)
    assert.equal(rollbackStatus.service.pid, rollbackPid)
    const rollbackExecutable = await stat(`/proc/${rollbackPid}/exe`, { bigint: true })
    assert.equal(rollbackExecutable.dev, processBinaryBefore.dev)
    assert.equal(rollbackExecutable.ino, processBinaryBefore.ino)
    const rollbackResponse = await fetch(`http://127.0.0.1:${port}/api/status`, { redirect: 'error', signal: AbortSignal.timeout(2_000) })
    assert.equal(rollbackResponse.ok, true)
    const rollbackBody = await rollbackResponse.json()
    assert.equal(rollbackBody.success, true)
    assert.equal(rollbackBody.data.version, version)
    assert.equal(rollbackBody.data.product_edition, edition)
    const rollbackReceipt = JSON.parse(await readFile(path.join(paths.state, 'installation.json'), 'utf8'))
    assert.equal(rollbackReceipt.version, version)
    assert.equal(rollbackReceipt.sha256, sha256)
    assert.equal(fakeUpstreamRequests, upstreamBefore, 'rollback must not replay any upstream HTTP request')
    const rollbackInspection = path.join(scratch, 'rollback-backup-inspection')
    await cp(path.join(rolledBack.backup, 'data'), rollbackInspection, { recursive: true, errorOnExist: true, force: false })
    assert.deepEqual(accounting('snapshot', path.join(rollbackInspection, path.relative(paths.data, database))), newerAccounting)
    assert.deepEqual(await readFile(path.join(rolledBack.backup, 'config')), Buffer.from(config))
    assert.deepEqual(await readFile(path.join(rolledBack.backup, 'receipt')), upgradedReceiptBytes)
    if (personalPermissions) {
      assert.deepEqual(personalDataACLs(paths.data), personalPermissions)
      assert.deepEqual(personalDataACLs(path.join(rolledBack.backup, 'data')), personalPermissions)
    }
    await unchangedExecution(async () => {
      assert.equal((await readNativeTransitionStatus(scope)).operation, 'rollback')
      await assert.rejects(executeNativeTransition(rollbackConfirmed, verifyTransitionFixture, rollbackOptions), { code: 'native_confirmation_consumed' })
    })
    assert.equal(fakeUpstreamRequests, upstreamBefore)
    console.log(`MYAPI_NATIVE_ROLLBACK_PASS scope=${scope} edition=${edition} same_database=true newer_19_unit_record_preserved=true backup_contains_latest=true original_binary_inode=true retained_binary_reused=true readonly_plan=true missing_policy_refused=true missing_edge_refused=true schema_change_refused=true retained_tampering_refused=true stale_confirmation_refused=true repeated_confirmation_refused=true fake_upstream_before=${upstreamBefore} fake_upstream_after=${fakeUpstreamRequests}`)

    const reupgradePlan = await stablePlanCheck(() => planNativeTransition(executionInputs, verifyTransitionFixture))
    assert.equal(reupgradePlan.operation, 'upgrade')
    assert.equal(reupgradePlan.reusesInstalledBinary, true)
    assert.equal(reupgradePlan.dataPolicy, 'keep_current_sqlite')
    const reupgradeConfirmed = { ...executionInputs, confirm: reupgradePlan.confirmation }
    const reupgraded = await executeNativeTransition(reupgradeConfirmed, verifyTransitionFixture)
    assert.equal(reupgraded.status, 'healthy')
    assert.equal(reupgraded.operation, 'upgrade')
    assert.equal(reupgraded.target.version, targetVersion)
    assert.equal(reupgraded.database, database)
    assert.equal(reupgraded.accountingVerified, true)
    const finalDatabase = await stat(database, { bigint: true })
    assert.equal(finalDatabase.dev, databaseIdentity.dev)
    assert.equal(finalDatabase.ino, databaseIdentity.ino)
    assert.deepEqual(accounting('snapshot'), newerAccounting)
    assert.equal(await readFile(paths.config, 'utf8'), config)
    assert.deepEqual(await readFile(paths.unit), unitBefore)
    const reupgradeStatus = await readNativeTransitionStatus(scope)
    assert.equal(reupgradeStatus.phase, 'committed')
    assert.equal(reupgradeStatus.operation, 'upgrade')
    assert.equal(reupgradeStatus.service.state, 'active')
    assert.equal(reupgradeStatus.activeBinary, currentStatus.activeBinary)
    const finalPid = reupgradeStatus.service.pid
    assert.ok(finalPid > 0 && finalPid !== rollbackPid)
    const reupgradeExecutable = await stat(`/proc/${finalPid}/exe`, { bigint: true })
    assert.equal(reupgradeExecutable.dev, targetExecutable.dev)
    assert.equal(reupgradeExecutable.ino, targetExecutable.ino)
    const reupgradeResponse = await fetch(`http://127.0.0.1:${port}/api/status`, { redirect: 'error', signal: AbortSignal.timeout(2_000) })
    assert.equal(reupgradeResponse.ok, true)
    const reupgradeBody = await reupgradeResponse.json()
    assert.equal(reupgradeBody.success, true)
    assert.equal(reupgradeBody.data.version, targetVersion)
    assert.equal(reupgradeBody.data.product_edition, edition)
    await unchangedExecution(() => assert.rejects(executeNativeTransition(reupgradeConfirmed, verifyTransitionFixture), { code: 'native_confirmation_consumed' }))
    assert.equal(fakeUpstreamRequests, upstreamBefore)
    console.log(`MYAPI_NATIVE_REUPGRADE_PASS scope=${scope} edition=${edition} retained_new_binary_inode=true same_database=true latest_accounting_preserved=true repeated_confirmation_refused=true fake_upstream_before=${upstreamBefore} fake_upstream_after=${fakeUpstreamRequests}`)

    const uninstallInputs = { scope }
    const uninstallJournal = path.join(paths.state, 'native-uninstall.json')
    const wantsParent = path.join(path.dirname(paths.unit), scope === 'system' ? 'multi-user.target.wants' : 'default.target.wants')
    const enableLink = path.join(wantsParent, 'my-api.service')
    await assert.rejects(lstat(enableLink), { code: 'ENOENT' })
    manager(['enable', 'my-api.service'])
    assert.equal(manager(['show', 'my-api.service', '--property=MainPID', '--value']).trim(), String(finalPid))
    assert.equal((await lstat(enableLink)).isSymbolicLink(), true)
    assert.equal(await realpath(enableLink), paths.unit)
    const wantsParentBefore = await stat(wantsParent, { bigint: true })
    const otherWants = (await readdir(wantsParent)).filter(name => name !== 'my-api.service').sort()
    const uninstallPreview = await stablePlanCheck(() => planNativeUninstall(uninstallInputs))
    assert.equal(uninstallPreview.eligible, true)
    assert.equal(uninstallPreview.operation, 'uninstall')
    assert.match(uninstallPreview.confirmation, /^[a-f0-9]{64}$/)
    assert.equal(uninstallPreview.service.pid, finalPid)
    const expectedRemoval = [enableLink, paths.unit, path.dirname(paths.binary), executionPaths.releases,
      currentBinaryBefore, path.dirname(currentBinaryBefore), path.dirname(path.dirname(currentBinaryBefore)),
      currentStatus.activeBinary, path.dirname(currentStatus.activeBinary), path.dirname(path.dirname(currentStatus.activeBinary))]
    assert.deepEqual(new Set(uninstallPreview.remove), new Set(expectedRemoval))
    const expectedPreservation = [paths.config, paths.data, executionPaths.logs, executionPaths.backups, paths.state, executionPaths.cache]
    assert.deepEqual(new Set(uninstallPreview.preserve), new Set(expectedPreservation))
    await stablePlanCheck(() => assert.rejects(executeNativeUninstall(uninstallInputs), { code: 'native_confirmation_required' }))
    async function refusedUninstall(code) {
      await stablePlanCheck(async () => {
        await assert.rejects(planNativeUninstall(uninstallInputs), { code })
        await assert.rejects(executeNativeUninstall({ scope, confirm: uninstallPreview.confirmation }), { code })
      })
    }

    // These owned fixtures alter only service-manager inventory, not the live
    // process or its environment. Restore them without restarting the service.
    await writeFile(paths.unit, Buffer.concat([unitBefore, Buffer.from('\n# synthetic foreign unit bytes\n')]))
    manager(['daemon-reload'])
    try {
      await refusedUninstall('native_service_mismatch')
    } finally {
      await writeFile(paths.unit, unitBefore)
      manager(['daemon-reload'])
    }
    const dropInDirectory = paths.unit + '.d'
    const dropIn = path.join(dropInDirectory, '99-native-qualification.conf')
    await mkdir(dropInDirectory, { mode: 0o700 })
    await writeFile(dropIn, '[Service]\n# synthetic foreign drop-in; no changed runtime environment\n', { flag: 'wx', mode: 0o600 })
    manager(['daemon-reload'])
    try {
      await refusedUninstall('native_service_mismatch')
    } finally {
      await unlink(dropIn)
      await rmdir(dropInDirectory)
      manager(['daemon-reload'])
    }
    const unownedProgram = path.join(path.dirname(currentStatus.activeBinary), 'uninstall-unowned-fixture')
    await writeFile(unownedProgram, 'synthetic unowned program entry', { flag: 'wx', mode: 0o600 })
    try {
      await refusedUninstall('native_release_conflict')
    } finally { await unlink(unownedProgram) }

    const settledTransition = await readFile(stateFile)
    await writeFile(stateFile, JSON.stringify({ ...JSON.parse(settledTransition), phase: 'stop_requested' }) + '\n')
    try {
      await refusedUninstall('native_transition_pending')
    } finally { await writeFile(stateFile, settledTransition) }
    // This exclusively-created pre-inspection probe is never an execution
    // result. Both operations must leave it untouched and the real PID active;
    // only the fixture removes its own probe before the successful operation.
    await writeFile(uninstallJournal, JSON.stringify({ schema: 1, operation: 'uninstall', scope,
      phase: 'prepared', confirmation: uninstallPreview.confirmation, current: JSON.parse(settledTransition).target,
      remove: uninstallPreview.remove, preserve: uninstallPreview.preserve }) + '\n', { flag: 'wx', mode: 0o600 })
    try {
      await refusedUninstall('native_uninstall_pending')
    } finally { await unlink(uninstallJournal) }
    await writeFile(paths.config, config + '# synthetic uninstall confirmation change\n')
    try {
      await stablePlanCheck(() => assert.rejects(executeNativeUninstall({ scope, confirm: uninstallPreview.confirmation }),
        { code: 'native_confirmation_stale' }))
    } finally { await writeFile(paths.config, config) }

    const uncertainStopPlan = await stablePlanCheck(() => planNativeUninstall(uninstallInputs))
    await stablePlanCheck(async () => {
      const before = await executionSnapshot({ includeAutonomous: true })
      await assert.rejects(executeNativeUninstall({ scope, confirm: uncertainStopPlan.confirmation }, {
        prepareDrain: async () => ({ stop: async () => ({ drained: false }), close: async () => {} }),
      }), { code: 'native_stop_unconfirmed' })
      const stoppedUnknownBytes = await readFile(uninstallJournal)
      const stoppedUnknown = JSON.parse(stoppedUnknownBytes)
      assert.equal(stoppedUnknown.schema, 1)
      assert.equal(stoppedUnknown.scope, scope)
      assert.equal(stoppedUnknown.operation, 'uninstall')
      assert.equal(stoppedUnknown.phase, 'stop_requested')
      assert.equal(stoppedUnknown.confirmation, uncertainStopPlan.confirmation)
      assert.deepEqual(stoppedUnknown.current, reupgradeStatus.target)
      assert.deepEqual(stoppedUnknown.remove, uncertainStopPlan.remove)
      assert.deepEqual(stoppedUnknown.preserve, uncertainStopPlan.preserve)
      const after = await executionSnapshot({ includeAutonomous: true })
      assert.ok(after.files[uninstallJournal])
      delete after.files[uninstallJournal]
      assert.deepEqual(after, before, 'an unconfirmed drain may only publish its pending uninstall record')
      const uncertainStatus = await readNativeTransitionStatus(scope)
      assert.equal(uncertainStatus.phase, 'stop_requested')
      assert.equal(uncertainStatus.operation, 'uninstall')
      assert.equal(uncertainStatus.service.pid, finalPid)
      assert.equal(uncertainStatus.service.state, 'active')
      assert.equal(uncertainStatus.activeBinary, reupgradeStatus.activeBinary)
      await unchangedExecution(async () => {
        await assert.rejects(planNativeUninstall(uninstallInputs), { code: 'native_uninstall_pending' })
        await assert.rejects(executeNativeUninstall({ scope, confirm: uncertainStopPlan.confirmation }), { code: 'native_uninstall_pending' })
      })
      assert.deepEqual(await readFile(uninstallJournal), stoppedUnknownBytes)
      const journalInfo = await lstat(uninstallJournal)
      assert.equal(journalInfo.uid, uid)
      assert.equal(journalInfo.nlink, 1)
      assert.equal(journalInfo.mode & 0o777, 0o600)
      // Only this owned GHA fixture can remove its explicitly injected false-
      // drain record after checking every field and unchanged live evidence.
      // A real unconfirmed stop remains pending and is never auto-cleared.
      await unlink(uninstallJournal)
    })
    assert.equal(fakeUpstreamRequests, upstreamBefore)
    console.log(`MYAPI_NATIVE_UNINSTALL_STOP_GUARD_PASS scope=${scope} edition=${edition} real_service_running=true persisted_phase=stop_requested only_pending_record_written=true pending_replay_refused=true database_wal_unchanged=true fixture_record_checked_before_removal=true`)

    const uninstallPlan = await stablePlanCheck(() => planNativeUninstall(uninstallInputs))
    assert.equal(uninstallPlan.service.pid, finalPid)
    assert.deepEqual(new Set(uninstallPlan.remove), new Set(expectedRemoval))
    assert.deepEqual(new Set(uninstallPlan.preserve), new Set(expectedPreservation))
    const backupsBeforeUninstall = await readdir(executionPaths.backups)
    assert.deepEqual(new Set(backupsBeforeUninstall), new Set([upgraded.backup, rolledBack.backup, reupgraded.backup].map(filename => path.basename(filename))))
    const preservationBefore = {}
    async function preserveInventory(filename, logs = false) {
      const info = await lstat(filename, { bigint: true })
      assert.ok(info.isDirectory() || info.isFile())
      const row = { metadata: [String(info.dev), String(info.ino), String(info.mode), String(info.uid), String(info.gid)] }
      if (info.isFile()) row.bytes = await readFile(filename)
      preservationBefore[filename] = { ...row, logs }
      if (info.isDirectory()) {
        for (const name of (await readdir(filename)).sort()) await preserveInventory(path.join(filename, name), logs)
      }
    }
    await preserveInventory(paths.config)
    await preserveInventory(executionPaths.backups)
    await preserveInventory(executionPaths.logs, true)
    await preserveInventory(executionPaths.cache)
    const preservedDirectories = await Promise.all([paths.data, paths.state, executionPaths.cache].map(async filename => {
      const info = await stat(filename, { bigint: true })
      return { filename, dev: info.dev, ino: info.ino }
    }))
    const uninstallConfirmed = { scope, confirm: uninstallPlan.confirmation }
    const uninstalled = await executeNativeUninstall(uninstallConfirmed)
    assert.equal(uninstalled.status, 'uninstalled')
    assert.equal(uninstalled.operation, 'uninstall')
    for (const filename of expectedRemoval) await assert.rejects(lstat(filename), { code: 'ENOENT' })
    const wantsParentAfter = await stat(wantsParent, { bigint: true })
    assert.equal(wantsParentAfter.dev, wantsParentBefore.dev)
    assert.equal(wantsParentAfter.ino, wantsParentBefore.ino)
    assert.deepEqual((await readdir(wantsParent)).sort(), otherWants, 'remove only the exact managed want link, preserving its parent and unrelated entries')
    for (const { filename, dev, ino } of preservedDirectories) {
      const info = await stat(filename, { bigint: true })
      assert.equal(info.dev, dev)
      assert.equal(info.ino, ino)
    }
    for (const [filename, row] of Object.entries(preservationBefore)) {
      const info = await lstat(filename, { bigint: true })
      assert.deepEqual([String(info.dev), String(info.ino), String(info.mode), String(info.uid), String(info.gid)], row.metadata)
      if (row.bytes) {
        const bytes = await readFile(filename)
        if (row.logs) assert.deepEqual(bytes.subarray(0, row.bytes.length), row.bytes, 'drain may append logs but must preserve their complete original bytes')
        else assert.deepEqual(bytes, row.bytes)
      }
    }
    const uninstalledDatabase = await stat(database, { bigint: true })
    assert.equal(uninstalledDatabase.dev, databaseIdentity.dev)
    assert.equal(uninstalledDatabase.ino, databaseIdentity.ino)
    assert.deepEqual(accounting('snapshot'), newerAccounting)
    assert.deepEqual(await readdir(executionPaths.backups), backupsBeforeUninstall)
    assert.equal(await readFile(paths.config, 'utf8'), config)
    if (personalPermissions) {
      assert.deepEqual(personalDataACLs(paths.data), personalPermissions)
      for (const backup of [upgraded.backup, rolledBack.backup, reupgraded.backup]) {
        assert.deepEqual(personalDataACLs(path.join(backup, 'data')), personalPermissions)
      }
    }
    const uninstallStatus = await readNativeTransitionStatus(scope)
    assert.equal(uninstallStatus.phase, 'uninstalled')
    assert.equal(uninstallStatus.operation, 'uninstall')
    assert.equal(uninstallStatus.service.state, 'inactive')
    assert.equal(uninstallStatus.service.pid, 0)
    assert.equal(uninstallStatus.activeBinary, null)
    assert.equal(JSON.parse(await readFile(uninstallJournal, 'utf8')).phase, 'uninstalled')
    assert.equal((await stat(uninstallJournal)).mode & 0o777, 0o600)
    async function uninstalledSnapshot() {
      const files = {}
      for (const filename of [...Object.keys(preservationBefore), uninstallJournal,
        path.join(paths.state, 'installation.json'), stateFile]) {
        const info = await lstat(filename, { bigint: true })
        files[filename] = { metadata: [String(info.dev), String(info.ino), String(info.mode), String(info.uid), String(info.gid)],
          bytes: info.isFile() ? await readFile(filename) : null }
      }
      return { files, accounting: accounting('snapshot'), backups: await readdir(executionPaths.backups),
        wants: (await readdir(wantsParent)).sort(), status: await readNativeTransitionStatus(scope) }
    }
    const repeatedUninstallBefore = await uninstalledSnapshot()
    await assert.rejects(executeNativeUninstall(uninstallConfirmed), { code: 'native_confirmation_consumed' })
    assert.deepEqual(await uninstalledSnapshot(), repeatedUninstallBefore)
    assert.equal(fakeUpstreamRequests, upstreamBefore)
    console.log(`MYAPI_NATIVE_UNINSTALL_PASS scope=${scope} edition=${edition} readonly_plan=true foreign_unit_refused=true dropin_refused=true unowned_program_refused=true pending_transition_refused=true pending_uninstall_refused=true stale_confirmation_refused=true unconfirmed_drain_refused=true service_stopped=true owned_program_removed=true exact_want_link_removed=true want_parent_preserved=true same_database=true latest_19_unit_record_preserved=true configuration_logs_three_backups_preserved=true repeated_confirmation_refused=true fake_upstream_before=${upstreamBefore} fake_upstream_after=${fakeUpstreamRequests}`)
  }
  if (executionMode === 'interrupt') assert.equal(manager(['show', 'my-api.service', '--property=MainPID', '--value']).trim(), '0')
  else assert.equal((await readNativeTransitionStatus(scope)).service.pid, 0)
  assert.ok((await stat(path.join(paths.data, 'my-api.db'))).isFile())
  console.log(`MYAPI_NATIVE_INSTALL_PASS scope=${scope} edition=${edition} health=true configuration_preserved=true port_conflict=true signature_refusal=true repeat_refusal=true stopped=true`)
} finally {
  if (fakeUpstream?.listening) {
    fakeUpstream.closeAllConnections()
    await new Promise(resolve => fakeUpstream.close(resolve))
  }
  await rm(scratch, { recursive: true })
}
