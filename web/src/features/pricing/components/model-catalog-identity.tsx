import { getLobeIcon } from '@/lib/lobe-icon'

import type { PricingModel } from '../types'

/** Shared catalog identity; full names stay readable in cards, rows and details. */
export function ModelCatalogIdentity(props: { model: PricingModel; heading?: 'h1' | 'h3' }) {
  const key = props.model.icon || props.model.vendor_icon
  const icon = key ? getLobeIcon(key, 24) : null
  const Name = props.heading ?? 'span'
  return (
    <div className='flex min-w-0 items-start gap-2.5'>
      <span aria-hidden='true' className='bg-muted/40 flex size-9 shrink-0 items-center justify-center rounded-lg'>{icon || props.model.model_name.charAt(0).toUpperCase() || '?'}</span>
      <div className='min-w-0'>
        <Name className='block font-mono font-semibold wrap-anywhere'>{props.model.model_name}</Name>
        {props.model.vendor_name && <p className='text-muted-foreground mt-0.5 text-xs wrap-anywhere'>{props.model.vendor_name}</p>}
      </div>
    </div>
  )
}
