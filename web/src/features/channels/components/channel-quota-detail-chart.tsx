/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { CanceledError } from 'axios'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useAuthStore } from '@/stores/auth-store'

import { getChannelQuotaHistory } from '../api'
import type { QuotaCustomRange } from '../hooks/use-quota-history-time'
import type { ChannelTaskProps, ChannelTaskReadState } from '../channel-task-state'
import {
  quotaWindowLabel,
  boundedQuotaDuration,
  quotaDurationOptions,
  quotaHistoryRangeOptions,
  quotaHistoryRangeSeconds,
  quotaHistoryGranularityOptions,
  type QuotaHistoryChartStyle,
  type QuotaHistoryMetric,
} from '../lib/quota-history'
import type {
  ChannelQuotaAnalysisMethod,
  ChannelQuotaChangeItem,
  ChannelQuotaHistoryGranularity,
  ChannelQuotaHistoryRange,
} from '../types'
import { QuotaHistoryTrend } from './quota-history-trend'

function isGranularity(value: string): value is ChannelQuotaHistoryGranularity {
  return quotaHistoryGranularityOptions.includes(
    value as ChannelQuotaHistoryGranularity
  )
}

function isMetric(value: string): value is QuotaHistoryMetric {
  return [
    'available',
    'used',
    'total',
    'consumption',
    'rate_per_minute',
  ].includes(value)
}

function isChartStyle(value: string): value is QuotaHistoryChartStyle {
  return ['line', 'area', 'bar'].includes(value)
}

function isAnalysisMethod(value: string): value is ChannelQuotaAnalysisMethod {
  return ['latest_interval', 'observed_window', 'ewma'].includes(value)
}

/**
 * Detailed chart for one resolved provider quota series. The parent owns the
 * selected time range so its list, headline, and detail chart never describe
 * different periods. This component owns only display preferences.
 */
export function ChannelQuotaDetailChart({
  item,
  range,
  onRangeChange,
  initialMetric = 'consumption',
  refreshEpoch = 0,
  customRange,
  onCustomRangeChange,
  enabled = true,
  title,
  onReadStateChange,
}: {
  item: ChannelQuotaChangeItem
  range: ChannelQuotaHistoryRange
  onRangeChange: (range: ChannelQuotaHistoryRange) => void
  initialMetric?: QuotaHistoryMetric
  refreshEpoch?: number
  customRange?: QuotaCustomRange
  onCustomRangeChange?: (range: QuotaCustomRange) => void
  enabled?: boolean
  title?: string
  onReadStateChange?: ChannelTaskProps['onReadStateChange']
}) {
  const { t } = useTranslation()
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const [granularity, setGranularity] =
    useState<ChannelQuotaHistoryGranularity>('auto')
  const [metric, setMetric] = useState<QuotaHistoryMetric>(initialMetric)
  const [analysisMethod, setAnalysisMethod] =
    useState<ChannelQuotaAnalysisMethod>('observed_window')
  const [requestedRateWindow, setRequestedRateWindow] = useState(60 * 60)
  const [requestedHalfLife, setRequestedHalfLife] = useState(30 * 60)
  const [manualChartStyle, setChartStyle] =
    useState<QuotaHistoryChartStyle | null>(null)
  const chartStyle =
    manualChartStyle ?? (metric === 'consumption' ? 'bar' : 'line')
  const timezoneOffset = -new Date().getTimezoneOffset()
  const start = range === 'custom' ? customRange?.start : undefined
  const end = range === 'custom' ? customRange?.end : undefined
  const rangeSeconds = quotaHistoryRangeSeconds(range, customRange)
  const rateWindowSeconds = boundedQuotaDuration(
    requestedRateWindow,
    rangeSeconds
  )
  const ewmaHalfLifeSeconds = boundedQuotaDuration(
    requestedHalfLife,
    rateWindowSeconds
  )
  const rateWindowOptions = useMemo(
    () => quotaDurationOptions(rangeSeconds, rateWindowSeconds),
    [rangeSeconds, rateWindowSeconds]
  )
  const halfLifeOptions = useMemo(
    () => quotaDurationOptions(rateWindowSeconds, ewmaHalfLifeSeconds),
    [ewmaHalfLifeSeconds, rateWindowSeconds]
  )

  const query = useQuery({
    queryKey: [
      'channel-quota-detail-chart',
      userId,
      sessionId,
      item.channel_id,
      item.metric_type ?? null,
      item.source ?? null,
      item.window_type ?? null,
      item.plan_type ?? null,
      item.unit ?? null,
      item.currency ?? null,
      item.window_seconds ?? null,
      range,
      start,
      end,
      granularity,
      timezoneOffset,
      rateWindowSeconds,
      ewmaHalfLifeSeconds,
      refreshEpoch,
      identity,
      item.series_id ?? null,
      item.series_id ? null : item.account_label ?? null,
    ],
    queryFn: async ({ signal }) => {
      const auth = useAuthStore.getState().auth
      if (signal.aborted || identity !== `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`) throw new CanceledError('Quota detail identity changed')
      const response = await getChannelQuotaHistory(item.channel_id, {
        range,
        start,
        end,
        metric_type: item.metric_type,
        source: item.source,
        window_type: item.window_type,
        plan_type: item.plan_type,
        unit: item.unit,
        currency: item.currency,
        window_seconds: item.window_seconds,
        granularity,
        timezone_offset: timezoneOffset,
        rate_window: rateWindowSeconds,
        ewma_half_life: ewmaHalfLifeSeconds,
        // The backend reports an explicit incomplete response instead of
        // silently returning a misleading partial history.
        limit: 5000,
      }, { signal })
      const current = useAuthStore.getState().auth
      if (signal.aborted || identity !== `${current.user?.id ?? null}:${current.user?.role ?? 0}:${current.session?.sid ?? null}`) throw new CanceledError('Quota detail identity changed')
      const returnedSeriesId = response.data?.series_id ?? response.data?.series?.id
      if (response.success && item.series_id && returnedSeriesId && item.series_id !== returnedSeriesId) throw new Error(t('This history belongs to a different account window. Refresh the directory before continuing.'))
      return response
    },
    staleTime: 60 * 1000,
    refetchInterval: enabled ? 60 * 1000 : false,
    enabled: enabled && (range !== 'custom' || Boolean(start && end)),
    retry: false,
  })
  const savedData = query.data?.data
  const hasRead = query.data?.success === true && savedData != null
  const failed = query.isError || (query.data !== undefined && !hasRead)
  const incomplete = savedData?.source_complete === false || savedData?.points_complete === false || savedData?.complete === false
  const errorCount = savedData?.data_quality?.error_count
  const failedObservations = hasRead ? savedData?.current?.status === 'error' || (typeof errorCount === 'number' && Number.isFinite(errorCount) && errorCount > 0) || (Array.isArray(savedData?.points) && savedData.points.some((point) => point.status === 'error' || (typeof point.failed_count === 'number' && Number.isFinite(point.failed_count) && point.failed_count > 0))) : undefined
  const readAt = Math.max(query.dataUpdatedAt, query.errorUpdatedAt)
  let phase: ChannelTaskReadState['phase'] = 'unread'
  if (failed) phase = 'error'
  else if (query.isFetching) phase = 'loading'
  else if (hasRead) phase = 'ready'
  useEffect(() => {
    const auth = useAuthStore.getState().auth
    if (identity !== `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`) return
    onReadStateChange?.({ phase, readAt: readAt > 0 ? readAt / 1000 : undefined, incomplete, failedObservations })
  }, [identity, onReadStateChange, phase, readAt, incomplete, failedObservations])
  const seriesTitle = `${item.account_label || item.name} · ${quotaWindowLabel(item.window_type, t)}`

  return (
    <>
    {!failed && hasRead && !(query.data?.data?.series_id ?? query.data?.data?.series?.id) && <p role='status' className='text-muted-foreground text-sm'>{t('This history does not provide an account-window identity.')}</p>}
    <QuotaHistoryTrend
      data={query.data?.data}
      title={title ?? `${t('Detailed quota history')} · ${seriesTitle}`}
      range={range}
      granularity={granularity}
      metric={metric}
      chartStyle={chartStyle}
      rangeOptions={quotaHistoryRangeOptions}
      granularityOptions={quotaHistoryGranularityOptions}
      isLoading={query.isLoading}
      error={query.isError ? query.error : undefined}
      errorMessage={
        query.data?.success === false ? query.data.message : undefined
      }
      onRangeChange={(value) => {
        if (
          quotaHistoryRangeOptions.includes(value as ChannelQuotaHistoryRange)
        ) {
          onRangeChange(value as ChannelQuotaHistoryRange)
        }
      }}
      onGranularityChange={(value) => {
        if (isGranularity(value)) setGranularity(value)
      }}
      onMetricChange={(value) => {
        if (isMetric(value)) setMetric(value)
      }}
      onChartStyleChange={(value) => {
        if (isChartStyle(value)) setChartStyle(value)
      }}
      analysisMethod={analysisMethod}
      rateWindowSeconds={rateWindowSeconds}
      ewmaHalfLifeSeconds={ewmaHalfLifeSeconds}
      rateWindowOptions={rateWindowOptions}
      ewmaHalfLifeOptions={halfLifeOptions}
      onAnalysisMethodChange={(value) => {
        if (isAnalysisMethod(value)) setAnalysisMethod(value)
      }}
      onRateWindowChange={setRequestedRateWindow}
      onEWMAHalfLifeChange={setRequestedHalfLife}
      onRefresh={() => { if (enabled) void query.refetch() }}
      className='min-w-0'
      customRange={customRange}
      onCustomRangeChange={onCustomRangeChange}
    />
    </>
  )
}
