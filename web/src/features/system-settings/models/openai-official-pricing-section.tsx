import {
  useIsMutating,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatTimestampToDate } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import { SettingsSection } from '../components/settings-section'
import {
  fetchOfficialOpenAIPricing,
  fetchFrozenOpenAIPriceSource,
  saveOfficialOpenAIPriceSource,
} from './openai-official-pricing-api'
import { OpenAIPriceVersionForm } from './openai-price-version-form'
import { PricePublicationPanel } from './price-publication-panel'

export function OpenAIOfficialPricingSection() {
  const userID = useAuthStore((state) => state.auth.user?.id)
  const role = useAuthStore((state) => state.auth.user?.role)
  return (
    <PriceSourceSession key={`${userID}:${role}`} userID={userID} role={role} />
  )
}

function PriceSourceSession(props: {
  userID: number | undefined
  role: number | undefined
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const publicationsPending = useIsMutating({
    mutationKey: [
      'openai-price-publication',
      props.userID,
      props.role,
      'write',
    ],
  })
  const [selection, setSelection] = useState<
    { kind: 'latest' } | { kind: 'frozen'; digest: string }
  >({ kind: 'latest' })
  const queryKey = [
    'official-openai-price-source',
    props.userID,
    props.role,
    selection.kind,
    selection.kind === 'frozen' ? selection.digest : '',
  ]
  const query = useQuery({
    queryKey,
    queryFn: ({ signal }) =>
      selection.kind === 'latest'
        ? fetchOfficialOpenAIPricing(signal)
        : fetchFrozenOpenAIPriceSource(selection.digest, signal),
    enabled: false,
    retry: false,
    refetchOnWindowFocus: false,
  })
  const save = useMutation({
    mutationFn: () => saveOfficialOpenAIPriceSource(),
    retry: false,
    onSuccess: (data) => {
      queryClient.setQueryData(
        [
          'official-openai-price-source',
          props.userID,
          props.role,
          'frozen',
          data.content_sha256,
        ],
        data
      )
      setSelection({ kind: 'frozen', digest: data.content_sha256 })
    },
  })
  const busy = query.isFetching || save.isPending || publicationsPending > 0
  const readSource = (digest?: string) => {
    save.reset()
    const next = digest
      ? { kind: 'frozen' as const, digest }
      : { kind: 'latest' as const }
    setSelection(next)
    void queryClient
      .fetchQuery({
        queryKey: [
          'official-openai-price-source',
          props.userID,
          props.role,
          next.kind,
          digest ?? '',
        ],
        queryFn: ({ signal }) =>
          digest
            ? fetchFrozenOpenAIPriceSource(digest, signal)
            : fetchOfficialOpenAIPricing(signal),
        retry: false,
        staleTime: 0,
      })
      .catch(() => {})
  }
  const title = t('OpenAI official pricing source')
  const formatRate = (value: string | null | undefined) =>
    value == null ? t('Not quoted') : `$${value}`

  if (props.role !== 100) {
    return (
      <SettingsSection title={title}>
        <Alert>
          <AlertDescription>
            {t('Root access is required for this price source.')}
          </AlertDescription>
        </Alert>
      </SettingsSection>
    )
  }
  return (
    <SettingsSection title={title} className='min-w-0'>
      <Alert>
        <AlertDescription>
          {t('Fetching or saving a source does not change effective prices.')}
        </AlertDescription>
      </Alert>
      <div className='flex flex-wrap items-center gap-3'>
        <Button type='button' disabled={busy} onClick={() => readSource()}>
          {query.isFetching && <Spinner aria-hidden='true' />}
          {query.isFetching ? t('Loading...') : t('Fetch official pricing')}
        </Button>
        <Button
          type='button'
          variant='outline'
          disabled={busy}
          onClick={() => save.mutate()}
        >
          {save.isPending && <Spinner aria-hidden='true' />}
          {save.isPending ? t('Saving...') : t('Save official source version')}
        </Button>
        <a
          href='https://developers.openai.com/api/docs/pricing'
          target='_blank'
          rel='noopener noreferrer'
          className='text-primary text-sm underline underline-offset-4'
        >
          {t('Official pricing page')}
        </a>
      </div>
      <OpenAIPriceVersionForm disabled={busy} onRead={readSource} />
      <PricePublicationPanel
        key={selection.kind === 'frozen' ? selection.digest : 'history'}
        digest={
          selection.kind === 'frozen' && query.data && !query.isError
            ? selection.digest
            : undefined
        }
        userID={props.userID}
        role={props.role}
      />
      {save.isError && (
        <Alert role='alert' variant='destructive'>
          <AlertDescription>
            {t('Could not save the source. Effective prices are unchanged.')}
          </AlertDescription>
        </Alert>
      )}
      {selection.kind === 'frozen' && query.data && !query.isError && (
        <p className='text-muted-foreground text-sm'>
          {t(
            'Frozen source loaded. Publishing requires a separate confirmation.'
          )}
        </p>
      )}
      {query.isError && (
        <Alert role='alert' variant='destructive'>
          <AlertDescription>
            {t('Official source unavailable. Effective prices are unchanged.')}
          </AlertDescription>
        </Alert>
      )}
      {!query.data && !query.isError && !query.isFetching && (
        <p className='text-muted-foreground text-sm'>
          {t(
            'Fetch official pricing to inspect the source. No prices will be applied.'
          )}
        </p>
      )}
      {query.data && !query.isError && (
        <>
          <dl className='grid min-w-0 gap-2 text-sm'>
            <div>
              <dt className='text-muted-foreground'>{t('Fetched at')}</dt>
              <dd>{formatTimestampToDate(query.data.fetched_at)}</dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>{t('Source version')}</dt>
              <dd className='font-mono break-all'>
                {query.data.content_sha256}
              </dd>
            </div>
          </dl>
          <p className='text-muted-foreground text-sm'>
            {t('Standard text-token prices in USD per 1M tokens.')}
          </p>
          <p className='text-muted-foreground text-xs lg:hidden'>
            {t('Scroll horizontally to view all prices.')}
          </p>
          <div className='max-w-full min-w-0 overflow-x-auto'>
            <Table className='min-w-[680px]' tabIndex={0} aria-label={title}>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Model')}</TableHead>
                  <TableHead>{t('Context')}</TableHead>
                  <TableHead>{t('Input')}</TableHead>
                  <TableHead>{t('Cache Read')}</TableHead>
                  <TableHead>{t('Cache Write')}</TableHead>
                  <TableHead>{t('Output')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {query.data.models.flatMap((model) => [
                  <TableRow key={`${model.model}-short`}>
                    <TableCell className='max-w-64 break-words whitespace-normal'>
                      {model.source_label}
                    </TableCell>
                    <TableCell>{t('Short context')}</TableCell>
                    <TableCell>
                      {formatRate(model.short_context.input_usd_per_million)}
                    </TableCell>
                    <TableCell>
                      {formatRate(
                        model.short_context.cached_input_usd_per_million
                      )}
                    </TableCell>
                    <TableCell>
                      {formatRate(
                        model.short_context.cache_write_usd_per_million
                      )}
                    </TableCell>
                    <TableCell>
                      {formatRate(model.short_context.output_usd_per_million)}
                    </TableCell>
                  </TableRow>,
                  <TableRow key={`${model.model}-long`}>
                    <TableCell className='max-w-64 break-words whitespace-normal'>
                      {model.source_label}
                    </TableCell>
                    <TableCell>{t('Long context')}</TableCell>
                    <TableCell>
                      {formatRate(model.long_context?.input_usd_per_million)}
                    </TableCell>
                    <TableCell>
                      {formatRate(
                        model.long_context?.cached_input_usd_per_million
                      )}
                    </TableCell>
                    <TableCell>
                      {formatRate(
                        model.long_context?.cache_write_usd_per_million
                      )}
                    </TableCell>
                    <TableCell>
                      {formatRate(model.long_context?.output_usd_per_million)}
                    </TableCell>
                  </TableRow>,
                ])}
              </TableBody>
            </Table>
          </div>
        </>
      )}
    </SettingsSection>
  )
}
