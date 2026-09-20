/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { SortingState } from '@tanstack/react-table'
import axios from 'axios'

import { toIntlLocale } from '@/i18n/languages'

import type {
  ChannelRoutingPreviewResponse,
  ChannelSortBy,
  ChannelSortOrder,
} from '../types'

export const CHANNEL_SORTABLE_COLUMNS = new Set<ChannelSortBy>([
  'id',
  'name',
  'priority',
  'weight',
  'balance',
  'response_time',
  'test_time',
])

export function isChannelSortDisabledInTagMode(
  columnId: string,
  tagMode: boolean
): boolean {
  return tagMode && (columnId === 'priority' || columnId === 'weight')
}

export function getChannelSortParams(
  sorting: SortingState,
  tagMode = false
): {
  sort_by?: ChannelSortBy
  sort_order?: ChannelSortOrder
} {
  const activeSort = sorting[0]
  if (
    !activeSort ||
    !CHANNEL_SORTABLE_COLUMNS.has(activeSort.id as ChannelSortBy) ||
    (tagMode && (activeSort.id === 'priority' || activeSort.id === 'weight'))
  ) {
    return {}
  }
  return {
    sort_by: activeSort.id as ChannelSortBy,
    sort_order: activeSort.desc ? 'desc' : 'asc',
  }
}

export function createChannelRoutingPercentFormatter(
  locale?: string | null
): Intl.NumberFormat {
  return new Intl.NumberFormat(toIntlLocale(locale), {
    style: 'percent',
    maximumFractionDigits: 2,
  })
}

export type ChannelRoutingPreviewErrorCode =
  | 'routing_preview_invalid_params'
  | 'routing_preview_auto_group_unsupported'
  | 'routing_preview_permission_denied'
  | 'routing_preview_database_error'
  | 'routing_preview_network_error'
  | 'routing_preview_request_failed'

const ROUTING_PREVIEW_ERROR_CODES = new Set<ChannelRoutingPreviewErrorCode>([
  'routing_preview_invalid_params',
  'routing_preview_auto_group_unsupported',
  'routing_preview_database_error',
  'routing_preview_network_error',
  'routing_preview_request_failed',
])

function normalizeRoutingPreviewErrorCode(
  value: unknown
): ChannelRoutingPreviewErrorCode | null {
  if (
    typeof value === 'string' &&
    ROUTING_PREVIEW_ERROR_CODES.has(value as ChannelRoutingPreviewErrorCode)
  ) {
    return value as ChannelRoutingPreviewErrorCode
  }
  return null
}

export function resolveChannelRoutingPreviewErrorCode(
  error: unknown,
  response?: ChannelRoutingPreviewResponse
): ChannelRoutingPreviewErrorCode | null {
  if (response && !response.success) {
    return (
      normalizeRoutingPreviewErrorCode(response.code) ??
      'routing_preview_request_failed'
    )
  }
  if (!error) {
    return null
  }
  if (axios.isAxiosError<{ code?: unknown }>(error)) {
    const responseCode = normalizeRoutingPreviewErrorCode(
      error.response?.data?.code
    )
    if (responseCode) {
      return responseCode
    }
    if (error.response?.status === 401 || error.response?.status === 403) {
      return 'routing_preview_permission_denied'
    }
    if (!error.response) {
      return 'routing_preview_network_error'
    }
  }
  return 'routing_preview_request_failed'
}
