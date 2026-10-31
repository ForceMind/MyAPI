/* Copyright (C) 2026 ForceMind. AGPL-3.0-or-later. */
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import { connectionsToTableSearch, tableToConnectionsSearch, type ConnectionReturn } from './connection-state'

export function ConnectionViewSwitcher(props: {
  view: 'cards' | 'table'
  search: ConnectionReturn
  showOther: boolean
}) {
  const { t } = useTranslation()
  if (!props.showOther) return null
  const activeClass = cn(buttonVariants({ variant: 'secondary', size: 'sm' }), 'h-auto min-h-8')
  const linkClass = cn(buttonVariants({ variant: 'ghost', size: 'sm' }), 'h-auto min-h-8')
  const tableSearch = connectionsToTableSearch(props.search)
  return (
    <nav aria-label={t('Connection views')} className='flex min-w-0 flex-wrap gap-1'>
      {props.view === 'cards' ? (
        <span aria-current='page' className={activeClass}>{t('Card view')}</span>
      ) : (
        <Link to='/connections' search={tableToConnectionsSearch(tableSearch)} data-myapi-button='' className={linkClass}>{t('Card view')}</Link>
      )}
      {props.view === 'table' ? (
        <span aria-current='page' className={activeClass}>{t('Table view')}</span>
      ) : (
        <Link to='/channels' search={tableSearch} data-myapi-button='' className={linkClass}>{t('Table view')}</Link>
      )}
    </nav>
  )
}
