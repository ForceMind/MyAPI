import { describe, expect, it } from 'vitest'

import { pricingSearchSchema } from '../catalog-state'

describe('model catalog return state', () => {
  it('retains all existing filters and display modes with bounded pagination', () => {
    const state = { search: 'model', sort: 'price-low', vendor: 'provider', group: 'default', quotaType: 'token', endpointType: 'openai', tag: 'vision', tokenUnit: 'K', view: 'table', rechargePrice: false, page: 3, pageSize: 50, focusId: 9 }
    expect(pricingSearchSchema.parse(state)).toEqual(state)
  })
  it('discards invalid new page and focus values without weakening existing mode validation', () => {
    expect(pricingSearchSchema.parse({ page: -1, pageSize: 101, focusId: 0, view: 'unknown', credential: 'must-not-persist', redirect: 'https://invalid.example' })).toEqual({ page: undefined, pageSize: undefined, focusId: undefined, view: undefined })
    expect(pricingSearchSchema.safeParse({ tokenUnit: 'unknown' }).success).toBe(false)
  })
})
