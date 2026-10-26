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
import { useTranslation } from 'react-i18next'

import { StatTile } from '@/components/stat-tile'
import { formatQuota } from '@/lib/format'

import type { UserWalletData } from '../types'

interface WalletStatsCardProps {
  user: UserWalletData | null
  loading?: boolean
  showBalance?: boolean
}

/**
 * The wallet's figures as stat tiles. An account that could not be read
 * shows "Unknown", never 0; without commercial funding the balance is left
 * out and the recorded usage stays.
 */
export function WalletStatsCard(props: WalletStatsCardProps) {
  const { t } = useTranslation()
  const user = props.user
  const unknown = t('Unknown')
  const showBalance = props.showBalance !== false

  return (
    <dl
      className={
        showBalance
          ? 'grid min-w-0 grid-cols-1 gap-2 min-[360px]:grid-cols-2 sm:gap-3 lg:grid-cols-3'
          : 'grid min-w-0 grid-cols-1 gap-2 min-[360px]:grid-cols-2 sm:gap-3'
      }
    >
      {showBalance && (
        <StatTile
          label={t('Current Balance')}
          value={user ? formatQuota(user.quota) : unknown}
          detail={t('Remaining quota')}
          loading={props.loading}
        />
      )}
      <StatTile
        label={t('Total Usage')}
        value={user ? formatQuota(user.used_quota) : unknown}
        detail={t('Total consumed quota')}
        loading={props.loading}
      />
      <StatTile
        label={t('API Requests')}
        value={user ? user.request_count.toLocaleString() : unknown}
        detail={t('Total requests made')}
        loading={props.loading}
      />
    </dl>
  )
}
