/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
export const CONTENT_PREVIEW_LIMIT = 1024 * 1024

/**
 * Read at most a one-MiB UTF-8 prefix without encoding the entire input.
 * Valid surrogate pairs stay together. Lone surrogates cost three bytes,
 * matching the UTF-8 replacement used by TextEncoder.
 */
export function previewContent(body: string): { preview: string; limited: boolean } {
  let index = 0
  let bytes = 0
  while (index < body.length && bytes < CONTENT_PREVIEW_LIMIT) {
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
    if (bytes + byteCount > CONTENT_PREVIEW_LIMIT) break
    bytes += byteCount
    index += width
  }
  return { preview: body.slice(0, index), limited: index < body.length }
}
