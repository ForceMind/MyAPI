import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { PageSectionLinks } from '../page-section-links'

describe('page task navigation', () => {
  it('moves keyboard focus to mounted evidence without resetting an edited form', async () => {
    const user = userEvent.setup()
    const scroll = vi.spyOn(HTMLElement.prototype, 'scrollIntoView')
    render(
      <>
        <PageSectionLinks
          sections={[{ id: 'route-preview', label: 'Routing Preview' }]}
        />
        <section id='route-preview' tabIndex={-1} aria-label='Routing Preview'>
          <input aria-label='Public model' />
        </section>
      </>
    )
    await user.type(screen.getByRole('textbox'), 'public-model')
    const link = screen.getByRole('link', { name: 'Routing Preview' })
    link.focus()
    await user.keyboard('{Enter}')
    expect(
      screen.getByRole('region', { name: 'Routing Preview' })
    ).toHaveFocus()
    expect(screen.getByRole('textbox')).toHaveValue('public-model')
    expect(scroll).toHaveBeenCalledWith({ block: 'start', behavior: 'auto' })
    expect(window.location.hash).toBe('')
  })
})
