/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { ReactNode } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { useQuery } from '@tanstack/react-query'
import { useMediaQuery } from '@/hooks'

import { UsageLogsTable } from '../usage-logs-table'

vi.mock('@tanstack/react-query', () => ({
  useQuery: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  getRouteApi: () => ({
    useSearch: () => ({}),
    useNavigate: () => vi.fn(),
  }),
}))

vi.mock('@/hooks', () => ({
  useMediaQuery: vi.fn(() => false),
}))

vi.mock('@/hooks/use-table-url-state', () => ({
  useTableUrlState: () => ({
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    ensurePageInRange: vi.fn(),
  }),
}))

vi.mock('@/components/data-table', () => ({
  DataTablePage: (props: {
    emptyTitle?: ReactNode
    emptyDescription?: ReactNode
    emptyAction?: ReactNode
    fixedHeight?: boolean
    mobile?: ReactNode
  }) => (
    <div data-testid='data-table-page'>
      {props.fixedHeight === false ? (
        props.mobile
      ) : (
        <>
          <div>{props.emptyTitle}</div>
          <div>{props.emptyDescription}</div>
          {props.emptyAction}
        </>
      )}
    </div>
  ),
  DataTableRow: () => null,
  useDataTable: () => ({ table: {} }),
}))

vi.mock('@/components/error-state', () => ({
  ErrorState: (props: {
    title?: string
    description?: string
    onRetry?: () => void
  }) => (
    <section>
      <h2>{props.title}</h2>
      <p>{props.description}</p>
      {props.onRetry != null && (
        <button type='button' onClick={props.onRetry}>
          Retry
        </button>
      )}
    </section>
  ),
}))

vi.mock('@/components/ui/button', () => ({
  Button: (props: {
    children?: ReactNode
    onClick?: () => void
  }) => (
    <button type='button' onClick={props.onClick}>
      {props.children}
    </button>
  ),
}))

vi.mock('../usage-logs-provider', () => ({
  useLogsViewScope: () => ({ isAdminView: false }),
}))

vi.mock('../lib/columns', () => ({
  useColumnsByCategory: () => [],
}))

vi.mock('../lib/format', () => ({
  parseLogOther: () => null,
}))

vi.mock('../lib/utils', () => ({
  fetchLogsByCategory: vi.fn(),
}))

vi.mock('../common-logs-filter-bar', () => ({
  CommonLogsFilterBar: () => null,
}))

vi.mock('../task-logs-filter-bar', () => ({
  TaskLogsFilterBar: () => null,
}))

vi.mock('../usage-logs-mobile-card', () => ({
  UsageLogsMobileList: () => null,
}))

function mockQuery(error = new Error('upstream unavailable')) {
  const refetch = vi.fn()
  vi.mocked(useQuery).mockReturnValue({
    data: undefined,
    error,
    isError: true,
    isLoading: false,
    isFetching: false,
    refetch,
  } as never)
  return refetch
}

describe('usage logs table error state', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useMediaQuery).mockReturnValue(false)
  })

  test('shows a non-401 load error and retries from the desktop empty state', () => {
    const refetch = mockQuery()

    render(<UsageLogsTable logCategory='common' />)

    expect(screen.getByText('Failed to load logs')).toBeInTheDocument()
    expect(screen.getByText('upstream unavailable')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(refetch).toHaveBeenCalledTimes(1)
  })

  test('keeps the same error and retry action visible on mobile', () => {
    const refetch = mockQuery()
    vi.mocked(useMediaQuery).mockReturnValue(true)

    render(<UsageLogsTable logCategory='common' />)

    expect(screen.getByText('Failed to load logs')).toBeInTheDocument()
    expect(screen.getByText('upstream unavailable')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(refetch).toHaveBeenCalledTimes(1)
  })
})
