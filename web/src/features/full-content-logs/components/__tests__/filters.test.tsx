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

import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, test, vi } from 'vitest'

import type { FullContentLogFilters } from '../../types'
import { FullContentLogFilterBar } from '../full-content-log-filter-bar'

const EMPTY_FILTERS: FullContentLogFilters = {
  model: '',
  token: '',
  requestId: '',
  startTime: '',
  endTime: '',
}

function FilterHarness(props: { onApply: () => void; onReset: () => void }) {
  const [filters, setFilters] = useState(EMPTY_FILTERS)
  return (
    <FullContentLogFilterBar
      filters={filters}
      modelOptions={['gpt-5.6-luna']}
      tokenOptions={[{ id: 7, name: 'test-key' }]}
      onChange={setFilters}
      onApply={props.onApply}
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
})
