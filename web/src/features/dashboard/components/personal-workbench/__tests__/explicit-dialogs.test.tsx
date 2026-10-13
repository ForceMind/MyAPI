import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AnchorHTMLAttributes } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { PersonalWorkbench } from '../personal-workbench'
import { usePersonalWorkbenchData } from '../use-personal-workbench-data'

vi.mock('@tanstack/react-router', () => ({
  Link: (
    props: AnchorHTMLAttributes<HTMLAnchorElement> & { to?: string }
  ) => {
    const { to, children, className } = props
    return (
      <a href={to} className={className}>
        {children}
      </a>
    )
  },
}))
vi.mock('../use-personal-workbench-data', () => ({
  usePersonalWorkbenchData: vi.fn(),
}))
vi.mock('@/hooks/use-sidebar-config', () => ({
  useIsSidebarModuleVisible: () => true,
}))
vi.mock('../request-path-section', () => ({
  RequestPathSection: (props: {
    onSelectKey: (keyId: number) => void
  }) => (
    <button onClick={() => props.onSelectKey(31)}>Select test Key</button>
  ),
}))
vi.mock('../key-budget-section', () => ({
  KeyBudgetSection: (props: { onOpenSettings: () => void }) => (
    <button onClick={props.onOpenSettings}>Open budget settings</button>
  ),
}))
vi.mock('../recent-records-section', () => ({
  RecentRecordsSection: () => null,
}))
vi.mock('../account-windows-section', () => ({
  AccountWindowsSection: () => null,
}))
vi.mock('../workbench-actions', () => ({
  WorkbenchActions: (props: { onOpenPolicy: () => void }) => (
    <button onClick={props.onOpenPolicy}>Manage usage policy</button>
  ),
}))
vi.mock('@/features/keys/components/token-budget-dialog', () => ({
  TokenBudgetDialog: () => <div data-testid='token-budget-dialog' />,
}))
vi.mock('@/features/users/components/user-usage-policy-dialog', () => ({
  UserUsagePolicyDialog: () => <div data-testid='usage-policy-dialog' />,
}))

const selectedKey = {
  id: 31,
  name: 'chosen-key',
  status: 1,
  group: 'default',
  remain_quota: 100,
  unlimited_quota: false,
  expired_time: -1,
}
const idleQuery = {
  data: undefined,
  isLoading: false,
  isFetching: false,
  isError: false,
  isRefetchError: false,
  refetch: vi.fn(),
}

beforeEach(() => {
  useAuthStore.getState().auth.setUser({
    id: 7,
    username: 'root',
    role: ROLE.SUPER_ADMIN,
  })
  vi.mocked(usePersonalWorkbenchData).mockImplementation((props) =>
    ({
      status: { status: null, confirmed: true },
      selfQuery: idleQuery,
      keysQuery: idleQuery,
      confirmedKeys: [selectedKey],
      keysError: false,
      budgetQuery: idleQuery,
      recentQuery: idleQuery,
      accountsQuery: idleQuery,
      freshUser: {
        id: 7,
        username: 'root',
        role: ROLE.SUPER_ADMIN,
        quota: 0,
        self_use_no_balance: true,
        usage_policy_revision: 1,
      },
      identityConfirmed: true,
      identityError: false,
      canReadChannels: true,
      selectedKey: props.selectedKeyId === 31 ? selectedKey : undefined,
    }) as never
  )
})

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
})

test('does not mount write-capable dialogs until the user explicitly opens them', async () => {
  const user = userEvent.setup()
  render(<PersonalWorkbench />)

  expect(screen.queryByTestId('token-budget-dialog')).not.toBeInTheDocument()
  expect(screen.queryByTestId('usage-policy-dialog')).not.toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Select test Key' }))
  expect(screen.queryByTestId('token-budget-dialog')).not.toBeInTheDocument()
  await user.click(
    screen.getByRole('button', { name: 'Open budget settings' })
  )
  expect(screen.getByTestId('token-budget-dialog')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Manage usage policy' }))
  expect(screen.getByTestId('usage-policy-dialog')).toBeInTheDocument()
})
