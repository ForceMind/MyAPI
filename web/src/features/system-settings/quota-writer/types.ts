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

/**
 * Quota writer mode transition APIs.
 *
 * Semantics that the UI must express accurately:
 * - Modes only move forward: legacy -> bridge -> authoritative; authoritative
 *   can never be downgraded.
 * - Bridge is a silent drain mode: new relay sessions are rejected while
 *   in-flight sessions can still settle; subscription balance purchases and
 *   admin quota adjustments are unavailable in bridge mode.
 * - legacy -> bridge requires an operator acknowledgement note (1-512 chars,
 *   persisted as evidence); bridge -> authoritative requires every audit hard
 *   condition to pass.
 */

export type QuotaWriterMode = 'legacy' | 'bridge' | 'authoritative'

export interface QuotaWriterEpochState {
  id: number
  schema_version: number
  mode: QuotaWriterMode
  epoch: number
  lock_version: number
  updated_at: number
}

export interface QuotaWriterRegistration {
  name: string
  migrated: boolean
}

export interface QuotaWriterAudit {
  can_enable: boolean
  state?: QuotaWriterEpochState
  writers: QuotaWriterRegistration[]
  all_writers_migrated: boolean
  batch_queue_empty: boolean
  projection_pending: number
  balance_drain_pending: number
  balance_drain_inflight_zero: boolean
  maintenance_backfill_done: boolean
  redis_epoch_consistent: boolean
  cluster_drain_ack: boolean
  inflight_sessions: number
  inflight_zero: boolean
  missing_or_failed_checks?: string[]
}

export interface QuotaWriterStatus {
  state: QuotaWriterEpochState
  audit: QuotaWriterAudit
  inflight_sessions: number
}

export interface QuotaWriterTransitionPlan {
  current: QuotaWriterEpochState
  target_mode: QuotaWriterMode
  proposed_epoch: number
  audit: QuotaWriterAudit
  ready: boolean
  validation?: string[] | null
}

export type QuotaWriterTransitionStatus = 'applying' | 'succeeded' | 'failed'

export interface QuotaWriterTransition {
  id: number
  operator_user_id: number
  from_mode: QuotaWriterMode
  to_mode: QuotaWriterMode
  from_epoch: number
  to_epoch: number
  status: QuotaWriterTransitionStatus
  pre_audit: string
  post_audit: string
  cluster_drain_ack: boolean
  ack_note: string
  failure_reason: string
  created_at: number
  finished_at: number
}

export interface QuotaWriterTransitionPage {
  page: number
  page_size: number
  total: number
  items: QuotaWriterTransition[] | null
}

export interface QuotaWriterDrainReport {
  rounds: number
  batch_queue_remaining: number
  balance_drain_pending: number
  balance_drain_inflight: boolean
  projection_pending: number
  complete: boolean
}

export interface QuotaWriterApplyRequest {
  target_mode: QuotaWriterMode
  expected_epoch: number
  ack_note: string
}

/** 409 payload returned when audit hard conditions block an apply. */
export interface QuotaWriterApplyConflictData {
  transition?: QuotaWriterTransition
  missing?: string[]
}

export interface QuotaWriterApiEnvelope<T> {
  success: boolean
  message?: string
  data?: T
}
