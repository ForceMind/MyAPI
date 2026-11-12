import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { createHash, generateKeyPairSync, sign, verify } from 'node:crypto'
import { createReadStream } from 'node:fs'
import { mkdir, mkdtemp, readFile, rm, rmdir, stat, unlink, writeFile } from 'node:fs/promises'
import { createServer } from 'node:net'
import { userInfo } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { installNative } from '../../cli/lib/native-install.mjs'
import { verifyNativePackage } from '../../cli/lib/native-package-verification.mjs'

assert.equal(process.env.GITHUB_ACTIONS, 'true')
assert.equal(process.env.GITHUB_REPOSITORY, 'ForceMind/MyAPI')
assert.equal(process.env.GITHUB_EVENT_NAME, 'workflow_dispatch')
assert.equal(process.env.GITHUB_REF, 'refs/heads/dev')
const [scope, binary] = process.argv.slice(2)
assert.ok(['system', 'user'].includes(scope))
assert.ok(path.isAbsolute(binary))
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
  manager(['stop', 'my-api.service'])
  assert.equal(manager(['show', 'my-api.service', '--property=MainPID', '--value']).trim(), '0')
  assert.ok((await stat(path.join(paths.data, 'my-api.db'))).isFile())
  console.log(`MYAPI_NATIVE_INSTALL_PASS scope=${scope} edition=${edition} health=true configuration_preserved=true port_conflict=true signature_refusal=true repeat_refusal=true stopped=true`)
} finally {
  await rm(scratch, { recursive: true })
}
