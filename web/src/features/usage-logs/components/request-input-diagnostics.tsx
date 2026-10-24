import { Alert02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { toIntlLocale } from '@/i18n/languages'

import type { LogOtherData } from '../types'

export function RequestInputDiagnostics(props: { other: LogOtherData | null }) {
  const { t, i18n } = useTranslation()
  const estimate = props.other?.request_input_estimate
  const limit = estimate?.limit_tokens ?? 0
  const validEstimate =
    estimate &&
    Number.isSafeInteger(estimate.input_tokens) &&
    estimate.input_tokens >= 0
  const contextExceeded =
    props.other?.upstream_error_code === 'context_length_exceeded'
  const rejected = props.other?.preflight_rejected === true
  if (!validEstimate && !contextExceeded && !rejected) return null
  return (
    <div className='min-w-0 space-y-3'>
      {(contextExceeded || rejected) && (
        <Alert>
          <HugeiconsIcon icon={Alert02Icon} aria-hidden='true' />
          <AlertTitle>
            {rejected ? t('Blocked before upstream') : t('Request failed')}
          </AlertTitle>
          <AlertDescription>
            {contextExceeded
              ? t(
                  'Input exceeds the upstream context window. Shorten the input and try again.'
                )
              : t(
                  'The request exceeded a configured input limit or could not be estimated.'
                )}
          </AlertDescription>
        </Alert>
      )}
      {validEstimate && (
        <div className='bg-muted/40 rounded-lg border p-3 text-sm'>
          <dl className='space-y-2'>
            <div className='flex flex-wrap justify-between gap-2'>
              <dt>{t('Estimated input Tokens')}</dt>
              <dd className='font-mono tabular-nums'>
                {estimate.input_tokens.toLocaleString(
                  toIntlLocale(i18n.language)
                )}
              </dd>
            </div>
            {Number.isSafeInteger(limit) && limit > 0 && (
              <div className='flex flex-wrap justify-between gap-2'>
                <dt>{t('Configured input limit')}</dt>
                <dd className='font-mono tabular-nums'>
                  {limit.toLocaleString(toIntlLocale(i18n.language))}
                </dd>
              </div>
            )}
          </dl>
          <p className='text-muted-foreground mt-2'>
            {t('Local estimate; not used as reported usage or for settlement.')}
          </p>
          {!estimate.complete && (
            <p className='text-muted-foreground mt-1'>
              {t('Text only; media Tokens are not included.')}
            </p>
          )}
        </div>
      )}
      {contextExceeded && (
        <p className='text-muted-foreground text-xs'>
          {t('Upstream error code')}: <code>context_length_exceeded</code>
        </p>
      )}
    </div>
  )
}
