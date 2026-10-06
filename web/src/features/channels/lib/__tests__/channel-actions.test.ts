/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { QueryClient } from '@tanstack/react-query'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { updateChannelStatus, batchUpdateChannelStatus } from '../../api'
import {
  handleBatchEnable,
  handleEnableChannel,
  resolveChannelOperationAttempt,
} from '../channel-actions'

const toastMocks = vi.hoisted(() => ({
  success: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
}))

vi.mock('i18next', () => ({
  default: {
    t: (key: string) => key,
  },
}))

vi.mock('sonner', () => ({ toast: toastMocks }))

vi.mock('../../api', () => ({
  copyChannel: vi.fn(),
  deleteChannel: vi.fn(),
  testChannel: vi.fn(),
  updateChannel: vi.fn(),
  updateChannelStatus: vi.fn(),
  batchUpdateChannelStatus: vi.fn(),
  batchDeleteChannels: vi.fn(),
  batchSetChannelTag: vi.fn(),
  enableTagChannels: vi.fn(),
  disableTagChannels: vi.fn(),
  deleteDisabledChannels: vi.fn(),
  fixChannelAbilities: vi.fn(),
  editTagChannels: vi.fn(),
  testAllChannels: vi.fn(),
  updateAllChannelsBalance: vi.fn(),
}))

function queryClientStub() {
  return {
    invalidateQueries: vi.fn().mockResolvedValue(undefined),
  } as unknown as QueryClient
}

describe('channel operation key reuse', () => {
  test('reuses the key for the same payload and rotates it when input changes', () => {
    const first = resolveChannelOperationAttempt(null, 'payload-a')
    const retry = resolveChannelOperationAttempt(first, 'payload-a')
    const changed = resolveChannelOperationAttempt(retry, 'payload-b')

    expect(retry).toBe(first)
    expect(changed.operationKey).not.toBe(first.operationKey)
  })
})

describe('channel mutation cache publication feedback', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  test('warns but still refreshes the list after a committed cache-pending update', async () => {
    vi.mocked(updateChannelStatus).mockResolvedValue({
      success: true,
      committed: true,
      cache_pending: true,
      data_generation: 8,
      published_generation: 7,
      data: true,
    })
    const queryClient = queryClientStub()
    const onSuccess = vi.fn()

    await handleEnableChannel(42, queryClient, onSuccess)

    expect(toastMocks.warning).toHaveBeenCalledTimes(1)
    expect(toastMocks.error).not.toHaveBeenCalled()
    expect(queryClient.invalidateQueries).toHaveBeenCalledTimes(1)
    expect(onSuccess).toHaveBeenCalledTimes(1)
  })

  test('refreshes the list when a repeated batch operation changes zero rows', async () => {
    vi.mocked(batchUpdateChannelStatus).mockResolvedValue({
      success: true,
      committed: true,
      cache_pending: false,
      data_generation: 9,
      published_generation: 9,
      data: 0,
    })
    const queryClient = queryClientStub()
    const onSuccess = vi.fn()

    await handleBatchEnable([1, 2], queryClient, onSuccess)

    expect(toastMocks.success).toHaveBeenCalledTimes(1)
    expect(toastMocks.error).not.toHaveBeenCalled()
    expect(queryClient.invalidateQueries).toHaveBeenCalledTimes(1)
    expect(onSuccess).toHaveBeenCalledTimes(1)
  })
})
