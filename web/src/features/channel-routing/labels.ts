/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { TFunction } from 'i18next'

import type { RoutingQuotaState } from './types'

const previewReasonKeys: Record<string, string> = {
  smart_routing_disabled: 'Intelligent allocation is disabled.',
  balanced_path: 'This request type uses balanced routing.',
  no_eligible_channel: 'No eligible channels match this request.',
  quota_insufficient_comparable_data:
    'Comparable remaining quota is unavailable, so normal routing is used.',
  chat_lower_remaining_quota:
    'Chat routing favors lower comparable remaining quota.',
  work_higher_remaining_quota:
    'Work routing favors higher comparable remaining quota.',
  priority_weight: 'Selected by routing order and traffic share.',
  quota_lower_pool: 'Included in the lower remaining quota pool.',
  quota_higher_pool: 'Included in the higher remaining quota pool.',
  quota_not_selected: 'Not in the selected quota pool.',
  quota_stale: 'Quota data is stale.',
  quota_unknown: 'Quota data is unknown.',
  quota_exhausted: 'Quota is exhausted.',
  lower_priority: 'A higher-priority channel was selected.',
  zero_weight: 'Traffic share is not configured for this channel.',
}

const quotaStateKeys: Record<RoutingQuotaState, string> = {
  fresh: 'Quota data is current',
  stale: 'Quota data is stale',
  unknown: 'Quota data is unknown',
  exhausted: 'Quota exhausted',
}

export function getRoutingReasonLabel(t: TFunction, code: string): string {
  return t(previewReasonKeys[code] ?? 'Routing policy decision.')
}

export function getQuotaStateLabel(
  t: TFunction,
  state: RoutingQuotaState
): string {
  return t(quotaStateKeys[state])
}

export function getWorkloadLabel(
  t: TFunction,
  workload: 'chat' | 'work' | 'balanced'
): string {
  if (workload === 'chat') return t('Chat')
  if (workload === 'work') return t('Work')
  return t('This request type uses balanced routing.')
}
