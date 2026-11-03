import { useTranslation } from 'react-i18next'

import { SummaryDisclosure } from '@/components/summary-disclosure'
import { Badge } from '@/components/ui/badge'

import type { ModelAvailability } from '../types'

export function ModelAvailabilityList(props: { models: ModelAvailability[] }) {
  const { t } = useTranslation()
  const reasons: Record<string, string> = {
    credentials_revoked: t('The user or Key is no longer authorized.'),
    policy_disabled: t('An assigned policy is disabled.'),
    public_model_denied: t('This public model is outside the assigned scope.'),
    upstream_model_denied: t(
      'The upstream model is outside the assigned scope.'
    ),
    channel_denied: t('The route is outside the assigned channel scope.'),
    policy_protocol_unsupported: t(
      'This request path is not supported by assigned access.'
    ),
    no_available_route: t('No eligible route is currently available.'),
    legacy_policy_denied: t('Existing access rules deny this model.'),
    token_model_denied: t('The Key model limits deny this model.'),
  }
  const available = props.models.filter((item) => item.allowed)
  const denied = props.models.filter((item) => !item.allowed)
  const renderModel = (item: ModelAvailability) => {
    const reasonCodes = item.reasons.length ? [...new Set(item.reasons)] : ['unknown']
    const unknownReason = !item.allowed && reasonCodes.some((reason) => !Object.hasOwn(reasons, reason))
    return (
      <li key={item.model} className='min-w-0 border-b py-2 last:border-b-0'>
        <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
          <p className='min-w-0 flex-1 font-mono text-sm break-all'>{item.model}</p>
          <div className='flex shrink-0 flex-wrap gap-1'>
            <Badge variant='outline' className={item.allowed ? 'text-foreground border-success/40' : 'text-foreground border-destructive/40'}>
              {item.allowed ? t('Available') : t('Denied')}
            </Badge>
            {unknownReason && <Badge variant='outline'>{t('Unknown')}</Badge>}
          </div>
        </div>
        {!item.allowed && (
          <ul className='text-muted-foreground mt-1 space-y-1 text-xs'>
            {reasonCodes.map((reason) => (
              <li key={reason}>
                {Object.hasOwn(reasons, reason) ? reasons[reason] : t('Access is unavailable. Ask an administrator.')}
              </li>
            ))}
          </ul>
        )}
      </li>
    )
  }
  return (
    <section className='min-w-0 space-y-2' aria-label={t('Public model availability')}>
      <h3 className='font-medium'>{t('Public model availability')}</h3>
      {props.models.length === 0 && <p role='status'>{t('No public models to display.')}</p>}
      {denied.length > 0 && <ul className='min-w-0' aria-label={t('Denied models')}>{denied.map(renderModel)}</ul>}
      {available.length > 0 && (
        <SummaryDisclosure summary={t('Available models ({{count}})', { count: available.length })}>
          <ul className='min-w-0'>{available.map(renderModel)}</ul>
        </SummaryDisclosure>
      )}
    </section>
  )
}
