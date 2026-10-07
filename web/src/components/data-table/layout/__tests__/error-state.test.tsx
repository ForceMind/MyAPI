/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { ErrorState } from '@/components/error-state'

import { DataTablePage } from '../data-table-page'

const columns = [{ accessorKey: 'name', header: 'Name' }]
const data = [{ name: 'Previously loaded row' }]

function ErrorPage(props: { cardView?: boolean }) {
  const [failed, setFailed] = useState(true)
  const table = useReactTable({
    data,
    columns,
    getCoreRowModel: getCoreRowModel(),
  })
  return (
    <DataTablePage
      table={table}
      columns={columns}
      toolbar={
        <input aria-label='Filter records' defaultValue='Preserved search' />
      }
      enableCardView={props.cardView}
      mobile={<button type='button'>Stale mobile action</button>}
      renderCard={() => <button type='button'>Stale card action</button>}
      bulkActions={<button type='button'>Delete selection</button>}
      afterTable={<p>Previously loaded summary</p>}
      paginationInFooter={false}
      errorState={
        failed ? (
          <ErrorState
            title='List unavailable'
            onRetry={() => setFailed(false)}
          />
        ) : undefined
      }
    />
  )
}

afterEach(() => vi.restoreAllMocks())

test.each(['desktop', 'card', 'mobile'] as const)(
  '%s error feedback keeps filters but removes stale content, actions and counts',
  (mode) => {
    if (mode === 'mobile') {
      const matchMedia = window.matchMedia
      vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
        ...matchMedia(query),
        matches: query === '(max-width: 640px)',
      }))
    }
    render(<ErrorPage cardView={mode === 'card'} />)
    expect(screen.getByRole('alert')).toHaveTextContent('List unavailable')
    expect(screen.getByRole('textbox', { name: 'Filter records' })).toHaveValue(
      'Preserved search'
    )
    expect(screen.queryByText('Previously loaded row')).not.toBeInTheDocument()
    expect(
      screen.queryByText('Previously loaded summary')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Stale .* action/ })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Delete selection' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Go to next page' })
    ).not.toBeInTheDocument()
  }
)

test('keyboard retry restores normal content and controls while retaining the filter', async () => {
  const user = userEvent.setup()
  render(<ErrorPage />)
  screen.getByRole('button', { name: 'Retry' }).focus()
  await user.keyboard('{Enter}')
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(screen.getByText('Previously loaded row')).toBeVisible()
  expect(screen.getByText('Previously loaded summary')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Delete selection' })).toBeEnabled()
  expect(screen.getByRole('button', { name: 'Go to next page' })).toBeVisible()
  expect(screen.getByRole('textbox', { name: 'Filter records' })).toHaveValue(
    'Preserved search'
  )
})
