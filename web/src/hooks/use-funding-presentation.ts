import { resolveUserFundingCapabilities } from '@/lib/self-use-build'

import { useStatus } from './use-status'

// Stored status is a loading placeholder, not authority to reopen commerce.
// This is presentation only; server mutation/epoch gates remain authoritative.
export function useFundingPresentation() {
  const { status, confirmed } = useStatus()
  const capabilities = resolveUserFundingCapabilities(status)
  return {
    capabilities,
    ready: confirmed && capabilities.ready,
    commercialEnabled:
      confirmed && capabilities.ready && capabilities.mode === 'enabled',
  }
}
