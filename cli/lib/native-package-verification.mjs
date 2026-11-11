import { execFile } from 'node:child_process'
import { createHash } from 'node:crypto'
import { constants, createWriteStream } from 'node:fs'
import { chmod, mkdtemp, open, readFile, rm, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { Transform } from 'node:stream'
import { pipeline } from 'node:stream/promises'
import { pathToFileURL } from 'node:url'
import { promisify } from 'node:util'
import { compareSemver, selectReleaseArtifact } from './release-manifest.mjs'

const run = promisify(execFile)
const maxDocumentBytes = 4 * 1024 * 1024
const releaseBase = 'https://github.com/ForceMind/MyAPI'
const issuer = 'https://token.actions.githubusercontent.com'
// Immutable assets of Cosign v3.1.4 and the pinned Sigstore TUF target.
const cosign = {
  amd64: { size: 145688404, digest: '424e2c97d0b45fd7a6b608a87f090f35633f4c23ece5f092d48bba11aaf4bbfa' },
  arm64: { size: 136437761, digest: 'ade83b7aac62e131cacfe96d958948b2150cc9efec46a570a2649b8e27dedad2' },
}
const rootDigest = '6494e21ea73fa7ee769f85f57d5a3e6a08725eae1e38c755fc3517c9e6bc0b66'
const inputFlags = constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK

function refuse(code, message) {
  throw Object.assign(new Error(message), { code })
}

async function readDocument(filename) {
  const file = await open(filename, inputFlags)
  try {
    const stat = await file.stat()
    const { size } = stat
    if (!stat.isFile() || size < 1 || size > maxDocumentBytes) {
      refuse('invalid_document', 'Manifest, bundle and trust root must be bounded regular files.')
    }
    const bytes = Buffer.alloc(size + 1)
    const { bytesRead } = await file.read(bytes, 0, bytes.length, 0)
    if (bytesRead !== size) refuse('invalid_document', 'Document changed or could not be read completely.')
    return bytes.subarray(0, size)
  } finally {
    await file.close()
  }
}

async function verifyManifestSignature(bytes, inputs, arch, tag) {
  const root = await readDocument(inputs.root)
  if (createHash('sha256').update(root).digest('hex') !== rootDigest) {
    refuse('trust_root_mismatch', 'Use the pinned Sigstore trust root.')
  }
  const bundle = await readDocument(inputs.bundle)
  const workspace = await mkdtemp('/tmp/myapi-native-verify-')
  try {
    const executable = path.join(workspace, 'cosign')
    const input = await open(inputs.cosign, inputFlags)
    try {
      const pin = cosign[arch]
      const stat = await input.stat()
      if (!stat.isFile() || stat.size !== pin.size) refuse('cosign_mismatch', 'Use the pinned Cosign binary for this CPU.')
      const hash = createHash('sha256')
      await pipeline(input.createReadStream({ autoClose: false, start: 0, end: pin.size - 1 }),
        new Transform({
          transform(chunk, _encoding, callback) {
            hash.update(chunk)
            callback(null, chunk)
          },
        }), createWriteStream(executable, { flags: 'wx', mode: 0o600 }))
      if (hash.digest('hex') !== pin.digest) refuse('cosign_mismatch', 'Cosign binary digest does not match the pin.')
    } finally {
      await input.close()
    }
    await chmod(executable, 0o700)
    const manifestPath = path.join(workspace, 'manifest.json')
    const bundlePath = path.join(workspace, 'bundle.json')
    const rootPath = path.join(workspace, 'root.json')
    for (const [filename, data] of [[manifestPath, bytes], [bundlePath, bundle], [rootPath, root]]) {
      await writeFile(filename, data, { flags: 'wx', mode: 0o600 })
    }
    try {
      await run(executable, ['verify-blob', '--offline', '--new-bundle-format', '--trusted-root', rootPath,
        '--certificate-oidc-issuer', issuer,
        '--certificate-identity', `${releaseBase}/.github/workflows/release.yml@refs/tags/${tag}`,
        '--bundle', bundlePath, '--', manifestPath], {
        cwd: workspace, env: { HOME: workspace }, timeout: 30_000, maxBuffer: 128 * 1024,
      })
    } catch (error) {
      if (error.code !== 1 || error.killed) refuse('verifier_failed', 'Cosign could not finish verification.')
      if (/no matching CertificateIdentity found[^\n]*expected SAN value /.test(error.stderr)) {
        refuse('signer_mismatch', 'Manifest signer does not match the release workflow and tag.')
      }
      refuse('signature_rejected', 'Manifest signature or release signing identity did not pass verification.')
    }
  } finally {
    await rm(workspace, { recursive: true })
  }
}

async function binaryDigest(filename, arch) {
  const file = await open(filename, inputFlags)
  try {
    const before = await file.stat({ bigint: true })
    const header = Buffer.alloc(64)
    if (!before.isFile() || before.size < 64n || before.size > BigInt(Number.MAX_SAFE_INTEGER)) {
      refuse('invalid_binary', 'The package must be a regular Linux binary.')
    }
    const { bytesRead } = await file.read(header, 0, header.length, 0)
    const machine = arch === 'amd64' ? 62 : 183
    if (bytesRead !== header.length || !header.subarray(0, 7).equals(Buffer.from([0x7f, 69, 76, 70, 2, 1, 1])) ||
      ![2, 3].includes(header.readUInt16LE(16)) || header.readUInt16LE(18) !== machine) {
      refuse('invalid_binary', 'The package must be a Linux ELF executable for the requested CPU.')
    }
    const hash = createHash('sha256').update(header)
    let bytesHashed = header.length
    if (before.size > BigInt(header.length)) {
      for await (const chunk of file.createReadStream({
        autoClose: false, start: header.length, end: Number(before.size) - 1,
      })) {
        hash.update(chunk)
        bytesHashed += chunk.length
      }
    }
    const after = await file.stat({ bigint: true })
    if (BigInt(bytesHashed) !== before.size || after.size !== before.size ||
      after.mtimeNs !== before.mtimeNs || after.ctimeNs !== before.ctimeNs) {
      refuse('package_changed', 'The package changed during verification.')
    }
    return hash.digest('hex')
  } finally {
    await file.close()
  }
}

// The CLI supplies the pinned verifier. Tests may exercise the same package
// checks with locally generated signatures without trusting them in the CLI.
export async function verifyNativePackage(inputs, { arch, bootstrapVersion, verifySignature }) {
  const { tag, scope, edition } = inputs
  if (!['amd64', 'arm64'].includes(arch) || !['system', 'user'].includes(scope) ||
    !['full', 'lite'].includes(edition) || (scope === 'user' && edition === 'full')) {
    refuse('invalid_target', 'Choose system Full/Lite or user Lite on Linux amd64 or arm64.')
  }
  const version = tag.slice(1)
  if (tag.length > 257 || !/^v[\da-zA-Z.+-]+$/.test(tag)) refuse('invalid_tag', 'Provide an exact v-prefixed release tag.')
  try {
    compareSemver(version, version)
  } catch {
    refuse('invalid_tag', 'Provide an exact v-prefixed release tag.')
  }
  const bytes = await readDocument(inputs.manifest)
  await verifySignature(bytes, inputs, arch, tag)
  let manifest
  try {
    manifest = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes))
  } catch {
    refuse('invalid_manifest', 'The signed Manifest must be UTF-8 JSON.')
  }
  const artifact = selectReleaseArtifact(manifest, {
    operation: 'fresh-install', edition, installationShape: scope === 'system' ? 'server-native' : 'personal-native',
    os: 'linux', arch, kind: 'binary', bootstrapVersion,
  })
  if (manifest.product.version !== version) refuse('version_mismatch', 'Signed version does not match the requested tag.')
  const prefix = `${releaseBase}/releases/download/${tag}/`
  if (!artifact.location.startsWith(prefix) || !/^[a-zA-Z0-9_-][a-zA-Z0-9._-]*$/.test(artifact.location.slice(prefix.length))) {
    refuse('source_mismatch', 'The package must be listed under the requested ForceMind/MyAPI release tag.')
  }
  const sha256 = await binaryDigest(inputs.package, arch)
  if (sha256 !== artifact.sha256_or_digest.toLowerCase()) refuse('digest_mismatch', 'Package digest does not match the signed Manifest.')
  return { version, edition, scope, arch, artifact: artifact.id, sourceSha: manifest.product.source_sha, sha256 }
}

async function main(args) {
  const required = ['scope', 'edition', 'tag', 'manifest', 'bundle', 'package', 'cosign', 'root']
  const inputs = Object.create(null)
  for (let index = 0; index < args.length; index += 2) {
    const key = args[index].slice(2)
    const value = args[index + 1]
    if (!args[index].startsWith('--') || !required.includes(key) || Object.hasOwn(inputs, key) || !value) {
      refuse('invalid_arguments', 'Provide each required option exactly once.')
    }
    inputs[key] = value
  }
  if (required.some(key => !Object.hasOwn(inputs, key))) refuse('invalid_arguments', 'Provide each required option exactly once.')
  const arch = process.platform === 'linux' && { x64: 'amd64', arm64: 'arm64' }[process.arch]
  const bootstrapVersion = (await readFile(new URL('../../VERSION', import.meta.url), 'utf8')).trim()
  const result = await verifyNativePackage(inputs, { arch, bootstrapVersion, verifySignature: verifyManifestSignature })
  console.log(JSON.stringify(result))
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main(process.argv.slice(2)).catch(error => {
    const code = error.code || 'verification_failed'
    const reason = /^[a-z_]+$/.test(code) ? error.message : 'Check that the input files are readable.'
    console.error(`MyAPI package verification failed [${code}]. ${reason}`)
    process.exitCode = 2
  })
}
