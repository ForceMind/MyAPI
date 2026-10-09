import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useAuthStore } from '@/stores/auth-store'

import { getPendingUsageReviews } from '../usage-review-api'
import { UsageReviewPanel } from './usage-review-panel'

export function PendingUsageReviews() {
  const actor = useAuthStore((state) => state.auth.user)
  if (!actor || actor.role !== 100) return null
  return (
    <PendingUsageReviewSession
      key={`${actor.id}:${actor.role}`}
      actorId={actor.id}
    />
  )
}

function PendingUsageReviewSession(props: { actorId: number }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [writer, setWriter] = useState<'authoritative' | 'legacy'>(
    'authoritative'
  )
  const [after, setAfter] = useState('0')
  const [selected, setSelected] = useState<string | null>(null)
  const query = useQuery({
    queryKey: ['pending-usage-reviews', props.actorId, writer, after],
    queryFn: ({ signal }) => getPendingUsageReviews(writer, after, signal),
    enabled: open && !selected,
    retry: false,
    refetchOnWindowFocus: false,
  })
  return (
    <>
      <Button
        variant='outline'
        onClick={() => {
          setAfter('0')
          setSelected(null)
          setOpen(true)
        }}
      >
        {t('Pending requests')}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent
          className='max-h-[85dvh] min-w-0 overflow-y-auto sm:max-w-2xl'
          showCloseButton={false}
        >
          <DialogHeader>
            <DialogTitle>{t('Pending requests')}</DialogTitle>
            <DialogDescription>
              {t('Review the settlement status before taking any action.')}
            </DialogDescription>
          </DialogHeader>
          {selected ? (
            <div className='min-w-0 space-y-3'>
              <Button
                variant='outline'
                onClick={() => {
                  setSelected(null)
                  void query.refetch()
                }}
              >
                {t('Back')}
              </Button>
              <p className='text-sm break-all'>
                {t('Request ID')}: {selected}
              </p>
              <UsageReviewPanel key={selected} requestId={selected} />
            </div>
          ) : (
            <div className='min-w-0 space-y-3'>
              <div className='flex flex-wrap gap-2'>
                <Button
                  variant={writer === 'authoritative' ? 'default' : 'outline'}
                  aria-pressed={writer === 'authoritative'}
                  onClick={() => {
                    setWriter('authoritative')
                    setAfter('0')
                  }}
                >
                  {t('Current reservations')}
                </Button>
                <Button
                  variant={writer === 'legacy' ? 'default' : 'outline'}
                  aria-pressed={writer === 'legacy'}
                  onClick={() => {
                    setWriter('legacy')
                    setAfter('0')
                  }}
                >
                  {t('Legacy reservations')}
                </Button>
                <Button
                  variant='outline'
                  disabled={query.isFetching}
                  onClick={() => {
                    setAfter('0')
                    void query.refetch()
                  }}
                >
                  {t('Refresh')}
                </Button>
              </div>
              {query.isError && <p role='alert'>{t('Operation failed')}</p>}
              {query.isFetching && <p role='status'>{t('Loading...')}</p>}
              {!query.isFetching && query.data?.items.length === 0 && (
                <p>{t('No requests on this page')}</p>
              )}
              {!query.isFetching &&
                query.data?.items.map((item) => (
                  <Button
                    key={item.request_id}
                    variant='outline'
                    className='h-auto w-full justify-start text-left break-all whitespace-normal'
                    onClick={() => setSelected(item.request_id)}
                  >
                    {t('Request ID')}: {item.request_id}
                  </Button>
                ))}
              <Button
                variant='outline'
                disabled={query.isFetching || !query.data?.next_after}
                onClick={() => {
                  if (query.data?.next_after) setAfter(query.data.next_after)
                }}
              >
                {t('Next')}
              </Button>
            </div>
          )}
          <Button variant='outline' onClick={() => setOpen(false)}>
            {t('Close')}
          </Button>
        </DialogContent>
      </Dialog>
    </>
  )
}
