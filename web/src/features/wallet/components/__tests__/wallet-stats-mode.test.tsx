import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { WalletStatsCard } from '../wallet-stats-card'

test('closed presentation omits balance while preserving recorded usage and request count', () => {
  render(
    <WalletStatsCard
      showBalance={false}
      user={{
        id: 1,
        username: 'fixture',
        quota: 100,
        used_quota: 20,
        request_count: 3,
        aff_quota: 0,
        aff_history_quota: 0,
        aff_count: 0,
        group: 'default',
      }}
    />
  )
  expect(screen.queryByText('Current Balance')).not.toBeInTheDocument()
  expect(screen.getByText('Total Usage')).toBeInTheDocument()
  expect(screen.getByText('API Requests')).toBeInTheDocument()
})

test('explicit commercial presentation retains the old balance card', () => {
  render(
    <WalletStatsCard
      showBalance
      user={{
        id: 1,
        username: 'fixture',
        quota: 100,
        used_quota: 20,
        request_count: 3,
        aff_quota: 0,
        aff_history_quota: 0,
        aff_count: 0,
        group: 'default',
      }}
    />
  )
  expect(screen.getByText('Current Balance')).toBeInTheDocument()
})

test('an account that could not be read shows Unknown, never a zero balance or usage', () => {
  render(<WalletStatsCard showBalance user={null} />)
  for (const label of ['Current Balance', 'Total Usage', 'API Requests']) {
    // The figure follows its label in the stat tile.
    expect(screen.getByText(label).nextElementSibling).toHaveTextContent(
      /^Unknown$/
    )
  }
})

test('a partial verified account keeps its known zero and usage while its missing balance stays Unknown', () => {
  render(<WalletStatsCard showBalance user={{ id: 1, username: 'fixture', group: 'default', request_count: 0, used_quota: 20 }} />)
  expect(screen.getByText('Current Balance').nextElementSibling).toHaveTextContent(/^Unknown$/)
  expect(screen.getByText('Total Usage').nextElementSibling).not.toHaveTextContent('Unknown')
  expect(screen.getByText('API Requests').nextElementSibling).toHaveTextContent(/^0$/)
})
