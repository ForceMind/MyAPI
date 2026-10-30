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
import {
  ArrowDown01Icon,
  FilterHorizontalIcon,
  Loading03Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { Table } from '@tanstack/react-table'
import { useState, type ComponentProps, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTableViewOptions } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from '@/components/ui/drawer'
import { Input } from '@/components/ui/input'
import { useMediaQuery } from '@/hooks'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { cn } from '@/lib/utils'

interface LogsFilterToolbarProps<TData> {
  table: Table<TData>
  primaryFilters: ReactNode
  advancedFilters?: ReactNode
  /**
   * Keep the date range and frequent filters directly reachable on a phone.
   * Less frequent filters open in the bottom sheet.
   */
  mobilePinnedFilters?: ReactNode
  mobileFilters?: ReactNode
  mobileFilterCount?: number
  /** Shown below the filters, on a phone as well. */
  stats?: ReactNode
  actionStart?: ReactNode
  hasActiveFilters: boolean
  hasAdvancedActiveFilters?: boolean
  advancedFilterCount?: number
  searchLoading?: boolean
  onReset: () => void
  onSearch: () => void
  className?: string
}

interface LogsFilterFieldProps {
  children: ReactNode
  wide?: boolean
  className?: string
}

export function LogsFilterField(props: LogsFilterFieldProps) {
  return (
    <div
      className={cn(
        'min-w-0 [&_[data-slot=select-trigger]]:min-h-11 [&_[data-slot=select-trigger]]:w-full [&_[data-slot=select-trigger]]:text-sm [&_[data-slot=select-value]]:leading-5',
        props.wide && 'sm:col-span-2',
        props.className
      )}
    >
      {props.children}
    </div>
  )
}

export function LogsFilterInput(props: ComponentProps<typeof Input>) {
  return (
    <Input
      {...props}
      className={cn('min-h-11 min-w-0 text-sm leading-5', props.className)}
    />
  )
}

export function LogsFilterToolbar<TData>(props: LogsFilterToolbarProps<TData>) {
  const { t } = useTranslation()
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [mobileFiltersOpen, setMobileFiltersOpen] = useState(false)
  const isMobile = useMediaQuery(CONSOLE_MOBILE_QUERY)

  const hasAdvancedFilters = props.advancedFilters != null
  const activeAdvancedCount =
    props.advancedFilterCount ?? (props.hasAdvancedActiveFilters ? 1 : 0)
  const activeMobileFilterCount = props.mobileFilterCount ?? activeAdvancedCount
  const searchIcon = props.searchLoading ? (
    <HugeiconsIcon
      icon={Loading03Icon}
      className='animate-spin'
      aria-hidden='true'
    />
  ) : null

  const handleMobileReset = () => {
    props.onReset()
    setMobileFiltersOpen(false)
  }

  const handleMobileSearch = () => {
    props.onSearch()
    setMobileFiltersOpen(false)
  }

  if (isMobile && props.mobilePinnedFilters != null) {
    return (
      <Drawer open={mobileFiltersOpen} onOpenChange={setMobileFiltersOpen}>
        <div
          data-testid='logs-filter-summary'
          className={cn(
            'bg-card min-w-0 space-y-2 rounded-xl border p-3',
            props.className
          )}
        >
          <div className='grid min-w-0 grid-cols-1 gap-2 min-[360px]:grid-cols-2 [&>*:first-child]:col-span-full'>
            {props.mobilePinnedFilters}
          </div>
          <div className='flex flex-wrap items-center justify-end gap-2'>
            {props.actionStart}
            {hasAdvancedFilters && (
              <DrawerTrigger asChild>
                <Button
                  type='button'
                  variant='outline'
                  className={cn(
                    'min-h-11 shrink-0 gap-1.5',
                    activeMobileFilterCount > 0 &&
                      'border-primary/50 text-primary'
                  )}
                >
                  <HugeiconsIcon
                    icon={FilterHorizontalIcon}
                    aria-hidden='true'
                  />
                  {t('Filter')}
                  {activeMobileFilterCount > 0 && (
                    <Badge className='size-5 justify-center p-0 text-[10px]'>
                      {activeMobileFilterCount}
                    </Badge>
                  )}
                </Button>
              </DrawerTrigger>
            )}
            <Button
              type='button'
              className='min-h-11'
              onClick={props.onReset}
              variant='outline'
              disabled={!props.hasActiveFilters}
            >
              {t('Reset')}
            </Button>
            <Button
              type='button'
              className='min-h-11'
              onClick={handleMobileSearch}
              disabled={props.searchLoading}
            >
              {searchIcon}
              {t('Search')}
            </Button>
          </div>
        </div>
        {props.stats}

        <DrawerContent className='max-h-[85dvh] p-0'>
          <div className='mx-auto flex w-full max-w-md flex-1 flex-col overflow-hidden'>
            <DrawerHeader className='border-border/70 border-b px-4 py-3 text-left'>
              <DrawerTitle>{t('Filter')}</DrawerTitle>
              <DrawerDescription>
                {t('Adjust filters, then search to refresh the logs.')}
              </DrawerDescription>
            </DrawerHeader>
            <div className='flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-4 py-3'>
              {props.mobileFilters ?? (
                <>
                  {props.primaryFilters}
                  {props.advancedFilters}
                </>
              )}
            </div>
            <DrawerFooter className='border-border/70 grid grid-cols-2 gap-2 border-t px-4 py-3'>
              <Button
                type='button'
                variant='outline'
                className='min-h-11'
                onClick={handleMobileReset}
                disabled={!props.hasActiveFilters}
              >
                {t('Reset')}
              </Button>
              <Button
                type='button'
                className='min-h-11'
                onClick={handleMobileSearch}
                disabled={props.searchLoading}
              >
                {searchIcon}
                {t('Search')}
              </Button>
            </DrawerFooter>
          </div>
        </DrawerContent>
      </Drawer>
    )
  }

  return (
    <>
      <div className={cn('bg-card rounded-xl border p-3', props.className)}>
        <div className='flex flex-wrap items-start gap-2'>
          <div className='grid min-w-0 flex-1 grid-cols-1 gap-2 sm:grid-cols-[repeat(auto-fit,minmax(10rem,1fr))]'>
            {props.primaryFilters}
          </div>
          {hasAdvancedFilters && (
            <div className='flex shrink-0 items-center justify-end'>
              <Button
                type='button'
                variant='ghost'
                onClick={() => setAdvancedOpen((open) => !open)}
                aria-expanded={advancedOpen}
                className={cn(
                  'text-muted-foreground hover:text-foreground gap-1 px-2',
                  props.hasAdvancedActiveFilters &&
                    !advancedOpen &&
                    'text-primary hover:text-primary'
                )}
              >
                {advancedOpen ? t('Collapse') : t('Expand')}
                {activeAdvancedCount > 0 && (
                  <Badge className='ml-0.5 size-5 justify-center p-0 text-[10px]'>
                    {activeAdvancedCount}
                  </Badge>
                )}
                <HugeiconsIcon
                  icon={ArrowDown01Icon}
                  aria-hidden='true'
                  className={cn(
                    'size-3.5 transition-transform duration-200',
                    advancedOpen && 'rotate-180'
                  )}
                />
              </Button>
            </div>
          )}
        </div>

        {advancedOpen && props.advancedFilters && (
          <div className='mt-2 grid grid-cols-1 gap-2 sm:grid-cols-[repeat(auto-fit,minmax(10rem,1fr))]'>
            {props.advancedFilters}
          </div>
        )}

        <div className='mt-2 flex flex-wrap items-center justify-end gap-1.5 sm:gap-2'>
          {props.actionStart}
          <Button
            type='button'
            variant='outline'
            onClick={props.onReset}
            disabled={!props.hasActiveFilters}
          >
            {t('Reset')}
          </Button>
          <Button
            type='button'
            onClick={props.onSearch}
            disabled={props.searchLoading}
          >
            {searchIcon}
            {t('Search')}
          </Button>
          <DataTableViewOptions table={props.table} />
        </div>
      </div>
      {props.stats}
    </>
  )
}
