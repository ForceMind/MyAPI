import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { DEFAULT_TOKEN_UNIT } from '../constants'
import { getDynamicDisplayGroupRatio, getDynamicPricingSummary } from '../lib/dynamic-price'
import { hasRequestPrice, isTokenBasedModel } from '../lib/model-helpers'
import { formatPrice, formatRequestPrice } from '../lib/price'
import type { PricingModel, TokenUnit } from '../types'

export function ModelCatalogPrice(props: { model: PricingModel; tokenUnit?: TokenUnit; priceRate?: number; usdExchangeRate?: number; showRechargePrice?: boolean; selectedGroup?: string; includeCached?: boolean }) {
  const { t } = useTranslation()
  const tokenUnit = props.tokenUnit ?? DEFAULT_TOKEN_UNIT
  const priceRate = props.priceRate ?? 1
  const usdExchangeRate = props.usdExchangeRate ?? 1
  const showRechargePrice = props.showRechargePrice ?? false
  const isTokenBased = isTokenBasedModel(props.model)
  const tokenUnitLabel = tokenUnit === 'K' ? '1K' : '1M'
  const isDynamicPricing =
    props.model.billing_mode === 'tiered_expr' &&
    Boolean(props.model.billing_expr)
  const hasCachedPrice = props.includeCached !== false && isTokenBased && props.model.cache_ratio != null
  const dynamicSummary = isDynamicPricing
    ? getDynamicPricingSummary(props.model, {
        tokenUnit,
        showRechargePrice,
        priceRate,
        usdExchangeRate,
        groupRatioMultiplier: getDynamicDisplayGroupRatio(
          props.model,
          props.selectedGroup
        ),
      })
    : null

  let priceSummary: ReactNode
  if (dynamicSummary) {
    if (dynamicSummary.isSpecialExpression) {
      priceSummary = (
        <span className='min-w-0'>
          <span className='text-amber-700 dark:text-amber-300'>
            {t('Special billing expression')}
          </span>
          <span className='block text-xs'>{t('Unable to parse structured pricing')}</span>
          <code className='text-muted-foreground mt-0.5 block font-mono text-[11px] break-all'>
            {dynamicSummary.rawExpression}
          </code>
        </span>
      )
    } else if (dynamicSummary.primaryEntries.length > 0) {
      priceSummary = (
        <>
          {dynamicSummary.primaryEntries.map((entry) => (
            <span
              key={entry.key}
              className='text-muted-foreground whitespace-nowrap'
            >
              {t(entry.shortLabel)}{' '}
              <span className='text-foreground font-mono font-semibold'>
                {entry.formatted}
              </span>
            </span>
          ))}
        </>
      )
    } else {
      priceSummary = (
        <span className='text-muted-foreground text-sm'>
          {t('Dynamic Pricing')}
        </span>
      )
    }
  } else if (isTokenBased) {
    priceSummary = (
      <>
        <span className='text-muted-foreground whitespace-nowrap'>
          {t('Input')}{' '}
          <span className='text-foreground font-mono font-semibold'>
            {formatPrice(
              props.model,
              'input',
              tokenUnit,
              showRechargePrice,
              priceRate,
              usdExchangeRate,
              props.selectedGroup
            )}
          </span>
        </span>
        <span className='text-muted-foreground whitespace-nowrap'>
          {t('Output')}{' '}
          <span className='text-foreground font-mono font-semibold'>
            {formatPrice(
              props.model,
              'output',
              tokenUnit,
              showRechargePrice,
              priceRate,
              usdExchangeRate,
              props.selectedGroup
            )}
          </span>
        </span>
        {hasCachedPrice && (
          <span className='text-muted-foreground whitespace-nowrap'>
            {t('Cached')}{' '}
            <span className='text-foreground font-mono font-semibold'>
              {formatPrice(
                props.model,
                'cache',
                tokenUnit,
                showRechargePrice,
                priceRate,
                usdExchangeRate,
                props.selectedGroup
              )}
            </span>
          </span>
        )}
      </>
    )
  } else {
    priceSummary = (
      <span className='text-muted-foreground whitespace-nowrap'>
        <span className='text-foreground font-mono font-semibold'>
          {hasRequestPrice(props.model) ? formatRequestPrice(
            props.model,
            showRechargePrice,
            priceRate,
            usdExchangeRate,
            props.selectedGroup
          ) : t('Unknown')}
        </span>{' '}
        / {t('request')}
      </span>
    )
  }

  return <div className='min-w-0'><div className='flex flex-wrap gap-x-3 gap-y-1 text-sm'>{priceSummary}</div><p className='text-muted-foreground mt-1 text-xs'>{isTokenBased ? `${t('Prices shown per')} ${tokenUnitLabel} tokens` : t('Per Request')}{dynamicSummary && dynamicSummary.tierCount > 1 && ` · ${t('{{count}} tiers', { count: dynamicSummary.tierCount })}`}</p></div>
}
