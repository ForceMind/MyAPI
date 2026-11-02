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
import { Search01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

export interface EmptyStateProps {
  emptyCatalog?: boolean
  searchQuery?: string
  hasActiveFilters: boolean
  onClearFilters: () => void
}

export function EmptyState(props: EmptyStateProps) {
  const { t } = useTranslation()
  const hasSearch = Boolean(props.searchQuery?.trim())
  let description = t('No models match your current filters.')
  if (props.emptyCatalog) description = t('The catalog returned no enabled models.')
  if (hasSearch) description = t('No results for "{{query}}". Try adjusting your search or filters.', { query: props.searchQuery })

  return (
    <div className='flex min-h-[320px] flex-col items-center justify-center rounded-lg border border-dashed px-6 py-12 text-center'>
      <HugeiconsIcon icon={Search01Icon} aria-hidden='true' className='text-muted-foreground/40 mb-3 size-10' />

      <h3 className='text-foreground mb-1 text-base font-semibold'>
        {props.emptyCatalog ? t('No models are available in this catalog.') : t('No models found')}
      </h3>

      <p className='text-muted-foreground mb-5 max-w-xs text-sm'>
        {description}
      </p>

      {(props.hasActiveFilters || hasSearch) && (
        <Button variant='outline' size='sm' onClick={props.onClearFilters}>
          {t('Clear all filters')}
        </Button>
      )}
    </div>
  )
}
