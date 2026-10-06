import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import {
  getAssignedPolicy,
  previewAssignedPolicy,
  removeAssignedPolicy,
  saveAssignedPolicy,
} from '../api'
import { useAccessSession, type AccessSession } from '../lib/session'
import type { PolicyCandidate, PolicyTarget } from '../types'
import { AccessPolicyErrorMessage } from './access-policy-error'
import { AccessPolicyForm } from './access-policy-form'
import { ModelAvailabilityList } from './model-availability'

type Action =
  | { kind: 'save' | 'preview'; candidate: PolicyCandidate }
  | { kind: 'remove' }

export function AccessPolicyEditor(props: {
  target: PolicyTarget
  scope: AccessSession
  onSaved?: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const session = useAccessSession(props.scope)
  const sending = useRef(false)
  const [generation, setGeneration] = useState(0)
  const queryKey = [
    'assigned-access',
    props.scope.userId,
    props.scope.role,
    props.scope.sid,
    props.target.subject,
    props.target.id,
    props.target.ownerUserId,
  ]
  const query = useQuery({
    queryKey,
    queryFn: ({ signal }) => getAssignedPolicy(props.target, signal),
    enabled: props.scope.role >= 10,
    retry: false,
    gcTime: 0,
    staleTime: 0,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  const mutation = useMutation({
    mutationFn: async (action: Action) => {
      if (props.scope.role < 10 || !query.data || !session.isCurrent()) {
        throw new Error('Session changed')
      }
      if (action.kind === 'preview') {
        return {
          kind: 'preview' as const,
          data: await session.run((signal) =>
            previewAssignedPolicy(props.target, action.candidate, signal)
          ),
        }
      }
      const data = await session.run((signal) => {
        if (action.kind === 'remove') {
          return removeAssignedPolicy(props.target, query.data.revision, signal)
        }
        return saveAssignedPolicy(
          props.target,
          { ...action.candidate, expected_revision: query.data.revision },
          signal
        )
      })
      return { kind: 'saved' as const, data }
    },
    retry: false,
    onSuccess: (result) => {
      if (!session.isCurrent() || result.kind !== 'saved') return
      client.setQueryData(queryKey, result.data)
      void client.invalidateQueries({
        queryKey: [
          'owner-access',
          props.scope.userId,
          props.scope.role,
          props.scope.sid,
        ],
      })
      props.onSaved?.()
    },
    onSettled: () => {
      sending.current = false
    },
  })
  const busy = query.isFetching || mutation.isPending
  const locked = mutation.isError && mutation.variables?.kind !== 'preview'
  const perform = (action: Action): void => {
    if (busy || locked || sending.current || !session.isCurrent()) return
    sending.current = true
    mutation.mutate(action)
  }
  const data = query.isError ? undefined : query.data
  return (
    <div className='min-w-0 space-y-4' aria-busy={busy}>
      {query.isPending && <p role='status'>{t('Loading...')}</p>}
      {query.isError && <AccessPolicyErrorMessage error={query.error} />}
      {data && (
        <>
          <p className='text-sm'>
            {data.assigned
              ? t('Administrator assignment is present.')
              : t('No assignment. Existing access rules apply.')}
          </p>
          <AccessPolicyForm
            key={`${data.revision}:${generation}`}
            policy={data}
            busy={busy}
            locked={locked}
            onSave={(candidate) => perform({ kind: 'save', candidate })}
            onPreview={(candidate) => perform({ kind: 'preview', candidate })}
            onRemove={() => perform({ kind: 'remove' })}
            onChange={() => mutation.reset()}
          />
        </>
      )}
      {mutation.isError && (
        <AccessPolicyErrorMessage
          error={mutation.error}
          write={mutation.variables?.kind !== 'preview'}
        />
      )}
      {mutation.data?.kind === 'saved' && (
        <p role='status'>{t('Assignment saved.')}</p>
      )}
      {mutation.data?.kind === 'preview' && (
        <div className='min-w-0 space-y-3' aria-live='polite'>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Preview only. No changes have been saved and no access has been granted.'
            )}
          </p>
          <ModelAvailabilityList models={mutation.data.data.models} />
        </div>
      )}
      <Button
        type='button'
        variant='outline'
        disabled={busy}
        className='h-auto min-h-8 whitespace-normal'
        onClick={async () => {
          const result = await query.refetch()
          if (!session.isCurrent()) return
          if (result.isSuccess) {
            mutation.reset()
            setGeneration((value) => value + 1)
          }
        }}
      >
        {t('Refresh policy')}
      </Button>
    </div>
  )
}
