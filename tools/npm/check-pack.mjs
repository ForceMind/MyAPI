/*
MyAPI distribution tooling for the New API based custom source release.
Copyright (C) 2026 ForceMind

Licensed under the GNU Affero General Public License version 3 or later.
*/
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { spawnSync } from 'node:child_process'

const result = spawnSync(
  'npm',
  ['pack', '--dry-run', '--json', '--ignore-scripts'],
  { encoding: 'utf8', shell: false }
)

if (result.status !== 0) {
  process.stderr.write(result.stderr)
  process.exit(result.status || 1)
}

const reports = JSON.parse(result.stdout)
if (!Array.isArray(reports) || reports.length !== 1) {
  throw new Error('npm pack returned an unexpected report')
}

const report = reports[0]
const paths = report.files.map((file) => file.path)
const manifest = JSON.parse(readFileSync('SOURCE_MANIFEST.json', 'utf8'))
const metadata = JSON.parse(readFileSync('package.json', 'utf8'))
if (
  manifest.distribution !== metadata.name ||
  manifest.version !== metadata.version
) {
  throw new Error('SOURCE_MANIFEST.json distribution metadata is stale')
}
const required = [
  'package.json',
  'cli/myapi.mjs',
  'Dockerfile',
  'deploy/docker-compose.yml',
  'AGENTS.md',
  'LICENSE',
  'NOTICE',
  'README.md',
  'SOURCE_MANIFEST.json',
  'THIRD-PARTY-LICENSES.md',
  'web/public/myapi-logo-v1.png',
]
for (const file of required) {
  if (!paths.includes(file)) throw new Error(`required package file is missing: ${file}`)
}

const forbidden = paths.filter((file) => {
  if (/(^|\/)\.env\.example$/.test(file)) return false
  return (
    /(^|\/)\.env(?:\.|$)/.test(file) ||
    /(^|\/)(?:node_modules|dist|coverage)(?:\/|$)/.test(file) ||
    /^(?:data|logs|backups|cache)(?:\/|$)/.test(file) ||
    /^deploy\/(?:data|logs|backups|cache)(?:\/|$)/.test(file) ||
    /\.(?:db(?:-.*)?|sqlite3?|log|tgz)$/i.test(file)
  )
})
if (forbidden.length > 0) {
  throw new Error(`sensitive or generated files would be published:\n${forbidden.join('\n')}`)
}

const credentialPatterns = [
  ['private key', /^-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----$/m],
  ['npm token', /\bnpm_[A-Za-z0-9]{36}\b/],
  ['GitHub token', /\bgh[pousr]_[A-Za-z0-9]{36,255}\b/],
  ['AWS access key', /\bAKIA[0-9A-Z]{16}\b/],
  ['API key', /\bsk[_-][A-Za-z0-9_-]{32,}\b/],
  [
    'JWT',
    /\beyJ[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}\b/,
  ],
]
const credentialFindings = []
for (const file of paths) {
  const bytes = readFileSync(file)
  if (bytes.length > 2 * 1024 * 1024 || bytes.includes(0)) continue
  const content = bytes.toString('utf8')
  for (const [name, pattern] of credentialPatterns) {
    if (pattern.test(content)) credentialFindings.push(`${file}: ${name}`)
  }
}
if (credentialFindings.length > 0) {
  throw new Error(
    `credential-like content would be published:\n${credentialFindings.join('\n')}`
  )
}

const head = spawnSync('git', ['rev-parse', 'HEAD'], {
  encoding: 'utf8',
  shell: false,
})
if (head.status !== 0) throw new Error('unable to resolve the source commit')
if (manifest.sourceTreeDirty !== false) {
  throw new Error('SOURCE_MANIFEST.json was generated from a dirty source tree')
}
if (manifest.sourceCommit !== head.stdout.trim()) {
  throw new Error('SOURCE_MANIFEST.json does not match the current source commit')
}

const missingHashes = paths.filter(
  (file) => file !== 'SOURCE_MANIFEST.json' && !manifest.files?.[file]
)
if (missingHashes.length > 0) {
  throw new Error(
    `package files are missing from SOURCE_MANIFEST.json:\n${missingHashes.join('\n')}`
  )
}

const unpackedPaths = new Set(paths)
const extraHashes = Object.keys(manifest.files ?? {}).filter(
  (file) => !unpackedPaths.has(file)
)
if (extraHashes.length > 0) {
  throw new Error(
    `SOURCE_MANIFEST.json contains files absent from the package:\n${extraHashes.join('\n')}`
  )
}

const mismatchedHashes = Object.entries(manifest.files ?? {})
  .filter(([file, expected]) => {
    const actual = createHash('sha256').update(readFileSync(file)).digest('hex')
    return actual !== expected
  })
  .map(([file]) => file)
if (mismatchedHashes.length > 0) {
  throw new Error(
    `SOURCE_MANIFEST.json contains stale hashes:\n${mismatchedHashes.join('\n')}`
  )
}

const maximumUnpackedBytes = 50 * 1024 * 1024
if (report.unpackedSize > maximumUnpackedBytes) {
  throw new Error(
    `package is too large: ${report.unpackedSize} bytes exceeds ${maximumUnpackedBytes}`
  )
}

console.log(
  `Package check passed: ${report.entryCount} files, ${report.unpackedSize} unpacked bytes.`
)
