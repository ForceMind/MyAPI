import type { SystemStatus } from '@/features/auth/types'
import type { UserFundingMode } from '@/lib/self-use-build'

import type { UserUsagePolicy } from '../user-usage-policy-api'

// Presentation of two independently fetched snapshots, never request admission.
// Read the server fields directly: the minimal-build display override is not
// evidence that the server has disabled commercial funding.
export function resolveUsagePolicyPresentation(input: {
  userId: number
  policy: UserUsagePolicy | undefined
  policyConfirmed: boolean
  status: SystemStatus | null
  statusConfirmed: boolean
}): {
  mode: UserFundingMode | null
  savedNoBalance: boolean | null
  condition: 'unknown' | 'wallet' | 'inactive' | 'supported'
} {
  const policy = input.policy
  const policyValid =
    input.policyConfirmed &&
    !!policy &&
    Number.isSafeInteger(input.userId) &&
    input.userId > 0 &&
    typeof policy.no_balance === 'boolean' &&
    policy.user_id === input.userId &&
    Number.isSafeInteger(policy.revision) &&
    policy.revision >= 0 &&
    (!policy.no_balance || policy.revision > 0)
  const savedNoBalance = policyValid ? policy.no_balance : null
  const capabilities = input.status?.user_funding_capabilities
  const mode = capabilities?.mode
  const fundingValid =
    input.statusConfirmed &&
    capabilities?.ready === true &&
    Number.isSafeInteger(capabilities.epoch) &&
    capabilities.epoch > 0 &&
    (mode === 'enabled' || mode === 'retirement' || mode === 'disabled') &&
    (input.status?.user_funding_mode === undefined ||
      input.status.user_funding_mode === mode)
  if (!fundingValid || savedNoBalance === null) {
    return {
      mode: fundingValid ? mode : null,
      savedNoBalance,
      condition: 'unknown',
    }
  }
  if (!savedNoBalance) return { mode, savedNoBalance, condition: 'wallet' }
  return {
    mode,
    savedNoBalance,
    condition: mode === 'disabled' ? 'supported' : 'inactive',
  }
}
