import { screen } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'

import type { SystemStatus } from '@/features/auth/types'

import { MobileDrawer } from '../mobile-drawer'
import { renderShellRoute } from './shell-test-utils'

function status(mode: 'enabled' | 'disabled' | 'retirement'): SystemStatus & {
  user_funding_capabilities: NonNullable<SystemStatus['user_funding_capabilities']>
} {
  return {
    user_funding_mode: mode,
    user_funding_capabilities: {
      mode, epoch: 1, ready: true,
      can_top_up: true, can_redeem: true,
      can_transfer_affiliate_rewards: true,
      can_purchase_subscription: true, can_view_funding_history: true,
    },
  }
}

afterEach(() => localStorage.clear())

test.each(['enabled', 'disabled', 'retirement', 'mode-conflict', 'zero-epoch', 'unsafe-epoch'] as const)('mobile navigation respects %s server state', async (state) => {
  const snapshot = status(state === 'disabled' || state === 'retirement' ? state : 'enabled')
  if (state === 'mode-conflict') snapshot.user_funding_mode = 'disabled'
  if (state === 'zero-epoch' || state === 'unsafe-epoch') {
    snapshot.user_funding_capabilities.epoch = state === 'zero-epoch' ? 0 : Number.MAX_SAFE_INTEGER + 1
  }
  await renderShellRoute(
    <MobileDrawer
      isOpen onClose={() => {}} homeUrl='/' displayLogo={<span>MyAPI</span>}
      displaySiteName='MyAPI' loading={false} logoLoaded
      mobileLinksList={[]} showAuthButtons
      user={{ id: 71, username: 'synthetic-owner', role: 1 }}
    />,
    { status: snapshot }
  )
  expect(screen.getByRole('link', { name: 'Profile' })).toBeInTheDocument()
  expect(screen.queryByRole('link', { name: 'Wallet' }) !== null).toBe(state === 'enabled')
  expect(screen.queryByRole('link', { name: 'Funding history' }) !== null).toBe(state === 'retirement')
})
