import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { formatTimestampToDate } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import {
  fetchOpenAIPriceCheck,
  setOpenAIPriceCheckEnabled,
  startOpenAIPriceCheck,
} from './openai-price-check-api'
import { OpenAIPriceCheckStatus } from './openai-price-check-status'

export function OpenAIPriceCheckPanel(props: {
  userID?: number
  role?: number
  disabled: boolean
  onRead: (digest: string) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [opened, setOpened] = useState(false)
  const sending = useRef(false)
  const key = ['openai-price-check', props.userID, props.role]
  const query = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => fetchOpenAIPriceCheck(signal),
    enabled: opened && props.role === 100,
    retry: false,
    refetchOnWindowFocus: false,
    refetchInterval: (state) =>
      ['pending', 'running'].includes(
        state.state.data?.last_attempt_status ?? ''
      )
        ? 3000
        : false,
  })
  const mutation = useMutation({
    mutationFn: async (action: 'check' | boolean) => {
      await client.cancelQueries({ queryKey: key })
      const user = useAuthStore.getState().auth.user
      if (!user || user.id !== props.userID || user.role !== 100) {
        throw new Error('Session changed')
      }
      if (action === 'check') return startOpenAIPriceCheck()
      await setOpenAIPriceCheckEnabled(action)
      return null
    },
    retry: false,
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: key })
    },
    onSettled: () => {
      sending.current = false
    },
  })
  if (props.role !== 100) return null
  const busy = props.disabled || query.isFetching || mutation.isPending
  const data = query.data
  const active = ['pending', 'running'].includes(
    query.data?.last_attempt_status ?? ''
  )
  const submit = (action: 'check' | boolean) => {
    if (sending.current || busy) return
    sending.current = true
    mutation.mutate(action)
  }
  return (
    <>
      <Button
        className='h-auto min-h-9 max-w-full whitespace-normal'
        type='button'
        variant='outline'
        aria-expanded={opened}
        disabled={mutation.isPending}
        onClick={() => setOpened(!opened)}
      >
        {t('Official source checks')}
      </Button>
      {opened && (
        <section
          className='min-w-0 space-y-3 rounded-md border p-3'
          aria-label={t('Official source checks')}
        >
          <p className='text-muted-foreground text-sm'>
            {t(
              'Checks save source evidence for Root review. They never publish prices or override locks.'
            )}
          </p>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Scheduled checks are off by default. When enabled, checks run every 24 hours; evidence is stale after 72 hours without a successful check.'
            )}
          </p>
          <div className='flex flex-wrap gap-2'>
            <Button
              className='h-auto min-h-9 max-w-full whitespace-normal'
              type='button'
              disabled={
                busy ||
                active ||
                !query.data ||
                query.isError ||
                mutation.isError
              }
              onClick={() => submit('check')}
            >
              {active ? t('Processing...') : t('Check official source now')}
            </Button>
            <Button
              className='h-auto min-h-9 max-w-full whitespace-normal'
              type='button'
              variant='outline'
              disabled={busy}
              onClick={() => {
                mutation.reset()
                void query.refetch()
              }}
            >
              {t('Refresh check status')}
            </Button>
            {data && !query.isError && (
              <Button
                className='h-auto min-h-9 max-w-full whitespace-normal'
                type='button'
                variant='outline'
                disabled={busy || mutation.isError}
                onClick={() => submit(!data.enabled)}
              >
                {data.enabled
                  ? t('Disable daily source checks')
                  : t('Enable daily source checks')}
              </Button>
            )}
          </div>
          {query.isPending && <p role='status'>{t('Loading...')}</p>}
          {query.isError && (
            <Alert role='alert' variant='destructive'>
              <AlertDescription>
                {t(
                  'Could not load source check status. Refresh before making changes.'
                )}
              </AlertDescription>
            </Alert>
          )}
          {mutation.isError && (
            <Alert role='alert' variant='destructive'>
              <AlertDescription>
                {t(
                  'Source check request failed. Refresh status before retrying; saved evidence and effective prices are unchanged.'
                )}
              </AlertDescription>
            </Alert>
          )}
          {mutation.isSuccess && mutation.data && (
            <p role='status' className='text-sm'>
              {mutation.data.created
                ? t('Source check queued.')
                : t('An existing source check was reused.')}
            </p>
          )}
          {data && !query.isError && (
            <>
              <OpenAIPriceCheckStatus state={data} />
              {data.pending_source_sha256 && (
                <Button
                  className='h-auto min-h-9 max-w-full whitespace-normal'
                  type='button'
                  variant='outline'
                  disabled={busy}
                  onClick={() => props.onRead(data.pending_source_sha256)}
                >
                  {t('Review saved source')}
                </Button>
              )}
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Last successful check is separate from source creation and price publication. An unchanged source can have a new successful check.'
                )}
              </p>
              {data.next_check_at > 0 && data.enabled && (
                <p className='text-xs'>
                  {t('Next scheduled check')}:{' '}
                  {formatTimestampToDate(data.next_check_at)}
                </p>
              )}
            </>
          )}
        </section>
      )}
    </>
  )
}
