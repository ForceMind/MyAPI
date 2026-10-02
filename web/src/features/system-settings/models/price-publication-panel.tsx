import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatTimestampToDate } from '@/lib/format'

import {
  applyPricePublication,
  fetchPricePublicationPreview,
  fetchPricePublicationState,
  newPricePublicationID,
  type PricePublicationRequest,
} from './price-publication-api'
import { PricePublicationConfirmation } from './price-publication-confirmation'

type PendingChange = {
  request: PricePublicationRequest
  title: string
  before?: string
  after?: string
}

export function PricePublicationPanel(props: {
  digest?: string
  userID: number | undefined
  role: number | undefined
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [change, setChange] = useState<PendingChange | null>(null)
  const [opened, setOpened] = useState(false)
  const queryPrefix = ['openai-price-publication', props.userID, props.role]
  const preview = useQuery({
    queryKey: [...queryPrefix, 'preview', props.digest],
    queryFn: ({ signal }) =>
      fetchPricePublicationPreview(props.digest ?? '', signal),
    enabled: opened && props.role === 100 && !!props.digest,
    retry: false,
    refetchOnWindowFocus: false,
  })
  const state = useQuery({
    queryKey: [...queryPrefix, 'state'],
    queryFn: ({ signal }) => fetchPricePublicationState(signal),
    enabled: opened && props.role === 100,
    retry: false,
    refetchOnWindowFocus: false,
  })
  const mutation = useMutation({
    mutationKey: [...queryPrefix, 'write'],
    mutationFn: applyPricePublication,
    retry: false,
    onSuccess: () => {
      setChange(null)
      void client.invalidateQueries({ queryKey: queryPrefix })
    },
  })
  if (props.role !== 100) return null
  const busy = preview.isFetching || state.isFetching || mutation.isPending
  const disabled = busy || !!change || !state.data?.runtime_ready
  const prepare = (
    draft: Omit<PendingChange, 'request'> & {
      request: Omit<PricePublicationRequest, 'id' | 'confirmed'>
    }
  ) => {
    mutation.reset()
    setChange({
      ...draft,
      request: {
        ...draft.request,
        id: newPricePublicationID(),
        confirmed: true,
      },
    })
  }
  const errorCode =
    mutation.error instanceof Error ? mutation.error.message : ''
  let errorText = t(
    'Price operation failed. Retry the same operation or refresh its status.'
  )
  if (
    errorCode === 'price_publication_conflict' ||
    errorCode === 'price_publication_locked'
  ) {
    errorText = t(
      'Prices changed or are locked. Cancel and refresh before reviewing again.'
    )
  }
  if (errorCode === 'price_publication_committed_runtime_unavailable') {
    errorText = t(
      'Saved, but pricing is unavailable until the server reloads. Refresh status; do not assume rollback.'
    )
  }
  return (
    <>
      <Button
        type='button'
        variant='outline'
        aria-expanded={opened}
        disabled={mutation.isPending}
        onClick={() => {
          setOpened(!opened)
          if (opened) {
            setChange(null)
            mutation.reset()
          }
        }}
      >
        {t('Effective price publication')}
      </Button>
      {opened && (
        <section
          className='min-w-0 space-y-3 rounded-md border p-3'
          aria-label={t('Effective price publication')}
        >
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <h3 className='font-medium'>{t('Effective price publication')}</h3>
            <Button
              type='button'
              variant='outline'
              disabled={busy}
              onClick={() => {
                void client.invalidateQueries({ queryKey: queryPrefix })
              }}
            >
              {t('Refresh')}
            </Button>
          </div>
          <Alert>
            <AlertDescription>
              {t(
                'Standard text reference tariffs only, not actual upstream bills. Multimodal, tools and other tiers are unsupported.'
              )}
            </AlertDescription>
          </Alert>
          {(preview.isError || state.isError) && (
            <Alert variant='destructive'>
              <AlertDescription>
                {t('Failed to load price publication state.')}
              </AlertDescription>
            </Alert>
          )}
          {state.data && !state.data.runtime_ready && (
            <Alert variant='destructive'>
              <AlertDescription>
                {t(
                  'Saved, but pricing is unavailable until the server reloads. Refresh status; do not assume rollback.'
                )}
              </AlertDescription>
            </Alert>
          )}
          {mutation.isError && (
            <Alert variant='destructive' role='alert'>
              <AlertDescription>{errorText}</AlertDescription>
            </Alert>
          )}
          {mutation.isSuccess && (
            <p role='status' className='text-sm'>
              {t('Price operation saved.')}
            </p>
          )}
          {change && (
            <PricePublicationConfirmation
              key={change.request.id}
              title={change.title}
              before={change.before}
              after={change.after}
              pending={mutation.isPending}
              onCancel={() => {
                setChange(null)
                mutation.reset()
              }}
              onConfirm={() => mutation.mutate(change.request)}
            />
          )}
          {preview.data && !preview.isError && (
            <div
              className='max-h-96 overflow-auto'
              tabIndex={0}
              aria-label={t('Price publication candidates')}
            >
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Model')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead>{t('Actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {preview.data.rows.map((row) => {
                    let status = t('Source not qualified')
                    if (row.eligible) status = t('Available')
                    if (row.locked) status = t('Locked')
                    return (
                      <TableRow key={row.model}>
                        <TableCell className='max-w-56 break-all whitespace-normal'>
                          {row.model}
                        </TableCell>
                        <TableCell>{status}</TableCell>
                        <TableCell>
                          <div className='flex flex-wrap gap-2'>
                            <Button
                              type='button'
                              size='sm'
                              disabled={disabled || !row.eligible}
                              aria-label={`${t('Review price change')}: ${row.model}`}
                              onClick={() => {
                                if (!row.candidate || !preview.data) return
                                prepare({
                                  title: row.model,
                                  before: `${row.current_mode}\n${row.current_expression}`,
                                  after: `${row.candidate.expression}\n${t('Locked')}`,
                                  request: {
                                    action: 'publish',
                                    expected_digest:
                                      preview.data.expected_digest,
                                    source_sha256: preview.data.source_sha256,
                                    models: [
                                      { model: row.model, locked: true },
                                    ],
                                  },
                                })
                              }}
                            >
                              {t('Review price change')}
                            </Button>
                            {row.current_mode === 'tiered_expr' && (
                              <Button
                                type='button'
                                size='sm'
                                variant='outline'
                                disabled={disabled}
                                aria-label={`${row.locked ? t('Unlock') : t('Lock')}: ${row.model}`}
                                onClick={() => {
                                  if (!preview.data) return
                                  prepare({
                                    title: `${row.locked ? t('Unlock') : t('Lock')}: ${row.model}`,
                                    before: row.current_expression,
                                    request: {
                                      action: 'lock',
                                      expected_digest:
                                        preview.data.expected_digest,
                                      models: [
                                        {
                                          model: row.model,
                                          locked: !row.locked,
                                        },
                                      ],
                                    },
                                  })
                                }}
                              >
                                {row.locked ? t('Unlock') : t('Lock')}
                              </Button>
                            )}
                          </div>
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
          )}
          {state.data && state.data.receipts.length > 0 && (
            <div className='space-y-2'>
              <h4 className='text-sm font-medium'>
                {t('Publication history')}
              </h4>
              {state.data.receipts.map((receipt) => (
                <div
                  key={receipt.id}
                  className='flex flex-wrap items-center gap-2 text-xs'
                >
                  <span>{formatTimestampToDate(receipt.created_at)}</span>
                  <code>{receipt.id.slice(0, 12)}</code>
                  <Button
                    type='button'
                    size='sm'
                    variant='outline'
                    disabled={
                      disabled ||
                      receipt.revision !== state.data?.snapshot.state.revision
                    }
                    aria-label={`${t('Rollback')}: ${receipt.id}`}
                    onClick={() => {
                      if (!state.data) return
                      prepare({
                        title: `${t('Rollback')}: ${receipt.id}`,
                        before: t(
                          'Rollback only restores an unchanged price generation; later edits are protected.'
                        ),
                        request: {
                          action: 'rollback',
                          rollback_of: receipt.id,
                          expected_digest: state.data.expected_digest,
                        },
                      })
                    }}
                  >
                    {t('Rollback')}
                  </Button>
                </div>
              ))}
            </div>
          )}
        </section>
      )}
    </>
  )
}
