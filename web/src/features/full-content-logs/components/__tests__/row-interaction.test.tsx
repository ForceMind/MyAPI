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
import { describe, expect, test, vi } from 'vitest'

import {
  FullContentLogMobileCard,
  FullContentLogRow,
} from '../full-content-log-row'

const LOG = {
  timestamp: '2026-08-28T01:00:00Z',
  request_id: 'request-123',
  method: 'POST',
  path: '/v1/responses',
  model: 'gpt-5.6-luna',
  status: 200,
  duration_ms: 321,
  request_bytes: 100,
  response_bytes: 200,
  chunk_count: 2,
  token_id: 7,
  token_name: 'test-key',
}

describe('full content log row', () => {
  test('opens online detail from the row or the fixed action', async () => {
    const user = userEvent.setup()
    const onView = vi.fn()

    render(
      <table>
        <tbody>
          <FullContentLogRow log={LOG} onView={onView} />
        </tbody>
      </table>
    )

    await user.click(screen.getByText('gpt-5.6-luna'))
    expect(onView).toHaveBeenLastCalledWith('request-123')

    await user.click(screen.getByRole('button', { name: 'View content' }))
    expect(onView).toHaveBeenCalledTimes(2)
  })

  test('keeps the detail action reachable in the mobile card layout', async () => {
    const user = userEvent.setup()
    const onView = vi.fn()

    render(<FullContentLogMobileCard log={LOG} onView={onView} />)

    expect(screen.getByText('POST /v1/responses')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'View content' }))

    expect(onView).toHaveBeenCalledWith('request-123')
  })
})
