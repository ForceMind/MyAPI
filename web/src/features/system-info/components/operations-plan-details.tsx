/* Copyright (C) 2026 ForceMind. Licensed under AGPL-3.0-or-later. */
import { useTranslation } from 'react-i18next'

import { operationsProofScope, type OperationsPlan } from '../operations-api'

export function OperationsPlanDetails(props: { plan: OperationsPlan }) {
  const { t, i18n } = useTranslation()
  const plan = props.plan
  const unknown = t('Unknown')
  const rows: [string, string | number][] = [
    [t('Upgrade plan ID'), plan.id],
    [t('Plan digest'), plan.digest],
    [t('Approval verification scope'), operationsProofScope(plan)],
    [t('Target instance'), plan.instance_id],
    [t('Plan owner ID'), plan.owner_id],
    [t('Management key ID'), plan.key_id],
    [t('Source image ID'), plan.source_image_id],
    [t('Target image ID'), plan.target_image_id],
    [t('Source container ID'), plan.source_container_id],
    [t('Target container ID'), plan.target_container_id || unknown],
    [t('Source configuration digest'), plan.source_config_digest],
    [t('Source commit SHA'), plan.source_sha],
    [t('Source tree SHA'), plan.tree_sha],
    [t('Build run ID'), plan.run_id],
    [t('Producer attempt'), plan.producer_attempt],
    [t('Artifact ID'), plan.artifact_id],
    [t('Manifest SHA256'), plan.manifest_sha256],
    [t('Source version'), plan.source_version || unknown],
    [t('Version'), plan.version],
    [t('Platform'), plan.platform],
    [t('Created'), new Date(plan.created_at * 1000).toLocaleString(i18n.language)],
    [t('Expires'), new Date(plan.expires_at * 1000).toLocaleString(i18n.language)],
    [t('Approved at'), plan.approved_at ? new Date(plan.approved_at * 1000).toLocaleString(i18n.language) : unknown],
    [t('Approval expires'), plan.approval_expires_at ? new Date(plan.approval_expires_at * 1000).toLocaleString(i18n.language) : unknown],
    [t('Backup created'), plan.backup_created ? t('Yes') : t('No')],
    [t('Target start attempted'), plan.target_start_attempted ? t('Yes') : t('No')],
    [t('Operation error code'), plan.code || unknown],
  ]
  if (typeof plan.source_mutation_attempted === 'boolean') rows.push([t('Source mutation attempted'), plan.source_mutation_attempted ? t('Yes') : t('No')])
  if (typeof plan.source_restart_disabled === 'boolean') rows.push([t('Source auto-restart disabled'), plan.source_restart_disabled ? t('Yes') : t('No')])
  return <dl className='grid min-w-0 gap-3 text-sm sm:grid-cols-2'>
    {rows.map(([label, value]) => <div className='min-w-0 space-y-1' key={label}><dt className='text-muted-foreground'>{label}</dt><dd className='font-mono [overflow-wrap:anywhere]'>{value}</dd></div>)}
  </dl>
}
