import { render, screen } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { AffiliateRewardsCard } from '../affiliate-rewards-card'

test('historical referral statistics remain while promotion and transfer are hidden', () => {
  render(
    <AffiliateRewardsCard
      historyOnly
      canTransfer={false}
      affiliateLink='synthetic-private-referral'
      onTransfer={vi.fn()}
      user={{
        id: 1,
        username: 'fixture',
        quota: 100,
        used_quota: 20,
        request_count: 3,
        aff_quota: 10,
        aff_history_quota: 20,
        aff_count: 2,
        group: 'default',
      }}
    />
  )
  expect(screen.getByText('Total Earned')).toBeInTheDocument()
  expect(screen.getByText('Invites')).toBeInTheDocument()
  expect(screen.queryByText('Referral Program')).not.toBeInTheDocument()
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Transfer to Balance' })
  ).not.toBeInTheDocument()
})
