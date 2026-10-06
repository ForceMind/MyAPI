import { describe, expect, test } from 'vitest'

import { parseEndpointKeys } from './prefill-group-shared'

describe('parseEndpointKeys', () => {
  test('keeps string values before object names and removes unsupported values', () => {
    expect(
      parseEndpointKeys(
        JSON.stringify(['https://api.example', { name: 'fallback' }, 1, {}])
      )
    ).toEqual(['https://api.example', 'fallback'])
  })
})
