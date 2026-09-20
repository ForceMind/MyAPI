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

/** Build-time switch used by the private, single-operator deployment. */
export const SELF_USE_MINIMAL = import.meta.env.VITE_SELF_USE_MINIMAL === 'true'

export type UserFundingMode = 'enabled' | 'retirement' | 'disabled'
export const USER_FUNDING_UNAVAILABLE_MESSAGE_KEY =
  'User funding is unavailable'

export interface UserFundingCapabilities {
  mode: UserFundingMode
  epoch: number
  ready: boolean
  can_top_up: boolean
  can_redeem: boolean
  can_transfer_affiliate_rewards: boolean
  can_purchase_subscription: boolean
  can_view_funding_history: boolean
}

interface UserFundingCapabilitySource {
  data?: UserFundingCapabilitySource
  user_funding_mode?: UserFundingMode
  user_funding_capabilities?: Partial<UserFundingCapabilities>
}

interface UserFundingErrorLike {
  code?: string
  response?: {
    data?: {
      code?: string
    }
  }
}

function getFundingCapabilityData(
  source: UserFundingCapabilitySource | null | undefined
): UserFundingCapabilitySource | null {
  if (!source) return null
  return source.data ?? source
}

export function resolveUserFundingCapabilities(
  ...sources: (UserFundingCapabilitySource | null | undefined)[]
): UserFundingCapabilities {
  const source = sources
    .map(getFundingCapabilityData)
    .find((item) => item?.user_funding_capabilities || item?.user_funding_mode)
  const explicit = source?.user_funding_capabilities
  const mode = SELF_USE_MINIMAL
    ? 'disabled'
    : explicit?.mode || source?.user_funding_mode || 'disabled'
  const ready = explicit?.ready === true
  const enabled = ready && mode === 'enabled'

  return {
    mode,
    epoch: explicit?.epoch ?? 0,
    ready,
    can_top_up: enabled && explicit?.can_top_up === true,
    can_redeem: enabled && explicit?.can_redeem === true,
    can_transfer_affiliate_rewards:
      enabled && explicit?.can_transfer_affiliate_rewards === true,
    can_purchase_subscription:
      enabled && explicit?.can_purchase_subscription === true,
    // History is a read-only exception: it remains observable even when the
    // server has not made funding mutations available.
    can_view_funding_history: explicit?.can_view_funding_history ?? true,
  }
}

export function areUserFundingCapabilitiesReady(
  source: UserFundingCapabilitySource | null | undefined,
  loading: boolean
): boolean {
  return !loading && resolveUserFundingCapabilities(source).ready
}

export function isUserFundingUnavailableError(error: unknown): boolean {
  const candidate = error as UserFundingErrorLike | null
  return (
    candidate?.code === 'user_funding_unavailable' ||
    candidate?.response?.data?.code === 'user_funding_unavailable'
  )
}
