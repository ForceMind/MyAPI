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
import { ArrowRight01Icon, Copy01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { memo } from 'react'
import { useTranslation } from 'react-i18next'

import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { cn } from '@/lib/utils'

import { parseTags } from '../lib/filters'
import { modelCatalogFocusKey } from '../catalog-focus'
import type { PricingModel, TokenUnit } from '../types'
import { ModelCatalogPrice } from './model-catalog-price'
import { ModelCatalogIdentity } from './model-catalog-identity'
import { ModelBillingModeBadge } from './model-billing-mode-badge'
import { ModelPerfBadge, type ModelPerfBadgeData } from './model-perf-badge'

export interface ModelCardProps {
  model: PricingModel
  onClick: () => void
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  selectedGroup?: string
  perf?: ModelPerfBadgeData
}

export const ModelCard = memo(function ModelCard(props: ModelCardProps) {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const tags = parseTags(props.model.tags)
  const groups = props.model.enable_groups || []
  const endpoints = props.model.supported_endpoint_types || []

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    copyToClipboard(props.model.model_name || '')
  }


  return (
    <div
      className={cn(
        'group bg-card rounded-card relative flex flex-col border p-3 transition-colors sm:p-5',
        'hover:bg-muted/20'
      )}
    >
      {/* Header: icon + name + price + actions */}
      <div className='flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
        <div className='min-w-0 flex-1'>
          <ModelCatalogIdentity model={props.model} heading='h3' />
          <div className='mt-3'><ModelCatalogPrice {...props} /></div>
        </div>

        <div className='flex shrink-0 items-center gap-1.5'>
          <button
            type='button'
            onClick={props.onClick}
            data-pricing-model={modelCatalogFocusKey(props.model.model_name)}
            aria-label={t('View model {{model}}', { model: props.model.model_name })}
            className='text-muted-foreground hover:text-foreground hover:bg-muted inline-flex min-h-11 items-center gap-1 rounded-md border px-2 py-1 text-xs transition-colors sm:px-2.5 sm:py-1.5'
          >
            {t('Details')}
            <HugeiconsIcon icon={ArrowRight01Icon} aria-hidden='true' className='size-3.5' />
          </button>
          <button
            type='button'
            onClick={handleCopy}
            className='text-muted-foreground hover:text-foreground hover:bg-muted min-h-11 min-w-11 rounded-md border p-1.5 transition-colors'
            title={t('Copy model name')}
            aria-label={t('Copy model name')}
          >
            <HugeiconsIcon icon={Copy01Icon} aria-hidden='true' className='size-3.5' />
          </button>
        </div>
      </div>

      {/* Description */}
      <p className='text-muted-foreground mt-2 flex-1 text-[13px] leading-relaxed sm:mt-4 sm:min-h-[2.5rem]'>
        {props.model.description || t('No description available.')}
      </p>

      <div className='mt-4 flex flex-wrap items-center justify-between gap-3 border-t pt-3'>
        <ModelBillingModeBadge model={props.model} />
        <ModelPerfBadge perf={props.perf} />
      </div>
      {(groups.length > 0 || endpoints.length > 0 || tags.length > 0) && <details className='mt-3 border-t pt-2'>
        <summary className='min-h-11 cursor-pointer py-3 text-sm'>{t('Groups, endpoints and tags')}</summary>
        <dl className='space-y-3 pb-2 text-xs'>
          {[[t('Groups'), groups], [t('Endpoints'), endpoints], [t('Tags'), tags]].map(([label, values]) => <div key={label as string}><dt className='text-muted-foreground'>{label}</dt><dd className='mt-1 wrap-anywhere'>{(values as string[]).join(' · ') || '—'}</dd></div>)}
        </dl>
      </details>}

    </div>
  )
})
