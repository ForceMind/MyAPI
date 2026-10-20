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
  Navigate: () => (
    <a data-testid='workbench-redirect' href='/dashboard/overview'>
      Console
    </a>
  ),
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

  test('visitors get a complete gateway entry and a sign-in action without balance promises', () => {
    render(<Home />)
    const workflow = screen.getByRole('region', { name: 'Platform workflow' })
    expect(
      within(workflow).getByRole('heading', { name: /Connections & access/ })
    ).toBeInTheDocument()
    expect(
      within(workflow).getByRole('heading', { name: /Records & diagnostics/ })
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Sign in' })).toHaveAttribute(
      'href',
      '/sign-in'
    )
    expect(screen.queryByText(/remaining credit/i)).not.toBeInTheDocument()
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
      screen.getByRole('heading', {
        name: 'Your gateway. Every request in view.',
      })
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

  test.each([ROLE.SUPER_ADMIN, ROLE.USER])(
    'signed-in role %s enters the workbench from the empty default home',
    (role) => {
      useAuthStore
        .getState()
        .auth.setUser({ id: 1, username: 'operator', role })
      render(<Home />)
      expect(screen.getByTestId('workbench-redirect')).toHaveAttribute(
        'href',
        '/dashboard/overview'
      )
    }
  )

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
      screen.queryByRole('heading', {
        name: 'Your gateway. Every request in view.',
      })
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
      screen.queryByRole('heading', {
        name: 'Your gateway. Every request in view.',
      })
    ).not.toBeInTheDocument()
  })
})
