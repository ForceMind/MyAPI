/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { describe, expect, test } from 'vitest'

import { CONTENT_PREVIEW_LIMIT, previewContent } from '../content-preview'

describe('saved content UTF-8 preview', () => {
  test('a Chinese code point crossing the byte boundary stays outside the preview', () => {
    const prefix = 'a'.repeat(CONTENT_PREVIEW_LIMIT - 2)
    const result = previewContent(prefix + '中tail')

    expect(result).toEqual({ preview: prefix, limited: true })
    expect(new TextEncoder().encode(result.preview).length).toBeLessThanOrEqual(CONTENT_PREVIEW_LIMIT)
  })

  test('an emoji crossing the byte boundary is never split into a lone surrogate', () => {
    const prefix = 'a'.repeat(CONTENT_PREVIEW_LIMIT - 3)
    expect(previewContent(prefix + '😀tail')).toEqual({ preview: prefix, limited: true })
    const exact = 'a'.repeat(CONTENT_PREVIEW_LIMIT - 4) + '😀'
    expect(previewContent(exact)).toEqual({ preview: exact, limited: false })
    expect(previewContent(exact + '尾')).toEqual({ preview: exact, limited: true })
  })
})
