import { useTranslation } from 'react-i18next'

import { tokenBudgetEvidenceSchema } from '../lib/token-budget-evidence'

export function TokenBudgetEvidence(props: { evidence: unknown }) {
  const { t } = useTranslation()
  const result = tokenBudgetEvidenceSchema.safeParse(props.evidence)
  if (!result.success || !result.data.bound_source) return null
  const evidence = result.data
  const chat = evidence.bound_source === 'openai_chat_context_window'
  const total =
    evidence.actual_input != null && evidence.actual_output != null
      ? evidence.actual_input + evidence.actual_output
      : undefined
  const countText = (value: number | null | undefined) =>
    value == null ? t('Not confirmed') : value.toLocaleString()
  return (
    <section
      className='min-w-0 space-y-2 rounded-md border p-3 text-xs'
      aria-label={t('Reservation and actual usage')}
    >
      <p className='font-medium'>{t('Reservation and actual usage')}</p>
      <p className='break-all'>
        {t('Reservation bound source')}: {evidence.bound_source}
      </p>
      {chat && (
        <p>
          {t(
            'Chat reserves the 1,050,000-token context ceiling, not a measured input count. The total includes input and completion; the output cap is not added twice.'
          )}
        </p>
      )}
      <dl className='grid min-w-0 grid-cols-2 gap-2'>
        <div>
          <dt>{t('Input reservation bound')}</dt>
          <dd className='break-all'>
            {countText(evidence.input_tokens_bound)}
          </dd>
        </div>
        <div>
          <dt>{t('Output reservation cap')}</dt>
          <dd className='break-all'>{countText(evidence.max_output_tokens)}</dd>
        </div>
        <div>
          <dt>{t('Total reservation bound')}</dt>
          <dd className='break-all'>{countText(evidence.reserved)}</dd>
        </div>
        <div>
          <dt>{t('Reserved USD')}</dt>
          <dd className='break-all'>
            {evidence.fee_reserved_usd ?? t('Not available')}
          </dd>
        </div>
        <div>
          <dt>{t('Confirmed input tokens')}</dt>
          <dd className='break-all'>{countText(evidence.actual_input)}</dd>
        </div>
        <div>
          <dt>{t('Confirmed output tokens')}</dt>
          <dd className='break-all'>{countText(evidence.actual_output)}</dd>
        </div>
        <div>
          <dt>{t('Confirmed total tokens')}</dt>
          <dd className='break-all'>
            {countText(
              total != null && Number.isSafeInteger(total) ? total : undefined
            )}
          </dd>
        </div>
        {evidence.actual_fee_usd != null && (
          <div>
            <dt>{t('Confirmed API usage cost (USD)')}</dt>
            <dd className='break-all'>{evidence.actual_fee_usd}</dd>
          </div>
        )}
      </dl>
    </section>
  )
}
