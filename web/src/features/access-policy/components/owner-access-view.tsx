import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { getOwnerAccess } from '../api'
import type { AccessSession } from '../lib/session'
import { AccessPolicyErrorMessage } from './access-policy-error'
import { ModelAvailabilityList } from './model-availability'

export function OwnerAccessView(props: {
  tokenId: number
  scope: AccessSession
}) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: [
      'owner-access',
      props.scope.userId,
      props.scope.role,
      props.scope.sid,
      props.tokenId,
    ],
    queryFn: ({ signal }) => getOwnerAccess(props.tokenId, signal),
    retry: false,
    gcTime: 0,
    staleTime: 0,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  return (
    <div className='min-w-0 space-y-4' aria-busy={query.isFetching}>
      {props.scope.role < 10 && (
        <p className='text-muted-foreground text-sm'>
          {t('Only administrators can change assigned access.')}
        </p>
      )}
      {query.isPending && <p role='status'>{t('Loading...')}</p>}
      {query.isError && <AccessPolicyErrorMessage error={query.error} />}
      {!query.isError && query.data && (
        <>
          <p>
            {query.data.assigned
              ? t('Administrator assignment is present.')
              : t('No assignment. Existing access rules apply.')}
          </p>
          <ModelAvailabilityList models={query.data.models} />
        </>
      )}
      <Button
        type='button'
        variant='outline'
        disabled={query.isFetching}
        onClick={() => void query.refetch()}
      >
        {t('Refresh')}
      </Button>
    </div>
  )
}
