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
import { useQuery } from '@tanstack/react-query'
import type { Cell, Table } from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { useDataTable } from '@/components/data-table'
import { useMediaQuery } from '@/hooks'

import { LOG_TYPE_ENUM } from '../../constants'
import type { UsageLog } from '../../data/schema'
import { UsageLogsTable } from '../usage-logs-table'

/** Keep the page shell real so this test exercises DataTablePage's mobile slot. */
vi.mock('@/components/data-table', async () => {
  const actual = await vi.importActual<
    typeof import('@/components/data-table')
  >('@/components/data-table')
  return {
    ...actual,
    DataTableRow: () => null,
    useDataTable: vi.fn(),
  }
})

vi.mock('@/components/data-table/core/pagination', () => ({
  DataTablePagination: () => null,
}))

vi.mock('@tanstack/react-query', () => ({
  useQuery: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  getRouteApi: () => ({
    useSearch: () => ({}),
    useNavigate: () => vi.fn(),
  }),
}))

vi.mock('@/hooks', async () => {
  const actual = await vi.importActual<typeof import('@/hooks')>('@/hooks')
  return { ...actual, useMediaQuery: vi.fn(() => true) }
})

vi.mock('@/hooks/use-table-url-state', () => ({
  useTableUrlState: () => ({
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    ensurePageInRange: vi.fn(),
  }),
}))

vi.mock('../usage-logs-provider', () => ({
  useLogsViewScope: () => ({ isAdminView: false }),
  useUsageLogsContext: () => ({
    sensitiveVisible: true,
    setSelectedUserId: vi.fn(),
    setUserInfoDialogOpen: vi.fn(),
  }),
}))

vi.mock('../lib/columns', () => ({
  useColumnsByCategory: () => [],
}))

vi.mock('../lib/format', () => ({
  parseLogOther: () => null,
}))

vi.mock('../lib/utils', () => ({
  fetchLogsByCategory: vi.fn(),
  getLogTypeConfig: () => ({ label: 'Success', color: 'success' }),
  isDisplayableLogType: () => true,
  isTimingLogType: () => false,
}))

vi.mock('../common-logs-filter-bar', () => ({
  CommonLogsFilterBar: () => null,
}))

vi.mock('../task-logs-filter-bar', () => ({
  TaskLogsFilterBar: () => null,
}))

function makeCell(
  id: string,
  value: ReactNode,
  row: { original: UsageLog }
): Cell<UsageLog, unknown> {
  return {
    column: {
      id,
      columnDef: { cell: () => value },
    },
    getContext: () => ({}),
    row,
  } as unknown as Cell<UsageLog, unknown>
}

function makeTable(): Table<Record<string, unknown>> {
  const original: UsageLog = {
    id: 1,
    user_id: 2,
    created_at: 1_756_560_000,
    type: LOG_TYPE_ENUM.CONSUME,
    content: 'request payload',
    username: 'teammate',
    token_name: 'research-key',
    model_name: 'gpt-5',
    quota: 12,
    prompt_tokens: 10,
    completion_tokens: 8,
    use_time: 1,
    is_stream: false,
    channel: 3,
    channel_name: 'Codex',
    token_id: 4,
    group: 'default',
    ip: '',
    other: '',
    request_id: 'req-mobile-integration',
    upstream_request_id: '',
  }
  const row = { original } as { original: UsageLog }
  const cells = [
    makeCell('created_at', 'time', row),
    makeCell('model_name', 'gpt-5', row),
    makeCell('quota', '12', row),
    makeCell('channel', 'Codex', row),
    makeCell('user', 'teammate', row),
    makeCell('token_name', 'research-key', row),
    makeCell('use_time', '1s', row),
    makeCell('prompt_tokens', '10', row),
    makeCell('content', <button type='button'>View content</button>, row),
  ]

  return {
    getRowModel: () => ({
      rows: [
        {
          id: 'row-1',
          original,
          getVisibleCells: () => cells,
          getAllCells: () => cells,
        },
      ],
    }),
  } as unknown as Table<Record<string, unknown>>
}

describe('usage logs mobile integration', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useMediaQuery).mockReturnValue(true)
    vi.mocked(useQuery).mockReturnValue({
      data: { items: [{ id: 1 }], total: 1 },
      error: null,
      isError: false,
      isLoading: false,
      isFetching: false,
      refetch: vi.fn(),
    } as never)
    vi.mocked(useDataTable).mockReturnValue({ table: makeTable() } as never)
  })

  test('renders API log card and details action through the real page shell', () => {
    render(<UsageLogsTable logCategory='common' />)

    expect(screen.getByText('gpt-5')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'View content' })
    ).toBeInTheDocument()
  })
})
