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

import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { useMediaQuery } from '@/hooks'

import type { FullContentLogFilters } from '../../types'
import { FullContentLogFilterBar } from '../full-content-log-filter-bar'

vi.mock('@/hooks', async () => {
  const actual = await vi.importActual<typeof import('@/hooks')>('@/hooks')
  return { ...actual, useMediaQuery: vi.fn(() => false) }
})

beforeEach(() => {
  vi.mocked(useMediaQuery).mockReturnValue(false)
})

const EMPTY_FILTERS: FullContentLogFilters = {
  model: '',
  token: '',
  requestId: '',
  startTime: '',
  endTime: '',
}

function FilterHarness(props: {
  onApply: (filters: FullContentLogFilters) => void
  onReset: () => void
  initialFilters?: FullContentLogFilters
}) {
  const [filters, setFilters] = useState(props.initialFilters ?? EMPTY_FILTERS)
  return (
    <FullContentLogFilterBar
      filters={filters}
      modelOptions={['gpt-5.6-luna']}
      tokenOptions={[{ id: 7, name: 'test-key' }]}
      onChange={setFilters}
      onApply={() => props.onApply(filters)}
      onReset={props.onReset}
    />
  )
}

describe('full content log filters', () => {
  test('submits model, token and request filters from the keyboard', async () => {
    const user = userEvent.setup()
    const onApply = vi.fn()

    render(<FilterHarness onApply={onApply} onReset={() => undefined} />)
    await user.type(screen.getByLabelText('Model'), 'gpt-5.6-luna')
    await user.type(screen.getByLabelText('API Key name or ID'), 'test')
    await user.type(screen.getByLabelText('Request ID'), 'request-123')
    await user.keyboard('{Enter}')

    expect(onApply).toHaveBeenCalledTimes(1)
    expect(screen.getByLabelText('Model')).toHaveValue('gpt-5.6-luna')
    expect(screen.getByLabelText('API Key name or ID')).toHaveValue('test')
    expect(screen.getByLabelText('Request ID')).toHaveValue('request-123')
    expect(screen.getByDisplayValue('gpt-5.6-luna')).toHaveAttribute(
      'list',
      'full-log-model-options'
    )
  })

  test('exposes a named reset control', async () => {
    const user = userEvent.setup()
    const onReset = vi.fn()

    render(<FilterHarness onApply={() => undefined} onReset={onReset} />)
    await user.click(screen.getByRole('button', { name: 'Reset filters' }))

    expect(onReset).toHaveBeenCalledTimes(1)
  })

  test('selects a calendar range into the filter draft and submits exact local timestamps only on Search', async () => {
    const user = userEvent.setup()
    const onApply = vi.fn()
    render(
      <FilterHarness
        onApply={onApply}
        onReset={() => undefined}
        initialFilters={{
          ...EMPTY_FILTERS,
          model: 'gpt-5.6-luna',
          startTime: '2026-10-01T00:00:00',
          endTime: '2026-10-01T23:59:59',
        }}
      />
    )

    await user.click(screen.getByRole('button', { name: /^Date Range:/ }))
    expect(screen.getAllByRole('grid')).toHaveLength(1)
    await user.click(screen.getByRole('button', { name: /October 29/ }))
    await user.click(screen.getByRole('button', { name: 'Next month' }))
    await user.click(screen.getByRole('button', { name: /November 2nd/ }))
    expect(onApply).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Apply range' }))
    expect(onApply).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /^Date Range:/ })).toHaveTextContent(
      '2026-10-29 00:00:00 – 2026-11-02 23:59:59'
    )
    await user.click(screen.getByRole('button', { name: 'Search' }))

    expect(onApply).toHaveBeenCalledExactlyOnceWith({
      ...EMPTY_FILTERS,
      model: 'gpt-5.6-luna',
      startTime: '2026-10-29T00:00:00',
      endTime: '2026-11-02T23:59:59',
    })
  })

  test('cancelling a calendar draft preserves the original date filter values for Search', async () => {
    const user = userEvent.setup()
    const onApply = vi.fn()
    const filters = {
      ...EMPTY_FILTERS,
      startTime: '2026-10-12T10:23:45',
      endTime: '2026-10-13T11:34:56',
    }
    render(
      <FilterHarness onApply={onApply} onReset={() => undefined} initialFilters={filters} />
    )
    await user.click(screen.getByRole('button', { name: /^Date Range:/ }))
    await user.click(screen.getByRole('button', { name: 'Today' }))
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onApply).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Search' }))
    expect(onApply).toHaveBeenCalledExactlyOnceWith(filters)
  })

  test('Enter in the calendar time editor applies its draft without submitting Search', async () => {
    const user = userEvent.setup()
    const onApply = vi.fn()
    render(
      <FilterHarness
        onApply={onApply}
        onReset={() => undefined}
        initialFilters={{
          ...EMPTY_FILTERS,
          startTime: '2026-10-12T00:00:00',
          endTime: '2026-10-12T23:59:59',
        }}
      />
    )
    await user.click(screen.getByRole('button', { name: /^Date Range:/ }))
    const startTime = screen.getByLabelText('Start Time')
    fireEvent.change(startTime, { target: { value: '10:23:45' } })
    startTime.focus()
    await user.keyboard('{Enter}')

    expect(onApply).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /^Date Range:/ })).toHaveTextContent(
      '2026-10-12 10:23:45 – 2026-10-12 23:59:59'
    )
    await user.click(screen.getByRole('button', { name: 'Search' }))
    expect(onApply).toHaveBeenCalledExactlyOnceWith({
      ...EMPTY_FILTERS,
      startTime: '2026-10-12T10:23:45',
      endTime: '2026-10-12T23:59:59',
    })
  })

  test('a phone keeps the filters behind one line that counts the filters in use', async () => {
    vi.mocked(useMediaQuery).mockReturnValue(true)
    const user = userEvent.setup()

    render(<FilterHarness onApply={() => undefined} onReset={() => undefined} />)
    const toggle = screen.getByRole('button', { name: 'Filters' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByLabelText('Model')).not.toBeVisible()

    await user.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    await user.type(screen.getByLabelText('Model'), 'gpt-5.6-luna')
    expect(toggle).toHaveTextContent('1')
  })
})
