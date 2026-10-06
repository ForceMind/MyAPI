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
