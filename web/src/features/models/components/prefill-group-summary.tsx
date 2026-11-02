import { Delete02Icon, Edit02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'

import type { PrefillGroup } from '../types'
import { PREFILL_GROUP_TYPE_META, parseEndpointKeys, parseStringItems } from './prefill-group-shared'

export function PrefillGroupSummary(props: { group: PrefillGroup; section?: 'identity' | 'items' }) {
  const { t } = useTranslation()
  const meta = PREFILL_GROUP_TYPE_META[props.group.type]
  const items = props.group.type === 'endpoint' ? parseEndpointKeys(props.group.items) : parseStringItems(props.group.items)
  const occurrences = new Map<string, number>()
  const itemEntries = items.map((value) => {
    const occurrence = (occurrences.get(value) ?? 0) + 1
    occurrences.set(value, occurrence)
    return { value, key: JSON.stringify([value, occurrence]) }
  })
  return (
    <div className='min-w-0 space-y-3 break-words [overflow-wrap:anywhere]'>
      {props.section !== 'items' && (
        <div className='space-y-1.5'>
          <div className='flex flex-wrap items-center gap-2'>
            <h3 className='min-w-0 font-medium'>{props.group.name}</h3>
            <span className='text-muted-foreground font-mono text-xs'>#{props.group.id}</span>
            <StatusBadge variant={meta.badge} size='sm' copyable={false}>{t(meta.label)}</StatusBadge>
          </div>
          <p className='text-muted-foreground whitespace-pre-wrap text-sm'>{props.group.description || t('No description provided')}</p>
        </div>
      )}
      {props.section !== 'identity' && (
        <div className='space-y-2'>
          <p className='text-muted-foreground text-xs'>{t('Items')}: {items.length}</p>
          {items.length === 0 ? <p className='text-muted-foreground text-sm'>{props.group.type === 'endpoint' ? t('No endpoint mappings configured.') : t('No items configured yet.')}</p> : (
            <ul className='flex flex-wrap gap-1.5'>
              {itemEntries.slice(0, 4).map((item) => <li key={item.key} className='bg-muted max-w-full rounded-md px-2 py-1 text-xs'>{item.value}</li>)}
            </ul>
          )}
          {(items.length > 4 || props.group.type === 'endpoint') && (
            <details className='min-w-0 text-sm'>
              <summary className='text-primary flex min-h-11 cursor-pointer items-center underline-offset-4 hover:underline'>{t('View all items')}</summary>
              {props.group.type === 'endpoint' ? (
                <pre className='bg-muted max-w-full whitespace-pre-wrap rounded-md p-3 font-mono text-xs break-words [overflow-wrap:anywhere]'>{typeof props.group.items === 'string' ? props.group.items : JSON.stringify(props.group.items, null, 2)}</pre>
              ) : <ul className='list-inside list-disc space-y-1'>{itemEntries.map((item) => <li key={item.key}>{item.value}</li>)}</ul>}
            </details>
          )}
        </div>
      )}
    </div>
  )
}

export function PrefillGroupActions(props: { group: PrefillGroup; onEdit: (group: PrefillGroup) => void; onDelete: (group: PrefillGroup) => void }) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-wrap items-center gap-1'>
      <Button className='min-h-11 min-w-11' variant='outline' data-prefill-edit={props.group.id} aria-label={t('Edit group "{{name}}"', { name: props.group.name })} onClick={() => props.onEdit(props.group)}><HugeiconsIcon icon={Edit02Icon} aria-hidden='true' className='size-4' />{t('Edit')}</Button>
      <Button className='text-destructive min-h-11 min-w-11' variant='ghost' aria-label={t('Delete group "{{name}}"', { name: props.group.name })} onClick={() => props.onDelete(props.group)}><HugeiconsIcon icon={Delete02Icon} aria-hidden='true' className='size-4' />{t('Delete')}</Button>
    </div>
  )
}
