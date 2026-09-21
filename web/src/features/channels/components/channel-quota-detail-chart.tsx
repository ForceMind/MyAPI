/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useAuthStore } from '@/stores/auth-store'

import { getChannelQuotaHistory } from '../api'
import type { QuotaCustomRange } from '../hooks/use-quota-history-time'
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
  return ['line', 'area', 'bar', 'scatter'].includes(value)
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
}) {
  const { t } = useTranslation()
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
    ],
    queryFn: () =>
      getChannelQuotaHistory(item.channel_id, {
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
      }),
    staleTime: 60 * 1000,
    refetchInterval: 60 * 1000,
    enabled: enabled && (range !== 'custom' || Boolean(start && end)),
    retry: false,
  })
  const seriesTitle = `${item.account_label || item.name} · ${quotaWindowLabel(item.window_type, t)}`

  return (
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
      onRefresh={() => void query.refetch()}
      className='min-w-0'
      customRange={customRange}
      onCustomRangeChange={onCustomRangeChange}
    />
  )
}
