import { useTranslation } from 'react-i18next'

import { accessPolicyErrorKind } from '../api'

export function AccessPolicyErrorMessage(props: {
  error: unknown
  write?: boolean
}) {
  const { t } = useTranslation()
  const kind = accessPolicyErrorKind(props.error)
  let message = t('Unable to load access policy. Try refreshing.')
  if (props.write) {
    message = t(
      'The change could not be confirmed. Refresh before editing again.'
    )
  }
  if (kind === 'conflict') {
    message = t(
      'This policy changed elsewhere. Refresh and review before saving again.'
    )
  }
  if (kind === 'forbidden') {
    message = t(
      'You do not have permission to view or change this access policy.'
    )
  }
  if (kind === 'missing') {
    message = t('This user or Key is no longer available.')
  }
  if (kind === 'identity') {
    message = t('The policy does not belong to the selected user or Key.')
  }
  return (
    <p role='alert' className='text-destructive text-sm break-words'>
      {message}
    </p>
  )
}
