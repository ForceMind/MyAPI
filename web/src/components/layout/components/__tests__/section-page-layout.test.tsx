import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import { SidebarInset } from '@/components/ui/sidebar'

import { PageFooterPortal } from '../page-footer'
import { SectionPageLayout } from '../section-page-layout'

describe('shared task page', () => {
  it('provides one focusable main target inside the application inset', () => {
    render(
      <SidebarInset>
        <SectionPageLayout>
          <SectionPageLayout.Title>API Keys</SectionPageLayout.Title>
          <SectionPageLayout.Content>
            Existing key list
          </SectionPageLayout.Content>
        </SectionPageLayout>
      </SidebarInset>
    )
    const main = screen.getByRole('main')
    expect(main).toHaveAttribute('id', 'content')
    main.focus()
    expect(main).toHaveFocus()
  })

  it('exposes the complete task name as a primary heading without ellipsis', () => {
    const title =
      'Routing reliability and account-window recovery configuration'
    render(
      <SectionPageLayout>
        <SectionPageLayout.Title>{title}</SectionPageLayout.Title>
        <SectionPageLayout.Content>Existing settings</SectionPageLayout.Content>
      </SectionPageLayout>
    )
    const heading = screen.getByRole('heading', { name: title, level: 1 })
    expect(heading).not.toHaveClass('truncate')
    expect(heading).toHaveClass('wrap-break-word')
  })

  it('keeps content state and footer actions reachable when the title changes', async () => {
    const user = userEvent.setup()
    const page = (title: string) => (
      <SectionPageLayout>
        <SectionPageLayout.Title>{title}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <button type='button'>Refresh</button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <input aria-label='Existing filter' />
          <PageFooterPortal>
            <button type='button'>Next page</button>
          </PageFooterPortal>
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
    const view = render(page('Usage Logs'))
    await user.type(
      screen.getByRole('textbox', { name: 'Existing filter' }),
      'request-42'
    )
    view.rerender(page('Usage Logs · filtered'))
    expect(screen.getByRole('textbox')).toHaveValue('request-42')
    await user.tab()
    expect(screen.getByRole('button', { name: 'Next page' })).toHaveFocus()
  })
})
