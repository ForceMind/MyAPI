/*
 * OCI promotion validation that tolerates attestations alongside platform images.
 * Copyright (C) 2026 ForceMind
 */
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

function parseJson(value, label) {
  try {
    return JSON.parse(value)
  } catch {
    throw new Error(`${label} is not valid JSON`)
  }
}

function platformChildren(manifest, expectedArchitectures, label) {
  const children = (manifest.manifests ?? []).filter(
    (child) => child.platform && !isAttestation(child)
  )
  if (children.length !== expectedArchitectures.length) {
    throw new Error(`${label} has an unexpected number of platform image children`)
  }
  const result = {}
  for (const child of children) {
    const architecture = child.platform?.architecture
    if (
      child.platform?.os !== 'linux' ||
      !expectedArchitectures.includes(architecture) ||
      result[architecture] ||
      !/^sha256:[0-9a-f]{64}$/.test(child.digest)
    ) {
      throw new Error(`${label} contains an unexpected platform image child`)
    }
    result[architecture] = child.digest
  }
  return result
}

function isAttestation(child) {
  return child.annotations?.['vnd.docker.reference.type'] === 'attestation-manifest'
}

function validateAttestations(manifest, label, platformDigests) {
  for (const child of manifest.manifests ?? []) {
    if (isAttestation(child)) {
      const reference = child.annotations?.['vnd.docker.reference.digest']
      if (
        !/^sha256:[0-9a-f]{64}$/.test(child.digest) ||
        !platformDigests.has(reference)
      ) {
        throw new Error(`${label} contains an invalid attestation manifest`)
      }
      continue
    }
    if (child.platform) continue
    if (!/^sha256:[0-9a-f]{64}$/.test(child.digest)) {
      throw new Error(`${label} contains an unrecognized non-platform manifest`)
    }
    throw new Error(`${label} contains an unrecognized non-platform manifest`)
  }
}

export function validatePromotionSources({ versionRaw, sources }) {
  const version = typeof versionRaw === 'string' ? parseJson(versionRaw, 'version index') : versionRaw
  const versionChildren = platformChildren(version, ['amd64', 'arm64'], 'version index')
  validateAttestations(version, 'version index', new Set(Object.values(versionChildren)))
  const result = {}
  for (const architecture of ['amd64', 'arm64']) {
    const source = sources[architecture]
    if (!source || !/^sha256:[0-9a-f]{64}$/.test(source.artifactDigest)) {
      throw new Error(`missing artifact digest for ${architecture}`)
    }
    if (source.rootDigest !== source.artifactDigest) {
      throw new Error(`source root digest for ${architecture} does not match its artifact`)
    }
    const sourceManifest = typeof source.raw === 'string' ? parseJson(source.raw, `${architecture} source`) : source.raw
    const sourceChild = platformChildren(sourceManifest, [architecture], `${architecture} source`)[architecture]
    validateAttestations(sourceManifest, `${architecture} source`, new Set([sourceChild]))
    const versionChild = versionChildren[architecture]
    if (versionChild !== sourceChild) {
      throw new Error(`version index linux/${architecture} child does not match its source`)
    }
    result[architecture] = sourceChild
  }
  return result
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const input = parseJson(process.argv[2] ?? readFileSync(0, 'utf8'), 'promotion input')
  console.log(JSON.stringify(validatePromotionSources(input)))
}
