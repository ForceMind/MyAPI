import { useTranslation } from 'react-i18next'

import type { UsageReview } from '../usage-review-api'

export function UsageSettlementStatus(props: {
  status: UsageReview['settlement_status']
}) {
  const { t } = useTranslation()
  let title: string
  let description: string
  switch (props.status) {
    case 'pending':
      title = t('Automatic settlement pending')
      description = t(
        'This request already has an automatic settlement record. Manual input is unavailable here.'
      )
      break
    case 'applied_journal_pending':
      title = t('Settlement applied; record finalization pending')
      description = t(
        'The verified usage has been settled. The request record still needs finalization.'
      )
      break
    case 'applied':
      title = t('Settlement applied')
      description = t('The verified usage has been settled.')
      break
    case 'manual':
      title = t('Settlement needs administrator attention')
      description = t(
        'Automatic settlement requires investigation. Manual reconciliation is unavailable here.'
      )
      break
    default:
      return null
  }
  return (
    <div role='status' className='min-w-0 space-y-1 text-sm'>
      <p className='font-medium'>{title}</p>
      <p className='text-muted-foreground text-xs'>{description}</p>
    </div>
  )
}
