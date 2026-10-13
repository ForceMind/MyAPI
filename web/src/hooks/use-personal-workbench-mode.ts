import type { UserFundingMode } from '@/lib/self-use-build'

import { useStatus } from './use-status'

// Select the personal interface from a confirmed server snapshot. The Lite
// build's display override does not establish the server's funding mode.
export function useConfirmedFundingMode(): UserFundingMode | null {
  const { status, confirmed } = useStatus()
  const capabilities = status?.user_funding_capabilities
  if (
    confirmed &&
    capabilities?.ready === true &&
    (capabilities.mode === 'disabled' ||
      capabilities.mode === 'retirement' ||
      capabilities.mode === 'enabled') &&
    Number.isSafeInteger(capabilities.epoch) &&
    capabilities.epoch > 0 &&
    status?.user_funding_mode === capabilities.mode
  ) {
    return capabilities.mode
  }
  return null
}

export function usePersonalWorkbenchMode(): boolean {
  return useConfirmedFundingMode() === 'disabled'
}
