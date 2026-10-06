import assert from 'node:assert/strict'
import test from 'node:test'

import { validatePromotionSources } from './oci-manifest.mjs'

const digest = (char) => `sha256:${char.repeat(64)}`
const source = (architecture, char) => ({
  manifests: [
    { digest: digest(char), platform: { os: 'linux', architecture } },
    {
      digest: digest('f'),
      platform: { os: 'unknown', architecture: 'unknown' },
      annotations: {
        'vnd.docker.reference.type': 'attestation-manifest',
        'vnd.docker.reference.digest': digest(char),
      },
    },
  ],
})

test('accepts OCI provenance and SBOM attestations while mapping real platform children', () => {
  const amd64 = source('amd64', 'a')
  const arm64 = source('arm64', 'b')
  const version = {
    manifests: [
      amd64.manifests[0],
      arm64.manifests[0],
      amd64.manifests[1],
      {
        digest: digest('e'),
        platform: { os: 'unknown', architecture: 'unknown' },
        annotations: {
          'vnd.docker.reference.type': 'attestation-manifest',
          'vnd.docker.reference.digest': digest('a'),
        },
      },
    ],
  }
  assert.deepEqual(
    validatePromotionSources({
      versionRaw: version,
      sources: {
        amd64: { artifactDigest: digest('c'), rootDigest: digest('c'), raw: amd64 },
        arm64: { artifactDigest: digest('d'), rootDigest: digest('d'), raw: arm64 },
      },
    }),
    { amd64: digest('a'), arm64: digest('b') }
  )
})

test('rejects an attestation that references an unrelated platform digest', () => {
  const amd64 = source('amd64', 'a')
  const arm64 = source('arm64', 'b')
  amd64.manifests[1].annotations['vnd.docker.reference.digest'] = digest('0')
  assert.throws(() =>
    validatePromotionSources({
      versionRaw: { manifests: [amd64.manifests[0], arm64.manifests[0]] },
      sources: {
        amd64: { artifactDigest: digest('c'), rootDigest: digest('c'), raw: amd64 },
        arm64: { artifactDigest: digest('d'), rootDigest: digest('d'), raw: arm64 },
      },
    })
  )
})

test('rejects a version platform child that differs from its immutable source', () => {
  assert.throws(() =>
    validatePromotionSources({
      versionRaw: { manifests: [source('amd64', '0').manifests[0], source('arm64', 'b').manifests[0]] },
      sources: {
        amd64: { artifactDigest: digest('c'), rootDigest: digest('c'), raw: source('amd64', 'a') },
        arm64: { artifactDigest: digest('d'), rootDigest: digest('d'), raw: source('arm64', 'b') },
      },
    })
  )
})

test('rejects undeclared platform children in the version index and source index', () => {
  const amd64 = source('amd64', 'a')
  const arm64 = source('arm64', 'b')
  const sources = {
    amd64: { artifactDigest: digest('c'), rootDigest: digest('c'), raw: amd64 },
    arm64: { artifactDigest: digest('d'), rootDigest: digest('d'), raw: arm64 },
  }
  assert.throws(() =>
    validatePromotionSources({
      versionRaw: {
        manifests: [
          amd64.manifests[0],
          arm64.manifests[0],
          { digest: digest('e'), platform: { os: 'linux', architecture: 'ppc64le' } },
        ],
      },
      sources,
    })
  )
  assert.throws(() =>
    validatePromotionSources({
      versionRaw: { manifests: [amd64.manifests[0], arm64.manifests[0]] },
      sources: {
        ...sources,
        amd64: {
          ...sources.amd64,
          raw: { manifests: [amd64.manifests[0], arm64.manifests[0]] },
        },
      },
    })
  )
})
