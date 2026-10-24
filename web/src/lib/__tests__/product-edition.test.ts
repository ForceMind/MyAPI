import { describe, expect, test } from 'vitest'

import { PRODUCT_CAPABILITIES, resolveProductEdition } from '../product-edition'

const allFalse = Object.fromEntries(
  PRODUCT_CAPABILITIES.map((capability) => [capability, false])
)

describe('product edition resolution', () => {
  test.each<[string, Record<string, unknown> | null]>([
    ['no status', null],
    ['an older backend without edition fields', { version: 'v1' }],
    ['an unknown edition value', { product_edition: 'enterprise' }],
  ])('%s resolves to Full with every capability', (_name, status) => {
    const profile = resolveProductEdition(status, false)
    expect(profile.edition).toBe('full')
    for (const capability of PRODUCT_CAPABILITIES) {
      expect(profile.capabilities[capability]).toBe(true)
    }
  })

  test('a Lite backend hides exactly the capabilities it reports as false', () => {
    const profile = resolveProductEdition(
      {
        product_edition: 'lite',
        capabilities: { ...allFalse, rankings: true },
      },
      false
    )
    expect(profile.edition).toBe('lite')
    expect(profile.capabilities.payments).toBe(false)
    expect(profile.capabilities.midjourney).toBe(false)
    expect(profile.capabilities.rankings).toBe(true)
  })

  test('a capability missing from the response keeps Full behaviour for that entry', () => {
    const profile = resolveProductEdition(
      { product_edition: 'lite', capabilities: { payments: false } },
      false
    )
    expect(profile.capabilities.payments).toBe(false)
    expect(profile.capabilities.redemption_codes).toBe(true)
  })

  test('the envelope returned by /api/status is unwrapped', () => {
    const profile = resolveProductEdition(
      { data: { product_edition: 'lite', capabilities: allFalse } },
      false
    )
    expect(profile.edition).toBe('lite')
    expect(profile.capabilities.model_deployments).toBe(false)
  })

  test('the self-use build flag keeps its Lite surface even against a Full backend', () => {
    const profile = resolveProductEdition({ product_edition: 'full' }, true)
    expect(profile.edition).toBe('lite')
    for (const capability of PRODUCT_CAPABILITIES) {
      expect(profile.capabilities[capability]).toBe(false)
    }
  })
})
