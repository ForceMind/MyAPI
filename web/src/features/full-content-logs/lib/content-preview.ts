/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
export const CONTENT_PREVIEW_LIMIT = 1024 * 1024
export const CONTENT_EXPANDED_LIMIT = 16 * CONTENT_PREVIEW_LIMIT

/**
 * Read a bounded UTF-8 prefix without encoding the entire input (one MiB by default).
 * Valid surrogate pairs stay together. Lone surrogates cost three bytes,
 * matching the UTF-8 replacement used by TextEncoder.
 */
export function previewContent(body: string, limitBytes = CONTENT_PREVIEW_LIMIT): { preview: string; limited: boolean } {
  const limit = Number.isSafeInteger(limitBytes) && limitBytes > 0 ? Math.min(limitBytes, CONTENT_EXPANDED_LIMIT) : CONTENT_PREVIEW_LIMIT
  let index = 0
  let bytes = 0
  while (index < body.length && bytes < limit) {
    const code = body.charCodeAt(index)
    let width = 1
    let byteCount = 1
    if (code > 0x7f && code <= 0x7ff) byteCount = 2
    else if (code > 0x7ff) byteCount = 3
    if (code >= 0xd800 && code <= 0xdbff && index + 1 < body.length) {
      const next = body.charCodeAt(index + 1)
      if (next >= 0xdc00 && next <= 0xdfff) {
        width = 2
        byteCount = 4
      }
    }
    if (bytes + byteCount > limit) break
    bytes += byteCount
    index += width
  }
  return { preview: body.slice(0, index), limited: index < body.length }
}
