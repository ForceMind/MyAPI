import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AnchorHTMLAttributes } from 'react'
import { useState } from 'react'
import { describe, expect, test, vi } from 'vitest'

import type { PlaygroundKey } from '@/features/playground/types'

import { KeyBudgetSection } from '../key-budget-section'
import { RequestPathSection } from '../request-path-section'

vi.mock('@tanstack/react-router', () => ({
  Link: (
    props: AnchorHTMLAttributes<HTMLAnchorElement> & { to?: string }
  ) => {
    const { to, children, ...attributes } = props
    return (
      <a href={to} {...attributes}>
        {children}
      </a>
    )
  },
}))
vi.mock('@/hooks/use-sidebar-config', () => ({
  useIsSidebarModuleVisible: () => true,
}))

function key(id: number): PlaygroundKey {
  return {
    id,
    name: `personal-key-${id}`,
    status: 1,
    group: 'default',
    remain_quota: id * 10,
    unlimited_quota: false,
    expired_time: -1,
  }
}

function SelectionHarness(props: { keys: PlaygroundKey[] }) {
  const [selectedKeyId, setSelectedKeyId] = useState<number | null>(null)
  const [visibleCount, setVisibleCount] = useState(10)
  return (
    <RequestPathSection
      keys={props.keys}
      selectedKey={props.keys.find((item) => item.id === selectedKeyId)}
      selectedKeyId={selectedKeyId}
      visibleCount={visibleCount}
      isLoading={false}
      isError={false}
      isFetching={false}
      onSelectKey={setSelectedKeyId}
      onLoadMore={() => setVisibleCount((count) => count + 10)}
      onRefresh={() => undefined}
    />
  )
}

describe('personal workbench Key selection and limits', () => {
  test('does not display an unsafe internal allowance as an exact number', () => {
    render(
      <KeyBudgetSection
        selectedKey={{ ...key(7), remain_quota: Number.MAX_SAFE_INTEGER + 1 }}
        budget={undefined}
        isLoading={false}
        isError={false}
        isFetching={false}
        onRefresh={() => undefined}
        onOpenSettings={() => undefined}
      />
    )
    expect(screen.getAllByText('Unknown', { exact: true })).toHaveLength(4)
  })

  test('keeps every loaded Key unselected until the user chooses one', async () => {
    const user = userEvent.setup()
    render(<SelectionHarness keys={Array.from({ length: 12 }, (_, i) => key(i + 1))} />)

    const select = screen.getByRole('combobox', { name: 'Select an API Key' })
    expect(select).toHaveValue('')
    expect(within(select).getAllByRole('option')).toHaveLength(11)
    expect(screen.getByText('No Key selected')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Load more Keys' }))
    expect(within(select).getAllByRole('option')).toHaveLength(13)
    await user.selectOptions(select, '12')

    expect(select).toHaveValue('12')
  })

  test('shows unknown strict limits when the budget read fails', () => {
    render(
      <KeyBudgetSection
        selectedKey={key(7)}
        budget={undefined}
        isLoading={false}
        isError
        isFetching={false}
        onRefresh={() => undefined}
        onOpenSettings={() => undefined}
      />
    )

    const section = screen.getByTestId('key-budget-section')
    expect(
      within(section).getByText('Unable to load this Key budget')
    ).toBeInTheDocument()
    expect(within(section).getAllByText('Unknown')).toHaveLength(3)
    expect(within(section).queryByText('Not configured')).not.toBeInTheDocument()
  })

  test('keeps units separate across legal budget modes', () => {
    const { rerender } = render(
      <KeyBudgetSection
        selectedKey={key(7)}
        budget={{
          policy: {
            token_id: 7,
            user_id: 4,
            enabled: true,
            account_threshold_enabled: false,
            account_min_remaining_bps: 0,
            account_max_age_seconds: 300,
            fee_enabled: true,
            fee_limit_usd: '12.500000',
            fee_used_usd: '1.250000',
            fee_reserved_usd: '0.000000',
            limit: 5000,
            used: 1000,
            reserved: 0,
            pending_request_id: '',
            revision: 1,
          },
          pending: null,
        }}
        isLoading={false}
        isError={false}
        isFetching={false}
        onRefresh={() => undefined}
        onOpenSettings={() => undefined}
      />
    )

    const section = screen.getByTestId('key-budget-section')
    expect(within(section).getByText('Internal units')).toBeInTheDocument()
    expect(within(section).getByText('Token')).toBeInTheDocument()
    expect(within(section).getByText('USD 12.500000')).toBeInTheDocument()

    rerender(
      <KeyBudgetSection
        selectedKey={key(7)}
        budget={{
          policy: {
            token_id: 7,
            user_id: 4,
            enabled: false,
            account_threshold_enabled: true,
            account_min_remaining_bps: 2500,
            account_max_age_seconds: 300,
            fee_enabled: false,
            fee_limit_usd: '0.000000',
            fee_used_usd: '0.000000',
            fee_reserved_usd: '0.000000',
            limit: 0,
            used: 0,
            reserved: 0,
            pending_request_id: '',
            revision: 2,
          },
          pending: null,
        }}
        isLoading={false}
        isError={false}
        isFetching={false}
        onRefresh={() => undefined}
        onOpenSettings={() => undefined}
      />
    )

    expect(within(section).getAllByText('25.00%')).toHaveLength(2)
    expect(within(section).getAllByText('Not configured')).toHaveLength(2)
  })
})
