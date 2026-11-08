/*
 * Isolated, read-only Sigstore qualification. Copyright (C) 2026 ForceMind.
 * This tool does not authorize a MyAPI artifact or connect to any installer.
 * Inputs are existing official static test vectors; no signing/tlog writes.
 */
import { execFile, execFileSync } from 'node:child_process'
import { createHash, X509Certificate } from 'node:crypto'
import { constants } from 'node:fs'
import { lstat, mkdir, open, readdir, realpath, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { types } from 'node:util'

const repository = fileURLToPath(new URL('../..', import.meta.url))
const requireThat = (value, code) => { if (!value) throw new Error(code) }
const hash = bytes => createHash('sha256').update(bytes).digest('hex')
const safeError = error => /^SIGSTORE_[A-Z0-9_]{1,68}$/.test(error?.message) ? error.message : 'SIGSTORE_IO_FAILURE'
const controlIDs = ['tampered-artifact.txt', 'tampered.bundle.json']
const targets = ['public_message_valid', 'custom_intoto_valid', 'artifact_tampered', 'signature_tampered', 'wrong_subject', 'wrong_issuer', 'certificate_time_outside']

export const COSIGN_PIN = Object.freeze({
  version: 'v3.1.4', file: 'cosign-linux-amd64',
  url: 'https://github.com/sigstore/cosign/releases/download/v3.1.4/cosign-linux-amd64',
  sha256: '424e2c97d0b45fd7a6b608a87f090f35633f4c23ece5f092d48bba11aaf4bbfa',
})
export const VECTOR_COMMIT = '9611941d54398f2e3f6383b6f744442a56d2fb2a'
export const ROOT_COMMIT = 'c9bda74ad2221f938f7d2e0295ca3aad2da710a8'
export const OFFICIAL_IDENTITY = Object.freeze({
  subject: 'https://github.com/sigstore-conformance/extremely-dangerous-public-oidc-beacon/.github/workflows/extremely-dangerous-oidc-beacon.yml@refs/heads/main',
  issuer: 'https://token.actions.githubusercontent.com',
})
// Git blob IDs bind official fixture bytes to the reviewed repository commit.
// They are not artifact signature proofs. The public trust root has its own
// SHA-256 pin published by root-signing metadata/targets.json (version 14).
export const INPUT_PINS = Object.freeze([
  Object.freeze({ id: 'trusted-root.json', sha256: '6494e21ea73fa7ee769f85f57d5a3e6a08725eae1e38c755fc3517c9e6bc0b66', bytes: 6787,
    url: `https://raw.githubusercontent.com/sigstore/root-signing/${ROOT_COMMIT}/targets/trusted_root.json` }),
  Object.freeze({ id: 'artifact.txt', blob: '52c1fa7a106f88642e4e3cdc0c96bc0d4514c2cf', bytes: 109,
    source: 'a.txt' }),
  Object.freeze({ id: 'message.bundle.json', blob: 'b1788a91feb4bf38cbadd48692be07c38c466276', bytes: 10061,
    source: 'happy-path-v0.3/bundle.sigstore.json' }),
  Object.freeze({ id: 'intoto-artifact.txt', blob: 'fa6b1abff522a63181569917e0ff545d6fc3c2e1', bytes: 109,
    source: 'intoto-with-custom-trust-root/artifact' }),
  Object.freeze({ id: 'intoto.bundle.json', blob: 'f88e885da67da04e6e6e022615f1178627f95fe7', bytes: 10723,
    source: 'intoto-with-custom-trust-root/bundle.sigstore.json' }),
  Object.freeze({ id: 'intoto-time-invalid.bundle.json', blob: '4393ebc4a612b0cfefc52e49545f185ff2571e05', bytes: 9989,
    source: 'intoto-set-outside-signing-cert-validity_fail/bundle.sigstore.json' }),
  Object.freeze({ id: 'fixture-trusted-root.json', blob: '4598d26ac2594bdf0f275f8838899d318dbe0a1a', bytes: 3009,
    source: 'intoto-with-custom-trust-root/trusted_root.json' }),
])

function exactData(value, keys) {
  requireThat(value && typeof value === 'object' && !types.isProxy(value) && !Array.isArray(value) &&
    [Object.prototype, null].includes(Object.getPrototypeOf(value)), 'SIGSTORE_INVALID_FIELDS')
  const descriptors = Object.getOwnPropertyDescriptors(value)
  requireThat(Reflect.ownKeys(descriptors).length === keys.length && keys.every(key =>
    descriptors[key] && Object.hasOwn(descriptors[key], 'value') && descriptors[key].enumerable), 'SIGSTORE_INVALID_FIELDS')
}

export function qualificationScope(env, args, platform = process.platform, architecture = process.arch) {
  requireThat(Array.isArray(args) && args.length === 0 && env?.MYAPI_SIGNATURE_QUALIFICATION === '1' &&
    env.GITHUB_ACTIONS === 'true' && env.GITHUB_EVENT_NAME === 'workflow_dispatch' && env.GITHUB_REF === 'refs/heads/dev' &&
    env.GITHUB_REPOSITORY === 'ForceMind/MyAPI', 'SIGSTORE_SCOPE_REJECTED')
  requireThat(platform === 'linux' && architecture === 'x64', 'SIGSTORE_PLATFORM_REJECTED')
  requireThat(typeof env.GITHUB_SHA === 'string' && /^[a-f0-9]{40}$/.test(env.GITHUB_SHA) &&
    typeof env.GITHUB_RUN_ID === 'string' && /^[1-9][0-9]{0,19}$/.test(env.GITHUB_RUN_ID) &&
    typeof env.GITHUB_RUN_ATTEMPT === 'string' && /^[1-9][0-9]{0,5}$/.test(env.GITHUB_RUN_ATTEMPT) && typeof env.RUNNER_TEMP === 'string' &&
    env.RUNNER_TEMP.length <= 4096 && path.isAbsolute(env.RUNNER_TEMP) && path.resolve(env.RUNNER_TEMP) === env.RUNNER_TEMP &&
    env.RUNNER_TEMP !== path.parse(env.RUNNER_TEMP).root && !/[\u0000-\u001f\u007f]/.test(env.RUNNER_TEMP), 'SIGSTORE_IDENTITY_REJECTED')
  return Object.freeze({ schema_version: 1, scope: 'official-vector-only', source_sha: env.GITHUB_SHA,
    run_id: env.GITHUB_RUN_ID, run_attempt: env.GITHUB_RUN_ATTEMPT })
}

export function validateOwnership(value, scope) {
  exactData(value, ['schema_version', 'scope', 'source_sha', 'run_id', 'run_attempt'])
  requireThat(value.schema_version === 1 && value.scope === 'official-vector-only' &&
    value.source_sha === scope.source_sha && value.run_id === scope.run_id && value.run_attempt === scope.run_attempt,
    'SIGSTORE_OWNERSHIP_MISMATCH')
}

export function assertInputPin(bytes, pin) {
  requireThat(!types.isProxy(bytes) && bytes instanceof Uint8Array && bytes.byteLength > 0 && bytes.byteLength <= 200 * 1024 * 1024,
    'SIGSTORE_INVALID_BYTES')
  requireThat(pin && typeof pin === 'object' && !types.isProxy(pin), 'SIGSTORE_INVALID_PIN')
  const allowed = ['id', 'sha256', 'blob', 'bytes', 'source', 'url']
  const properties = Object.getOwnPropertyDescriptors(pin)
  requireThat([Object.prototype, null].includes(Object.getPrototypeOf(pin)) &&
    Reflect.ownKeys(properties).every(key => allowed.includes(key) && Object.hasOwn(properties[key], 'value') && properties[key].enumerable) &&
    Number.isSafeInteger(pin.bytes) && pin.bytes > 0 && pin.bytes === bytes.byteLength &&
    ((typeof pin.sha256 === 'string' && /^[a-f0-9]{64}$/.test(pin.sha256) && pin.blob === undefined) ||
     (typeof pin.blob === 'string' && /^[a-f0-9]{40}$/.test(pin.blob) && pin.sha256 === undefined)), 'SIGSTORE_INVALID_PIN')
  const actual = pin.sha256 ? hash(bytes) : createHash('sha1').update(`blob ${bytes.byteLength}\0`).update(bytes).digest('hex')
  requireThat(actual === (pin.sha256 ?? pin.blob), 'SIGSTORE_INPUT_PIN_MISMATCH')
  return hash(bytes)
}

function base64(value) {
  requireThat(typeof value === 'string' && value.length > 0 && value.length <= 1024 * 1024 &&
    /^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(value), 'SIGSTORE_BUNDLE_SHAPE')
  const bytes = Buffer.from(value, 'base64')
  requireThat(bytes.length > 0 && bytes.toString('base64') === value, 'SIGSTORE_BUNDLE_SHAPE')
  return bytes
}

// Structure only: even a well-formed certificate/SET is still untrusted until
// the actual pinned cosign process verifies it. No caller "verified" claim.
export function inspectBundleStructure(raw) {
  requireThat(!types.isProxy(raw) && raw instanceof Uint8Array && raw.byteLength > 0 && raw.byteLength <= 1024 * 1024,
    'SIGSTORE_INVALID_BYTES')
  let value
  try { value = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(raw)) }
  catch { throw new Error('SIGSTORE_BUNDLE_SHAPE') }
  requireThat(value && typeof value === 'object' && !Array.isArray(value), 'SIGSTORE_BUNDLE_SHAPE')
  const top = ['mediaType', 'verificationMaterial', value.messageSignature ? 'messageSignature' : 'dsseEnvelope']
  exactData(value, top)
  requireThat(['application/vnd.dev.sigstore.bundle+json;version=0.2', 'application/vnd.dev.sigstore.bundle+json;version=0.3',
    'application/vnd.dev.sigstore.bundle.v0.3+json'].includes(value.mediaType), 'SIGSTORE_BUNDLE_SHAPE')
  const material = value.verificationMaterial
  requireThat(material && typeof material === 'object' && !Array.isArray(material) &&
    Object.keys(material).every(key => ['certificate', 'x509CertificateChain', 'tlogEntries', 'timestampVerificationData'].includes(key)),
    'SIGSTORE_BUNDLE_SHAPE')
  requireThat(Boolean(material.certificate) !== Boolean(material.x509CertificateChain), 'SIGSTORE_BUNDLE_SHAPE')
  const certificates = material.certificate ? [material.certificate] : material.x509CertificateChain?.certificates
  requireThat(Array.isArray(certificates) && certificates.length >= 1 && certificates.length <= 8, 'SIGSTORE_BUNDLE_SHAPE')
  for (const certificate of certificates) { exactData(certificate, ['rawBytes']); base64(certificate.rawBytes) }
  requireThat(Array.isArray(material.tlogEntries) && material.tlogEntries.length >= 1 && material.tlogEntries.length <= 8,
    'SIGSTORE_TIME_EVIDENCE_MISSING')
  const times = []
  for (const entry of material.tlogEntries) {
    requireThat(entry && typeof entry === 'object' && typeof entry.integratedTime === 'string' &&
      /^(0|[1-9][0-9]{0,12})$/.test(entry.integratedTime) && typeof entry.inclusionProof?.checkpoint?.envelope === 'string' && entry.inclusionProof.checkpoint.envelope.length > 0 &&
      entry.inclusionPromise?.signedEntryTimestamp, 'SIGSTORE_TIME_EVIDENCE_MISSING')
    base64(entry.inclusionPromise.signedEntryTimestamp)
    const milliseconds = Number(entry.integratedTime) * 1000
    requireThat(Number.isSafeInteger(milliseconds), 'SIGSTORE_TIME_EVIDENCE_MISSING')
    times.push(milliseconds)
  }
  if (value.messageSignature) {
    exactData(value.messageSignature, ['messageDigest', 'signature'])
    exactData(value.messageSignature.messageDigest, ['algorithm', 'digest'])
    requireThat(value.messageSignature.messageDigest.algorithm === 'SHA2_256' &&
      base64(value.messageSignature.messageDigest.digest).length === 32, 'SIGSTORE_BUNDLE_SHAPE')
    base64(value.messageSignature.signature)
  } else {
    exactData(value.dsseEnvelope, ['payload', 'payloadType', 'signatures'])
    requireThat(value.dsseEnvelope.payloadType === 'application/vnd.in-toto+json' && Array.isArray(value.dsseEnvelope.signatures) &&
      value.dsseEnvelope.signatures.length === 1, 'SIGSTORE_BUNDLE_SHAPE')
    base64(value.dsseEnvelope.payload)
    base64(value.dsseEnvelope.signatures[0]?.sig)
  }
  return Object.freeze({ kind: value.messageSignature ? 'message' : 'dsse', cryptographically_verified: false,
    observer_times: Object.freeze(times), leaf_raw_base64: certificates[0].rawBytes })
}

// --offline alone may still fetch TUF. The explicit local --trusted-root
// makes v3.1.4 SetTrustedMaterial return before TUF/client initialization.
export function cosignArguments(options) {
  exactData(options, ['artifact', 'bundle', 'root', 'identity'])
  for (const key of ['artifact', 'bundle', 'root']) {
    requireThat(typeof options[key] === 'string' && path.isAbsolute(options[key]) &&
      !/[\u0000-\u001f\u007f]/.test(options[key]), 'SIGSTORE_ARGUMENT_REJECTED')
  }
  requireThat(['official', 'wrong-subject', 'wrong-issuer'].includes(options.identity), 'SIGSTORE_ARGUMENT_REJECTED')
  return ['verify-blob', '--offline', '--new-bundle-format', '--trusted-root', options.root,
    '--certificate-identity', options.identity === 'wrong-subject' ? 'https://invalid.example.test/not-the-signer' : OFFICIAL_IDENTITY.subject,
    '--certificate-oidc-issuer', options.identity === 'wrong-issuer' ? 'https://invalid.example.test/not-the-issuer' : OFFICIAL_IDENTITY.issuer,
    '--bundle', options.bundle, '--', options.artifact]
}

// These are pinned cosign v3.1.4 / sigstore-go v1.3.0 failure strings, never
// generic "verification failed" or a caller-supplied success boolean.
export function classifyCosignRejection(text) {
  if (typeof text !== 'string' || text.length > 256 * 1024 ||
      /no such file|permission denied|access denied|unknown (?:flag|command)|usage:|panic:|runtime error|out of memory|fatal error|deadline exceeded|input\/output error|exec format error|timed? out|timeout|connection refused|network is unreachable|unexpected end of json|failed to load|loading trusted root/i.test(text)) return 'unclassified'
  if (/integrated time outside certificate validity/.test(text)) return 'certificate-time'
  if (/no matching CertificateIdentity found[^\n]*expected SAN value /.test(text)) return 'subject'
  if (/no matching CertificateIdentity found[^\n]*expected issuer value /.test(text)) return 'issuer'
  if (/artifact digest does not match message digest|artifact does not match digest|provided artifact digests? (?:do|does) not match/.test(text)) return 'artifact-digest'
  if (/transparency log signature does not match|could not verify message: invalid signature when validating (?:ASN\.1|IEEE_P1363) encoded signature/.test(text)) return 'signature'
  return 'unclassified'
}

function identity(stat) {
  return [stat.dev, stat.ino, stat.size, stat.mode, stat.uid, stat.gid, stat.nlink, stat.mtimeNs, stat.ctimeNs].map(String).join(':')
}

export async function snapshotInput(filename, executable = false) {
  requireThat(typeof process.getuid === 'function', 'SIGSTORE_PLATFORM_REJECTED')
  requireThat(typeof executable === 'boolean', 'SIGSTORE_INPUT_FILE_UNSAFE')
  requireThat(typeof filename === 'string' && path.isAbsolute(filename) && await realpath(filename) === filename, 'SIGSTORE_INPUT_PATH_UNSAFE')
  const initial = await lstat(filename, { bigint: true })
  const maximum = executable ? 200 * 1024 * 1024 : 1024 * 1024
  requireThat(initial.isFile() && !initial.isSymbolicLink() && initial.nlink === 1n && initial.uid === BigInt(process.getuid()) &&
    (initial.mode & 0o7777n) === (executable ? 0o700n : 0o600n) && initial.size > 0n && initial.size <= BigInt(maximum), 'SIGSTORE_INPUT_FILE_UNSAFE')
  const descriptor = await open(filename, constants.O_RDONLY | constants.O_NOFOLLOW)
  try {
    const opened = await descriptor.stat({ bigint: true })
    requireThat(identity(opened) === identity(initial), 'SIGSTORE_INPUT_CHANGED')
    const bytes = await descriptor.readFile()
    const after = await descriptor.stat({ bigint: true })
    const named = await lstat(filename, { bigint: true })
    requireThat(identity(after) === identity(initial) && identity(named) === identity(initial), 'SIGSTORE_INPUT_CHANGED')
    return { bytes, sha256: hash(bytes), identity: identity(initial), byte_length: bytes.length }
  } finally { await descriptor.close() }
}

function executeCosign(filename, args, workingDirectory, onSpawn) {
  return new Promise(resolve => {
    let spawned = false
    const child = execFile(filename, args, { cwd: workingDirectory, shell: false, encoding: 'utf8',
      env: { PATH: '/usr/bin:/bin', LANG: 'C', LC_ALL: 'C', TZ: 'UTC' }, timeout: 30000, killSignal: 'SIGKILL', maxBuffer: 128 * 1024 },
    (error, stdout, stderr) => {
      const exit = error ? error.code : 0
      if (!spawned || error?.killed || error?.signal || !Number.isSafeInteger(exit) || exit < 0 || exit > 125) {
        resolve({ exit: null, outcome: 'tool-failure', rejection: 'unclassified' }); return
      }
      if (exit === 0) { resolve({ exit, outcome: 'accepted', rejection: null }); return }
      const rejection = classifyCosignRejection(stderr + '\n' + stdout)
      resolve({ exit, outcome: rejection === 'unclassified' ? 'tool-failure' : 'rejected', rejection })
    })
    child.once('spawn', () => { spawned = true; onSpawn() })
  })
}

export async function runQualification(env = process.env, args = process.argv.slice(2)) {
  const report = { schema_version: 1, scope: 'official-vector-only', source_sha: null, run_id: null, run_attempt: null,
    passed: false, phase: 'scope', error_code: null, read_only: true, offline: true,
    myapi_artifact_trusted: false, installer_connected: false,
    tool: { version: COSIGN_PIN.version, expected_sha256: COSIGN_PIN.sha256, before_sha256: null, after_sha256: null, unchanged: false },
    inputs: [...INPUT_PINS.map(item => item.id), 'ownership.json', ...controlIDs].map(id => ({ id, bytes: null, before_sha256: null, after_sha256: null, pin_verified: false, unchanged: false })),
    cases: targets.map((id, index) => ({ id, expected: index < 2 ? 'accept' : 'reject', passed: false, cosign_exit_code: null, outcome: 'not-run', rejection: null })),
    tested_counts: { cosign_invocations: 0, positive_accepts: 0, negative_rejections: 0 },
    inputs_unchanged: false, tool_unchanged: false, historical_certificate_accepted: false }
  let inputDirectory = null, reportDirectory = null, inputIdentity = null, reportIdentity = null, controlIdentity = null, toolBefore = null, scope = null
  const snapshots = new Map()
  const git = (...parts) => execFileSync('git', ['-c', 'core.fileMode=false', ...parts], { cwd: repository, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], timeout: 10000 }).trim()
  try {
    scope = qualificationScope(env, args)
    Object.assign(report, { source_sha: scope.source_sha, run_id: scope.run_id, run_attempt: scope.run_attempt })
    requireThat(git('rev-parse', 'HEAD') === scope.source_sha && git('status', '--porcelain', '--untracked-files=no') === '', 'SIGSTORE_SOURCE_MISMATCH')
    const temporary = await realpath(env.RUNNER_TEMP)
    const parent = await lstat(temporary)
    requireThat(temporary === env.RUNNER_TEMP && parent.isDirectory() && parent.uid === process.getuid(), 'SIGSTORE_TEMP_UNSAFE')
    const suffix = `${scope.source_sha}-${scope.run_id}-${scope.run_attempt}`
    inputDirectory = path.join(temporary, `myapi-signature-input-${suffix}`)
    reportDirectory = path.join(temporary, `myapi-signature-report-${suffix}`)
    report.phase = 'inputs'
    const directory = await lstat(inputDirectory, { bigint: true })
    requireThat(directory.isDirectory() && !directory.isSymbolicLink() && await realpath(inputDirectory) === inputDirectory &&
      directory.uid === BigInt(process.getuid()) && (directory.mode & 0o7777n) === 0o700n, 'SIGSTORE_INPUT_DIRECTORY_UNSAFE')
    inputIdentity = identity(directory)
    const expected = [...INPUT_PINS.map(item => item.id), 'ownership.json', COSIGN_PIN.file].sort()
    requireThat((await readdir(inputDirectory)).sort().join('\n') === expected.join('\n'), 'SIGSTORE_INPUT_SET_MISMATCH')
    await mkdir(reportDirectory, { mode: 0o700 })
    const created = await lstat(reportDirectory, { bigint: true })
    requireThat(created.isDirectory() && !created.isSymbolicLink() && created.uid === BigInt(process.getuid()) &&
      (created.mode & 0o7777n) === 0o700n && await realpath(reportDirectory) === reportDirectory, 'SIGSTORE_REPORT_DIRECTORY_UNSAFE')
    reportIdentity = `${created.dev}:${created.ino}`
    for (const item of report.inputs.filter(item => !controlIDs.includes(item.id))) {
      const snapshot = await snapshotInput(path.join(inputDirectory, item.id))
      snapshots.set(item.id, snapshot)
      item.bytes = snapshot.byte_length
      item.before_sha256 = snapshot.sha256
      if (item.id === 'ownership.json') {
        let ownership
        try { ownership = JSON.parse(snapshot.bytes.toString('utf8')) } catch { throw new Error('SIGSTORE_OWNERSHIP_MISMATCH') }
        validateOwnership(ownership, scope)
      } else assertInputPin(snapshot.bytes, INPUT_PINS.find(pin => pin.id === item.id))
      item.pin_verified = true
    }
    toolBefore = await snapshotInput(path.join(inputDirectory, COSIGN_PIN.file), true)
    report.tool.before_sha256 = toolBefore.sha256
    requireThat(toolBefore.sha256 === COSIGN_PIN.sha256, 'SIGSTORE_TOOL_PIN_MISMATCH')
    toolBefore.bytes = null
    const message = snapshots.get('message.bundle.json').bytes
    const intoto = snapshots.get('intoto.bundle.json').bytes
    const invalidTime = snapshots.get('intoto-time-invalid.bundle.json').bytes
    const publicStructure = inspectBundleStructure(message)
    requireThat(publicStructure.kind === 'message' && inspectBundleStructure(intoto).kind === 'dsse' &&
      inspectBundleStructure(invalidTime).kind === 'dsse', 'SIGSTORE_VECTOR_KIND_MISMATCH')
    const certificate = new X509Certificate(Buffer.from(publicStructure.leaf_raw_base64, 'base64'))
    const expiredNow = Date.parse(certificate.validTo) < Date.now()
    const invalidStructure = inspectBundleStructure(invalidTime)
    const invalidCertificate = new X509Certificate(Buffer.from(invalidStructure.leaf_raw_base64, 'base64'))
    const earliest = Date.parse(invalidCertificate.validFrom), latest = Date.parse(invalidCertificate.validTo)
    requireThat(Number.isFinite(earliest) && Number.isFinite(latest) && invalidStructure.observer_times.some(time => time < earliest || time > latest),
      'SIGSTORE_VECTOR_TIME_NOT_TARGETED')
    report.phase = 'verification'
    const invoke = async (index, artifact, bundle, root, identityPolicy, allowed) => {
      const row = report.cases[index]
      const result = await executeCosign(path.join(inputDirectory, COSIGN_PIN.file), cosignArguments({ artifact, bundle, root, identity: identityPolicy }),
        reportDirectory, () => { report.tested_counts.cosign_invocations++ })
      Object.assign(row, { cosign_exit_code: result.exit, outcome: result.outcome, rejection: result.rejection })
      row.passed = row.expected === 'accept' ? result.outcome === 'accepted' && result.exit === 0
        : result.outcome === 'rejected' && result.exit > 0 && allowed.includes(result.rejection)
      if (row.passed && row.expected === 'accept') report.tested_counts.positive_accepts++
      if (row.passed && row.expected === 'reject') report.tested_counts.negative_rejections++
      requireThat(row.passed, result.outcome === 'tool-failure' ? 'SIGSTORE_TOOL_FAILURE' : 'SIGSTORE_CASE_MISMATCH')
    }
    const file = id => path.join(inputDirectory, id)
    await invoke(0, file('artifact.txt'), file('message.bundle.json'), file('trusted-root.json'), 'official', [])
    report.historical_certificate_accepted = expiredNow
    requireThat(expiredNow, 'SIGSTORE_HISTORICAL_BOUNDARY_NOT_EXERCISED')
    await invoke(1, file('intoto-artifact.txt'), file('intoto.bundle.json'), file('fixture-trusted-root.json'), 'official', [])
    const alteredArtifact = path.join(reportDirectory, 'tampered-artifact.txt')
    const alteredArtifactBytes = Buffer.concat([snapshots.get('artifact.txt').bytes, Buffer.from('\nchanged control\n')])
    await writeFile(alteredArtifact, alteredArtifactBytes, { flag: 'wx', mode: 0o600 })
    const alteredBundle = path.join(reportDirectory, 'tampered.bundle.json')
    const changed = JSON.parse(message.toString('utf8'))
    const signature = Buffer.from(changed.messageSignature.signature, 'base64')
    signature[signature.length - 1] ^= 1
    changed.messageSignature.signature = signature.toString('base64')
    const alteredBundleBytes = Buffer.from(JSON.stringify(changed))
    await writeFile(alteredBundle, alteredBundleBytes, { flag: 'wx', mode: 0o600 })
    for (const [id, expectedBytes] of [[controlIDs[0], alteredArtifactBytes], [controlIDs[1], alteredBundleBytes]]) {
      const snapshot = await snapshotInput(path.join(reportDirectory, id))
      const original = id === controlIDs[0] ? snapshots.get('artifact.txt') : snapshots.get('message.bundle.json')
      requireThat(snapshot.sha256 === hash(expectedBytes) && snapshot.bytes.equals(expectedBytes) && snapshot.sha256 !== original.sha256,
        'SIGSTORE_NEGATIVE_NOT_MUTATED')
      snapshots.set(id, snapshot)
      const item = report.inputs.find(value => value.id === id)
      Object.assign(item, { bytes: snapshot.byte_length, before_sha256: snapshot.sha256, pin_verified: true })
    }
    inspectBundleStructure(Buffer.from(JSON.stringify(changed)))
    controlIdentity = identity(await lstat(reportDirectory, { bigint: true }))
    await invoke(2, alteredArtifact, file('message.bundle.json'), file('trusted-root.json'), 'official', ['artifact-digest'])
    await invoke(3, file('artifact.txt'), alteredBundle, file('trusted-root.json'), 'official', ['signature'])
    await invoke(4, file('artifact.txt'), file('message.bundle.json'), file('trusted-root.json'), 'wrong-subject', ['subject'])
    await invoke(5, file('artifact.txt'), file('message.bundle.json'), file('trusted-root.json'), 'wrong-issuer', ['issuer'])
    await invoke(6, file('intoto-artifact.txt'), file('intoto-time-invalid.bundle.json'), file('fixture-trusted-root.json'), 'official', ['certificate-time'])
    report.phase = 'preservation'
  } catch (error) { report.error_code = safeError(error) }
  finally {
    if (inputDirectory && inputIdentity) {
      try {
        const current = await lstat(inputDirectory, { bigint: true })
        requireThat(current.isDirectory() && !current.isSymbolicLink() && identity(current) === inputIdentity &&
          current.uid === BigInt(process.getuid()) && (current.mode & 0o7777n) === 0o700n, 'SIGSTORE_INPUT_DIRECTORY_CHANGED')
        requireThat((await readdir(inputDirectory)).sort().join('\n') === [...INPUT_PINS.map(item => item.id), 'ownership.json', COSIGN_PIN.file].sort().join('\n'), 'SIGSTORE_INPUT_SET_MISMATCH')
        if (controlIdentity) requireThat(identity(await lstat(reportDirectory, { bigint: true })) === controlIdentity,
          'SIGSTORE_CONTROL_DIRECTORY_CHANGED')
        for (const item of report.inputs) {
          const before = snapshots.get(item.id)
          if (!before) continue
          try {
            const after = await snapshotInput(path.join(controlIDs.includes(item.id) ? reportDirectory : inputDirectory, item.id))
            item.after_sha256 = after.sha256
            item.unchanged = before.sha256 === after.sha256 && before.identity === after.identity
            requireThat(item.unchanged, 'SIGSTORE_INPUT_CHANGED')
          } catch (error) { report.error_code ??= safeError(error) }
        }
        report.inputs_unchanged = report.inputs.every(item => item.unchanged && item.pin_verified)
        if (toolBefore) {
          try {
            const after = await snapshotInput(path.join(inputDirectory, COSIGN_PIN.file), true)
            report.tool.after_sha256 = after.sha256
            report.tool.unchanged = after.sha256 === toolBefore.sha256 && after.identity === toolBefore.identity && after.sha256 === COSIGN_PIN.sha256
            report.tool_unchanged = report.tool.unchanged
            requireThat(report.tool_unchanged, 'SIGSTORE_TOOL_CHANGED')
          } catch (error) { report.error_code ??= safeError(error) }
        }
        requireThat(git('rev-parse', 'HEAD') === scope.source_sha && git('status', '--porcelain', '--untracked-files=no') === '', 'SIGSTORE_SOURCE_CHANGED')
      } catch (error) { report.error_code ??= safeError(error) }
    }
  }
  report.passed = report.error_code === null && report.inputs_unchanged && report.tool_unchanged && report.historical_certificate_accepted &&
    report.cases.every(row => row.passed) && report.tested_counts.cosign_invocations === 7 &&
    report.tested_counts.positive_accepts === 2 && report.tested_counts.negative_rejections === 5
  if (report.passed) report.phase = 'completed'
  else report.error_code ??= 'SIGSTORE_INCOMPLETE'
  if (reportIdentity) {
    try {
      const directory = await lstat(reportDirectory, { bigint: true })
      requireThat(directory.isDirectory() && !directory.isSymbolicLink() && `${directory.dev}:${directory.ino}` === reportIdentity &&
        directory.uid === BigInt(process.getuid()) && (directory.mode & 0o7777n) === 0o700n, 'SIGSTORE_REPORT_DIRECTORY_CHANGED')
      await writeFile(path.join(reportDirectory, 'report.json'), JSON.stringify(report, null, 2) + '\n', { flag: 'wx', mode: 0o600 })
    }
    catch { report.passed = false; report.error_code ??= 'SIGSTORE_REPORT_WRITE_FAILED' }
  }
  return report
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const report = await runQualification()
  process.stdout.write(JSON.stringify(report) + '\n')
  process.exitCode = report.passed ? 0 : 1
}
