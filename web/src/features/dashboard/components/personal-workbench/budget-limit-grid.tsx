import {
  Activity02Icon,
  InformationCircleIcon,
  LinkSquare02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import type { TokenBudgetView } from '@/features/keys/token-budget-api'
import type { PlaygroundKey } from '@/features/playground/types'
import { toIntlLocale } from '@/i18n/languages'

function LimitTile(props: {
  icon: ReactNode
  title: string
  value: string
  unit?: string
  description: string
  details?: { label: string; value: string }[]
}) {
  return (
    <div className='bg-muted/30 min-w-0 rounded-xl border p-3.5'>
      <div className='flex items-start justify-between gap-3'>
        <div className='min-w-0'>
          <p className='text-muted-foreground text-xs font-medium'>
            {props.title}
          </p>
          <p className='mt-1 min-w-0 break-words text-xl font-semibold tabular-nums'>
            {props.value}
            {props.unit ? (
              <span className='text-muted-foreground ml-1 text-xs font-normal'>
                {props.unit}
              </span>
            ) : null}
          </p>
        </div>
        <span className='bg-background text-primary rounded-lg border p-2'>
          {props.icon}
        </span>
      </div>
      {props.details?.length ? (
        <dl className='mt-3 grid grid-cols-2 gap-2 border-t pt-3 text-xs'>
          {props.details.map((detail) => (
            <div key={detail.label} className='min-w-0'>
              <dt className='text-muted-foreground'>{detail.label}</dt>
              <dd className='mt-0.5 break-all font-medium tabular-nums'>
                {detail.value}
              </dd>
            </div>
          ))}
        </dl>
      ) : null}
      <p className='text-muted-foreground mt-3 text-xs leading-relaxed'>
        {props.description}
      </p>
    </div>
  )
}

export function BudgetLimitGrid(props: {
  selectedKey: PlaygroundKey
  budget: TokenBudgetView | undefined
  loading: boolean
}) {
  const { i18n, t } = useTranslation()
  const locale = toIntlLocale(i18n.language)
  const usdUnit = t('USD')
  const policy = props.budget?.policy
  let internalValue = t('Unknown')
  if (props.selectedKey.unlimited_quota) {
    internalValue = t('Unlimited internal quota')
  } else if (Number.isSafeInteger(props.selectedKey.remain_quota)) {
    internalValue = new Intl.NumberFormat(locale).format(props.selectedKey.remain_quota)
  }

  let tokenValue = t('Unknown')
  let tokenUnit: string | undefined
  let tokenDetails: { label: string; value: string }[] | undefined
  let feeValue = t('Unknown')
  let feeDetails: { label: string; value: string }[] | undefined
  let thresholdValue = t('Unknown')
  let thresholdDetails: { label: string; value: string }[] | undefined
  if (policy) {
    tokenValue = t('Not configured')
    if (policy.enabled) {
      tokenValue = new Intl.NumberFormat(locale).format(policy.limit)
      tokenUnit = t('Token')
      tokenDetails = [
        { label: t('Used'), value: policy.used.toLocaleString(locale) },
        { label: t('Reserved'), value: policy.reserved.toLocaleString(locale) },
      ]
    }
    feeValue = t('Not configured')
    if (policy.fee_enabled) {
      feeValue = `${usdUnit} ${policy.fee_limit_usd}`
      feeDetails = [
        {
          label: t('Used'),
          value: `${usdUnit} ${policy.fee_used_usd}`,
        },
        {
          label: t('Reserved'),
          value: `${usdUnit} ${policy.fee_reserved_usd}`,
        },
      ]
    }
    thresholdValue = t('Not configured')
    if (policy.account_threshold_enabled) {
      thresholdValue = `${(policy.account_min_remaining_bps / 100).toFixed(2)}%`
      thresholdDetails = [
        { label: t('Minimum remaining'), value: thresholdValue },
        {
          label: t('Maximum snapshot age'),
          value: t('{{count}} seconds', {
            count: policy.account_max_age_seconds,
          }),
        },
      ]
    }
  }

  return (
    <div className='grid min-w-0 gap-3 sm:grid-cols-2'>
      <LimitTile
        icon={
          <HugeiconsIcon
            icon={Activity02Icon}
            strokeWidth={2}
            className='size-4'
            aria-hidden='true'
          />
        }
        title={t('Internal Key quota')}
        value={internalValue}
        unit={props.selectedKey.unlimited_quota ? undefined : t('Internal units')}
        description={t('Ordinary Key quota uses internal units.')}
      />
      {props.loading ? (
        <>
          <Skeleton className='h-40 rounded-xl' />
          <Skeleton className='h-40 rounded-xl' />
          <Skeleton className='h-40 rounded-xl' />
        </>
      ) : (
        <>
          <LimitTile
            icon={
              <HugeiconsIcon
                icon={InformationCircleIcon}
                strokeWidth={2}
                className='size-4'
                aria-hidden='true'
              />
            }
            title={t('Strict Token budget')}
            value={tokenValue}
            unit={tokenUnit}
            details={tokenDetails}
            description={t('Strict Token budget uses input plus output Token.')}
          />
          <LimitTile
            icon={
              <HugeiconsIcon
                icon={Activity02Icon}
                strokeWidth={2}
                className='size-4'
                aria-hidden='true'
              />
            }
            title={t('Strict USD budget')}
            value={feeValue}
            details={feeDetails}
            description={t('Strict USD budget uses exact USD amounts.')}
          />
          <LimitTile
            icon={
              <HugeiconsIcon
                icon={LinkSquare02Icon}
                strokeWidth={2}
                className='size-4'
                aria-hidden='true'
              />
            }
            title={t('Account window threshold')}
            value={thresholdValue}
            details={thresholdDetails}
            description={t(
              'Account threshold checks a supported upstream account window.'
            )}
          />
        </>
      )}
    </div>
  )
}
