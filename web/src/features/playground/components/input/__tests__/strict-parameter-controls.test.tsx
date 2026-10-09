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
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, test, vi } from 'vitest'

import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../../../constants'
import type { PlaygroundConfig, PlaygroundParameterKey } from '../../../types'
import { PlaygroundParameterPanel } from '../playground-parameter-panel'

const unsupportedParameters: PlaygroundParameterKey[] = [
  'temperature',
  'top_p',
  'frequency_penalty',
  'presence_penalty',
  'seed',
]

function StrictParameterPanel(props: { maxTokens: number }) {
  const [config, setConfig] = useState({
    ...DEFAULT_CONFIG,
    max_tokens: props.maxTokens,
  })
  return (
    <PlaygroundParameterPanel
      config={config}
      onConfigChange={(key, value) =>
        setConfig((current) => ({ ...current, [key]: value }))
      }
      onParameterEnabledChange={() => undefined}
      parameterEnabled={DEFAULT_PARAMETER_ENABLED}
      strictChatParameters
      unsupportedParameters={unsupportedParameters}
      unsupportedProvider='Strict Token budget'
    />
  )
}

describe('strict chat parameter controls', () => {
  test('required Max Tokens stays editable even when its saved switch is off', async () => {
    const user = userEvent.setup()
    const onParameterEnabledChange = vi.fn()
    render(
      <PlaygroundParameterPanel
        config={DEFAULT_CONFIG}
        onConfigChange={() => undefined}
        onParameterEnabledChange={onParameterEnabledChange}
        parameterEnabled={DEFAULT_PARAMETER_ENABLED}
        strictChatParameters
        unsupportedParameters={unsupportedParameters}
        unsupportedProvider='Strict Token budget'
      />
    )
    expect(
      screen.getByRole('button', { name: 'Parameters' })
    ).toHaveTextContent('1')
    await user.click(screen.getByRole('button', { name: 'Parameters' }))
    const maxTokens = screen.getByRole('spinbutton', { name: 'Max Tokens' })
    expect(maxTokens).toBeEnabled()
    expect(maxTokens).toBeRequired()
    expect(maxTokens).toHaveAttribute('min', '1')
    expect(maxTokens).toHaveAttribute('max', '128000')
    expect(screen.getByText('Required')).toBeVisible()
    const toggle = screen.getByRole('switch', { name: 'Enable Max Tokens' })
    expect(toggle).toBeChecked()
    expect(toggle).toHaveAttribute('aria-disabled', 'true')
    await user.click(toggle)
    expect(onParameterEnabledChange).not.toHaveBeenCalled()
  })

  test.each([
    { saved: 0, displayed: 1 },
    { saved: -1, displayed: 1 },
    { saved: 200000, displayed: 128000 },
    { saved: 127999.75, displayed: 127999 },
    { saved: Number.NaN, displayed: 4096 },
    { saved: Number.POSITIVE_INFINITY, displayed: 4096 },
  ])(
    'saved Max Tokens $saved displays normalized $displayed without overwriting preferences',
    async ({ saved, displayed }) => {
      const user = userEvent.setup()
      const onConfigChange = vi.fn()
      render(
        <PlaygroundParameterPanel
          config={{ ...DEFAULT_CONFIG, max_tokens: saved }}
          onConfigChange={onConfigChange}
          onParameterEnabledChange={() => undefined}
          parameterEnabled={DEFAULT_PARAMETER_ENABLED}
          strictChatParameters
          unsupportedParameters={unsupportedParameters}
        />
      )
      await user.click(screen.getByRole('button', { name: 'Parameters' }))
      expect(
        screen.getByRole('spinbutton', { name: 'Max Tokens' })
      ).toHaveValue(displayed)
      expect(
        within(screen.getByRole('group', { name: 'Max Tokens' })).getByText(
          String(displayed)
        )
      ).toBeVisible()
      expect(onConfigChange).not.toHaveBeenCalled()
    }
  )

  test.each([
    { entered: '0', expected: 1 },
    { entered: '999999', expected: 128000 },
    { entered: '12.9', expected: 12 },
    { entered: '', expected: 1 },
  ])(
    'editing strict Max Tokens to $entered keeps the displayed and saved value at $expected',
    async ({ entered, expected }) => {
      const user = userEvent.setup()
      render(<StrictParameterPanel maxTokens={4096} />)
      await user.click(screen.getByRole('button', { name: 'Parameters' }))
      const input = screen.getByRole('spinbutton', { name: 'Max Tokens' })
      fireEvent.change(input, { target: { value: entered } })
      expect(input).toHaveValue(expected)
      expect(
        within(screen.getByRole('group', { name: 'Max Tokens' })).getByText(
          String(expected)
        )
      ).toBeVisible()
    }
  )

  test('strict sampling controls explain their disabled state and reject keyboard changes', async () => {
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue(
      new DOMRect(0, 0, 200, 24)
    )
    const user = userEvent.setup()
    const onConfigChange = vi.fn()
    const onParameterEnabledChange = vi.fn()
    const config: PlaygroundConfig = { ...DEFAULT_CONFIG, seed: 23 }
    render(
      <PlaygroundParameterPanel
        config={config}
        onConfigChange={onConfigChange}
        onParameterEnabledChange={onParameterEnabledChange}
        parameterEnabled={{ ...DEFAULT_PARAMETER_ENABLED, seed: true }}
        strictChatParameters
        unsupportedParameters={unsupportedParameters}
        unsupportedProvider='Strict Token budget'
      />
    )
    await user.click(screen.getByRole('button', { name: 'Parameters' }))
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Strict Token budget does not support these parameters'
    )
    for (const label of [
      'Temperature',
      'Top P',
      'Frequency Penalty',
      'Presence Penalty',
      'Seed',
    ]) {
      const toggle = screen.getByRole('switch', { name: `Enable ${label}` })
      expect(toggle).not.toBeChecked()
      expect(toggle).toHaveAttribute('aria-disabled', 'true')
      await user.click(toggle)
    }
    for (const label of [
      'Temperature',
      'Top P',
      'Frequency Penalty',
      'Presence Penalty',
    ]) {
      const slider = screen.getByRole('slider', { name: label })
      expect(slider).toBeDisabled()
      slider.focus()
      await user.keyboard('{ArrowRight}')
    }
    expect(screen.getByRole('spinbutton', { name: 'Seed' })).toBeDisabled()
    expect(onConfigChange).not.toHaveBeenCalled()
    expect(onParameterEnabledChange).not.toHaveBeenCalled()
  })
})
