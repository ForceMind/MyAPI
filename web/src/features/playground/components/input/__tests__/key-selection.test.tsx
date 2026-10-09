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

import { isPlaygroundKeyAvailable } from '../../../api'
import type { PlaygroundKey } from '../../../types'
import { PlaygroundInputControls } from '../playground-input-controls'

const primary: PlaygroundKey = {
  id: 21,
  name: 'International research workspace · 生产环境 · 本番環境 · primary',
  group: 'international_research_group_without_breaks_1234567890',
  status: 1,
  remain_quota: 100,
  unlimited_quota: false,
  expired_time: -1,
}
const secondary: PlaygroundKey = {
  ...primary,
  id: 22,
  name: `${primary.name} · secondary`,
}
const unavailable: PlaygroundKey = {
  ...primary,
  id: 23,
  name: 'Expired research key',
  expired_time: 1,
}
const keys = [primary, secondary, unavailable]

function Controls(props: {
  initialKeyId?: number | null
  disabled?: boolean
  isLoadingKeys?: boolean
  isGenerating?: boolean
  availableKeys?: PlaygroundKey[]
  onKeyChange?: (keyId: number | null) => void
  onStop?: () => void
}) {
  const [keyId, setKeyId] = useState<number | null>(props.initialKeyId ?? null)
  const availableKeys = props.availableKeys ?? keys
  const selected = availableKeys.find((key) => key.id === keyId)
  return (
    <PlaygroundInputControls
      disabled={props.disabled}
      isLoadingKeys={props.isLoadingKeys}
      isGenerating={props.isGenerating}
      canSend={Boolean(selected && isPlaygroundKeyAvailable(selected))}
      hasContent
      keys={availableKeys}
      keyId={keyId}
      models={[{ label: 'research-model', value: 'research-model' }]}
      modelValue='research-model'
      onKeyChange={(value) => {
        setKeyId(value)
        props.onKeyChange?.(value)
      }}
      onModelChange={() => undefined}
      onStop={props.onStop}
      tools={<button type='button'>Attach files</button>}
    />
  )
}

describe('Playground key identity and selection', () => {
  test('loaded keys stay unselected until explicit choice and can be explicitly cleared', async () => {
    const user = userEvent.setup()
    const onKeyChange = vi.fn()
    render(<Controls onKeyChange={onKeyChange} />)
    const select = screen.getByRole('combobox', { name: 'API key' })
    expect(select).toHaveValue('')
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
    expect(onKeyChange).not.toHaveBeenCalled()
    await user.tab()
    await user.tab()
    expect(select).toHaveFocus()
    await user.selectOptions(select, '22')
    expect(onKeyChange).toHaveBeenLastCalledWith(22)
    expect(select).toHaveAccessibleDescription(
      `${secondary.name} · ${secondary.group}`
    )
    expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled()
    await user.selectOptions(select, '')
    expect(onKeyChange).toHaveBeenLastCalledWith(null)
    expect(select).toHaveValue('')
    expect(select).not.toHaveAttribute('aria-describedby')
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
  })

  test('long selected identity occupies a full mobile row and wraps without dropping its suffix', async () => {
    const user = userEvent.setup()
    render(<Controls initialKeyId={22} />)
    const select = screen.getByRole('combobox', { name: 'API key' })
    const identity = screen.getByText(
      `${secondary.name} · ${secondary.group}`,
      { selector: 'p' }
    )
    expect(identity).toBeVisible()
    expect(identity).toHaveClass(
      'whitespace-normal',
      '[overflow-wrap:anywhere]'
    )
    expect(screen.getByRole('group', { name: 'API key' })).toHaveClass(
      'w-full',
      'basis-full',
      'sm:min-w-48'
    )
    expect(select).toHaveAccessibleDescription(
      `${secondary.name} · ${secondary.group}`
    )
    await user.selectOptions(select, '21')
    expect(
      screen.getByText(`${primary.name} · ${primary.group}`, { selector: 'p' })
    ).toBeVisible()
    expect(
      screen.queryByText(`${secondary.name} · ${secondary.group}`, {
        selector: 'p',
      })
    ).not.toBeInTheDocument()
  })

  test('unavailable keys keep their name and status but cannot be chosen or sent', async () => {
    const user = userEvent.setup()
    const onKeyChange = vi.fn()
    render(<Controls initialKeyId={23} onKeyChange={onKeyChange} />)
    const select = screen.getByRole('combobox', { name: 'API key' })
    expect(select).toHaveAccessibleDescription(
      `${unavailable.name} · ${unavailable.group} (Unavailable)`
    )
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
    expect(
      screen.getByRole('option', { name: /Expired research key/ })
    ).toBeDisabled()
    await user.selectOptions(select, '21')
    await user.selectOptions(select, '23')
    expect(select).toHaveValue('21')
    expect(onKeyChange).toHaveBeenCalledOnce()
  })

  test('a missing stored key never impersonates the first key', () => {
    render(<Controls initialKeyId={99} />)
    const select = screen.getByRole('combobox', { name: 'API key' })
    expect(select).toHaveValue('99')
    expect(select).toHaveAccessibleDescription('Unavailable API key · #99')
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
  })

  test('empty keys do not create an identity or enable sending', () => {
    render(<Controls availableKeys={[]} />)
    const select = screen.getByRole('combobox', { name: 'API key' })
    expect(select).toHaveValue('')
    expect(select).not.toHaveAttribute('aria-describedby')
    expect(screen.getAllByRole('option')).toHaveLength(1)
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
  })

  test.each([{ disabled: true }, { isLoadingKeys: true }])(
    'a blocked key control keeps the complete selected identity: %j',
    async (state) => {
      const user = userEvent.setup()
      const onKeyChange = vi.fn()
      render(
        <Controls initialKeyId={22} onKeyChange={onKeyChange} {...state} />
      )
      const select = screen.getByRole('combobox', { name: 'API key' })
      expect(select).toBeDisabled()
      expect(
        screen.getByText(`${secondary.name} · ${secondary.group}`, {
          selector: 'p',
        })
      ).toBeVisible()
      await user.selectOptions(select, '21')
      expect(select).toHaveValue('22')
      expect(onKeyChange).not.toHaveBeenCalled()
    }
  )

  test('generating retains the full identity and exposes an enabled Stop control', async () => {
    const user = userEvent.setup()
    const onStop = vi.fn()
    render(<Controls initialKeyId={22} disabled isGenerating onStop={onStop} />)
    expect(
      screen.getByText(`${secondary.name} · ${secondary.group}`, {
        selector: 'p',
      })
    ).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Send' })
    ).not.toBeInTheDocument()
    const stop = screen.getByRole('button', { name: 'Stop' })
    expect(stop).toBeEnabled()
    await user.click(stop)
    expect(onStop).toHaveBeenCalledOnce()
  })
})
