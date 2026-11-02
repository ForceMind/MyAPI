import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import type { PricingModel } from '../../types'
import { ModelCard } from '../model-card'

const model: PricingModel = { id: 7, model_name: 'synthetic-model-with-a-long-unbroken-name-for-small-screens', description: 'Full saved description remains readable without losing its ending.', vendor_name: 'Synthetic provider', quota_type: 0, model_ratio: 1, completion_ratio: 1, enable_groups: ['group-one', 'group-two', 'group-three'], supported_endpoint_types: ['openai', 'openai-response', 'anthropic'], tags: 'tag-one,tag-two,tag-three' }

describe('model catalog card accessibility', () => {
  it('opens model details with the keyboard and exposes full identity and description', async () => {
    const user = userEvent.setup()
    const open = vi.fn()
    render(<ModelCard model={model} onClick={open} />)
    expect(screen.getByRole('heading', { name: model.model_name })).toHaveClass('wrap-anywhere')
    expect(screen.getByText(model.description!)).toBeVisible()
    const action = screen.getByRole('button', { name: `View model ${model.model_name}` })
    action.focus()
    await user.keyboard('{Enter}')
    expect(open).toHaveBeenCalledTimes(1)
    expect(action).toHaveClass('min-h-11')
    expect(screen.getByRole('button', { name: 'Copy model name' })).toHaveClass('min-h-11', 'min-w-11')
  })

  it('exposes every saved group, endpoint and tag through the metadata disclosure', async () => {
    const user = userEvent.setup()
    render(<ModelCard model={model} onClick={vi.fn()} />)
    await user.click(screen.getByText('Groups, endpoints and tags'))
    expect(screen.getByText('group-one · group-two · group-three')).toBeVisible()
    expect(screen.getByText('openai · openai-response · anthropic')).toBeVisible()
    expect(screen.getByText('tag-one · tag-two · tag-three')).toBeVisible()
  })
})
