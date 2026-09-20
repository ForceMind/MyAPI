import { describe, expect, test } from 'vitest'

import { arrayBufferToBase64Url, base64UrlToArrayBuffer } from './passkey'

describe('passkey base64url helpers', () => {
  test('round trips URL-safe bytes without padding', () => {
    const bytes = new Uint8Array([251, 255, 0, 1])
    const encoded = arrayBufferToBase64Url(bytes.buffer)

    expect(encoded).toBe('-_8AAQ')
    expect([...new Uint8Array(base64UrlToArrayBuffer(encoded))]).toEqual([
      251, 255, 0, 1,
    ])
  })
})
