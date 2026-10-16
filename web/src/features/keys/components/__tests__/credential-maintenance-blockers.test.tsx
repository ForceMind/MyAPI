/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { CredentialMaintenanceBlockers } from '../credential-maintenance-blockers'

test('budget blockers tell Root to review budgets without disabling them automatically', () => {
  render(
    <CredentialMaintenanceBlockers
      blockers={[
        {
          code: 'token_budget_conflicts_with_subscription_share',
          state: 'blocked',
        },
        {
          code: 'fee_budget_conflicts_with_subscription_share',
          state: 'blocked',
        },
      ]}
    />
  )

  expect(
    screen.getByText('This Key still has an active budget mode')
  ).toBeInTheDocument()
  expect(
    screen.getByText(
      'Review the token or fee budget first. MyAPI will not disable either budget automatically.'
    )
  ).toBeInTheDocument()
  expect(
    screen.getAllByText('This Key still has an active budget mode')
  ).toHaveLength(1)
})

test('unresolved request blockers direct Root to accounting records without automatic refunds', () => {
  render(
    <CredentialMaintenanceBlockers
      blockers={[
        { code: 'source_usage_history_unknown', state: 'unknown' },
        { code: 'target_usage_history_unknown', state: 'unknown' },
      ]}
    />
  )

  expect(
    screen.getByText('Request accounting is still unresolved')
  ).toBeInTheDocument()
  expect(
    screen.getByText(
      'Check the related request and allocation records before retrying. Do not automatically retry, refund, or rewrite usage.'
    )
  ).toBeInTheDocument()
  expect(
    screen.queryByText(/create trusted target baselines/i)
  ).not.toBeInTheDocument()
})

test('baseline blockers direct Root to existing configuration and a trusted sample', () => {
  render(
    <CredentialMaintenanceBlockers
      blockers={[
        { code: 'target_baseline_missing', state: 'unknown' },
        { code: 'credential_evidence_stale', state: 'unknown' },
        { code: 'subscription_projection_pending', state: 'unknown' },
      ]}
    />
  )

  expect(
    screen.getByText('A trusted account baseline is not ready')
  ).toBeInTheDocument()
  expect(
    screen.getByText(
      'Use the existing window-share configuration and wait for a fresh trusted sample. MyAPI will not create pools automatically.'
    )
  ).toBeInTheDocument()
  expect(
    screen.getAllByText('A trusted account baseline is not ready')
  ).toHaveLength(1)
})

test('an unknown future blocker keeps an explicit unknown recovery path', () => {
  render(
    <CredentialMaintenanceBlockers
      blockers={[{ code: 'future_state_probe', state: 'unknown' }]}
    />
  )

  expect(
    screen.getByText('A required migration check is still unknown')
  ).toBeInTheDocument()
  expect(
    screen.getByText(
      'Keep this migration paused and reload current state before deciding what to do.'
    )
  ).toBeInTheDocument()
})
