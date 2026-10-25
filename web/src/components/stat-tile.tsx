/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { ReactNode } from 'react'

import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

/**
 * One figure of an object page (design v2 7.2 `StatInline`): a label, the
 * value and one line of detail. Place it in a `<dl>`.
 */
export function StatTile(props: {
  label: string
  value: ReactNode
  detail?: ReactNode
  loading?: boolean
  danger?: boolean
}) {
  return (
    <div className='bg-card flex min-w-0 flex-col gap-0.5 rounded-lg border px-2.5 py-3 sm:px-4'>
      <dt className='text-caption text-muted-foreground wrap-anywhere'>
        {props.label}
      </dt>
      <dd
        className={cn(
          'text-lg leading-7 font-semibold tabular-nums wrap-anywhere sm:text-2xl sm:leading-8',
          props.danger && 'text-destructive'
        )}
      >
        {props.loading ? <Skeleton className='my-1 h-6 w-16' /> : props.value}
      </dd>
      {props.detail != null && (
        <dd className='text-caption text-muted-foreground wrap-anywhere'>
          {props.detail}
        </dd>
      )}
    </div>
  )
}
