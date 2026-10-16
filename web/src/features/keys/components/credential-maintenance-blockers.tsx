/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { TFunction } from 'i18next'
import { AlertTriangle } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

import type { CredentialMigrationBlocker } from '../credential-maintenance-api'

type BlockerCategory =
  | 'identity'
  | 'baseline'
  | 'pending'
  | 'budget'
  | 'capacity'
  | 'policy'
  | 'windows'
  | 'same-subject'
  | 'single-credential'
  | 'target-unavailable'
  | 'fallback'

type GroupedBlocker = {
  category: BlockerCategory
  state: CredentialMigrationBlocker['state']
}

const CATEGORY_BY_CODE: Record<string, BlockerCategory> = {
  credential_subject_unknown: 'identity',
  credential_evidence_unverified: 'identity',
  identity_keyring_unavailable: 'identity',
  source_credential_baseline_mismatch: 'identity',
  source_credential_subject_changed: 'identity',
  source_channel_ambiguous: 'identity',
  target_credential_baseline_mismatch: 'identity',

  credential_evidence_stale: 'baseline',
  subscription_baseline_missing: 'baseline',
  subscription_baseline_not_ready: 'baseline',
  subscription_generation_mismatch: 'baseline',
  subscription_pool_invalid: 'baseline',
  subscription_projection_pending: 'baseline',
  subscription_snapshot_missing: 'baseline',
  subscription_snapshot_untrusted: 'baseline',
  target_baseline_missing: 'baseline',

  source_usage_history_unknown: 'pending',
  target_usage_history_unknown: 'pending',
  token_budget_pending_unknown: 'pending',
  token_budget_reservation_unknown: 'pending',

  fee_budget_conflicts_with_subscription_share: 'budget',
  token_budget_conflicts_with_subscription_share: 'budget',

  subscription_capacity_invalid: 'capacity',
  target_capacity_exceeded: 'capacity',

  assigned_policy_unknown: 'policy',
  token_policy_disabled: 'policy',
  token_policy_target_denied: 'policy',
  user_policy_disabled: 'policy',
  user_policy_target_denied: 'policy',

  source_pool_set_incomplete: 'windows',
  source_window_count_unsupported: 'windows',
  source_window_duplicate: 'windows',
  target_share_mapping_mismatch: 'windows',
  target_window_duplicate: 'windows',
  target_window_set_mismatch: 'windows',

  target_subject_matches_source: 'same-subject',

  single_codex_credential_required: 'single-credential',
  target_channel_ineligible: 'single-credential',

  source_allocation_invalid: 'baseline',
  source_allocation_missing: 'baseline',
  source_pool_missing: 'baseline',
  source_pool_set_invalid: 'baseline',
  subscription_allocation_duplicate: 'baseline',
  subscription_allocation_invalid: 'baseline',
  subscription_allocation_owner_mismatch: 'baseline',
  subscription_usage_invalid: 'baseline',
  subscription_usage_missing: 'baseline',

  migration_target_unavailable: 'target-unavailable',
}

function groupBlockers(
  blockers: CredentialMigrationBlocker[]
): GroupedBlocker[] {
  const grouped = new Map<BlockerCategory, GroupedBlocker>()
  for (const blocker of blockers) {
    const category = CATEGORY_BY_CODE[blocker.code] ?? 'fallback'
    const current = grouped.get(category)
    if (!current) {
      grouped.set(category, { category, state: blocker.state })
      continue
    }
    if (blocker.state === 'unknown') current.state = 'unknown'
  }
  return [...grouped.values()]
}

function blockerGuidance(blocker: GroupedBlocker, t: TFunction) {
  switch (blocker.category) {
    case 'identity':
      return {
        title: t('The current credential identity is not verified'),
        description: t(
          'Verify the identity registry and current channel association, run a fresh account sample, then reload maintenance data.'
        ),
      }
    case 'baseline':
      return {
        title: t('A trusted account baseline is not ready'),
        description: t(
          'Use the existing window-share configuration and wait for a fresh trusted sample. MyAPI will not create pools automatically.'
        ),
      }
    case 'pending':
      return {
        title: t('Request accounting is still unresolved'),
        description: t(
          'Check the related request and allocation records before retrying. Do not automatically retry, refund, or rewrite usage.'
        ),
      }
    case 'budget':
      return {
        title: t('This Key still has an active budget mode'),
        description: t(
          'Review the token or fee budget first. MyAPI will not disable either budget automatically.'
        ),
      }
    case 'capacity':
      return {
        title: t('Target window capacity needs review'),
        description: t(
          'Verify existing target allocations. If capacity is insufficient, reduce other shares or choose another pool; this migration will not change percentages.'
        ),
      }
    case 'policy':
      return {
        title: t('Access policy needs review'),
        description: t(
          'Verify that both the user and Key policies allow the selected target, then create a new preview.'
        ),
      }
    case 'windows':
      return {
        title: t('Primary and secondary windows do not match'),
        description: t(
          'Configure matching existing target pools for every enabled source window. MyAPI will not create or close pools automatically.'
        ),
      }
    case 'same-subject':
      return {
        title: t('Target account matches the current account'),
        description: t(
          'Choose a channel that resolves to a different upstream account.'
        ),
      }
    case 'single-credential':
      return {
        title: t('A supported single-credential Codex channel is required'),
        description: t(
          'Choose an enabled Codex channel with exactly one credential.'
        ),
      }
    case 'target-unavailable':
      return {
        title: t('No eligible target account is ready'),
        description: t(
          'Review the available target channels and any target-specific items below, then reload maintenance data.'
        ),
      }
    case 'fallback':
      if (blocker.state === 'unknown') {
        return {
          title: t('A required migration check is still unknown'),
          description: t(
            'Keep this migration paused and reload current state before deciding what to do.'
          ),
        }
      }
      return {
        title: t('An unrecognized rule blocks this migration'),
        description: t(
          'Review the current configuration and reload maintenance data before trying again.'
        ),
      }
  }
}

export function CredentialMaintenanceBlockers(props: {
  blockers: CredentialMigrationBlocker[]
}) {
  const { t } = useTranslation()
  const blockers = groupBlockers(props.blockers)

  return (
    <Alert>
      <AlertTriangle aria-hidden='true' />
      <AlertTitle>{t('Account migration is unavailable')}</AlertTitle>
      <AlertDescription>
        <p>
          {t(
            'Resolve the items below, then reload maintenance data and create a new preview.'
          )}
        </p>
        {blockers.length > 0 && (
          <ul className='mt-3 flex flex-col gap-3'>
            {blockers.map((blocker) => {
              const copy = blockerGuidance(blocker, t)
              return (
                <li key={blocker.category} className='border-l-2 pl-3'>
                  <p className='text-foreground font-medium'>{copy.title}</p>
                  <p className='mt-0.5'>{copy.description}</p>
                </li>
              )
            })}
          </ul>
        )}
      </AlertDescription>
    </Alert>
  )
}
