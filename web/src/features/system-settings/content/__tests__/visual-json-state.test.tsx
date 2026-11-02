import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { RateLimitVisualEditor } from '../../request-limits/rate-limit-visual-editor'
import { ChatSettingsVisualEditor } from '../chat-settings-visual-editor'

afterEach(() => cleanup())

it('keeps a malformed group rate-limit source visible instead of presenting a confirmed empty group list', () => {
  const raw = '{"unfinished":'
  const change = vi.fn()
  render(<RateLimitVisualEditor value={raw} onChange={change} />)
  expect(screen.getByRole('alert')).toBeVisible()
  expect(screen.getByLabelText('Original JSON')).toHaveTextContent(raw)
  expect(change).not.toHaveBeenCalled()
})

it('keeps chat entries with unrepresentable false and zero values intact for JSON repair instead of stringifying or omitting them', () => {
  const raw = '[{"visible":"https://synthetic.invalid"},{"unknownA":false,"unknownB":0},{"flag":false},{"count":0}]'
  const change = vi.fn()
  render(<ChatSettingsVisualEditor value={raw} onChange={change} />)
  expect(screen.getByRole('alert')).toBeVisible()
  expect(screen.getByLabelText('Original JSON')).toHaveTextContent(raw)
  expect(change).not.toHaveBeenCalled()
})
