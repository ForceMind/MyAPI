/*
Copyright (C) 2023-2026 QuantumNous

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

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, test } from 'vitest'

import {
  areUserFundingCapabilitiesReady,
  isUserFundingUnavailableError,
  resolveUserFundingCapabilities,
} from '@/lib/self-use-build'

describe('user funding capabilities', () => {
  test('fails closed when the server does not return funding capabilities', () => {
    expect(resolveUserFundingCapabilities({})).toEqual({
      mode: 'disabled',
      epoch: 0,
      ready: false,
      can_top_up: false,
      can_redeem: false,
      can_transfer_affiliate_rewards: false,
      can_purchase_subscription: false,
      can_view_funding_history: true,
    })
  })

  test('fails closed when a legacy response only exposes the funding mode', () => {
    expect(
      resolveUserFundingCapabilities({
        user_funding_mode: 'retirement',
      })
    ).toEqual({
      mode: 'retirement',
      epoch: 0,
      ready: false,
      can_top_up: false,
      can_redeem: false,
      can_transfer_affiliate_rewards: false,
      can_purchase_subscription: false,
      can_view_funding_history: true,
    })
  })

  test('does not infer enabled capabilities from a partial server payload', () => {
    expect(
      resolveUserFundingCapabilities({
        user_funding_capabilities: {
          mode: 'enabled',
          epoch: 9,
          ready: true,
          can_top_up: true,
        },
      })
    ).toEqual({
      mode: 'enabled',
      epoch: 9,
      ready: true,
      can_top_up: true,
      can_redeem: false,
      can_transfer_affiliate_rewards: false,
      can_purchase_subscription: false,
      can_view_funding_history: true,
    })
  })

  test('uses the same nested capability payload returned by status and topup info', () => {
    const capabilities = resolveUserFundingCapabilities({
      data: {
        user_funding_capabilities: {
          mode: 'disabled',
          epoch: 7,
          ready: true,
          can_top_up: false,
          can_redeem: false,
          can_transfer_affiliate_rewards: false,
          can_purchase_subscription: false,
          can_view_funding_history: true,
        },
      },
    })

    expect(capabilities.mode).toBe('disabled')
    expect(capabilities.can_top_up).toBe(false)
    expect(capabilities.can_purchase_subscription).toBe(false)
    expect(capabilities.can_view_funding_history).toBe(true)
    expect(capabilities.epoch).toBe(7)
  })

  test('keeps direct wallet actions fail-closed until topup capabilities load', () => {
    expect(areUserFundingCapabilitiesReady(null, true)).toBe(false)
    expect(areUserFundingCapabilitiesReady(null, false)).toBe(false)
    expect(areUserFundingCapabilitiesReady({}, false)).toBe(false)
  })

  test('keeps mutations disabled while the server is not ready but preserves read-only funding history', () => {
    const capabilities = resolveUserFundingCapabilities({
      user_funding_capabilities: {
        mode: 'enabled',
        epoch: 8,
        ready: false,
        can_top_up: true,
        can_redeem: true,
        can_transfer_affiliate_rewards: true,
        can_purchase_subscription: true,
      },
    })

    expect(capabilities.ready).toBe(false)
    expect(capabilities.can_top_up).toBe(false)
    expect(capabilities.can_redeem).toBe(false)
    expect(capabilities.can_transfer_affiliate_rewards).toBe(false)
    expect(capabilities.can_purchase_subscription).toBe(false)
    expect(capabilities.can_view_funding_history).toBe(true)
  })

  test('recognizes the server funding error code from axios failures', () => {
    expect(
      isUserFundingUnavailableError({
        response: { data: { code: 'user_funding_unavailable' } },
      })
    ).toBe(true)
    expect(isUserFundingUnavailableError(new Error('network'))).toBe(false)
  })
})
