import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { useStatus } from '@/hooks/use-status'
import { MYAPI_DOCS_URL } from '@/lib/build-branding'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { useHomePageContent } from '../hooks'
import { Home } from '../index'

vi.mock('@tanstack/react-router', () => ({
  Link: (props: AnchorHTMLAttributes<HTMLAnchorElement> & { to?: string }) => {
    const { to, children, ...attributes } = props
    return (
      <a href={to} {...attributes}>
        {children}
      </a>
    )
  },
}))
vi.mock('@/components/layout', () => ({
  PublicLayout: (props: { children?: ReactNode }) => (
    <div>{props.children}</div>
  ),
}))
vi.mock('@/components/layout/components/footer', () => ({ Footer: () => null }))
vi.mock('@/components/rich-content', () => ({
  RichContent: (props: { content: string }) => (
    <div data-testid='custom-home'>{props.content}</div>
  ),
}))
vi.mock('@/context/theme-provider', () => ({
  useTheme: () => ({ resolvedTheme: 'light' }),
}))
vi.mock('@/hooks/use-status', () => ({ useStatus: vi.fn() }))
vi.mock('../hooks', () => ({ useHomePageContent: vi.fn() }))

describe('default self-hosted home', () => {
  beforeEach(() => {
    useAuthStore.getState().auth.reset('idle')
    vi.mocked(useStatus).mockReturnValue({
      status: { docs_link: '/docs' },
      loading: false,
      error: null,
      confirmed: true,
    })
    vi.mocked(useHomePageContent).mockReturnValue({
      content: '',
      isLoaded: true,
      isUrl: false,
      failed: false,
      retrying: false,
      retry: vi.fn(),
    })
  })

  afterEach(() => {
    useAuthStore.getState().auth.reset('idle')
    vi.clearAllMocks()
  })

  test('shows a three-step start path without promising protected actions to visitors', () => {
    render(<Home />)

    expect(screen.getByText('MyAPI · Self-hosted gateway')).toBeInTheDocument()

    const steps = screen.getByRole('region', {
      name: 'Three steps to get started',
    })
    expect(within(steps).getAllByRole('listitem')).toHaveLength(3)
    expect(
      within(steps).getByRole('heading', { name: 'Channels' })
    ).toBeInTheDocument()
    expect(
      within(steps).getByRole('heading', { name: 'API Keys' })
    ).toBeInTheDocument()
    expect(
      within(steps).getByRole('heading', { name: 'Remaining quota' })
    ).toBeInTheDocument()
    expect(
      within(steps).queryByRole('link', { name: /Open/ })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Sign in' })).toHaveAttribute(
      'href',
      '/sign-in'
    )
  })

  test('unavailable live home settings show one inline retry without hiding default actions', async () => {
    const user = userEvent.setup()
    const retry = vi.fn()
    vi.mocked(useHomePageContent).mockReturnValue({
      content: '',
      isLoaded: true,
      isUrl: false,
      failed: true,
      retrying: false,
      retry,
    })

    render(<Home />)

    expect(
      screen.getByRole('alert', { name: 'Failed to load home page content' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: 'Three steps to get started' })
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(retry).toHaveBeenCalledOnce()
  })

  test('missing runtime docs link keeps the homepage documentation action usable', () => {
    vi.mocked(useStatus).mockReturnValue({
      status: {},
      loading: false,
      error: null,
      confirmed: true,
    })

    render(<Home />)

    expect(screen.getByRole('button', { name: 'Docs' })).toHaveAttribute(
      'href',
      MYAPI_DOCS_URL
    )
  })

  test('links a signed-in root to channels, keys, and the quota overview', () => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
    render(<Home />)

    const steps = screen.getByRole('region', {
      name: 'Three steps to get started',
    })
    expect(
      within(steps).getByRole('link', { name: 'Open Channels' })
    ).toHaveAttribute('href', '/channels')
    expect(
      within(steps).getByRole('link', { name: 'Open API Keys' })
    ).toHaveAttribute('href', '/keys')
    expect(
      within(steps).getByRole('link', { name: 'Open Remaining quota' })
    ).toHaveAttribute('href', '/dashboard')
  })

  test('signed-in ordinary user sees only steps they can actually open', () => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 2, username: 'user', role: ROLE.USER })
    render(<Home />)

    const steps = screen.getByRole('region', {
      name: 'Three steps to get started',
    })
    expect(within(steps).getAllByRole('listitem')).toHaveLength(3)
    expect(
      within(steps).queryByRole('heading', { name: 'Channels' })
    ).toBeNull()
    expect(
      within(steps).queryByRole('heading', { name: 'Remaining quota' })
    ).toBeNull()
    expect(
      within(steps).getByRole('link', { name: 'Open API Keys' })
    ).toHaveAttribute('href', '/keys')
    expect(
      within(steps).getByRole('link', { name: 'Open Usage logs' })
    ).toHaveAttribute('href', '/usage-logs')
    expect(
      within(steps).getByRole('link', { name: 'Open Quota & history' })
    ).toHaveAttribute('href', '/wallet')
    expect(
      screen.queryByRole('heading', { name: 'Container or native process' })
    ).toBeNull()
    expect(
      screen.getByText(
        'Use your API key, review requests, and track remaining credit.'
      )
    ).toBeInTheDocument()
  })

  test('keeps administrator custom Markdown instead of the default home', () => {
    vi.mocked(useHomePageContent).mockReturnValue({
      content: '# Instance home',
      isLoaded: true,
      isUrl: false,
      failed: false,
      retrying: false,
      retry: vi.fn(),
    })
    render(<Home />)

    expect(screen.getByTestId('custom-home')).toHaveTextContent(
      '# Instance home'
    )
    expect(
      screen.queryByRole('heading', { name: 'Three steps to get started' })
    ).not.toBeInTheDocument()
  })

  test('keeps administrator URL home isolated in a sandboxed frame', () => {
    vi.mocked(useHomePageContent).mockReturnValue({
      content: 'https://example.invalid/home',
      isLoaded: true,
      isUrl: true,
      failed: false,
      retrying: false,
      retry: vi.fn(),
    })
    render(<Home />)

    const frame = screen.getByTitle('Custom Home Page')
    expect(frame).toHaveAttribute('src', 'https://example.invalid/home')
    expect(frame).toHaveAttribute('sandbox')
    expect(
      screen.queryByRole('heading', { name: 'Three steps to get started' })
    ).not.toBeInTheDocument()
  })
})
