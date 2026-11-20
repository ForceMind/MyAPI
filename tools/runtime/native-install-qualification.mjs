import assert from 'node:assert/strict'
import { execFileSync, spawn } from 'node:child_process'
import { createHash, generateKeyPairSync, randomBytes, sign, verify } from 'node:crypto'
import { createReadStream } from 'node:fs'
import { appendFile, copyFile, cp, lstat, mkdir, mkdtemp, readFile, readdir, readlink, realpath, rename, rm, rmdir, stat, truncate, unlink, writeFile } from 'node:fs/promises'
import { createServer } from 'node:net'
import { userInfo } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { installNative } from '../../cli/lib/native-install.mjs'
import { verifyNativePackage, verifyNativeTransitionPackages } from '../../cli/lib/native-package-verification.mjs'
import { executeNativeTransition, planNativeTransition, readNativeTransitionStatus } from '../../cli/lib/native-transition.mjs'

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
const accountingFixture = String.raw`
import json, os, pathlib, sqlite3, stat, sys
request = json.load(sys.stdin)
assert os.environ.get('GITHUB_ACTIONS') == 'true'
database = request['database']
assert os.path.isabs(database) and os.path.realpath(database) == database
info = os.lstat(database)
assert stat.S_ISREG(info.st_mode) and info.st_nlink == 1
action = request['action']
assert action in ('seed', 'charge', 'snapshot')
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
    else:
        db.execute('PRAGMA query_only=ON')
    def rows(statement):
        return [dict(row) for row in db.execute(statement)]
    value = {
        'users': rows('SELECT id,username,status,quota,quota_version,used_quota,request_count,self_use_no_balance,usage_policy_revision FROM users WHERE id=900001'),
        'tokens': rows('SELECT id,user_id,name,status,remain_quota,used_quota,quota_version,credential_version,unlimited_quota,model_limits_enabled,model_limits,"group",expired_time FROM tokens WHERE id=900001'),
        'logs': rows('SELECT id,user_id,request_id,token_id,quota,prompt_tokens,completion_tokens,type,content,other FROM logs WHERE user_id=900001 ORDER BY id'),
        'adjustments': rows('SELECT * FROM admin_quota_adjustments WHERE target_user_id=900001 ORDER BY id'),
        'options': rows('SELECT "key",value FROM options ORDER BY "key"'),
    }
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
  targetDocument.native_transitions = [{
    from: { version, source_sha: receipt.sourceSha, artifact_id: receipt.artifact, sha256 },
    to_artifact_id: receipt.artifact, storage: 'sqlite', allow_code_rollback: false,
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
    input: JSON.stringify({ action, database: filename, ...(action === 'seed' ? { token_secret: randomBytes(32).toString('hex') } : {}) }),
    ...(scope === 'system' && action !== 'snapshot' ? { uid: databaseOwner.uid, gid: databaseOwner.gid } : {}),
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
  async function executionSnapshot() {
    const files = {}
    async function visit(filename) {
      // The user release root also contains live data. Verify synthetic
      // accounting below; exclude autonomous task rows and SQLite lock bytes
      // rather than take an unsafe byte snapshot of a running database.
      if ([paths.data, executionPaths.logs, executionPaths.cache].includes(filename)) return
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
    })
    assert.equal(JSON.parse(await readFile(stateFile, 'utf8')).phase, 'stopped')
    console.log(`MYAPI_NATIVE_TRANSITION_INTERRUPT_PASS scope=${scope} edition=${edition} real_service=true persisted_phase=stopped child_exit=73 replay_refused=true restarted=false accounting_preserved=true manual_state_preserved=true`)
  } else {
    const upgraded = await executeNativeTransition(confirmedInputs, verifyTransitionFixture)
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
    console.log(`MYAPI_NATIVE_TRANSITION_EXECUTE_PASS scope=${scope} edition=${edition} same_database=true accounting_preserved=true after_plan_record_backed_up=true real_target_healthy=true repeated_confirmation_refused=true signature_change_refused=true digest_change_refused=true active_lock_refused=true configuration_preserved=true`)
    manager(['stop', 'my-api.service'])
  }
  assert.equal(manager(['show', 'my-api.service', '--property=MainPID', '--value']).trim(), '0')
  assert.ok((await stat(path.join(paths.data, 'my-api.db'))).isFile())
  console.log(`MYAPI_NATIVE_INSTALL_PASS scope=${scope} edition=${edition} health=true configuration_preserved=true port_conflict=true signature_refusal=true repeat_refusal=true stopped=true`)
} finally {
  await rm(scratch, { recursive: true })
}
