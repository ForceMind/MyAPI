import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { AuditChecklist } from '../components/audit-checklist'
import type { QuotaWriterAudit } from '../types'

function buildAudit(
  overrides: Partial<QuotaWriterAudit> = {}
): QuotaWriterAudit {
  return {
    can_enable: false,
    writers: [
      { name: 'mutate_user_quota', migrated: true },
      { name: 'delta_update_user_quota', migrated: false },
    ],
    all_writers_migrated: false,
    batch_queue_empty: false,
    projection_pending: 3,
    balance_drain_pending: 2,
    balance_drain_inflight_zero: true,
    maintenance_backfill_done: true,
    redis_epoch_consistent: true,
    cluster_drain_ack: true,
    inflight_sessions: 5,
    inflight_zero: false,
    missing_or_failed_checks: [
      'production_writer_registration',
      'batch_queue_empty',
      'balance_drain_pending',
      'projection_pending',
      'inflight_zero',
    ],
    ...overrides,
  }
}

function rowOf(label: string): HTMLElement {
  const row = screen.getByText(label).closest('[data-passed]')
  expect(row).not.toBeNull()
  return row as HTMLElement
}

describe('AuditChecklist', () => {
  test('highlights every failing hard condition as not met', () => {
    render(<AuditChecklist audit={buildAudit()} />)

    const failing = [
      'All writers migrated',
      'Batch queue empty',
      'Balance drain backlog cleared',
      'Projection backlog cleared',
      'No in-flight billing sessions',
    ]
    for (const label of failing) {
      expect(rowOf(label)).toHaveAttribute('data-passed', 'false')
    }
    expect(screen.getAllByText('Not met')).toHaveLength(failing.length)

    const passing = [
      'No balance drain in flight',
      'Maintenance backfill complete',
      'Redis epoch consistent',
      'Cluster drain acknowledged',
    ]
    for (const label of passing) {
      expect(rowOf(label)).toHaveAttribute('data-passed', 'true')
    }
    expect(screen.getAllByText('Passed')).toHaveLength(passing.length)
  })

  test('lists unmigrated writers and backlog counts next to failing checks', () => {
    render(<AuditChecklist audit={buildAudit()} />)

    expect(screen.getByText('Unmigrated writers')).toBeInTheDocument()
    expect(screen.getByText('delta_update_user_quota')).toBeInTheDocument()
    expect(screen.queryByText('mutate_user_quota')).not.toBeInTheDocument()

    expect(screen.getAllByText('3 pending').length).toBeGreaterThan(0)
    expect(screen.getByText('5 in flight')).toBeInTheDocument()
  })

  test('renders all checks as passed when the audit is clean', () => {
    render(
      <AuditChecklist
        audit={buildAudit({
          can_enable: true,
          writers: [{ name: 'mutate_user_quota', migrated: true }],
          all_writers_migrated: true,
          batch_queue_empty: true,
          projection_pending: 0,
          balance_drain_pending: 0,
          inflight_sessions: 0,
          inflight_zero: true,
          missing_or_failed_checks: [],
        })}
      />
    )

    expect(screen.queryByText('Not met')).not.toBeInTheDocument()
    expect(screen.queryByText('Unmigrated writers')).not.toBeInTheDocument()
  })
})
