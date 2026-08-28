/*
MyAPI distribution tooling for the New API based custom source release.
Copyright (C) 2026 ForceMind

Licensed under the GNU Affero General Public License version 3 or later.
*/
import { readFileSync } from 'node:fs'
import { spawnSync } from 'node:child_process'

function git(args) {
  const result = spawnSync('git', args, { encoding: 'utf8', shell: false })
  if (result.status !== 0) throw new Error(result.stderr || 'git command failed')
  return result.stdout.trim()
}

const metadata = JSON.parse(readFileSync('package.json', 'utf8'))
const version = readFileSync('VERSION', 'utf8').trim()
if (version !== metadata.version) {
  throw new Error(
    `VERSION (${version || 'empty'}) does not match package.json (${metadata.version})`
  )
}

const dirty = git(['status', '--porcelain=v1', '--untracked-files=all'])
if (dirty) {
  throw new Error(`refusing to publish a dirty source tree:\n${dirty}`)
}

const expectedTag = `v${metadata.version}`
const releaseTag = git(['tag', '--points-at', 'HEAD', '--list', expectedTag])
if (releaseTag !== expectedTag) {
  throw new Error(`HEAD must be tagged exactly with ${expectedTag}`)
}
if (
  (process.env.GITHUB_REF_TYPE && process.env.GITHUB_REF_TYPE !== 'tag') ||
  (process.env.GITHUB_REF_NAME && process.env.GITHUB_REF_NAME !== expectedTag)
) {
  throw new Error(
    `release ref ${process.env.GITHUB_REF_NAME || 'unknown'} is not ${expectedTag}`
  )
}

console.log(`Release state is clean for ${metadata.name} ${metadata.version}.`)
