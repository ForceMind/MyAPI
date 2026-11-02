import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../../../constants'
import { PlaygroundParameterPanel } from '../playground-parameter-panel'

describe('playground parameter surface', () => {
  it('names the parameter popup and preserves explicit zero while leaving disabled parameters unchanged', async () => {
    const user = userEvent.setup()
    const change = vi.fn()
    const enable = vi.fn()
    render(
      <PlaygroundParameterPanel
        config={{ ...DEFAULT_CONFIG, max_tokens: 0, seed: 17 }}
        parameterEnabled={{
          ...DEFAULT_PARAMETER_ENABLED,
          max_tokens: true,
          seed: true,
          frequency_penalty: false,
        }}
        onConfigChange={change}
        onParameterEnabledChange={enable}
      />
    )
    const trigger = screen.getByRole('button', { name: 'Parameters' })
    expect(trigger).toHaveClass('min-h-11', 'min-w-11')
    await user.click(trigger)
    const popup = await screen.findByRole('dialog', { name: 'Parameter settings' })
    expect(popup).toHaveAccessibleDescription('Only enabled parameters are sent with the request.')
    const details = within(popup).getByRole('region', { name: 'Parameters' })
    expect(details).toHaveAttribute('tabindex', '0')
    expect(details).toHaveClass('overflow-y-auto')
    expect(within(details).getByRole('spinbutton', { name: 'Max Tokens' })).toHaveValue(0)
    const seed = within(details).getByRole('spinbutton', { name: 'Seed' })
    expect(seed).toHaveValue(17)
    expect(change).not.toHaveBeenCalled()
    fireEvent.change(seed, { target: { value: '0' } })
    expect(change).toHaveBeenCalledExactlyOnceWith('seed', 0)
    expect(within(details).getByRole('switch', { name: 'Enable Frequency Penalty' })).not.toBeChecked()
    expect(enable).not.toHaveBeenCalled()
  })
})
