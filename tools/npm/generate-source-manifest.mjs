/*
MyAPI distribution tooling for the New API based custom source release.
Copyright (C) 2026 ForceMind

Licensed under the GNU Affero General Public License version 3 or later.
*/
import { createHash } from 'node:crypto'
import { readFileSync, writeFileSync } from 'node:fs'
import { spawnSync } from 'node:child_process'
import path from 'node:path'

const root = process.cwd()
const metadata = JSON.parse(readFileSync(path.join(root, 'package.json'), 'utf8'))
const packResult = spawnSync(
  'npm',
  ['pack', '--dry-run', '--json', '--ignore-scripts'],
  { cwd: root, encoding: 'utf8', shell: false }
)
if (packResult.status !== 0) {
  throw new Error(packResult.stderr || 'npm pack --dry-run failed')
}
const packReports = JSON.parse(packResult.stdout)
if (!Array.isArray(packReports) || packReports.length !== 1) {
  throw new Error('npm pack returned an unexpected report')
}
const packedPaths = packReports[0].files.map((file) => file.path).sort()

const files = {}
for (const relative of packedPaths) {
  if (relative === 'SOURCE_MANIFEST.json') continue
  const bytes = readFileSync(path.join(root, relative))
  files[relative] = createHash('sha256').update(bytes).digest('hex')
}

const head = spawnSync('git', ['rev-parse', 'HEAD'], {
  cwd: root,
  encoding: 'utf8',
  shell: false,
})
const commitTime = spawnSync('git', ['show', '-s', '--format=%cI', 'HEAD'], {
  cwd: root,
  encoding: 'utf8',
  shell: false,
})
const treeStatus = spawnSync(
  'git',
  ['status', '--porcelain=v1', '--untracked-files=all'],
  { cwd: root, encoding: 'utf8', shell: false }
)
const manifest = {
  schemaVersion: 1,
  distribution: metadata.name,
  version: metadata.version,
  basedOn: {
    project: 'New API',
    repository: 'https://github.com/QuantumNous/new-api',
    release: 'v1.0.0-rc.25',
    commit: 'f116414284162ad15d8925f7bca494c109b83e93',
  },
  sourceCommit: head.status === 0 ? head.stdout.trim() : null,
  sourceTreeDirty: treeStatus.status !== 0 || treeStatus.stdout.trim() !== '',
  generatedAt:
    commitTime.status === 0 && commitTime.stdout.trim()
      ? commitTime.stdout.trim()
      : new Date(0).toISOString(),
  files,
}

writeFileSync(
  path.join(root, 'SOURCE_MANIFEST.json'),
  `${JSON.stringify(manifest, null, 2)}\n`
)
console.log(`Wrote SOURCE_MANIFEST.json with ${Object.keys(files).length} files.`)
