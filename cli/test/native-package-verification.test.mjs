import assert from 'node:assert/strict'
import { createHash, generateKeyPairSync, sign, verify } from 'node:crypto'
import { mkdtemp, readFile, rm, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { test } from 'node:test'
import { verifyNativePackage } from '../lib/native-package-verification.mjs'

async function fixture(t, scope = 'system', edition = 'full', arch = 'amd64') {
  const directory = await mkdtemp(path.join(tmpdir(), 'myapi-native-test-'))
  t.after(() => rm(directory, { recursive: true, force: true }))
  const inputs = { tag: 'v1.2.0', scope, edition, manifest: path.join(directory, 'manifest.json'),
    package: path.join(directory, 'my-api') }
  const binary = Buffer.alloc(96, 0x2a)
  Buffer.from([0x7f, 69, 76, 70, 2, 1, 1]).copy(binary)
  binary.writeUInt16LE(2, 16)
  binary.writeUInt16LE(arch === 'amd64' ? 62 : 183, 18)
  const digest = createHash('sha256').update(binary).digest('hex')
  const artifact = {
    id: 'my-api-linux', edition, installation_shape: scope === 'system' ? 'server-native' : 'personal-native',
    os: 'linux', arch, kind: 'binary',
    location: 'https://github.com/ForceMind/MyAPI/releases/download/v1.2.0/my-api-linux',
    sha256_or_digest: digest, signature: 'manifest-signature',
    sbom_or_provenance: 'https://github.com/ForceMind/MyAPI/releases/download/v1.2.0/provenance.json',
  }
  const manifest = {
    schema_version: 1,
    product: { name: 'My API', version: '1.2.0', source_sha: 'a'.repeat(40), build_revision: 'b'.repeat(40) },
    bootstrap: { min_version: '1.0.0', max_version: '1.9.0' },
    compatibility: { config_schema: '1.0.0', data_schema: '1.0.0', supported_from: ['1.0.0'], rollback: 'required_manual' },
    artifacts: [artifact],
  }
  const key = generateKeyPairSync('ed25519')
  let signature
  async function signManifest() {
    const bytes = Buffer.from(JSON.stringify(manifest, null, 2) + '\n')
    signature = sign(null, bytes, key.privateKey)
    await writeFile(inputs.manifest, bytes)
  }
  await signManifest()
  await writeFile(inputs.package, binary)
  const policy = { arch, bootstrapVersion: '1.1.0', verifySignature(bytes) {
    if (!verify(null, bytes, key.publicKey, signature)) {
      throw Object.assign(new Error('Fixture signature rejected'), { code: 'signature_rejected' })
    }
  } }
  return { directory, inputs, binary, manifest, artifact, signManifest, policy, digest }
}

for (const [scope, edition, arch] of [['system', 'full', 'amd64'], ['user', 'lite', 'arm64']]) {
  test(`verifies the signed package bytes for ${scope} ${edition} ${arch}`, async t => {
    const { inputs, binary, policy, digest } = await fixture(t, scope, edition, arch)
    const originalManifest = await readFile(inputs.manifest)
    assert.deepEqual(await verifyNativePackage(inputs, policy), {
      version: '1.2.0', edition, scope, arch, artifact: 'my-api-linux', sourceSha: 'a'.repeat(40), sha256: digest,
    })
    assert.deepEqual(await readFile(inputs.manifest), originalManifest)
    assert.deepEqual(await readFile(inputs.package), binary)
    await writeFile(inputs.manifest, Buffer.concat([originalManifest, Buffer.from(' ')]))
    await assert.rejects(verifyNativePackage(inputs, policy), { code: 'signature_rejected' })
  })
}

test('rejects a signed Manifest for a different source, version, or target', async t => {
  const { inputs, manifest, artifact, signManifest, policy } = await fixture(t)
  await assert.rejects(verifyNativePackage({ ...inputs, tag: 'v1.2.1' }, policy), { code: 'version_mismatch' })
  await assert.rejects(verifyNativePackage(inputs, { ...policy, arch: 'arm64' }), { code: 'artifact_not_found' })
  await assert.rejects(verifyNativePackage({ ...inputs, edition: 'lite' }, policy), { code: 'artifact_not_found' })
  await assert.rejects(verifyNativePackage(inputs, { ...policy, bootstrapVersion: '2.0.0' }), { code: 'bootstrap_incompatible' })
  artifact.location = 'https://github.com/other/MyAPI/releases/download/v1.2.0/my-api-linux'
  await signManifest()
  await assert.rejects(verifyNativePackage(inputs, policy), { code: 'source_mismatch' })
  manifest.schema_version = 2
  await signManifest()
  await assert.rejects(verifyNativePackage(inputs, policy), { code: 'unsupported_schema_version' })
})

test('rejects a substituted payload, wrong ELF CPU, and symlink', async t => {
  const { directory, inputs, binary, policy } = await fixture(t)
  const symlinkPath = path.join(directory, 'replacement')
  await symlink(inputs.package, symlinkPath)
  await assert.rejects(verifyNativePackage({ ...inputs, package: symlinkPath }, policy), { code: 'ELOOP' })
  binary[90] ^= 1
  await writeFile(inputs.package, binary)
  await assert.rejects(verifyNativePackage(inputs, policy), { code: 'digest_mismatch' })
  binary.writeUInt16LE(183, 18)
  await writeFile(inputs.package, binary)
  await assert.rejects(verifyNativePackage(inputs, policy), { code: 'invalid_binary' })
})
