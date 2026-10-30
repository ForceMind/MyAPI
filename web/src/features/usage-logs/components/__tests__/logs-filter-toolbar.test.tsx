/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { Table } from '@tanstack/react-table'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { useMediaQuery } from '@/hooks'

import { LogsFilterToolbar } from '../logs-filter-toolbar'

vi.mock('@/hooks', async () => {
  const actual = await vi.importActual<typeof import('@/hooks')>('@/hooks')
  return { ...actual, useMediaQuery: vi.fn(() => false) }
})

const table = { getAllColumns: () => [] } as unknown as Table<unknown>

function renderToolbar(
  options: { onSearch?: () => void; loading?: boolean } = {}
) {
  return render(
    <LogsFilterToolbar
      table={table}
      primaryFilters={<input aria-label='Model Name' />}
      advancedFilters={<input aria-label='Request ID' />}
      mobilePinnedFilters={
        <>
          <button type='button'>2026-10-26 00:00 ~ 2026-10-26 23:59</button>
          <input aria-label='Model Name' />
        </>
      }
      mobileFilters={
        <>
          <input aria-label='Request ID' />
        </>
      }
      mobileFilterCount={2}
      stats={<section aria-label='Token usage statistics' />}
      hasActiveFilters
      onReset={() => undefined}
      onSearch={options.onSearch ?? (() => undefined)}
      searchLoading={options.loading}
    />
  )
}

describe('logs filter toolbar', () => {
  beforeEach(() => {
    vi.mocked(useMediaQuery).mockReturnValue(false)
  })

  test('a phone keeps frequent filters and a 44px Search on the page, with other filters in the sheet', () => {
    vi.mocked(useMediaQuery).mockReturnValue(true)
    renderToolbar()

    expect(
      screen.getByRole('button', { name: '2026-10-26 00:00 ~ 2026-10-26 23:59' })
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^Filter/ })).toHaveTextContent(
      '2'
    )
    expect(screen.getByLabelText('Model Name')).toBeInTheDocument()
    expect(screen.queryByLabelText('Request ID')).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Search' })
    ).toHaveClass('min-h-11')
    expect(
      screen.getByRole('region', { name: 'Token usage statistics' })
    ).toBeInTheDocument()
  })

  test('a phone searches directly without opening the additional filters', async () => {
    vi.mocked(useMediaQuery).mockReturnValue(true)
    const user = userEvent.setup()
    const onSearch = vi.fn()
    renderToolbar({ onSearch })

    await user.click(screen.getByRole('button', { name: 'Search' }))

    expect(onSearch).toHaveBeenCalledExactlyOnceWith()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  test('a phone opens additional filters and closes the sheet after Search', async () => {
    vi.mocked(useMediaQuery).mockReturnValue(true)
    const user = userEvent.setup()
    const onSearch = vi.fn()
    renderToolbar({ onSearch })
    await user.click(screen.getByRole('button', { name: /^Filter/ }))
    const sheet = await screen.findByRole('dialog')
    expect(within(sheet).getByLabelText('Request ID')).toBeInTheDocument()
    expect(within(sheet).getByRole('button', { name: 'Reset' })).toHaveClass(
      'min-h-11'
    )

    await user.click(within(sheet).getByRole('button', { name: 'Search' }))

    expect(onSearch).toHaveBeenCalledExactlyOnceWith()
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
  })

  test('a pending phone query disables the directly reachable Search', () => {
    vi.mocked(useMediaQuery).mockReturnValue(true)
    renderToolbar({ loading: true })

    expect(screen.getByRole('button', { name: 'Search' })).toBeDisabled()
  })

  test('a PC shows the filters, the search and the statistics on the page', () => {
    renderToolbar()

    expect(screen.getByLabelText('Model Name')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Search' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Expand' })).toHaveAttribute(
      'aria-expanded',
      'false'
    )
    expect(
      screen.queryByRole('button', { name: /^Filter/ })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('region', { name: 'Token usage statistics' })
    ).toBeInTheDocument()
  })
})
