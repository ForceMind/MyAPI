import assert from 'node:assert/strict'
import { execFileSync, spawn } from 'node:child_process'
import { createHash, generateKeyPairSync, sign, verify } from 'node:crypto'
import { createReadStream } from 'node:fs'
import { lstat, mkdir, mkdtemp, readFile, readdir, readlink, realpath, rename, rm, rmdir, stat, unlink, writeFile } from 'node:fs/promises'
import { createServer } from 'node:net'
import { userInfo } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { installNative } from '../../cli/lib/native-install.mjs'
import { verifyNativePackage, verifyNativeTransitionPackages } from '../../cli/lib/native-package-verification.mjs'
import { planNativeTransition } from '../../cli/lib/native-transition.mjs'

assert.equal(process.env.GITHUB_ACTIONS, 'true')
assert.equal(process.env.GITHUB_REPOSITORY, 'ForceMind/MyAPI')
assert.equal(process.env.GITHUB_EVENT_NAME, 'workflow_dispatch')
assert.equal(process.env.GITHUB_REF, 'refs/heads/dev')
const [scope, binary, targetBinary] = process.argv.slice(2)
assert.ok(['system', 'user'].includes(scope))
assert.ok(path.isAbsolute(binary))
assert.ok(path.isAbsolute(targetBinary))
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
// A source-bound CI fixture, built with a newer version but never launched in
// this read-only batch. This is not a published release or a migration test.
const [major, minor, patch] = version.match(/^(\d+)\.(\d+)\.(\d+)/).slice(1)
const targetVersion = `${major}.${minor}.${BigInt(patch) + 1n}`
const edition = scope === 'system' ? 'full' : 'lite'
const port = scope === 'system' ? '21951' : '21952'
const hash = createHash('sha256')
for await (const chunk of createReadStream(binary)) hash.update(chunk)
const sha256 = hash.digest('hex')
const scratch = await mkdtemp('/tmp/myapi-native-qualification-')
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
  const receipt = JSON.parse(await readFile(path.join(paths.state, 'installation.json'), 'utf8'))
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

  const database = path.join(paths.data, 'my-api.db')
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
    const qualified = await readOnlyCheck(() => planNativeTransition(transitionInputs, verifyTransitionFixture))
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
    const requalified = await readOnlyCheck(() => planNativeTransition(transitionInputs, verifyTransitionFixture))
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
  manager(['stop', 'my-api.service'])
  assert.equal(manager(['show', 'my-api.service', '--property=MainPID', '--value']).trim(), '0')
  assert.ok((await stat(path.join(paths.data, 'my-api.db'))).isFile())
  console.log(`MYAPI_NATIVE_INSTALL_PASS scope=${scope} edition=${edition} health=true configuration_preserved=true port_conflict=true signature_refusal=true repeat_refusal=true stopped=true`)
} finally {
  await rm(scratch, { recursive: true })
}
