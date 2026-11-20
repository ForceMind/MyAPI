import assert from 'node:assert/strict'
import { createHash, generateKeyPairSync, sign, verify } from 'node:crypto'
import { mkdtemp, readFile, rm, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { test } from 'node:test'
import { parseNativePackageArgs, verifyNativePackage, verifyNativeTransitionPackages } from '../lib/native-package-verification.mjs'

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

async function transitionFixture(t, scope = 'system', edition = 'full', arch = 'amd64') {
  const base = await fixture(t, scope, edition, arch)
  const older = base.manifest
  const newer = structuredClone(older)
  newer.schema_version = 2
  newer.product.version = '1.3.0'
  newer.product.source_sha = 'c'.repeat(40)
  newer.compatibility.supported_from = ['1.2.0']
  newer.compatibility.rollback = 'code_only'
  const targetBinary = Buffer.from(base.binary)
  targetBinary[90] ^= 1
  const targetDigest = createHash('sha256').update(targetBinary).digest('hex')
  Object.assign(newer.artifacts[0], {
    id: 'my-api-next',
    location: 'https://github.com/ForceMind/MyAPI/releases/download/v1.3.0/my-api-next',
    sha256_or_digest: targetDigest,
  })
  newer.native_transitions = [{
    from: { version: '1.2.0', source_sha: older.product.source_sha, artifact_id: older.artifacts[0].id, sha256: base.digest },
    to_artifact_id: 'my-api-next', storage: 'sqlite', allow_code_rollback: true,
  }]
  const inputs = {
    scope, edition, tag: 'v1.3.0', manifest: path.join(base.directory, 'target-manifest.json'),
    bundle: path.join(base.directory, 'target-bundle.json'), package: path.join(base.directory, 'target-binary'),
    'current-manifest': base.inputs.manifest, 'current-bundle': path.join(base.directory, 'current-bundle.json'),
    cosign: path.join(base.directory, 'unused-cosign'), root: path.join(base.directory, 'unused-root'),
  }
  const installed = { version: '1.2.0', edition, scope, arch, artifact: older.artifacts[0].id,
    sourceSha: older.product.source_sha, sha256: base.digest }
  const keys = new Map(['v1.2.0', 'v1.3.0'].map(tag => [tag, generateKeyPairSync('ed25519')]))
  async function signManifests() {
    for (const [tag, manifest, manifestPath, bundlePath] of [
      ['v1.2.0', older, inputs['current-manifest'], inputs['current-bundle']],
      ['v1.3.0', newer, inputs.manifest, inputs.bundle],
    ]) {
      const bytes = Buffer.from(JSON.stringify(manifest, null, 2) + '\n')
      await writeFile(manifestPath, bytes)
      await writeFile(bundlePath, sign(null, bytes, keys.get(tag).privateKey))
    }
  }
  const calls = []
  const policy = { arch, bootstrapVersion: '1.1.0', async verifySignature(bytes, roleInputs, cpu, tag) {
    calls.push({ tag, arch: cpu, manifest: roleInputs.manifest, bundle: roleInputs.bundle, inputTag: roleInputs.tag })
    const key = keys.get(tag)
    if (!key || !verify(null, bytes, key.publicKey, await readFile(roleInputs.bundle))) {
      throw Object.assign(new Error('Fixture signature rejected'), { code: 'signature_rejected' })
    }
  } }
  await signManifests()
  await writeFile(inputs.package, targetBinary)
  const targetIdentity = { version: '1.3.0', sourceSha: newer.product.source_sha,
    artifact: newer.artifacts[0].id, sha256: targetDigest }
  const { version, sourceSha, artifact, sha256 } = installed
  const currentIdentity = { version, sourceSha, artifact, sha256 }
  const rollbackInputs = { ...inputs, tag: 'v1.2.0',
    manifest: inputs['current-manifest'], bundle: inputs['current-bundle'], package: base.inputs.package,
    'current-manifest': inputs.manifest, 'current-bundle': inputs.bundle }
  return { ...base, inputs, older, newer, installed, policy, calls, signManifests, targetBinary,
    currentBinary: base.inputs.package, targetIdentity, currentIdentity, rollbackInputs,
    newerInstalled: { ...installed, ...targetIdentity } }
}

for (const [scope, edition, arch] of [['system', 'full', 'amd64'], ['system', 'lite', 'arm64'], ['user', 'lite', 'amd64']]) {
  test(`authenticates both original manifests and binaries for ${scope} ${edition} ${arch} upgrade`, async t => {
    const f = await transitionFixture(t, scope, edition, arch)
    const originals = await Promise.all([f.inputs['current-manifest'], f.inputs.manifest,
      f.currentBinary, f.inputs.package].map(filename => readFile(filename)))
    assert.deepEqual(await verifyNativeTransitionPackages(f.inputs, f.installed, f.currentBinary, 'upgrade', f.policy), {
      operation: 'upgrade', storage: 'sqlite', scope, edition, arch,
      current: f.currentIdentity, target: f.targetIdentity, configSchema: '1.0.0', dataSchema: '1.0.0',
    })
    assert.deepEqual(f.calls, [
      { tag: 'v1.2.0', inputTag: 'v1.2.0', arch, manifest: f.inputs['current-manifest'], bundle: f.inputs['current-bundle'] },
      { tag: 'v1.3.0', inputTag: 'v1.3.0', arch, manifest: f.inputs.manifest, bundle: f.inputs.bundle },
    ])
    assert.deepEqual(await Promise.all([f.inputs['current-manifest'], f.inputs.manifest,
      f.currentBinary, f.inputs.package].map(filename => readFile(filename))), originals)
  })
}

test('verifies both binary identities for an explicitly authorized rollback', async t => {
  const f = await transitionFixture(t)
  assert.deepEqual(await verifyNativeTransitionPackages(f.rollbackInputs, f.newerInstalled, f.inputs.package, 'rollback', f.policy), {
    operation: 'rollback', storage: 'sqlite', scope: 'system', edition: 'full', arch: 'amd64',
    current: f.targetIdentity, target: f.currentIdentity, configSchema: '1.0.0', dataSchema: '1.0.0',
  })
  assert.deepEqual(f.calls.map(call => call.tag), ['v1.3.0', 'v1.2.0'])
  f.newer.native_transitions[0].allow_code_rollback = false
  await f.signManifests()
  await assert.rejects(verifyNativeTransitionPackages(f.rollbackInputs, f.newerInstalled, f.inputs.package, 'rollback', f.policy),
    { code: 'native_rollback_forbidden' })
})

test('rejects modified original bytes, cross-tag bundles and substituted current manifests', async t => {
  const f = await transitionFixture(t)
  for (const field of ['current-manifest', 'manifest']) {
    await writeFile(f.inputs[field], Buffer.concat([await readFile(f.inputs[field]), Buffer.from(' ')]))
    await assert.rejects(verifyNativeTransitionPackages(f.inputs, f.installed, f.currentBinary, 'upgrade', f.policy),
      { code: 'signature_rejected' })
    await f.signManifests()
  }
  for (const replacement of [
    { 'current-bundle': f.inputs.bundle },
    { bundle: f.inputs['current-bundle'] },
    { 'current-manifest': f.inputs.manifest, 'current-bundle': f.inputs.bundle },
    { tag: 'v1.3.1' },
  ]) {
    await assert.rejects(verifyNativeTransitionPackages({ ...f.inputs, ...replacement }, f.installed, f.currentBinary, 'upgrade', f.policy),
      { code: 'signature_rejected' })
  }
})

test('requires each signed release location and target version to match its own tag', async t => {
  const f = await transitionFixture(t)
  for (const manifest of [f.older, f.newer]) {
    const originalLocation = manifest.artifacts[0].location
    for (const location of [originalLocation.replace('ForceMind', 'other'), originalLocation.replace('/v1.', '/v9.')]) {
      manifest.artifacts[0].location = location
      await f.signManifests()
      await assert.rejects(verifyNativeTransitionPackages(f.inputs, f.installed, f.currentBinary, 'upgrade', f.policy),
        { code: 'source_mismatch' })
    }
    manifest.artifacts[0].location = originalLocation
  }
  f.newer.product.version = '1.3.1'
  await f.signManifests()
  await assert.rejects(verifyNativeTransitionPackages(f.inputs, f.installed, f.currentBinary, 'upgrade', f.policy),
    { code: 'version_mismatch' })
})

test('rejects mismatched live binaries and target packages at both ELF and SHA boundaries', async t => {
  const f = await transitionFixture(t)
  for (const [filename, bytes] of [[f.currentBinary, f.binary], [f.inputs.package, f.targetBinary]]) {
    const corrupt = Buffer.from(bytes)
    corrupt[90] ^= 2
    await writeFile(filename, corrupt)
    await assert.rejects(verifyNativeTransitionPackages(f.inputs, f.installed, f.currentBinary, 'upgrade', f.policy),
      { code: 'digest_mismatch' })
    corrupt.writeUInt16LE(183, 18)
    await writeFile(filename, corrupt)
    await assert.rejects(verifyNativeTransitionPackages(f.inputs, f.installed, f.currentBinary, 'upgrade', f.policy),
      { code: 'invalid_binary' })
    await writeFile(filename, bytes)
  }
  const link = path.join(f.directory, 'live-link')
  await symlink(f.currentBinary, link)
  await assert.rejects(verifyNativeTransitionPackages(f.inputs, f.installed, link, 'upgrade', f.policy), { code: 'ELOOP' })
  await assert.rejects(verifyNativeTransitionPackages({ ...f.inputs, package: link }, f.installed, f.currentBinary, 'upgrade', f.policy),
    { code: 'ELOOP' })
})

test('keeps the exact seven-field installed identity and preserves scope, edition and CPU', async t => {
  const f = await transitionFixture(t)
  for (const identity of [
    { ...f.installed, version: '1.1.0' }, { ...f.installed, sourceSha: 'd'.repeat(40) },
    { ...f.installed, artifact: 'other' }, { ...f.installed, sha256: 'e'.repeat(64) },
    { ...f.installed, scope: 'user' }, { ...f.installed, edition: 'lite' }, { ...f.installed, arch: 'arm64' },
  ]) {
    // A separately signed old release cannot be substituted for the recorded tag.
    const code = identity.version === '1.1.0' ? 'signature_rejected' : 'installed_identity_mismatch'
    await assert.rejects(verifyNativeTransitionPackages(f.inputs, identity, f.currentBinary, 'upgrade', f.policy), { code })
  }
  await assert.rejects(verifyNativeTransitionPackages(f.inputs, { ...f.installed, storage: 'sqlite' }, f.currentBinary, 'upgrade', f.policy),
    { code: 'invalid_fields' })
  await assert.rejects(verifyNativeTransitionPackages(f.inputs, f.installed, f.currentBinary, 'upgrade', { ...f.policy, arch: 'arm64' }),
    { code: 'installed_identity_mismatch' })
})

test('enforces signed transition declarations, direction, matching schemas and bootstrap support', async t => {
  const f = await transitionFixture(t)
  await assert.rejects(verifyNativeTransitionPackages(f.inputs, f.installed, f.currentBinary, 'rollback', f.policy),
    { code: 'native_transition_direction_mismatch' })
  await assert.rejects(verifyNativeTransitionPackages(f.inputs, f.installed, f.currentBinary, 'upgrade', { ...f.policy, bootstrapVersion: '2.0.0' }),
    { code: 'bootstrap_incompatible' })
  for (const schema of ['config_schema', 'data_schema']) {
    f.newer.compatibility[schema] = '2.0.0'
    await f.signManifests()
    await assert.rejects(verifyNativeTransitionPackages(f.inputs, f.installed, f.currentBinary, 'upgrade', f.policy),
      { code: 'native_schema_mismatch' })
    f.newer.compatibility[schema] = '1.0.0'
  }
  f.newer.native_transitions[0].from.sha256 = 'f'.repeat(64)
  await f.signManifests()
  await assert.rejects(verifyNativeTransitionPackages(f.inputs, f.installed, f.currentBinary, 'upgrade', f.policy),
    { code: 'native_transition_missing' })
  f.newer.schema_version = 1
  delete f.newer.native_transitions
  await f.signManifests()
  await assert.rejects(verifyNativeTransitionPackages(f.inputs, f.installed, f.currentBinary, 'upgrade', f.policy),
    { code: 'native_transition_missing' })
})

test('does not expose architecture, bootstrap or signature test seams as command arguments', () => {
  const base = ['scope', 'edition', 'tag', 'manifest', 'bundle', 'package', 'cosign', 'root']
    .flatMap(key => [`--${key}`, key])
  for (const flag of ['arch', 'bootstrapVersion', 'verifySignature', 'verify-signature', 'current-tag']) {
    assert.throws(() => parseNativePackageArgs([...base, `--${flag}`, 'replacement'], ['current-manifest', 'current-bundle']),
      { code: 'invalid_arguments' })
  }
})
