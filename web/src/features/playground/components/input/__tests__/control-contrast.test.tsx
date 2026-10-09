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

import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../../../constants'
import { PlaygroundInput } from '../playground-input'

function renderComposer(generating = false) {
  const onStop = vi.fn()
  render(
    <PlaygroundInput
      config={{ ...DEFAULT_CONFIG, keyId: 8 }}
      canSend
      disabled={generating}
      isGenerating={generating}
      keys={[
        {
          id: 8,
          name: 'Research key',
          status: 1,
          group: 'default',
          remain_quota: 100,
          unlimited_quota: false,
          expired_time: -1,
        },
        {
          id: 9,
          name: 'Expired key',
          status: 1,
          group: 'default',
          remain_quota: 100,
          unlimited_quota: false,
          expired_time: 1,
        },
      ]}
      onKeyChange={() => undefined}
      onRefreshKeys={() => undefined}
      models={[{ label: 'vision', value: 'vision' }]}
      modelValue='vision'
      onModelChange={() => undefined}
      onConfigChange={() => undefined}
      onParameterEnabledChange={() => undefined}
      parameterEnabled={DEFAULT_PARAMETER_ENABLED}
      onSubmit={vi.fn().mockResolvedValue(true)}
      onStop={onStop}
    />
  )
  return { onStop }
}

describe('composer identity contrast', () => {
  test('an expired option and disabled clear tool do not dim the available key or the whole composer', () => {
    renderComposer()
    const key = screen.getByRole('combobox', { name: 'API key' })
    expect(key).toBeEnabled()
    expect(
      screen.getByRole('option', {
        name: 'Expired key · default (Unavailable)',
      })
    ).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Clear chat history' })
    ).toBeDisabled()
    expect(
      screen.getByText('Research key · default', { selector: 'p' })
    ).toBeVisible()
    // This is the container's opacity contract. Browser qualification additionally
    // measures effective ancestor opacity and composited text/background contrast.
    const composer = key.closest('[data-slot="input-group"]')
    expect(composer).toHaveClass('has-disabled:opacity-100')
    expect(composer).not.toHaveClass('has-disabled:opacity-50')
  })

  test('a pending request preserves identity contrast while disabling inputs and keeping Stop active', async () => {
    const user = userEvent.setup()
    const { onStop } = renderComposer(true)
    const key = screen.getByRole('combobox', { name: 'API key' })
    expect(key).toBeDisabled()
    expect(screen.getByRole('textbox', { name: 'Message' })).toBeDisabled()
    expect(
      screen.getByText('Research key · default', { selector: 'p' })
    ).toBeVisible()
    expect(key.closest('[data-slot="input-group"]')).toHaveClass(
      'has-disabled:opacity-100'
    )
    expect(
      screen.queryByRole('button', { name: 'Send' })
    ).not.toBeInTheDocument()
    const stop = screen.getByRole('button', { name: 'Stop' })
    expect(stop).toBeEnabled()
    await user.click(stop)
    expect(onStop).toHaveBeenCalledOnce()
  })
})
