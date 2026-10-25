/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { StatusVariant } from '@/components/status-badge'

import type { UsageLog } from '../data/schema'
import { getPersonalCallEvidence } from './personal-call-evidence'

type Fact = { label: string; variant: StatusVariant }

/**
 * The four independent facts of one request (design 8.2): business outcome,
 * HTTP status, usage evidence and settlement. A missing fact is reported as
 * missing, never as success or zero. Labels are i18n keys.
 */
export function getRequestFacts(log: UsageLog) {
  const evidence = getPersonalCallEvidence(log)
  const other = evidence.other

  let outcome: Fact = { label: 'Request failed', variant: 'danger' }
  if (evidence.completed) {
    outcome = { label: 'Request successful', variant: 'success' }
  } else if (
    other?.preflight_rejected ||
    other?.request_outcome === 'rejected'
  ) {
    outcome = { label: 'Rejected before sending', variant: 'warning' }
  }

  const status = other?.http_status
  const httpStatus =
    Number.isInteger(status) && (status ?? 0) >= 100 && (status ?? 0) <= 599
      ? (status as number)
      : null

  const usage: Fact = evidence.usageKnown
    ? { label: 'Usage reported', variant: 'success' }
    : { label: 'Usage not provided', variant: 'neutral' }

  let settlement: Fact = {
    label: 'Settlement not recorded',
    variant: 'neutral',
  }
  switch (other?.settlement_status) {
    case 'settled':
    case 'applied':
      settlement = { label: 'Settled', variant: 'success' }
      break
    case 'pending_review':
    case 'pending':
    case 'applied_journal_pending':
      settlement = { label: 'Settlement pending review', variant: 'warning' }
      break
    case 'manual':
      settlement = { label: 'Settlement needs attention', variant: 'warning' }
      break
    case 'not_dispatched':
      settlement = { label: 'Not sent, not charged', variant: 'neutral' }
      break
  }

  return {
    completed: evidence.completed,
    usageKnown: evidence.usageKnown,
    /** Usage provenance as recorded (`reported`, `estimated`, ...). */
    accuracy: other?.usage_accuracy,
    outcome,
    httpStatus,
    usage,
    settlement,
  }
}

/** Only consume (2) and error (5) rows describe a relayed request. */
export function isRequestLog(log: Pick<UsageLog, 'type'>) {
  return log.type === 2 || log.type === 5
}

/** Error rows that record settlement review rather than a failed request. */
const REVIEW_CONTENTS = new Set([
  'usage_settlement_pending',
  'usage_pending_review',
])

/**
 * A request failure, by the same rule as the call record filter
 * `result=failed` (`model/log.go` `applyLogResultFilter`): an error record
 * that is neither a usage settlement review nor a request rejected by the
 * input preflight before any upstream was contacted.
 */
export function isRequestFailure(
  log: Pick<UsageLog, 'type' | 'content' | 'other'>
) {
  if (log.type !== 5 || REVIEW_CONTENTS.has(log.content ?? '')) return false
  try {
    const other: unknown = JSON.parse(log.other || 'null')
    return !(
      other &&
      typeof other === 'object' &&
      (other as { preflight_rejected?: unknown }).preflight_rejected === true
    )
  } catch {
    // The server matches the stored text; do the same for unreadable JSON.
    return !(log.other ?? '').includes('"preflight_rejected":true')
  }
}
