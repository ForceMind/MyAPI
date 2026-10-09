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

import { PlaygroundEmptyState } from '../playground-empty-state'

describe('Playground empty state in a short mobile chat pane', () => {
  test('content starts at the top without reserving viewport-sized space above the mobile composer', () => {
    const { container } = render(
      <PlaygroundEmptyState onSelectPrompt={() => undefined} />
    )
    // The composer can leave much less room than a viewport-based minimum.
    // Real clipping/scroll reachability is checked in browser qualification.
    const emptyState = container.firstElementChild
    expect(emptyState).toHaveClass('items-start', 'py-2')
    expect(emptyState).not.toHaveClass('min-h-[min(520px,calc(100svh-18rem))]')
    expect(emptyState).toHaveClass(
      'md:min-h-[min(520px,calc(100svh-18rem))]',
      'md:items-center',
      'md:py-12'
    )
    expect(
      screen.getByRole('heading', { name: 'Start a playground chat' })
    ).toBeVisible()
    expect(
      screen.getByText(
        'Test a model with a starter prompt, or write your own request below.'
      )
    ).toBeVisible()
  })

  test('all starter prompts remain keyboard reachable after compacting the mobile empty state', async () => {
    const user = userEvent.setup()
    const onSelectPrompt = vi.fn()
    render(<PlaygroundEmptyState onSelectPrompt={onSelectPrompt} />)
    for (const prompt of [
      'Analyze data',
      'Summarize text',
      'Code',
      'Get advice',
    ]) {
      const button = screen.getByRole('button', { name: prompt })
      expect(button).toBeVisible()
      await user.tab()
      expect(button).toHaveFocus()
    }
    expect(onSelectPrompt).not.toHaveBeenCalled()
    await user.keyboard('{Enter}')
    expect(onSelectPrompt).toHaveBeenCalledExactlyOnceWith('Get advice')
  })
})
