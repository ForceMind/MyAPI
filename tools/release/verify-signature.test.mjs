/* Pure contracts only. Real cosign/vector qualification is a separate Actions entry. */
import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { chmod, link, mkdtemp, readFile, realpath, rm, symlink, writeFile } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'
import {
  assertInputPin, classifyCosignRejection, cosignArguments, inspectBundleStructure,
  OFFICIAL_IDENTITY, qualificationScope, runQualification, snapshotInput, validateOwnership,
} from './verify-signature.mjs'

const env = {
  MYAPI_SIGNATURE_QUALIFICATION: '1', GITHUB_ACTIONS: 'true', GITHUB_EVENT_NAME: 'workflow_dispatch',
  GITHUB_REF: 'refs/heads/dev', GITHUB_REPOSITORY: 'ForceMind/MyAPI', GITHUB_SHA: '1'.repeat(40),
  GITHUB_RUN_ID: '123', GITHUB_RUN_ATTEMPT: '1', RUNNER_TEMP: path.resolve(os.tmpdir(), 'owned-parent'),
}
const sha256 = bytes => createHash('sha256').update(bytes).digest('hex')
const bytes = value => Buffer.from(JSON.stringify(value))
const bundle = () => ({
  mediaType: 'application/vnd.dev.sigstore.bundle+json;version=0.3',
  verificationMaterial: { certificate: { rawBytes: 'AQID' }, tlogEntries: [{ integratedTime: '1710869186',
    inclusionProof: { checkpoint: { envelope: 'structural-placeholder-not-a-signed-checkpoint' } },
    inclusionPromise: { signedEntryTimestamp: 'AQID' } }] },
  messageSignature: { messageDigest: { algorithm: 'SHA2_256', digest: Buffer.alloc(32).toString('base64') }, signature: 'AQID' },
})

test('real verification scope requires exact manual dev identity, Linux x64 and no caller arguments', () => {
  assert.deepEqual(qualificationScope(env, [], 'linux', 'x64'), {
    schema_version: 1, scope: 'official-vector-only', source_sha: env.GITHUB_SHA, run_id: '123', run_attempt: '1',
  })
  for (const change of [
    { MYAPI_SIGNATURE_QUALIFICATION: true }, { MYAPI_SIGNATURE_QUALIFICATION: '0' }, { GITHUB_ACTIONS: 'false' },
    { GITHUB_EVENT_NAME: 'push' }, { GITHUB_REF: 'refs/heads/main' }, { GITHUB_REPOSITORY: 'other/project' },
    { GITHUB_SHA: 'short' }, { GITHUB_SHA: ['1'.repeat(40)] }, { GITHUB_RUN_ID: '0' }, { GITHUB_RUN_ID: 123 }, { GITHUB_RUN_ATTEMPT: '-1' }, { GITHUB_RUN_ATTEMPT: 1 },
    { RUNNER_TEMP: 'relative' }, { RUNNER_TEMP: path.parse(env.RUNNER_TEMP).root }, { RUNNER_TEMP: '/tmp/a/../b' },
  ]) assert.throws(() => qualificationScope({ ...env, ...change }, [], 'linux', 'x64'), /SIGSTORE_(?:SCOPE|IDENTITY)_REJECTED/)
  for (const args of [['--verified=true'], ['--artifact', '/somewhere'], ['--ignore-tlog']]) {
    assert.throws(() => qualificationScope(env, args, 'linux', 'x64'), /SIGSTORE_SCOPE_REJECTED/)
  }
  for (const [platform, arch] of [['darwin', 'arm64'], ['win32', 'x64'], ['linux', 'arm64']]) {
    assert.throws(() => qualificationScope(env, [], platform, arch), /SIGSTORE_PLATFORM_REJECTED/)
  }
})

test('ownership rejects missing, foreign, accessor and caller-verified data without executing getters', () => {
  const scope = qualificationScope(env, [], 'linux', 'x64')
  assert.doesNotThrow(() => validateOwnership({ ...scope }, scope))
  const missing = { ...scope }; delete missing.run_attempt
  for (const value of [missing, { ...scope, source_sha: '2'.repeat(40) }, { ...scope, scope: 'myapi-trusted' },
    { ...scope, verified: true }, { ...scope, verified: false }, { ...scope, run_id: 123 }, [], null]) {
    assert.throws(() => validateOwnership(value, scope), /SIGSTORE_/)
  }
  let accessed = false
  const accessor = { ...scope }
  Object.defineProperty(accessor, 'source_sha', { enumerable: true, get() { accessed = true; return scope.source_sha } })
  assert.throws(() => validateOwnership(accessor, scope), /SIGSTORE_INVALID_FIELDS/)
  assert.equal(accessed, false)
  assert.throws(() => validateOwnership(new Proxy(scope, {}), scope), /SIGSTORE_INVALID_FIELDS/)
})

test('raw input pins bind exact bytes including offsets and Git blob framing, never a verified boolean', () => {
  const full = Buffer.from('prefix\nraw artifact\ntrailer')
  const raw = new Uint8Array(full.buffer, full.byteOffset + 7, 13)
  const before = Buffer.from(full)
  const pin = { id: 'owned-control', bytes: raw.byteLength, sha256: sha256(raw) }
  assert.equal(assertInputPin(raw, pin), sha256(raw))
  const blob = createHash('sha1').update(`blob ${raw.byteLength}\0`).update(raw).digest('hex')
  assert.equal(assertInputPin(raw, { id: 'git-object-control', bytes: raw.byteLength, blob }), sha256(raw))
  assert.deepEqual(full, before)
  const changed = Uint8Array.from(raw); changed[0] ^= 1
  assert.throws(() => assertInputPin(changed, pin), /SIGSTORE_INPUT_PIN_MISMATCH/)
  for (const invalid of [{ ...pin, bytes: 0 }, { ...pin, bytes: NaN }, { ...pin, bytes: Infinity },
    { ...pin, bytes: pin.bytes + 1 }, { ...pin, sha256: '0' }, { ...pin, blob }, { ...pin, verified: true }]) {
    assert.throws(() => assertInputPin(raw, invalid), /SIGSTORE_INVALID_PIN/)
  }
  assert.throws(() => assertInputPin({ verified: true, bytes: raw }, pin), /SIGSTORE_INVALID_BYTES/)
  let called = false
  const accessor = { id: 'control', bytes: raw.byteLength, get sha256() { called = true; return pin.sha256 } }
  assert.throws(() => assertInputPin(raw, accessor), /SIGSTORE_INVALID_PIN/)
  assert.equal(called, false)
})

test('well-formed bundle structure remains explicitly unverified, including an intentionally non-certificate placeholder', () => {
  const structure = inspectBundleStructure(bytes(bundle()))
  assert.equal(structure.kind, 'message')
  assert.equal(structure.cryptographically_verified, false)
  assert.equal(structure.leaf_raw_base64, 'AQID', 'shape parsing cannot establish certificate trust')
  assert.deepEqual(structure.observer_times, [1710869186000])
  assert.equal(Object.isFrozen(structure), true)
  assert.equal(Object.isFrozen(structure.observer_times), true)
  const dsse = bundle()
  dsse.mediaType = 'application/vnd.dev.sigstore.bundle+json;version=0.2'
  dsse.verificationMaterial.x509CertificateChain = { certificates: [dsse.verificationMaterial.certificate] }
  delete dsse.verificationMaterial.certificate
  delete dsse.messageSignature
  dsse.dsseEnvelope = { payloadType: 'application/vnd.in-toto+json', payload: 'AQID', signatures: [{ sig: 'AQID' }] }
  assert.equal(inspectBundleStructure(bytes(dsse)).kind, 'dsse')
  assert.equal(inspectBundleStructure(bytes(dsse)).cryptographically_verified, false)
})

test('missing signature, certificate, digest and trusted-time evidence never become successful structural evidence', () => {
  const changes = [
    value => { value.verified = true },
    value => { value.verificationMaterial.verified = true },
    value => { delete value.verificationMaterial.certificate },
    value => { value.verificationMaterial.certificate = null },
    value => { value.verificationMaterial.x509CertificateChain = { certificates: [] } },
    value => { delete value.messageSignature },
    value => { value.messageSignature.signature = '' },
    value => { value.messageSignature.messageDigest.digest = 'AQID' },
    value => { value.messageSignature.messageDigest.algorithm = 'SHA2_512' },
    value => { value.mediaType = 'unknown' },
    value => { value.verificationMaterial.tlogEntries = [] },
    value => { delete value.verificationMaterial.tlogEntries[0].inclusionProof },
    value => { delete value.verificationMaterial.tlogEntries[0].inclusionPromise },
    value => { value.verificationMaterial.tlogEntries[0].inclusionProof.checkpoint.envelope = {} },
    value => { value.verificationMaterial.tlogEntries[0].integratedTime = null },
    value => { value.verificationMaterial.tlogEntries[0].integratedTime = 0 },
    value => { value.verificationMaterial.tlogEntries[0].integratedTime = '-1' },
    value => { value.verificationMaterial.tlogEntries[0].integratedTime = '9007199254740992' },
    value => { value.verificationMaterial.tlogEntries[0].inclusionPromise.signedEntryTimestamp = 'bad base64' },
  ]
  for (const change of changes) {
    const value = bundle(); change(value)
    assert.throws(() => inspectBundleStructure(bytes(value)), /SIGSTORE_(?:INVALID_FIELDS|BUNDLE_SHAPE|TIME_EVIDENCE_MISSING)/)
  }
  for (const value of [Buffer.alloc(0), Buffer.from('{'), Buffer.from([0xff]), Buffer.from('null'), Buffer.from('[]')]) {
    assert.throws(() => inspectBundleStructure(value), /SIGSTORE_(?:INVALID_BYTES|BUNDLE_SHAPE)/)
  }
})

test('cosign invocation fixes offline, local roots, exact identity and raw artifact with no skip/trust bypass flags', () => {
  const options = { artifact: path.resolve(os.tmpdir(), 'raw bytes #%.txt'), bundle: path.resolve(os.tmpdir(), 'bundle.json'),
    root: path.resolve(os.tmpdir(), 'trusted-root.json'), identity: 'official' }
  assert.deepEqual(cosignArguments(options), ['verify-blob', '--offline', '--new-bundle-format', '--trusted-root', options.root,
    '--certificate-identity', OFFICIAL_IDENTITY.subject, '--certificate-oidc-issuer', OFFICIAL_IDENTITY.issuer,
    '--bundle', options.bundle, '--', options.artifact])
  assert.equal(cosignArguments({ ...options, identity: 'wrong-subject' })[6], 'https://invalid.example.test/not-the-signer')
  assert.equal(cosignArguments({ ...options, identity: 'wrong-issuer' })[8], 'https://invalid.example.test/not-the-issuer')
  for (const value of [{ ...options, verified: true }, { ...options, identity: '.*' }, { ...options, root: 'https://root.example.test' },
    { ...options, artifact: 'relative' }, { ...options, bundle: '/tmp/a\n--ignore-tlog' }]) {
    assert.throws(() => cosignArguments(value), /SIGSTORE_(?:INVALID_FIELDS|ARGUMENT_REJECTED)/)
  }
})

test('only specific upstream security errors classify as a rejection; generic failures never do', () => {
  const vectors = [
    ['Error: failed to verify log inclusion: transparency log signature does not match', 'signature'],
    ['Error: failed to verify signature: could not verify message: invalid signature when validating ASN.1 encoded signature', 'signature'],
    ['Error: artifact digest does not match message digest', 'artifact-digest'],
    ['Error: no matching CertificateIdentity found, last error: expected SAN value "wrong", got "actual"', 'subject'],
    ['Error: no matching CertificateIdentity found, last error: expected issuer value "wrong", got "actual"', 'issuer'],
    ['Error: failed to verify log inclusion: integrated time outside certificate validity', 'certificate-time'],
  ]
  for (const [message, expected] of vectors) assert.equal(classifyCosignRejection(message), expected)
  for (const message of ['', 'Verified OK', 'verification failed', 'certificate expired today', 'leaf certificate verification failed',
    'context deadline exceeded', 'permission denied', 'unknown flag --new-bundle-format', 'no such file', 'out of memory',
    'timeout: integrated time outside certificate validity', 'context deadline exceeded: integrated time outside certificate validity', 'no such file: expected SAN value "x"',
    'panic: could not verify message: invalid signature when validating ASN.1 encoded signature']) {
    assert.equal(classifyCosignRejection(message), 'unclassified')
  }
})

test('rejected execution scope leaves a complete unknown report without fake cosign calls or accepted artifacts', async () => {
  const result = await runQualification({ MYAPI_SIGNATURE_QUALIFICATION: '0' }, [])
  assert.equal(result.passed, false)
  assert.equal(result.phase, 'scope')
  assert.equal(result.error_code, 'SIGSTORE_SCOPE_REJECTED')
  assert.equal(result.myapi_artifact_trusted, false)
  assert.equal(result.installer_connected, false)
  assert.equal(result.tool.before_sha256, null)
  assert.equal(result.tool.after_sha256, null)
  assert.equal(result.tool_unchanged, false)
  assert.equal(result.inputs_unchanged, false)
  assert.equal(result.historical_certificate_accepted, false)
  assert.equal(result.artifact_digest_bound, false)
  assert.equal(result.tampered_digest_differs, false)
  assert.deepEqual(result.tested_counts, { cosign_invocations: 0, positive_accepts: 0, negative_rejections: 0 })
  assert.deepEqual(result.inputs.map(item => item.id), [
    'trusted-root.json', 'artifact.txt', 'message.bundle.json', 'intoto-artifact.txt', 'intoto.bundle.json',
    'intoto-time-invalid.bundle.json', 'fixture-trusted-root.json', 'ownership.json', 'tampered-artifact.txt', 'tampered.bundle.json',
  ])
  assert.deepEqual(result.cases.map(item => item.id), [
    'public_message_valid', 'custom_intoto_valid', 'artifact_tampered', 'signature_tampered', 'wrong_subject', 'wrong_issuer', 'certificate_time_outside',
  ])
  for (const item of result.inputs) assert.deepEqual([item.bytes, item.before_sha256, item.after_sha256, item.pin_verified, item.unchanged], [null, null, null, false, false])
  for (const item of result.cases) assert.deepEqual([item.cosign_exit_code, item.outcome, item.passed], [null, 'not-run', false])
})

test('raw input snapshots preserve literal file bytes and detect deliberate changes, without implying signature validity', async t => {
  if (typeof process.getuid !== 'function') {
    await assert.rejects(snapshotInput(path.resolve(os.tmpdir(), 'never-read')), /SIGSTORE_PLATFORM_REJECTED/)
    return
  }
  const directory = await realpath(await mkdtemp(path.join(os.tmpdir(), 'sigstore-read-contract-')))
  t.after(() => rm(directory, { recursive: true, force: true }))
  const filename = path.join(directory, 'raw artifact #%.txt')
  const first = Buffer.from('literal\r\nbytes\n')
  await writeFile(filename, first, { mode: 0o600, flag: 'wx' })
  const before = await snapshotInput(filename)
  assert.deepEqual(before.bytes, first)
  assert.equal(before.sha256, sha256(first))
  assert.deepEqual(await readFile(filename), first)
  const next = Buffer.from('changed\r\nbytes\n')
  await writeFile(filename, next)
  const after = await snapshotInput(filename)
  assert.notEqual(after.sha256, before.sha256)
  assert.deepEqual(before.bytes, first)
  assert.deepEqual(after.bytes, next)
})

test('non-private modes, symlinks and hardlinks are rejected before a file can be passed to cosign', async t => {
  if (typeof process.getuid !== 'function') {
    await assert.rejects(snapshotInput(path.resolve(os.tmpdir(), 'never-read')), /SIGSTORE_PLATFORM_REJECTED/)
    return
  }
  const directory = await realpath(await mkdtemp(path.join(os.tmpdir(), 'sigstore-file-guard-')))
  t.after(() => rm(directory, { recursive: true, force: true }))
  const filename = path.join(directory, 'payload')
  await writeFile(filename, 'owned unsigned test text', { mode: 0o600, flag: 'wx' })
  await chmod(filename, 0o644)
  await assert.rejects(snapshotInput(filename), /SIGSTORE_INPUT_FILE_UNSAFE/)
  await chmod(filename, 0o600)
  const alias = path.join(directory, 'alias')
  await symlink(filename, alias)
  await assert.rejects(snapshotInput(alias), /SIGSTORE_INPUT_PATH_UNSAFE/)
  await link(filename, path.join(directory, 'hardlink'))
  await assert.rejects(snapshotInput(filename), /SIGSTORE_INPUT_FILE_UNSAFE/)
  await assert.rejects(snapshotInput(directory), /SIGSTORE_INPUT_FILE_UNSAFE/)
})
