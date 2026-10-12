/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import type { SystemStatus } from '@/features/auth/types'
import { formatQuota } from '@/lib/format'
import type { UserFundingMode } from '@/lib/self-use-build'

import { userSchema, type User } from '../../types'
import { useUsersColumns } from '../users-columns'

const funding = vi.hoisted(() => ({
  ready: true,
  mode: 'enabled' as UserFundingMode,
}))
const status = vi.hoisted(() => ({
  confirmed: true,
  mode: 'enabled' as UserFundingMode,
  advertisedMode: 'enabled' as UserFundingMode,
  ready: true,
  epoch: 1,
}))

vi.mock('@/hooks/use-funding-presentation', () => ({
  useFundingPresentation: () => ({
    ready: funding.ready,
    commercialEnabled: funding.ready && funding.mode === 'enabled',
    capabilities: { mode: funding.mode },
  }),
}))
vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: {
      user_funding_mode: status.advertisedMode,
      user_funding_capabilities: {
        mode: status.mode,
        ready: status.ready,
        epoch: status.epoch,
      },
    } as SystemStatus,
    confirmed: status.confirmed,
    loading: !status.confirmed,
    error: null,
  }),
}))

const baseUser = userSchema.parse({
  id: 22,
  username: 'list-user',
  display_name: 'List User',
  quota: 1_000_000,
  used_quota: 500_000,
  request_count: 1_234,
  group: 'default',
  aff_count: 7,
  aff_history_quota: 2_000_000,
  inviter_id: 9,
  status: 1,
  role: 1,
})

function UserFundingCells(props: { user: User }) {
  const columns = useUsersColumns()
  const table = useReactTable({
    columns,
    data: [props.user],
    getCoreRowModel: getCoreRowModel(),
  })
  const cells = table
    .getRowModel()
    .rows[0]?.getVisibleCells()
    .filter(
      (cell) =>
        cell.column.id === 'quota' || cell.column.id === 'invite_info'
    )

  return (
    <TooltipProvider>
      {cells?.map((cell) => (
        <div key={cell.id}>
          {flexRender(cell.column.columnDef.cell, cell.getContext())}
        </div>
      ))}
    </TooltipProvider>
  )
}

beforeEach(() => {
  funding.ready = true
  funding.mode = 'enabled'
  status.confirmed = true
  status.mode = 'enabled'
  status.advertisedMode = 'enabled'
  status.ready = true
  status.epoch = 1
})

test('confirmed commercial funding keeps wallet progress and invitation revenue', () => {
  render(<UserFundingCells user={baseUser} />)

  expect(screen.getByRole('progressbar')).toBeVisible()
  expect(screen.getByText(/Revenue:/)).toBeVisible()
  expect(screen.queryByText('Request Count')).not.toBeInTheDocument()
})

test('personal mode shows request count and an active no-wallet policy without money amounts', () => {
  funding.mode = 'disabled'
  status.mode = 'disabled'
  status.advertisedMode = 'disabled'
  render(
    <UserFundingCells
      user={{
        ...baseUser,
        self_use_no_balance: true,
        usage_policy_revision: 1,
      }}
    />
  )

  expect(screen.getByText('Request Count')).toBeVisible()
  expect(screen.getByText('1,234')).toBeVisible()
  expect(screen.getByText('No-wallet policy active')).toBeVisible()
  expect(
    screen.queryByText(formatQuota(baseUser.quota))
  ).not.toBeInTheDocument()
  expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
  expect(screen.queryByText(/Revenue:/)).not.toBeInTheDocument()
  expect(screen.getByText(/Invited:/)).toBeVisible()
})

test('personal mode reports that the stored user allowance still applies', () => {
  funding.mode = 'disabled'
  status.mode = 'disabled'
  status.advertisedMode = 'disabled'
  render(
    <UserFundingCells
      user={{
        ...baseUser,
        self_use_no_balance: false,
        usage_policy_revision: 0,
      }}
    />
  )

  expect(screen.getByText('Request Count')).toBeVisible()
  expect(
    screen.getByText('Stored user allowance remains in effect')
  ).toBeVisible()
  expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
  expect(screen.queryByText(/Revenue:/)).not.toBeInTheDocument()
})

test('retirement mode reports that the stored allowance still applies', () => {
  funding.mode = 'retirement'
  status.mode = 'retirement'
  status.advertisedMode = 'retirement'
  render(
    <UserFundingCells
      user={{
        ...baseUser,
        self_use_no_balance: true,
        usage_policy_revision: 1,
      }}
    />
  )

  expect(screen.getByText('Request Count')).toBeVisible()
  expect(
    screen.getByText('Stored user allowance remains in effect')
  ).toBeVisible()
  expect(screen.queryByText('No-wallet policy active')).not.toBeInTheDocument()
  expect(screen.queryByText(/Revenue:/)).not.toBeInTheDocument()
})

test.each([
  ['the policy value is absent', undefined, undefined],
  ['the policy revision is absent', true, undefined],
  ['the enabled policy has revision zero', true, 0],
] as const)(
  'shows unknown policy when %s',
  (_case, noBalance, policyRevision) => {
    funding.mode = 'disabled'
    status.mode = 'disabled'
    status.advertisedMode = 'disabled'
    render(
      <UserFundingCells
        user={{
          ...baseUser,
          self_use_no_balance: noBalance,
          usage_policy_revision: policyRevision,
        }}
      />
    )

    expect(screen.getByText('Request Count')).toBeVisible()
    expect(screen.getByText('Policy status unknown')).toBeVisible()
    expect(
      screen.queryByText(formatQuota(baseUser.quota))
    ).not.toBeInTheDocument()
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
    expect(screen.queryByText(/Revenue:/)).not.toBeInTheDocument()
  }
)

test.each([
  ['the confirmed modes conflict', 'enabled', 1],
  ['the capability epoch is zero', 'disabled', 0],
] as const)('shows unknown policy when %s', (_case, advertisedMode, epoch) => {
  funding.mode = 'disabled'
  status.mode = 'disabled'
  status.advertisedMode = advertisedMode
  status.epoch = epoch
  render(
    <UserFundingCells
      user={{
        ...baseUser,
        self_use_no_balance: true,
        usage_policy_revision: 1,
      }}
    />
  )

  expect(screen.getByText('Request Count')).toBeVisible()
  expect(screen.getByText('Policy status unknown')).toBeVisible()
  expect(
    screen.queryByText(formatQuota(baseUser.quota))
  ).not.toBeInTheDocument()
  expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
  expect(screen.queryByText(/Revenue:/)).not.toBeInTheDocument()
})

test.each([
  ['the server status is unconfirmed', false, true],
  ['the server capabilities are not ready', true, false],
] as const)('shows unknown policy when %s', (_case, confirmed, ready) => {
  funding.mode = 'disabled'
  status.mode = 'disabled'
  status.advertisedMode = 'disabled'
  status.confirmed = confirmed
  status.ready = ready
  render(
    <UserFundingCells
      user={{
        ...baseUser,
        self_use_no_balance: true,
        usage_policy_revision: 1,
      }}
    />
  )

  expect(screen.getByText('Policy status unknown')).toBeVisible()
  expect(screen.queryByText('No-wallet policy active')).not.toBeInTheDocument()
  expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
  expect(screen.queryByText(/Revenue:/)).not.toBeInTheDocument()
})

test('a minimal display override cannot turn a confirmed enabled server into an active no-wallet policy', () => {
  funding.mode = 'disabled'
  status.mode = 'enabled'
  status.advertisedMode = 'enabled'
  render(
    <UserFundingCells
      user={{
        ...baseUser,
        self_use_no_balance: true,
        usage_policy_revision: 1,
      }}
    />
  )

  expect(screen.getByText('Request Count')).toBeVisible()
  expect(
    screen.getByText('Stored user allowance remains in effect')
  ).toBeVisible()
  expect(screen.queryByText('No-wallet policy active')).not.toBeInTheDocument()
  expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
  expect(screen.queryByText(/Revenue:/)).not.toBeInTheDocument()
})
