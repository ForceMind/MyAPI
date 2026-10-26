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

import { CopyButton } from '@/components/copy-button'
import { StatTile } from '@/components/stat-tile'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { formatQuota } from '@/lib/format'

import type { UserWalletData } from '../types'

interface AffiliateRewardsCardProps {
  user: UserWalletData | null
  affiliateLink: string
  onTransfer: () => void
  canTransfer?: boolean
  complianceConfirmed?: boolean
  loading?: boolean
  historyOnly?: boolean
}

export function AffiliateRewardsCard({
  user,
  affiliateLink,
  onTransfer,
  canTransfer = true,
  complianceConfirmed = true,
  loading,
  historyOnly = false,
}: AffiliateRewardsCardProps) {
  const { t } = useTranslation()
  if (loading) {
    return (
      <Card data-card-hover='false'>
        <CardHeader>
          <Skeleton className='h-5 w-32' />
          <Skeleton className='h-4 w-48' />
        </CardHeader>
        <CardContent className='flex flex-col gap-3'>
          <Skeleton className='h-16 w-full rounded-lg' />
          <Skeleton className='h-10 w-full rounded-lg' />
        </CardContent>
      </Card>
    )
  }

  const unknown = t('Unknown')
  const hasRewards = (user?.aff_quota ?? 0) > 0

  return (
    <Card data-card-hover='false'>
      <CardHeader>
        <CardTitle>
          <h2>{historyOnly ? t('Funding history') : t('Referral Program')}</h2>
        </CardTitle>
        <CardDescription>
          {historyOnly
            ? t('History and recovery')
            : t(
                'Earn rewards when users join through your referral link. Transfer accumulated rewards to your balance anytime.'
              )}
        </CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-3'>
        {/* An account that could not be read shows Unknown, never 0. */}
        <dl className='grid min-w-0 grid-cols-1 gap-2 min-[360px]:grid-cols-3 sm:gap-3'>
          <StatTile
            label={t('Pending')}
            value={user ? formatQuota(user.aff_quota) : unknown}
          />
          <StatTile
            label={t('Total Earned')}
            value={user ? formatQuota(user.aff_history_quota) : unknown}
          />
          <StatTile
            label={t('Invites')}
            value={user ? String(user.aff_count) : unknown}
          />
        </dl>

        {!historyOnly && (
          <div className='flex min-w-0 flex-wrap items-center gap-2'>
            <Input
              value={affiliateLink}
              readOnly
              aria-label={t('Referral link')}
              className='min-w-0 flex-1 basis-48 font-mono text-xs'
            />
            <CopyButton
              value={affiliateLink}
              variant='outline'
              className='size-9'
              iconClassName='size-4'
              tooltip={t('Copy referral link')}
              aria-label={t('Copy referral link')}
            />
            {hasRewards && canTransfer ? (
              <Button
                type='button'
                onClick={onTransfer}
                disabled={!complianceConfirmed}
              >
                {t('Transfer to Balance')}
              </Button>
            ) : null}
          </div>
        )}
        {canTransfer && !complianceConfirmed ? (
          <p className='text-muted-foreground text-xs'>
            {t(
              'Referral reward transfer is disabled until the administrator confirms compliance terms.'
            )}
          </p>
        ) : null}
      </CardContent>
    </Card>
  )
}
