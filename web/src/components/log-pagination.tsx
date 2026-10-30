/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  LOG_PAGE_SIZES,
  getLogPageCount,
  type LogPageSize,
} from '@/lib/log-pagination'

export interface LogPaginationProps {
  label: string
  requestTotals?: boolean
  page: number
  pageSize: number
  total: number | undefined
  busy: boolean
  onPageChange: (page: number) => void
  onPageSizeChange: (pageSize: LogPageSize) => void
}

export function LogPagination(props: LogPaginationProps) {
  const { t } = useTranslation()
  const errorId = useId()
  const [jumpPage, setJumpPage] = useState('')
  const [jumpAttempted, setJumpAttempted] = useState(false)
  const totalPages = getLogPageCount(props.total, props.pageSize)
  const navigationDisabled = props.busy || totalPages === null
  const pageNumber = Number(jumpPage)
  const validJump =
    /^\d+$/.test(jumpPage) &&
    Number.isSafeInteger(pageNumber) &&
    pageNumber >= 1 &&
    totalPages !== null &&
    pageNumber <= totalPages
  const invalidJump = totalPages !== null && jumpAttempted && !validJump
  const firstVisiblePage = Math.max(
    1,
    Math.min(props.page - 2, (totalPages ?? 1) - 4)
  )
  const visiblePages = Array.from(
    { length: Math.min(5, totalPages ?? 0) },
    (_, index) => firstVisiblePage + index
  )

  let summary = props.requestTotals
    ? t('Request total unavailable')
    : t('Record total unavailable')
  if (props.busy) {
    summary = props.requestTotals
      ? t('Loading request total')
      : t('Loading record total')
  } else if (totalPages !== null) {
    summary = t(props.requestTotals
      ? 'Page {{page}} of {{totalPages}} · {{total}} requests'
      : 'Page {{page}} of {{totalPages}} · {{total}} records', {
      page: props.page,
      totalPages,
      total: props.total,
    })
  }

  return (
    <nav
      aria-label={props.label}
      className='flex min-w-0 flex-col gap-3 text-sm'
    >
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <span className='text-muted-foreground' role='status'>
          {summary}
        </span>
        <label className='flex items-center gap-2'>
          {t('Rows per page')}
          <NativeSelect
            aria-label={t('Rows per page')}
            className='[&_select]:min-h-11 [&_select]:min-w-11'
            value={props.pageSize}
            disabled={props.busy}
            onChange={(event) => {
              const size = Number(event.target.value)
              const pageSize = LOG_PAGE_SIZES.find(
                (value) => value === size
              )
              if (pageSize !== undefined) props.onPageSizeChange(pageSize)
            }}
          >
            {!LOG_PAGE_SIZES.some((size) => size === props.pageSize) && (
              <NativeSelectOption value={props.pageSize} disabled>
                {props.pageSize}
              </NativeSelectOption>
            )}
            {LOG_PAGE_SIZES.map((size) => (
              <NativeSelectOption key={size} value={size}>
                {size}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </label>
      </div>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='flex flex-wrap items-center gap-1'>
          <Button
            variant='outline'
            className='min-h-11 min-w-11'
            disabled={navigationDisabled || props.page <= 1}
            onClick={() => props.onPageChange(1)}
          >
            {t('First page')}
          </Button>
          <Button
            variant='outline'
            className='min-h-11 min-w-11'
            disabled={navigationDisabled || props.page <= 1}
            aria-label={props.requestTotals ? t('Previous') : t('Go to previous page')}
            onClick={() => props.onPageChange(props.page - 1)}
          >
            {t('Previous')}
          </Button>
          {visiblePages.map((page) => (
            <Button
              key={page}
              variant={page === props.page ? 'default' : 'outline'}
              size='icon'
              className='min-h-11 min-w-11'
              aria-label={t('Go to page {{page}}', { page })}
              aria-current={page === props.page ? 'page' : undefined}
              disabled={navigationDisabled || page === props.page}
              onClick={() => props.onPageChange(page)}
            >
              {page}
            </Button>
          ))}
          <Button
            variant='outline'
            className='min-h-11 min-w-11'
            disabled={navigationDisabled || props.page >= (totalPages ?? 1)}
            aria-label={props.requestTotals ? t('Next') : t('Go to next page')}
            onClick={() => props.onPageChange(props.page + 1)}
          >
            {t('Next')}
          </Button>
          <Button
            variant='outline'
            className='min-h-11 min-w-11'
            disabled={navigationDisabled || props.page >= (totalPages ?? 1)}
            onClick={() => totalPages !== null && props.onPageChange(totalPages)}
          >
            {t('Last page')}
          </Button>
        </div>
        <form
          className='flex items-center gap-2'
          onSubmit={(event) => {
            event.preventDefault()
            setJumpAttempted(true)
            if (navigationDisabled || !validJump) return
            props.onPageChange(pageNumber)
            setJumpPage('')
            setJumpAttempted(false)
          }}
        >
          <Input
            aria-label={t('Page number')}
            aria-invalid={invalidJump}
            aria-describedby={invalidJump ? errorId : undefined}
            inputMode='numeric'
            className='min-h-11 w-20'
            value={jumpPage}
            disabled={navigationDisabled}
            onChange={(event) => setJumpPage(event.target.value)}
          />
          <Button
            type='submit'
            variant='outline'
            className='min-h-11 min-w-11'
            disabled={navigationDisabled}
          >
            {t('Go to page')}
          </Button>
        </form>
      </div>
      {invalidJump && (
        <p id={errorId} role='alert' className='text-destructive'>
          {t('Enter a page between 1 and {{totalPages}}', { totalPages })}
        </p>
      )}
    </nav>
  )
}
