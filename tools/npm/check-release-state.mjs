/*
MyAPI distribution release-state validation tooling.
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
const moduleDeclaration = readFileSync('go.mod', 'utf8').match(/^module\s+([^\s]+)/m)?.[1]
const expectedModule = 'github.com/ForceMind/MyAPI'
if (moduleDeclaration !== expectedModule) {
  throw new Error(`go.mod must declare the MyAPI module (${expectedModule})`)
}
if (
  metadata.name !== '@forcemind/myapi' ||
  metadata.repository?.url !== 'git+https://github.com/ForceMind/MyAPI.git'
) {
  throw new Error('package metadata does not identify the MyAPI distribution')
}
if (version !== metadata.version) {
  throw new Error(
    `VERSION (${version || 'empty'}) does not match package.json (${metadata.version})`
  )
}

const expectedImageVersion = `v${metadata.version}`
const imageDefaultFiles = [
  'docker-compose.yml',
  'deploy/docker-compose.yml',
]
for (const file of imageDefaultFiles) {
  const contents = readFileSync(file, 'utf8')
  for (const match of contents.matchAll(/ghcr\.io\/forcemind\/myapi:v([0-9]+\.[0-9]+\.[0-9]+(?:[.-][0-9A-Za-z.-]+)?)/g)) {
    if (`v${match[1]}` !== expectedImageVersion) {
      throw new Error(`${file} contains a stale default image version (${match[1]}); expected ${metadata.version}`)
    }
  }
}

const dirty = git(['status', '--porcelain=v1', '--untracked-files=all'])
if (dirty) {
  throw new Error(`refusing to publish a dirty source tree:\n${dirty}`)
}

const expectedTag = `v${metadata.version}`
// v0.1.0 was an earlier distribution tag. It is intentionally immutable and
// must never be republished from a later tree, even if a caller checks out the
// old tag before running this script.
const protectedLegacyTags = new Set(['v0.1.0'])
if (protectedLegacyTags.has(expectedTag)) {
  throw new Error(`${expectedTag} is a protected legacy tag; release a new version instead`)
}

const requestedTag = process.env.MYAPI_RELEASE_TAG?.trim() || expectedTag
if (requestedTag !== expectedTag) {
  throw new Error(`requested release tag ${requestedTag} does not match ${expectedTag}`)
}
const head = git(['rev-parse', 'HEAD'])
const releaseTagCommit = git(['rev-parse', `refs/tags/${requestedTag}^{commit}`])
if (releaseTagCommit !== head) {
  throw new Error(`HEAD must equal the commit tagged ${requestedTag}`)
}
if (
  !process.env.MYAPI_RELEASE_TAG &&
  ((process.env.GITHUB_REF_TYPE && process.env.GITHUB_REF_TYPE !== 'tag') ||
    (process.env.GITHUB_REF_NAME && process.env.GITHUB_REF_NAME !== expectedTag))
) {
  throw new Error(
    `release ref ${process.env.GITHUB_REF_NAME || 'unknown'} is not ${expectedTag}`
  )
}

console.log(`Release state is clean for ${metadata.name} ${metadata.version} (${requestedTag}).`)
