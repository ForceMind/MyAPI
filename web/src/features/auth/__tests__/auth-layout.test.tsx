import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it } from 'vitest'

import { useSystemConfigStore } from '@/stores/system-config-store'

import { AuthLayout } from '../auth-layout'
import { TermsFooter } from '../components/terms-footer'

const initialConfig = useSystemConfigStore.getState()

afterEach(() => {
  useSystemConfigStore.setState(initialConfig)
  localStorage.clear()
})

async function renderAuth() {
  const route = createRootRoute({
    component: () => (
      <AuthLayout>
        <h2>Sign in</h2>
        <label htmlFor='account'>Account</label>
        <input id='account' />
        <TermsFooter
          status={{
            user_agreement_enabled: true,
            privacy_policy_enabled: true,
          }}
        />
      </AuthLayout>
    ),
  })
  const router = createRouter({
    routeTree: route,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  return render(<RouterProvider router={router} />)
}

describe('authentication layout', () => {
  it('retains custom branding and wraps long instance names above the task', async () => {
    const name = 'OperatorGatewayWithAnExceptionallyLongInstanceName'
    useSystemConfigStore
      .getState()
      .setConfig({ systemName: name, logo: '/custom-instance.png' })
    useSystemConfigStore.setState({ loading: false })
    await renderAuth()
    expect(screen.getByRole('heading', { name })).toHaveClass('wrap-anywhere')
    expect(screen.getByRole('img', { name: 'Logo' })).toHaveAttribute(
      'src',
      '/custom-instance.png'
    )
    expect(screen.getByRole('main')).toHaveAttribute('id', 'content')
    expect(screen.getByRole('main').parentElement).toHaveClass('min-h-svh')
    expect(screen.getByRole('main').parentElement).not.toHaveClass('h-svh')
  })

  it('preserves form values when branding finishes loading and keeps legal notices reachable', async () => {
    const user = userEvent.setup()
    useSystemConfigStore.setState({ loading: true })
    await renderAuth()
    await user.type(
      screen.getByRole('textbox', { name: 'Account' }),
      'operator'
    )
    act(() => useSystemConfigStore.setState({ loading: false }))
    expect(screen.getByRole('textbox', { name: 'Account' })).toHaveValue(
      'operator'
    )
    await user.tab()
    expect(screen.getByRole('link', { name: 'User Agreement' })).toHaveFocus()
    expect(
      screen.getByRole('link', { name: 'User Agreement' })
    ).toHaveAttribute('href', '/user-agreement')
    await user.tab()
    expect(screen.getByRole('link', { name: 'Privacy Policy' })).toHaveFocus()
    expect(
      screen.getByRole('link', { name: 'Privacy Policy' })
    ).toHaveAttribute('href', '/privacy-policy')
  })
})
