import { render } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { Turnstile } from '../turnstile'

afterEach(() => {
  delete window.turnstile
})

it('uses the supported compact challenge without changing token callbacks on rerender', () => {
  const renderWidget = vi.fn()
  const first = vi.fn()
  const latest = vi.fn()
  window.turnstile = { render: renderWidget }
  const view = render(
    <Turnstile siteKey='synthetic-site-key' onVerify={first} />
  )
  view.rerender(<Turnstile siteKey='synthetic-site-key' onVerify={latest} />)
  expect(renderWidget).toHaveBeenCalledOnce()
  const options = renderWidget.mock.calls[0][1] as Record<string, unknown>
  expect(options.size).toBe('compact')
  ;(options.callback as (token: string) => void)('synthetic-callback-result')
  expect(first).not.toHaveBeenCalled()
  expect(latest).toHaveBeenCalledWith('synthetic-callback-result')
})
