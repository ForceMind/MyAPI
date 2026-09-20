/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, test } from 'vitest'

import {
  createChannelRoutingPercentFormatter,
  getChannelSortParams,
  isChannelSortDisabledInTagMode,
  resolveChannelRoutingPreviewErrorCode,
} from '../channel-routing'

describe('channel display sorting parameters', () => {
  test('forwards weight sorting to the server', () => {
    expect(getChannelSortParams([{ id: 'weight', desc: true }])).toEqual({
      sort_by: 'weight',
      sort_order: 'desc',
    })
  })

  test('disables priority and weight sorting in tag aggregation mode', () => {
    expect(isChannelSortDisabledInTagMode('priority', true)).toBe(true)
    expect(isChannelSortDisabledInTagMode('weight', true)).toBe(true)
    expect(isChannelSortDisabledInTagMode('name', true)).toBe(false)
    expect(isChannelSortDisabledInTagMode('priority', false)).toBe(false)
    expect(
      getChannelSortParams([{ id: 'priority', desc: true }], true)
    ).toEqual({})
    expect(getChannelSortParams([{ id: 'weight', desc: false }], true)).toEqual(
      {}
    )
  })

  test('keeps unrelated sorting in tag aggregation mode', () => {
    expect(getChannelSortParams([{ id: 'name', desc: false }], true)).toEqual({
      sort_by: 'name',
      sort_order: 'asc',
    })
  })

  test('ignores unsupported display columns', () => {
    expect(getChannelSortParams([{ id: 'status', desc: false }])).toEqual({})
  })
})

describe('channel routing percentage locale', () => {
  test('maps zhCN to a valid Intl locale', () => {
    expect(
      createChannelRoutingPercentFormatter('zhCN').resolvedOptions().locale
    ).toBe('zh-CN')
  })

  test('maps zhTW to a valid Intl locale', () => {
    expect(
      createChannelRoutingPercentFormatter('zhTW').resolvedOptions().locale
    ).toBe('zh-TW')
  })
})

describe('channel routing preview errors', () => {
  test('maps authentication and authorization failures to the permission state', () => {
    expect(
      resolveChannelRoutingPreviewErrorCode({
        isAxiosError: true,
        response: { status: 401 },
      })
    ).toBe('routing_preview_permission_denied')
    expect(
      resolveChannelRoutingPreviewErrorCode({
        isAxiosError: true,
        response: { status: 403 },
      })
    ).toBe('routing_preview_permission_denied')
  })
})
