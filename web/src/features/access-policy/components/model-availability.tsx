import { useTranslation } from 'react-i18next'

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
  return (
    <section
      className='min-w-0 space-y-2'
      aria-label={t('Public model availability')}
    >
      <h3 className='font-medium'>{t('Public model availability')}</h3>
      {props.models.length === 0 && (
        <p role='status'>{t('No public models to display.')}</p>
      )}
      <ul className='max-h-64 min-w-0 space-y-2 overflow-y-auto'>
        {props.models.map((item) => (
          <li
            key={item.model}
            className='bg-card min-w-0 space-y-2 rounded-lg border p-3'
          >
            <div className='flex flex-wrap items-start justify-between gap-2'>
              <p className='min-w-0 flex-1 font-mono text-sm break-all'>
                {item.model}
              </p>
              <Badge
                variant='outline'
                className={
                  item.allowed
                    ? 'text-foreground border-success/40'
                    : 'text-foreground border-destructive/40'
                }
              >
                {item.allowed ? t('Available') : t('Denied')}
              </Badge>
            </div>
            {!item.allowed && (
              <ul className='text-muted-foreground space-y-1 text-xs'>
                {(item.reasons.length
                  ? [...new Set(item.reasons)]
                  : ['unknown']
                ).map((reason) => (
                  <li key={reason}>
                    {Object.hasOwn(reasons, reason)
                      ? reasons[reason]
                      : t('Access is unavailable. Ask an administrator.')}
                  </li>
                ))}
              </ul>
            )}
          </li>
        ))}
      </ul>
    </section>
  )
}
