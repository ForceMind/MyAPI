import { fireEvent, render } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { Turnstile } from './turnstile'

describe('Turnstile', () => {
  afterEach(() => {
    document.querySelector('#cf-turnstile')?.remove()
    delete window.turnstile
  })

  test('uses the latest callbacks for a pending script without accumulating listeners', () => {
    const firstVerify = vi.fn()
    const secondVerify = vi.fn()
    const firstExpire = vi.fn()
    const secondExpire = vi.fn()
    const renderWidget = vi.fn()
    const view = render(
      <Turnstile
        siteKey='site-key'
        onVerify={firstVerify}
        onExpire={firstExpire}
      />
    )
    const script = document.querySelector<HTMLScriptElement>('#cf-turnstile')
    expect(script).not.toBeNull()
    if (!script) {
      throw new Error('Turnstile script was not created')
    }

    view.rerender(
      <Turnstile
        siteKey='site-key'
        onVerify={secondVerify}
        onExpire={secondExpire}
      />
    )
    window.turnstile = { render: renderWidget }
    fireEvent.load(script)

    expect(renderWidget).toHaveBeenCalledTimes(1)
    const options = renderWidget.mock.calls[0][1] as Record<string, unknown>
    ;(options.callback as (token: string) => void)('token')
    expect(firstVerify).not.toHaveBeenCalled()
    expect(secondVerify).toHaveBeenCalledWith('token')

    view.unmount()
    fireEvent.error(script)
    expect(secondExpire).not.toHaveBeenCalled()
  })

  test('renders immediately when the Turnstile script is already available', () => {
    const renderWidget = vi.fn()
    window.turnstile = { render: renderWidget }

    render(<Turnstile siteKey='site-key' onVerify={vi.fn()} />)

    expect(renderWidget).toHaveBeenCalledTimes(1)
    expect(document.querySelector('#cf-turnstile')).toBeNull()
  })

  test('reports a script loading failure once', () => {
    const onExpire = vi.fn()
    render(
      <Turnstile siteKey='site-key' onVerify={vi.fn()} onExpire={onExpire} />
    )
    const script = document.querySelector<HTMLScriptElement>('#cf-turnstile')
    expect(script).not.toBeNull()
    if (!script) {
      throw new Error('Turnstile script was not created')
    }

    fireEvent.error(script)
    fireEvent.error(script)

    expect(onExpire).toHaveBeenCalledTimes(1)
  })
})
