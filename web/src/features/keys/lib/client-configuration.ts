import type { ApiKey } from '../types'
import { gatewayClientAddresses } from '@/lib/gateway-address'

export type KeyClientScope = Pick<ApiKey, 'model_limits_enabled' | 'model_limits'> &
  Partial<Pick<ApiKey, 'access_profile'>>

export type KeyClientAddresses = ReturnType<typeof keyClientAddresses>

/** Client import contracts use the installation root or one /v1 suffix. */
export const keyClientAddresses = gatewayClientAddresses

/** Catalog candidates are configuration hints, never evidence of a live request. */
export function keyClientModels(catalog: string[], scope?: KeyClientScope): string[] {
  const keyLimits = scope?.model_limits_enabled
    ? new Set((scope.model_limits ?? '').split(',').filter(Boolean))
    : null
  const profileLimits = scope?.access_profile?.model_allowlist
  const profileSet = profileLimits ? new Set(profileLimits) : null
  return [...new Set(catalog)].filter((model) =>
    Boolean(model.trim()) && (!keyLimits || keyLimits.has(model)) &&
    (!profileSet || profileSet.has(model))
  )
}

/** Manual IDs obey the same explicit restrictions; catalog membership is optional. */
export function keyClientModelAllowed(model: string, scope?: KeyClientScope): boolean {
  return keyClientModels([model], scope).length === 1
}

export function savedKeyClientAddresses() {
  let address: unknown
  try {
    const raw = localStorage.getItem('status')
    const status: unknown = raw ? JSON.parse(raw) : null
    if (status && typeof status === 'object' && 'server_address' in status) {
      address = status.server_address
    }
  } catch {
    // A missing or unreadable public setting falls back to the access origin.
  }
  return keyClientAddresses(address, window.location.origin)
}
