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
import { SELF_USE_MINIMAL } from './self-use-build'

/**
 * UI capabilities advertised by `/api/status` (`middleware/edition.go`
 * `CurrentEditionProfile`). They only decide which entry points the console
 * shows; the backend edition guard remains the security boundary.
 */
export const PRODUCT_CAPABILITIES = [
  'payments',
  'commercial_subscriptions',
  'redemption_codes',
  'checkin',
  'affiliate',
  'public_pricing',
  'rankings',
  'perf_metrics',
  'external_oauth',
  'midjourney',
  'model_deployments',
  'site_content',
  'user_registration',
] as const

export type ProductCapability = (typeof PRODUCT_CAPABILITIES)[number]
export type ProductEdition = 'full' | 'lite'

export type ProductEditionProfile = {
  edition: ProductEdition
  capabilities: Record<ProductCapability, boolean>
}

type EditionStatusSource = Record<string, unknown> & {
  data?: unknown
  product_edition?: unknown
  capabilities?: unknown
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/**
 * Older backends send neither field, so a missing edition or capability keeps
 * the Full surface. The self-use build removes Full-only routes at build time
 * and therefore always presents the Lite surface.
 */
export function resolveProductEdition(
  status: EditionStatusSource | null | undefined,
  selfUseMinimal: boolean = SELF_USE_MINIMAL
): ProductEditionProfile {
  if (selfUseMinimal) {
    return {
      edition: 'lite',
      capabilities: Object.fromEntries(
        PRODUCT_CAPABILITIES.map((capability) => [capability, false])
      ) as Record<ProductCapability, boolean>,
    }
  }

  const source = isRecord(status?.data) ? status.data : status
  const reported = isRecord(source?.capabilities) ? source.capabilities : {}
  return {
    edition: source?.product_edition === 'lite' ? 'lite' : 'full',
    capabilities: Object.fromEntries(
      PRODUCT_CAPABILITIES.map((capability) => [
        capability,
        reported[capability] !== false,
      ])
    ) as Record<ProductCapability, boolean>,
  }
}
