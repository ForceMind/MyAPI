/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { getSelfSubscriptionFull } from '@/features/subscriptions/api'
import {
  userSubscriptionSchema,
  type UserSubscription,
} from '@/features/subscriptions/types'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { linkButtonClassName } from '@/lib/link-button'
import { useAuthStore } from '@/stores/auth-store'

function SubscriptionState(props: { subscription: UserSubscription }) {
  const { t } = useTranslation()
  const now = Date.now() / 1000
  let variant: StatusVariant = 'neutral'
  let label = t('Expired')
  if (props.subscription.status === 'cancelled') {
    label = t('Cancelled')
  } else if (
    props.subscription.status === 'active' &&
    props.subscription.end_time >= now
  ) {
    variant = 'success'
    label = t('Active')
  }
  return <StatusBadge variant={variant} label={label} copyable={false} />
}

/**
 * The signed-in user's own subscriptions, read only (`GET
 * /api/subscription/self`, open in every edition because quota resets run
 * in every edition). Shown only when there is at least one; it offers no
 * purchase, renewal or preference control. Where the edition sells
 * subscriptions, it links to the wallet that manages them. A field the
 * server did not send is "Not provided".
 */
export function MySubscriptionCard(props: { canManage: boolean }) {
  const { t } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const showWallet = useIsSidebarModuleVisible('/wallet')
  const query = useQuery({
    queryKey: ['profile', 'subscriptions', userId, sessionId],
    queryFn: async ({ signal }) => {
      const response = await getSelfSubscriptionFull({
        signal,
        skipErrorHandler: true,
        skipBusinessError: true,
      })
      if (!response.success || !response.data) {
        throw new Error('Subscriptions unavailable')
      }
      // Records that do not match the subscription contract are not shown.
      return (response.data.all_subscriptions ?? []).flatMap((record) => {
        const parsed = userSubscriptionSchema.safeParse(record?.subscription)
        return parsed.success && parsed.data.user_id === userId
          ? [parsed.data]
          : []
      })
    },
    enabled: userId !== undefined,
    retry: false,
    refetchOnWindowFocus: false,
  })
  const subscriptions = query.isError ? [] : (query.data ?? [])
  if (subscriptions.length === 0) return null

  const notProvided = t('Not provided')
  return (
    <Card data-testid='my-subscription-card'>
      <CardHeader>
        <CardTitle>
          <h2>{t('My subscription')}</h2>
        </CardTitle>
        <CardDescription>
          {t('A read-only view of your subscriptions.')}
        </CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-3'>
        <ul className='flex flex-col gap-3'>
          {subscriptions.map((subscription) => (
            <li
              key={subscription.id}
              className='bg-background flex min-w-0 flex-col gap-2 rounded-lg border p-3'
            >
              <div className='flex min-w-0 flex-wrap items-center gap-2'>
                <span className='text-body font-medium'>
                  {t('Plan #{{id}}', { id: subscription.plan_id })}
                </span>
                <SubscriptionState subscription={subscription} />
              </div>
              <dl className='text-caption grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1'>
                <dt className='text-muted-foreground'>{t('Plan name')}</dt>
                <dd className='wrap-anywhere'>{notProvided}</dd>
                <dt className='text-muted-foreground'>{t('Quota used')}</dt>
                <dd className='wrap-anywhere tabular-nums'>
                  {subscription.amount_total > 0
                    ? t('{{used}} of {{total}}', {
                        used: formatQuota(subscription.amount_used),
                        total: formatQuota(subscription.amount_total),
                      })
                    : t('Unlimited')}
                </dd>
                <dt className='text-muted-foreground'>{t('Next reset')}</dt>
                <dd className='tabular-nums'>
                  {subscription.next_reset_time &&
                  subscription.next_reset_time > 0
                    ? formatTimestampToDate(subscription.next_reset_time)
                    : notProvided}
                </dd>
                <dt className='text-muted-foreground'>{t('Valid until')}</dt>
                <dd className='tabular-nums'>
                  {subscription.end_time > 0
                    ? formatTimestampToDate(subscription.end_time)
                    : notProvided}
                </dd>
              </dl>
            </li>
          ))}
        </ul>
        {props.canManage && showWallet && (
          <Link
            to='/wallet'
            data-myapi-button=''
            className={linkButtonClassName({ variant: 'outline' })}
          >
            {t('Manage in wallet')}
          </Link>
        )}
      </CardContent>
    </Card>
  )
}
