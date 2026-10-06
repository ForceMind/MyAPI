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
import type { TFunction } from 'i18next'

/**
 * User `group` is the legacy persisted account-tier field. Keep its value
 * intact for compatibility while giving administrators an explicit product
 * meaning in the UI.
 */
export function getAccountTierLabel(
  group: string | undefined,
  t: TFunction
): string {
  switch ((group || 'default').trim().toLowerCase()) {
    case 'default':
      return t('Standard account')
    case 'vip':
      return t('Priority account')
    default:
      return group || t('Standard account')
  }
}

export function getAccountTierDescription(
  group: string | undefined,
  t: TFunction
): string {
  switch ((group || 'default').trim().toLowerCase()) {
    case 'default':
      return t(
        'Controls this user account’s channel eligibility and billing tier.'
      )
    case 'vip':
      return t(
        'Priority account tier with the channel eligibility and billing rules configured for vip.'
      )
    default:
      return t(
        'Custom account tier; its channel eligibility and billing rules come from administrator settings.'
      )
  }
}

/** Resolve the stable account-tier identity used by the API payload. */
export function getAccountTierId(
  group: string | undefined,
  explicitId?: string
): string {
  const persisted = explicitId?.trim()
  if (persisted) return persisted
  switch ((group || 'default').trim().toLowerCase()) {
    case 'default':
      return 'standard'
    case 'vip':
      return 'priority'
    default:
      return group?.trim() || 'standard'
  }
}
