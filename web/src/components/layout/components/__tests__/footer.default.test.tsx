import { render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { expect, test, vi } from 'vitest'

import { Footer } from '../footer'

vi.mock('@tanstack/react-router', () => ({
  Link: (props: { to: string; children: ReactNode; className?: string }) => (
    <a href={props.to} className={props.className}>
      {props.children}
    </a>
  ),
}))
vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({
    systemName: 'MyAPI',
    logo: '',
    footerHtml: '',
    demoSiteEnabled: false,
  }),
}))
vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: {
      user_agreement_enabled: true,
      privacy_policy_enabled: true,
    },
  }),
}))

test('default footer shows one project attribution and keeps legal links', () => {
  render(<Footer />)

  const footer = screen.getByRole('contentinfo')
  expect(footer.textContent?.match(/©/g)).toHaveLength(1)
  expect(footer).toHaveTextContent('Self-hosted gateway')
  expect(footer).not.toHaveTextContent('Powerful API Management Platform')
  expect(
    within(footer).getByRole('link', { name: 'User Agreement' })
  ).toBeInTheDocument()
  expect(
    within(footer).getByRole('link', { name: 'Privacy Policy' })
  ).toBeInTheDocument()
})
