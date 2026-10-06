/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import axios from 'axios'
import type { ReactNode } from 'react'
import { toast } from 'sonner'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { getOptionsTypedBulkRevision, updateOptionsTypedBulk } from '../../api'
import {
  TYPED_BULK_REVISION_QUERY_KEY,
  toTypedBulkItem,
  useUpdateTypedBulkOptions,
} from '../use-typed-bulk-options'

vi.mock('../../api', () => ({
  getOptionsTypedBulkRevision: vi.fn(),
  updateOptionsTypedBulk: vi.fn(),
}))

vi.mock('sonner', () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
    info: vi.fn(),
  },
}))

const mockedGetRevision = vi.mocked(getOptionsTypedBulkRevision)
const mockedUpdateTypedBulk = vi.mocked(updateOptionsTypedBulk)

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

function createQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
}

function seedRevision(queryClient: QueryClient, revision: number) {
  queryClient.setQueryData(TYPED_BULK_REVISION_QUERY_KEY, {
    success: true,
    message: '',
    data: { revision },
  })
}

function httpError(status: number, message: string) {
  return new axios.AxiosError(
    `Request failed with status code ${status}`,
    'ERR_BAD_REQUEST',
    undefined,
    undefined,
    {
      status,
      statusText: '',
      headers: {},
      config: {} as never,
      data: { success: false, message },
    }
  )
}

describe('toTypedBulkItem', () => {
  test('maps a boolean change to the boolean type', () => {
    expect(toTypedBulkItem('checkin_setting.enabled', true)).toEqual({
      key: 'checkin_setting.enabled',
      type: 'boolean',
      value: true,
    })
  })

  test('maps a numeric change to the number type without stringifying', () => {
    expect(toTypedBulkItem('checkin_setting.min_quota', 0)).toEqual({
      key: 'checkin_setting.min_quota',
      type: 'number',
      value: 0,
    })
  })

  test('maps JSON text for map/object fields to the string type as-is', () => {
    expect(toTypedBulkItem('claude.model_headers_settings', '{"a":1}')).toEqual(
      {
        key: 'claude.model_headers_settings',
        type: 'string',
        value: '{"a":1}',
      }
    )
  })

  test('maps a string array to the string_list type', () => {
    expect(
      toTypedBulkItem('fetch_setting.domain_list', ['example.com', '*.a.b'])
    ).toEqual({
      key: 'fetch_setting.domain_list',
      type: 'string_list',
      value: ['example.com', '*.a.b'],
    })
  })

  test('maps an empty array to an empty string_list', () => {
    expect(toTypedBulkItem('fetch_setting.ip_list', [])).toEqual({
      key: 'fetch_setting.ip_list',
      type: 'string_list',
      value: [],
    })
  })

  test('encodes non-string arrays and objects as string JSON text', () => {
    expect(toTypedBulkItem('some.object_field', { a: 1 })).toEqual({
      key: 'some.object_field',
      type: 'string',
      value: '{"a":1}',
    })
  })
})

describe('useUpdateTypedBulkOptions', () => {
  beforeEach(() => {
    mockedGetRevision.mockResolvedValue({
      success: true,
      message: '',
      data: { revision: 3 },
    })
  })

  test('sends one typed bulk request with the loaded revision as expected_revision', async () => {
    const queryClient = createQueryClient()
    seedRevision(queryClient, 7)
    mockedUpdateTypedBulk.mockResolvedValue({
      success: true,
      message: '',
      data: { revision: 8, applied: ['grok.violation_deduction_enabled'] },
    })

    const { result } = renderHook(() => useUpdateTypedBulkOptions(), {
      wrapper: createWrapper(queryClient),
    })

    const items = [
      toTypedBulkItem('grok.violation_deduction_enabled', false),
      toTypedBulkItem('grok.violation_deduction_amount', 1.5),
    ]
    await result.current.mutateAsync(items)

    expect(mockedUpdateTypedBulk).toHaveBeenCalledTimes(1)
    expect(mockedUpdateTypedBulk).toHaveBeenCalledWith({
      expected_revision: 7,
      items,
    })
  })

  test('fetches the revision when the cache is empty before sending', async () => {
    const queryClient = createQueryClient()
    mockedUpdateTypedBulk.mockResolvedValue({
      success: true,
      message: '',
      data: { revision: 4, applied: [] },
    })

    const { result } = renderHook(() => useUpdateTypedBulkOptions(), {
      wrapper: createWrapper(queryClient),
    })

    await result.current.mutateAsync([toTypedBulkItem('passkey.enabled', true)])

    expect(mockedGetRevision).toHaveBeenCalledTimes(1)
    expect(mockedUpdateTypedBulk).toHaveBeenCalledWith({
      expected_revision: 3,
      items: [{ key: 'passkey.enabled', type: 'boolean', value: true }],
    })
  })

  test('stores the new revision and invalidates system options on success', async () => {
    const queryClient = createQueryClient()
    seedRevision(queryClient, 7)
    queryClient.setQueryData(['system-options'], { stale: true })
    mockedUpdateTypedBulk.mockResolvedValue({
      success: true,
      message: '',
      data: { revision: 8, applied: ['grok.violation_deduction_amount'] },
    })

    const { result } = renderHook(() => useUpdateTypedBulkOptions(), {
      wrapper: createWrapper(queryClient),
    })

    await result.current.mutateAsync([
      toTypedBulkItem('grok.violation_deduction_amount', 2),
    ])

    await waitFor(() => {
      expect(
        queryClient.getQueryData(TYPED_BULK_REVISION_QUERY_KEY)
      ).toMatchObject({ data: { revision: 8 } })
    })
    expect(queryClient.getQueryState(['system-options'])?.isInvalidated).toBe(
      true
    )
    expect(toast.success).toHaveBeenCalledWith('Setting updated successfully')
  })

  test('shows a conflict toast with a reload action on 409 and reload refreshes options and revision', async () => {
    const queryClient = createQueryClient()
    seedRevision(queryClient, 7)
    queryClient.setQueryData(['system-options'], { stale: true })
    mockedUpdateTypedBulk.mockRejectedValue(httpError(409, 'revision conflict'))

    const { result } = renderHook(() => useUpdateTypedBulkOptions(), {
      wrapper: createWrapper(queryClient),
    })

    await expect(
      result.current.mutateAsync([toTypedBulkItem('passkey.enabled', true)])
    ).rejects.toThrow()

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        'revision conflict',
        expect.objectContaining({
          action: expect.objectContaining({ label: 'Reload' }),
        })
      )
    })

    const options = vi.mocked(toast.error).mock.calls[0][1] as unknown as {
      action: { onClick: () => void }
    }
    options.action.onClick()

    expect(queryClient.getQueryState(['system-options'])?.isInvalidated).toBe(
      true
    )
    expect(
      queryClient.getQueryState(TYPED_BULK_REVISION_QUERY_KEY)?.isInvalidated
    ).toBe(true)
  })

  test('shows the server validation message on 400', async () => {
    const queryClient = createQueryClient()
    seedRevision(queryClient, 7)
    mockedUpdateTypedBulk.mockRejectedValue(
      httpError(400, 'invalid value for fetch_setting.allowed_ports')
    )

    const { result } = renderHook(() => useUpdateTypedBulkOptions(), {
      wrapper: createWrapper(queryClient),
    })

    await expect(
      result.current.mutateAsync([
        toTypedBulkItem('fetch_setting.allowed_ports', ['abc']),
      ])
    ).rejects.toThrow()

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        'invalid value for fetch_setting.allowed_ports'
      )
    })
    expect(toast.success).not.toHaveBeenCalled()
  })
})
