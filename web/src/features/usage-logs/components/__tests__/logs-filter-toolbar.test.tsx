/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { Table } from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { useMediaQuery } from '@/hooks'

import { LogsFilterToolbar } from '../logs-filter-toolbar'

vi.mock('@/hooks', async () => {
  const actual = await vi.importActual<typeof import('@/hooks')>('@/hooks')
  return { ...actual, useMediaQuery: vi.fn(() => false) }
})

const table = { getAllColumns: () => [] } as unknown as Table<unknown>

function renderToolbar() {
  return render(
    <LogsFilterToolbar
      table={table}
      primaryFilters={<input aria-label='Model Name' />}
      advancedFilters={<input aria-label='Request ID' />}
      mobilePinnedFilters={
        <button type='button'>2026-10-26 00:00 ~ 2026-10-26 23:59</button>
      }
      mobileFilters={
        <>
          <input aria-label='Model Name' />
          <input aria-label='Request ID' />
        </>
      }
      mobileFilterCount={2}
      stats={<section aria-label='Token usage statistics' />}
      hasActiveFilters
      onReset={() => undefined}
      onSearch={() => undefined}
    />
  )
}

describe('logs filter toolbar', () => {
  beforeEach(() => {
    vi.mocked(useMediaQuery).mockReturnValue(false)
  })

  test('a phone keeps one summary line and moves the other filters into the sheet', () => {
    vi.mocked(useMediaQuery).mockReturnValue(true)
    renderToolbar()

    expect(
      screen.getByRole('button', { name: '2026-10-26 00:00 ~ 2026-10-26 23:59' })
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^Filter/ })).toHaveTextContent(
      '2'
    )
    expect(screen.queryByLabelText('Model Name')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Request ID')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Search' })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('region', { name: 'Token usage statistics' })
    ).toBeInTheDocument()
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
