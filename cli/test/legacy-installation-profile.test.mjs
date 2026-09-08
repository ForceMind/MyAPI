import assert from 'node:assert/strict'
import test from 'node:test'

import {
  LegacyInstallationProfileError,
  resolveLegacyInstallationProfile,
} from '../lib/legacy-installation-profile.mjs'

function assertProfileError(code, callback) {
  assert.throws(callback, (error) =>
    error instanceof LegacyInstallationProfileError && error.code === code
  )
}

test('maps explicit Legacy LAN loopback configuration to Lite local without changing access scope', () => {
  const profile = resolveLegacyInstallationProfile({
    edition: 'lan',
    bindAddress: '127.0.0.1',
    allowLan: false,
  })

  assert.deepEqual(profile, {
    product_edition: 'lite',
    installation_shape: 'legacy-server-container',
    desired_access_mode: 'local',
    legacy_identity: 'lan',
    migration_status: 'compatible',
    warnings: [],
  })
})

test('accepts only the Legacy localhost loopback hostname', () => {
  const profile = resolveLegacyInstallationProfile({
    edition: 'lan',
    bindAddress: 'localhost',
    allowLan: false,
  })

  assert.equal(profile.desired_access_mode, 'local')
  assert.equal(profile.migration_status, 'compatible')
})

test('maps explicit private Legacy LAN configuration to Lite LAN', () => {
  const profile = resolveLegacyInstallationProfile({
    edition: 'lan',
    bindAddress: '192.168.10.8',
    allowLan: true,
  })

  assert.equal(profile.product_edition, 'lite')
  assert.equal(profile.desired_access_mode, 'lan')
  assert.equal(profile.migration_status, 'compatible')
})

test('keeps Full as Full while classifying only explicit local access evidence', () => {
  const profile = resolveLegacyInstallationProfile({
    edition: 'full',
    bindAddress: '::1',
    allowLan: false,
  })

  assert.equal(profile.product_edition, 'full')
  assert.equal(profile.legacy_identity, 'full')
  assert.equal(profile.desired_access_mode, 'local')
})

test('never infers public access from broad, public, missing, ambiguous, or reverse-proxy inputs', () => {
  const profiles = [
    { edition: 'lan', bindAddress: '0.0.0.0', allowLan: true },
    { edition: 'lan', bindAddress: '203.0.113.7', allowLan: true },
    { edition: 'lan', bindAddress: 'my-api.example.test', allowLan: true },
    { edition: 'lan', allowLan: true },
    { edition: 'full', bindAddress: '127.0.0.1', allowLan: false, reverseProxyConfigured: true },
  ].map(resolveLegacyInstallationProfile)

  for (const profile of profiles) {
    assert.equal(profile.desired_access_mode, 'needs_manual')
    assert.equal(profile.migration_status, 'needs_manual')
    assert.notEqual(profile.desired_access_mode, 'public')
  }
})

test('rejects unknown fields, prototypes, accessors, control characters, invalid values, and wrong types', () => {
  assertProfileError('invalid_fields', () =>
    resolveLegacyInstallationProfile({ edition: 'lan', bindAddress: '127.0.0.1', allowLan: false, unexpected: true })
  )
  assertProfileError('invalid_input', () =>
    resolveLegacyInstallationProfile(Object.create(null))
  )
  assertProfileError('invalid_input', () =>
    resolveLegacyInstallationProfile(new Proxy({ edition: 'lan' }, {}))
  )
  const revoked = Proxy.revocable({ edition: 'lan' }, {})
  revoked.revoke()
  for (const proxy of [
    revoked.proxy,
    new Proxy({ edition: 'lan' }, {
      ownKeys: () => { throw new Error('ownKeys trap must not run') },
    }),
    new Proxy({ edition: 'lan' }, {
      getPrototypeOf: () => { throw new Error('getPrototypeOf trap must not run') },
    }),
    new Proxy({ edition: 'lan' }, {
      getOwnPropertyDescriptor: () => { throw new Error('getOwnPropertyDescriptor trap must not run') },
    }),
  ]) {
    assertProfileError('invalid_input', () => resolveLegacyInstallationProfile(proxy))
  }
  const accessor = { edition: 'lan', bindAddress: '127.0.0.1', allowLan: false }
  Object.defineProperty(accessor, 'reverseProxyConfigured', { enumerable: true, get: () => false })
  assertProfileError('invalid_fields', () => resolveLegacyInstallationProfile(accessor))
  assertProfileError('invalid_value', () =>
    resolveLegacyInstallationProfile({ edition: 'lan\n', bindAddress: '127.0.0.1', allowLan: false })
  )
  assertProfileError('invalid_value', () =>
    resolveLegacyInstallationProfile({ edition: 'lite', bindAddress: '127.0.0.1', allowLan: false })
  )
  assertProfileError('invalid_value', () =>
    resolveLegacyInstallationProfile({ edition: 'lan', bindAddress: '127.0.0.1', allowLan: 'false' })
  )
})

test('returns a detached deeply frozen profile', () => {
  const input = { edition: 'lan', bindAddress: '10.0.0.4', allowLan: true }
  const profile = resolveLegacyInstallationProfile(input)
  input.edition = 'full'
  input.bindAddress = '127.0.0.1'

  assert.equal(profile.product_edition, 'lite')
  assert.equal(profile.desired_access_mode, 'lan')
  assert.equal(Object.isFrozen(profile), true)
  assert.equal(Object.isFrozen(profile.warnings), true)
  assert.throws(() => {
    profile.warnings.push('mutate')
  }, TypeError)
})
