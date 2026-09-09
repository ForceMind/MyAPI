/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useEffect, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Area,
  Bar,
  CartesianGrid,
  ComposedChart,
  Line,
  ReferenceLine,
  Scatter,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

import { ChartContainer } from '@/components/ui/chart'
import { toIntlLocale } from '@/i18n/languages'
import { cn } from '@/lib/utils'

import {
  nearestQuotaPoint,
  zoomQuotaTime,
  type QuotaTimeBounds,
} from '../lib/quota-comparison'
import { buildQuotaHistoryTrend } from '../lib/quota-history'
import type { ChannelQuotaHistoryData } from '../types'

export type ComparisonChartStyle = 'line' | 'bar' | 'scatter' | 'area'
export type ComparisonMetric = 'available' | 'consumption'
export interface ComparisonSeries {
  key: string
  label: string
  data: ChannelQuotaHistoryData
  color: string
}

export function QuotaComparisonChart(props: {
  series: ComparisonSeries[]
  metric: ComparisonMetric
  style: ComparisonChartStyle
  bounds: QuotaTimeBounds
  onZoom: (bounds: QuotaTimeBounds) => void
  className?: string
}) {
  const { t, i18n } = useTranslation()
  const hostRef = useRef<HTMLDivElement>(null)
  const current = useRef(props)
  current.current = props
  useEffect(() => {
    const host = hostRef.current
    if (!host) return
    let pending: QuotaTimeBounds | undefined
    let timer: ReturnType<typeof setTimeout> | undefined
    const wheel = (event: WheelEvent) => {
      if (!event.deltaY) return
      event.preventDefault()
      const state = current.current
      const rect = host.getBoundingClientRect()
      const anchor =
        (event.clientX - rect.left - 64) / Math.max(1, rect.width - 88)
      pending = zoomQuotaTime(
        pending ?? state.bounds,
        event.deltaY < 0 ? 0.8 : 1.25,
        anchor,
        Math.floor(Date.now() / 1000)
      )
      clearTimeout(timer)
      timer = setTimeout(() => {
        if (pending) current.current.onZoom(pending)
        pending = undefined
      }, 180)
    }
    host.addEventListener('wheel', wheel, { passive: false })
    return () => {
      host.removeEventListener('wheel', wheel)
      clearTimeout(timer)
    }
  }, [])
  const series = useMemo(
    () =>
      props.series.map((item, index) => ({
        ...item,
        field: `series${index}`,
        trend: buildQuotaHistoryTrend(item.data, props.metric),
      })),
    [props.series, props.metric]
  )
  const rows = useMemo(() => {
    const byTime = new Map<number, Record<string, number | null>>()
    for (const item of series) {
      for (const point of item.trend.points) {
        if (point.status === 'gap') continue
        const row = byTime.get(point.timestamp) ?? {
          timestamp: point.timestamp,
        }
        row[item.field] = point.value
        byTime.set(point.timestamp, row)
      }
    }
    return [...byTime.values()].sort(
      (a, b) => Number(a.timestamp) - Number(b.timestamp)
    )
  }, [series])
  const formatTime = (value: number, full = false) =>
    new Date(value * 1000).toLocaleString(
      toIntlLocale(i18n.language),
      full
        ? {
            year: 'numeric',
            month: '2-digit',
            day: '2-digit',
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit',
          }
        : {
            month: '2-digit',
            day: '2-digit',
            hour: '2-digit',
            minute: '2-digit',
          }
    )
  const formatNumber = (value: number) =>
    new Intl.NumberFormat(toIntlLocale(i18n.language), {
      maximumSignificantDigits: 15,
    }).format(value)
  const unit = props.series[0]?.data.currency || props.series[0]?.data.unit
  let displayUnit = unit ?? ''
  if (unit === 'percent') {
    displayUnit = props.metric === 'consumption' ? t('Percentage points') : '%'
  }
  const resets = [
    ...new Set(
      series.flatMap((item) =>
        item.trend.points
          .filter((point) => point.isResetBoundary)
          .map((point) => point.timestamp)
      )
    ),
  ]
  return (
    <div
      ref={hostRef}
      className='min-w-0'
      data-testid='quota-comparison-chart'
      aria-label={t('Quota comparison chart')}
    >
      <ChartContainer
        config={{}}
        className={cn(
          'aspect-auto h-[clamp(220px,42dvh,400px)] w-full',
          props.className
        )}
        initialDimension={{ width: 640, height: 440 }}
      >
        <ComposedChart
          data={rows}
          margin={{ top: 12, right: 24, bottom: 16, left: 0 }}
        >
          <CartesianGrid vertical={false} strokeDasharray='3 3' />
          <XAxis
            dataKey='timestamp'
            type='number'
            scale='time'
            domain={[props.bounds.start, props.bounds.end]}
            allowDataOverflow
            tickFormatter={(value) => formatTime(Number(value))}
            minTickGap={48}
            tick={{ fontSize: 11, fill: 'var(--muted-foreground)' }}
          />
          <YAxis
            width={64}
            domain={[0, 'auto']}
            tickFormatter={(value) => formatNumber(Number(value))}
            tick={{ fontSize: 11, fill: 'var(--muted-foreground)' }}
          />
          <Tooltip
            content={({ active, label }) => {
              if (!active || label == null) return null
              return (
                <div className='bg-popover text-popover-foreground max-w-[min(360px,85vw)] rounded-lg border p-3 text-xs shadow-md'>
                  <div className='mb-2 font-medium'>
                    {t('Nearest recorded samples')}
                  </div>
                  {series.map((item) => {
                    const point = nearestQuotaPoint(
                      item.trend.points.filter((p) => p.status !== 'gap'),
                      Number(label)
                    )
                    return (
                      <div key={item.key} className='mb-2'>
                        <span style={{ color: item.color }}>{item.label}</span>
                        <div>
                          {point?.value == null
                            ? t('Unavailable')
                            : `${formatNumber(point.value)} ${displayUnit}`}
                        </div>
                        <div className='text-muted-foreground'>
                          {point ? formatTime(point.observedAt, true) : ''}
                        </div>
                      </div>
                    )
                  })}
                </div>
              )
            }}
          />
          {resets.map((time) => (
            <ReferenceLine
              key={time}
              x={time}
              stroke='var(--muted-foreground)'
              strokeDasharray='4 4'
            />
          ))}
          {series.map((item) => {
            const data = item.trend.points
            if (props.style === 'bar') {
              return (
                <Bar
                  key={item.key}
                  dataKey={item.field}
                  name={item.label}
                  fill={item.color}
                  maxBarSize={20}
                  isAnimationActive={false}
                />
              )
            }
            if (props.style === 'scatter') {
              return (
                <Scatter
                  key={item.key}
                  data={data.filter((point) => point.value != null)}
                  dataKey='value'
                  name={item.label}
                  fill={item.color}
                  isAnimationActive={false}
                />
              )
            }
            if (props.style === 'area') {
              return (
                <Area
                  key={item.key}
                  data={data}
                  dataKey='value'
                  name={item.label}
                  type='linear'
                  stroke={item.color}
                  fill={item.color}
                  fillOpacity={0.12}
                  connectNulls={false}
                  dot={data.length < 80}
                  isAnimationActive={false}
                />
              )
            }
            return (
              <Line
                key={item.key}
                data={data}
                dataKey='value'
                name={item.label}
                type='linear'
                stroke={item.color}
                strokeWidth={2}
                connectNulls={false}
                dot={data.length < 80}
                isAnimationActive={false}
              />
            )
          })}
        </ComposedChart>
      </ChartContainer>
    </div>
  )
}
