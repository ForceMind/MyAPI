/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { Calendar03Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useId, useState } from 'react'
import type { DateRange } from 'react-day-picker'
import { enUS, fr, ja, ru, vi, zhCN, zhTW } from 'react-day-picker/locale'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import dayjs from '@/lib/dayjs'
import { cn } from '@/lib/utils'

const calendarLocales = { en: enUS, zh: zhCN, 'zh-TW': zhTW, fr, ja, ru, vi }

interface DateTimeRangePickerProps {
  start?: Date
  end?: Date
  onChange: (range: { start?: Date; end?: Date }) => void
  className?: string
}

/** A draft range is committed only by Apply; dismissing the popover discards it. */
export function DateTimeRangePicker(props: DateTimeRangePickerProps) {
  const { t, i18n } = useTranslation()
  const id = useId()
  const [open, setOpen] = useState(false)
  const [range, setRange] = useState<DateRange>()
  const [month, setMonth] = useState(new Date())
  const [selectingEnd, setSelectingEnd] = useState(false)
  const [startTime, setStartTime] = useState('00:00:00')
  const [endTime, setEndTime] = useState('23:59:59')
  const [error, setError] = useState('')
  const language = i18n.resolvedLanguage || i18n.language || 'en'
  const locale =
    calendarLocales[language as keyof typeof calendarLocales] ??
    calendarLocales[language.split('-')[0] as keyof typeof calendarLocales] ??
    enUS
  const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const startLabel = props.start
    ? dayjs(props.start).format('YYYY-MM-DD HH:mm:ss')
    : t('No start date')
  const endLabel = props.end
    ? dayjs(props.end).format('YYYY-MM-DD HH:mm:ss')
    : t('No end date')
  const label =
    props.start || props.end ? `${startLabel} – ${endLabel}` : t('Date Range')
  const validCalendarRange = [range?.from, range?.to].every(
    (date) => !date || Number.isFinite(date.getTime())
  )

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) {
      setRange(
        props.start || props.end
          ? { from: props.start, to: props.end }
          : undefined
      )
      const initialMonth = props.start ?? props.end
      setMonth(
        initialMonth && Number.isFinite(initialMonth.getTime())
          ? initialMonth
          : new Date()
      )
      setStartTime(
        props.start ? dayjs(props.start).format('HH:mm:ss') : '00:00:00'
      )
      setEndTime(props.end ? dayjs(props.end).format('HH:mm:ss') : '23:59:59')
      setSelectingEnd(false)
      setError('')
    }
    setOpen(nextOpen)
  }

  const applyPreset = (kind: 'today' | '7d' | '30d' | 'month') => {
    const today = dayjs()
    let start = today.startOf('day')
    let end = today.endOf('day')
    if (kind === '7d') start = start.subtract(6, 'day')
    if (kind === '30d') start = start.subtract(29, 'day')
    if (kind === 'month') {
      start = today.startOf('month')
      end = today.endOf('month')
    }
    setRange({ from: start.toDate(), to: end.toDate() })
    setMonth(start.toDate())
    setStartTime('00:00:00')
    setEndTime('23:59:59')
    setSelectingEnd(false)
    setError('')
  }

  const applyDraft = () => {
    if (selectingEnd) {
      setError(t('Select an end date.'))
      return
    }
    const dates = [range?.from, range?.to]
    const times = [startTime, endTime]
    const result: (Date | undefined)[] = []
    for (let index = 0; index < dates.length; index += 1) {
      const date = dates[index]
      if (!date) {
        result.push(undefined)
        continue
      }
      if (!Number.isFinite(date.getTime())) {
        setError(t('Select a valid date range.'))
        return
      }
      const time = times[index]
      if (!/^([01]\d|2[0-3]):[0-5]\d(?::[0-5]\d)?$/.test(time)) {
        setError(t('Enter a valid time.'))
        return
      }
      const [hours, minutes, seconds = 0] = time.split(':').map(Number)
      const value = new Date(date)
      value.setHours(hours, minutes, seconds, 0)
      // Local times skipped by daylight saving must not silently shift an hour.
      if (
        value.getFullYear() !== date.getFullYear() ||
        value.getMonth() !== date.getMonth() ||
        value.getDate() !== date.getDate() ||
        value.getHours() !== hours ||
        value.getMinutes() !== minutes ||
        value.getSeconds() !== seconds
      ) {
        setError(t('Enter a valid time.'))
        return
      }
      result.push(value)
    }
    const [start, end] = result
    if (start && end && start > end) {
      setError(t('End time must not be before start time.'))
      return
    }
    props.onChange({ start, end })
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger
        render={
          <Button
            type='button'
            variant='outline'
            className={cn(
              'min-h-11 w-full min-w-0 justify-start font-normal',
              props.className
            )}
          />
        }
        aria-label={`${t('Date Range')}: ${label}`}
      >
        <HugeiconsIcon
          icon={Calendar03Icon}
          aria-hidden='true'
          className='size-4 shrink-0'
        />
        <span className='truncate tabular-nums'>{label}</span>
      </PopoverTrigger>
      <PopoverContent
        align='start'
        collisionPadding={1}
        className='max-h-[min(46rem,calc(100dvh-2rem))] w-[min(380px,calc(100vw-2px))] overflow-y-auto p-1 min-[360px]:p-3'
        onKeyDown={(event) => {
          if (event.key === 'Enter' && event.target instanceof HTMLInputElement) {
            event.preventDefault()
            event.stopPropagation()
            applyDraft()
          }
        }}
      >
        <PopoverTitle>{t('Date Range')}</PopoverTitle>
        <p className='text-muted-foreground text-xs'>
          {t('Select a start and end date.')}
        </p>
        <div className='flex flex-wrap gap-2'>
          <Button
            type='button'
            variant='secondary'
            className='min-h-11'
            onClick={() => applyPreset('today')}
          >
            {t('Today')}
          </Button>
          <Button
            type='button'
            variant='secondary'
            className='min-h-11'
            onClick={() => applyPreset('7d')}
          >
            {t('Last 7 days')}
          </Button>
          <Button
            type='button'
            variant='secondary'
            className='min-h-11'
            onClick={() => applyPreset('30d')}
          >
            {t('Last 30 days')}
          </Button>
          <Button
            type='button'
            variant='secondary'
            className='min-h-11'
            onClick={() => applyPreset('month')}
          >
            {t('Current month')}
          </Button>
          <Button
            type='button'
            variant='ghost'
            className='min-h-11'
            onClick={() => {
              setRange(undefined)
              setSelectingEnd(false)
              setError('')
            }}
          >
            {t('Clear range')}
          </Button>
        </div>
        <div className='overflow-x-auto'>
          <Calendar
            mode='range'
            selected={validCalendarRange ? range : undefined}
            showOutsideDays={false}
            month={month}
            onMonthChange={setMonth}
            locale={locale}
            className='p-0 [--cell-size:2.75rem]'
            labels={{
              labelPrevious: () => t('Previous month'),
              labelNext: () => t('Next month'),
            }}
            onSelect={(_selected, date) => {
              if (selectingEnd && range?.from) {
                setRange({ from: range.from, to: date })
                setSelectingEnd(false)
              } else {
                setRange({ from: date, to: undefined })
                setSelectingEnd(true)
              }
              setError('')
            }}
          />
        </div>
        <div className='grid grid-cols-1 gap-3 min-[360px]:grid-cols-2'>
          <div className='min-w-0 space-y-1.5'>
            <Label htmlFor={`${id}-start`}>{t('Start Time')}</Label>
            <p className='text-sm tabular-nums'>
              {range?.from
                ? dayjs(range.from).format('YYYY-MM-DD')
                : t('No start date')}
            </p>
            <Input
              id={`${id}-start`}
              type='time'
              step='1'
              value={startTime}
              disabled={!range?.from}
              className='min-h-11'
              aria-invalid={Boolean(error)}
              aria-describedby={error ? `${id}-error` : undefined}
              onChange={(event) => {
                setStartTime(event.target.value)
                setError('')
              }}
            />
            <Button
              type='button'
              variant='ghost'
              className='min-h-11 w-full whitespace-normal px-1 text-xs'
              disabled={!range?.from}
              onClick={() => {
                setRange({ from: undefined, to: range?.to })
                setSelectingEnd(false)
                setError('')
              }}
            >
              {t('Clear start date')}
            </Button>
          </div>
          <div className='min-w-0 space-y-1.5'>
            <Label htmlFor={`${id}-end`}>{t('End Time')}</Label>
            <p className='text-sm tabular-nums'>
              {range?.to
                ? dayjs(range.to).format('YYYY-MM-DD')
                : t('No end date')}
            </p>
            <Input
              id={`${id}-end`}
              type='time'
              step='1'
              value={endTime}
              disabled={!range?.to}
              className='min-h-11'
              aria-invalid={Boolean(error)}
              aria-describedby={error ? `${id}-error` : undefined}
              onChange={(event) => {
                setEndTime(event.target.value)
                setError('')
              }}
            />
            <Button
              type='button'
              variant='ghost'
              className='min-h-11 w-full whitespace-normal px-1 text-xs'
              disabled={!range?.to}
              onClick={() => {
                setRange({ from: range?.from, to: undefined })
                setSelectingEnd(false)
                setError('')
              }}
            >
              {t('Clear end date')}
            </Button>
          </div>
        </div>
        <p className='text-muted-foreground break-words text-xs'>
          {t('Local time')}: {timeZone}
        </p>
        {error && (
          <p id={`${id}-error`} role='alert' className='text-destructive text-sm'>
            {error}
          </p>
        )}
        <div className='flex justify-end gap-2 border-t pt-3'>
          <Button
            type='button'
            variant='outline'
            className='min-h-11'
            onClick={() => setOpen(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='button' className='min-h-11' onClick={applyDraft}>
            {t('Apply range')}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
