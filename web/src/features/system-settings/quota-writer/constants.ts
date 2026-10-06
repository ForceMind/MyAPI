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
import { t as i18nextT } from 'i18next'

import type { StatusVariant } from '@/components/status-badge'

import type {
  QuotaWriterAudit,
  QuotaWriterMode,
  QuotaWriterTransitionStatus,
} from './types'

/**
 * One row of the readiness audit checklist. `id` matches the check names the
 * backend reports through `missing_or_failed_checks` so failing rows can be
 * cross-highlighted with the server-computed missing list.
 */
export interface AuditCheckView {
  id: string
  labelKey: string
  passed: boolean
  /** Backlog / in-flight count shown next to the row when positive. */
  count?: number
  countLabelKey?: string
}

export function buildAuditChecklist(audit: QuotaWriterAudit): AuditCheckView[] {
  return [
    {
      id: 'production_writer_registration',
      labelKey: 'All writers migrated',
      passed: audit.all_writers_migrated,
    },
    {
      id: 'batch_queue_empty',
      labelKey: 'Batch queue empty',
      passed: audit.batch_queue_empty,
    },
    {
      id: 'balance_drain_pending',
      labelKey: 'Balance drain backlog cleared',
      passed: audit.balance_drain_pending === 0,
      count: audit.balance_drain_pending,
      countLabelKey: '{{count}} pending',
    },
    {
      id: 'balance_drain_inflight_zero',
      labelKey: 'No balance drain in flight',
      passed: audit.balance_drain_inflight_zero,
    },
    {
      id: 'maintenance_backfill',
      labelKey: 'Maintenance backfill complete',
      passed: audit.maintenance_backfill_done,
    },
    {
      id: 'projection_pending',
      labelKey: 'Projection backlog cleared',
      passed: audit.projection_pending === 0,
      count: audit.projection_pending,
      countLabelKey: '{{count}} pending',
    },
    {
      id: 'redis_epoch',
      labelKey: 'Redis epoch consistent',
      passed: audit.redis_epoch_consistent,
    },
    {
      id: 'cluster_drain_ack',
      labelKey: 'Cluster drain acknowledged',
      passed: audit.cluster_drain_ack,
    },
    {
      id: 'inflight_zero',
      labelKey: 'No in-flight billing sessions',
      passed: audit.inflight_zero,
      count: audit.inflight_sessions,
      countLabelKey: '{{count}} in flight',
    },
  ]
}

/**
 * Maps backend finding keys (plan validation entries, missing audit checks)
 * to i18n message keys. Unknown keys fall back to the raw backend identifier.
 */
export const QUOTA_WRITER_FINDING_MESSAGE_KEYS: Record<string, string> = {
  epoch_exhausted:
    'Epoch space is exhausted; no further transitions are possible.',
  legacy_must_transition_to_bridge:
    'Legacy mode can only transition to bridge mode.',
  bridge_must_transition_to_authoritative:
    'Bridge mode can only transition to authoritative mode.',
  authoritative_mode_is_not_downgradable:
    'Authoritative mode cannot be downgraded.',
  already_authoritative: 'The writer is already in authoritative mode.',
  durable_write_audit_failed:
    'The durable write audit has failing hard conditions.',
  cluster_drain_ack:
    'A cluster drain acknowledgement is required for this transition.',
  production_writer_registration:
    'Some registered quota writers have not been migrated.',
  batch_queue_empty: 'The batch update queue is not empty.',
  balance_drain_pending: 'Balance drain generations are still pending.',
  balance_drain_inflight_zero: 'A balance drain is still in flight.',
  balance_drain_state: 'The balance drain state could not be audited.',
  maintenance_backfill: 'Maintenance backfills are not complete.',
  projection_pending: 'Projection obligations are still pending.',
  redis_epoch: 'The Redis epoch is not consistent with the persisted epoch.',
  inflight_zero: 'Billing sessions are still in flight.',
  writer_epoch_state: 'The persisted writer epoch state is unavailable.',
  redis_epoch_publish: 'The new epoch could not be published to Redis.',
  post_audit_error: 'The post-transition audit failed to run.',
  epoch_readback: 'The committed epoch could not be read back.',
}

/**
 * Translates a backend finding key (plan validation entry, missing audit
 * check, failure_reason part) through the message map; unknown identifiers
 * are rendered raw. Called during render of components that themselves use
 * `useTranslation`, so it re-resolves on language change.
 */
export function translateQuotaWriterFinding(key: string): string {
  return i18nextT(QUOTA_WRITER_FINDING_MESSAGE_KEYS[key] ?? key)
}

/**
 * Failure reasons are persisted as comma-joined finding keys; translate the
 * known parts and keep unknown identifiers raw.
 */
export function translateQuotaWriterFailureReason(reason: string): string {
  return reason
    .split(',')
    .map((part) => part.trim())
    .filter((part) => part.length > 0)
    .map((part) => translateQuotaWriterFinding(part))
    .join(', ')
}

export const QUOTA_WRITER_MODE_VARIANTS: Record<
  QuotaWriterMode,
  StatusVariant
> = {
  legacy: 'neutral',
  bridge: 'warning',
  authoritative: 'success',
}

export const QUOTA_WRITER_TRANSITION_STATUS_VARIANTS: Record<
  QuotaWriterTransitionStatus,
  StatusVariant
> = {
  applying: 'warning',
  succeeded: 'success',
  failed: 'danger',
}

export const QUOTA_WRITER_TRANSITION_STATUS_LABEL_KEYS: Record<
  QuotaWriterTransitionStatus,
  string
> = {
  applying: 'Applying',
  succeeded: 'Succeeded',
  failed: 'Failed',
}

/** Backend apply bound for the acknowledgement note (1-512 characters). */
export const QUOTA_WRITER_ACK_NOTE_MAX_LENGTH = 512

/** Backend drain budget bounds (clamped server-side to 1-100). */
export const QUOTA_WRITER_DRAIN_BUDGET_MAX = 100

/** Allowed forward targets per current mode; authoritative has none. */
export const QUOTA_WRITER_ALLOWED_TARGETS: Record<
  QuotaWriterMode,
  Array<'bridge' | 'authoritative'>
> = {
  legacy: ['bridge'],
  bridge: ['authoritative'],
  authoritative: [],
}
