/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { ArrowLeft, Info } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import {
  formatQuotaDuration,
  quotaWindowLabel,
} from '@/features/channels/lib/quota-history'
import { useMediaQuery } from '@/hooks/use-media-query'
import { formatTimestampToDate } from '@/lib/format'

import type { SubscriptionSharePool } from '../subscription-share-api'
import type { ApiKey } from '../types'
import {
  SubscriptionSharePoolCard,
  SubscriptionShareWindowChangeReview,
} from './subscription-share-pool-card'

type PersonalSubscriptionShareFormProps = {
  apiKey: ApiKey
  pools: SubscriptionSharePool[]
  selectedPool?: SubscriptionSharePool
  currentPool?: SubscriptionSharePool
  selection: string
  percentage: string
  shareBasisPoints: number | null
  inputError: string
  confirmed: boolean
  windowChangeConfirmed: boolean
  busy: boolean
  saving: boolean
  loading: boolean
  loadError: boolean
  mustReload: boolean
  reloadedAfterUnknown: boolean
  saveDisabled: boolean
  onSelect: (value: string) => void
  onPercentageChange: (value: string) => void
  onConfirmedChange: (value: boolean) => void
  onWindowChangeConfirmed: (value: boolean) => void
  onReload: () => void
  onAdvanced: () => void
  onSave: () => void
  onCancel: () => void
}

export function PersonalSubscriptionShareForm(
  props: PersonalSubscriptionShareFormProps
) {
  const { t } = useTranslation()
  const desktop = useMediaQuery('(min-width: 1280px)')
  const [accountId, setAccountId] = useState(() =>
    props.selectedPool?.channel_id === undefined
      ? ''
      : String(props.selectedPool.channel_id)
  )
  const reloadNotice = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!props.mustReload) return
    reloadNotice.current?.scrollIntoView({ block: 'center', behavior: 'auto' })
    reloadNotice.current?.focus({ preventScroll: true })
  }, [props.mustReload])
  const pool = props.selectedPool
  const hasAccountIds = props.pools.every(
    (item) => item.channel_id !== undefined
  )
  const accounts = new Map(
    props.pools.map((item) => [item.channel_id, item.channel_name])
  )
  const availableWindows = hasAccountIds
    ? props.pools.filter((item) => String(item.channel_id) === accountId)
    : props.pools
  const otherBps = pool ? pool.allocated_bps - pool.share_bps : null
  const totalBps =
    otherBps !== null && props.shareBasisPoints !== null
      ? otherBps + props.shareBasisPoints
      : null
  const knownUsage = pool && pool.pool_id > 0 && pool.baseline_time > 0
  const estimatedPercent =
    knownUsage && props.shareBasisPoints !== null && props.shareBasisPoints > 0
      ? pool.estimated_used_micro / (props.shareBasisPoints * 100)
      : null
  const windowChangeRequired =
    pool?.replacement_required || pool?.rebaseline_required
  let actionLabel = t('Save changes')
  if (pool?.replacement_required) actionLabel = t('Replace window')
  if (pool?.rebaseline_required) actionLabel = t('Resume observation')
  const actions = (
    <div className='flex flex-col-reverse gap-3 sm:flex-row sm:flex-wrap'>
      <Button
        className='min-h-11 flex-1 sm:flex-none'
        variant='outline'
        type='button'
        disabled={props.saving}
        onClick={props.onCancel}
      >
        {t('Cancel')}
      </Button>
      <Button
        className='min-h-11 flex-1 sm:min-w-40 sm:flex-none'
        disabled={props.saveDisabled}
        type='submit'
      >
        {props.saving && <Spinner data-icon='inline-start' />}
        {props.saving ? t('Saving...') : actionLabel}
      </Button>
    </div>
  )
  return (
    // A region inside the Key inspector: the page already owns main#content.
    <section
      className='pc-page @container/share-page min-w-0 space-y-6'
      aria-labelledby='personal-share-title'
    >
      <header className='space-y-2'>
        <Button
          variant='ghost'
          className='-ml-3 max-w-full'
          disabled={props.saving}
          onClick={props.onCancel}
        >
          <ArrowLeft className='size-4 shrink-0' aria-hidden='true' />
          <span className='truncate'>
            {t('API Keys')} / {props.apiKey.name}
          </span>
        </Button>
        <h1
          id='personal-share-title'
          className='text-2xl font-bold tracking-tight sm:text-[32px]'
        >
          {t('Adjust window share')}
        </h1>
        <p className='text-muted-foreground'>
          {t(
            'Keep your existing Key and manage its approximate native-window share.'
          )}
        </p>
      </header>
      {props.loading && (
        <div role='status' className='space-y-3'>
          <span className='sr-only'>{t('Loading...')}</span>
          <Skeleton className='h-80 w-full rounded-xl' />
        </div>
      )}
      {props.loadError && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Could not load subscription shares.')}</AlertTitle>
          <AlertDescription>
            <Button
              variant='outline'
              disabled={props.busy}
              onClick={props.onReload}
            >
              {t('Retry')}
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {!props.loading && !props.loadError && props.pools.length === 0 && (
        <Empty className='bg-card rounded-xl border'>
          <EmptyHeader>
            <EmptyTitle>
              {t('No eligible subscription windows are available.')}
            </EmptyTitle>
          </EmptyHeader>
        </Empty>
      )}
      {!props.loading && !props.loadError && props.pools.length > 0 && (
        <form
          id='personal-share-form'
          className='grid min-w-0 gap-5 @4xl/share-page:grid-cols-[minmax(0,1.7fr)_minmax(260px,1fr)]'
          aria-label={t('Adjust window share')}
          onSubmit={(event) => {
            event.preventDefault()
            if (!props.saveDisabled) props.onSave()
          }}
        >
          <div className='min-w-0'>
            <Card className='min-w-0 gap-6 p-5 sm:p-7'>
              <CardContent className='space-y-6 px-0'>
                <FieldSet>
                  <FieldLegend>{t('Basic information')}</FieldLegend>
                  <FieldGroup className='gap-4'>
                    <Field>
                      <FieldLabel htmlFor='personal-share-name'>
                        {t('Key name')}
                      </FieldLabel>
                      <Input
                        id='personal-share-name'
                        value={props.apiKey.name}
                        readOnly
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor='personal-share-id'>
                        {t('Key identifier')}
                      </FieldLabel>
                      <Input
                        id='personal-share-id'
                        value={`#${props.apiKey.id}`}
                        readOnly
                        className='bg-muted/60 text-muted-foreground'
                      />
                    </Field>
                  </FieldGroup>
                </FieldSet>
                <Separator />
                <FieldSet>
                  <FieldLegend>{t('Upstream and window')}</FieldLegend>
                  <FieldGroup className='gap-4'>
                    {hasAccountIds && (
                      <Field data-disabled={props.busy || props.mustReload}>
                        <FieldLabel htmlFor='personal-share-account'>
                          {t('Subscription account')}
                        </FieldLabel>
                        <NativeSelect
                          id='personal-share-account'
                          className='w-full'
                          value={accountId}
                          aria-describedby={
                            accountId
                              ? 'personal-share-account-identity'
                              : undefined
                          }
                          disabled={props.busy || props.mustReload}
                          onChange={(event) => {
                            setAccountId(event.target.value)
                            props.onSelect('')
                          }}
                        >
                          <NativeSelectOption value=''>
                            {t('Select a subscription account')}
                          </NativeSelectOption>
                          {[...accounts].map(([id, name]) => (
                            <NativeSelectOption key={id} value={String(id)}>
                              {name} · #{id}
                            </NativeSelectOption>
                          ))}
                        </NativeSelect>
                        {accountId && (
                          <FieldDescription
                            id='personal-share-account-identity'
                            className='break-all'
                          >
                            {accounts.get(Number(accountId))} · #{accountId}
                          </FieldDescription>
                        )}
                        <FieldDescription>
                          {t(
                            'Choosing a connection only filters observed candidates. The server decides whether a sample belongs to the same subscription account and native window slot.'
                          )}
                        </FieldDescription>
                      </Field>
                    )}
                    <Field data-disabled={props.busy || props.mustReload}>
                      <FieldLabel htmlFor='personal-share-window'>
                        {t('Choose one subscription window')}
                      </FieldLabel>
                      <NativeSelect
                        id='personal-share-window'
                        className='w-full'
                        value={props.selection}
                        aria-describedby={
                          pool
                            ? 'personal-share-window-identity personal-share-window-help'
                            : 'personal-share-window-help'
                        }
                        disabled={props.busy || props.mustReload}
                        onChange={(event) => props.onSelect(event.target.value)}
                      >
                        <NativeSelectOption value=''>
                          {t('Select a subscription window')}
                        </NativeSelectOption>
                        {availableWindows.map((item) => (
                          <NativeSelectOption
                            key={`${item.pool_id}:${item.snapshot_id}`}
                            value={`${item.pool_id}:${item.snapshot_id}`}
                            disabled={item.state === 'closed'}
                          >
                            {!hasAccountIds && `${item.channel_name} · `}
                            {formatQuotaDuration(item.window_seconds, t)} ·{' '}
                            {quotaWindowLabel(item.window_slot, t)} ·{' '}
                            {quotaWindowLabel(item.window_type, t)}
                          </NativeSelectOption>
                        ))}
                      </NativeSelect>
                      {pool && (
                        <FieldDescription
                          id='personal-share-window-identity'
                          className='break-words'
                        >
                          {!hasAccountIds && `${pool.channel_name} · `}
                          {formatQuotaDuration(pool.window_seconds, t)} ·{' '}
                          {quotaWindowLabel(pool.window_slot, t)} ·{' '}
                          {quotaWindowLabel(pool.window_type, t)}
                        </FieldDescription>
                      )}
                      <FieldDescription id='personal-share-window-help'>
                        {t(
                          'No window is selected automatically. Each window is measured and limited separately.'
                        )}
                      </FieldDescription>
                    </Field>
                    {pool && (
                      <p className='text-muted-foreground text-sm break-words'>
                        {t('Observed at')}:{' '}
                        {formatTimestampToDate(pool.observed_at)} ·{' '}
                        {t('Sample age')}:{' '}
                        {t('{{seconds}} seconds', {
                          seconds: pool.sample_age_seconds.toLocaleString(),
                        })}
                      </p>
                    )}
                  </FieldGroup>
                </FieldSet>
                {pool && (
                  <>
                    <Separator />
                    <FieldSet>
                      <FieldLegend>{t('Allocation settings')}</FieldLegend>
                      <FieldGroup className='gap-4'>
                        <Field
                          data-invalid={!!props.inputError}
                          data-disabled={
                            props.busy ||
                            props.mustReload ||
                            pool.rebaseline_required
                          }
                        >
                          <FieldLabel htmlFor='personal-share-percentage'>
                            {t('Share of total window')}
                          </FieldLabel>
                          <InputGroup className='h-12 max-w-64'>
                            <InputGroupInput
                              id='personal-share-percentage'
                              className='h-12 text-2xl font-semibold'
                              inputMode='decimal'
                              autoComplete='off'
                              value={props.percentage}
                              aria-invalid={!!props.inputError}
                              aria-describedby='personal-share-percentage-help'
                              disabled={
                                props.busy ||
                                props.mustReload ||
                                pool.rebaseline_required
                              }
                              onChange={(event) =>
                                props.onPercentageChange(event.target.value)
                              }
                            />
                            <InputGroupAddon align='inline-end'>
                              %
                            </InputGroupAddon>
                          </InputGroup>
                          <FieldDescription id='personal-share-percentage-help'>
                            {t(
                              'Approximate share. Existing usage is kept; in-flight requests may exceed the limit.'
                            )}
                          </FieldDescription>
                          {props.inputError && (
                            <FieldError>{props.inputError}</FieldError>
                          )}
                          {props.shareBasisPoints === 0 && (
                            <FieldDescription>
                              {t(
                                '0% pauses new requests and keeps existing usage.'
                              )}
                            </FieldDescription>
                          )}
                          {totalBps === 10000 && (
                            <FieldDescription>
                              {t(
                                '100% allocation is allowed. Shares do not reserve upstream capacity.'
                              )}
                            </FieldDescription>
                          )}
                        </Field>
                        {pool.pool_id === 0 && (
                          <Alert>
                            <AlertTitle>
                              {t('This creates a managed pool')}
                            </AlertTitle>
                            <AlertDescription>
                              {t(
                                'After activation, other Keys without an allocation cannot use this managed subscription window. Existing Keys and channel credentials are not recreated.'
                              )}
                            </AlertDescription>
                          </Alert>
                        )}
                        {windowChangeRequired && (
                          <SubscriptionShareWindowChangeReview
                            currentPool={props.currentPool}
                            targetPool={pool}
                            mode={
                              pool.rebaseline_required
                                ? 'rebaseline'
                                : 'replacement'
                            }
                          />
                        )}
                        {windowChangeRequired && (
                          <Field orientation='horizontal'>
                            <Checkbox
                              id='personal-share-window-confirmation'
                              checked={props.windowChangeConfirmed}
                              disabled={props.busy || props.mustReload}
                              onCheckedChange={(checked) =>
                                props.onWindowChangeConfirmed(checked === true)
                              }
                            />
                            <FieldLabel htmlFor='personal-share-window-confirmation'>
                              {pool.rebaseline_required
                                ? t(
                                    'I understand this recovery affects every Key in the pool.'
                                  )
                                : t(
                                    'I understand this replacement affects every Key in the pool.'
                                  )}
                            </FieldLabel>
                          </Field>
                        )}
                        <Field
                          orientation='horizontal'
                          data-disabled={props.busy || props.mustReload}
                        >
                          <Checkbox
                            id='personal-share-confirmation'
                            checked={props.confirmed}
                            disabled={props.busy || props.mustReload}
                            onCheckedChange={(checked) =>
                              props.onConfirmedChange(checked === true)
                            }
                          />
                          <FieldLabel htmlFor='personal-share-confirmation'>
                            {t(
                              'I understand this is an estimate with no verified error bound and may stop requests only after the share is exceeded.'
                            )}
                          </FieldLabel>
                        </Field>
                      </FieldGroup>
                    </FieldSet>
                  </>
                )}
                {props.mustReload && (
                  <Alert
                    variant='destructive'
                    ref={reloadNotice}
                    tabIndex={-1}
                    aria-labelledby='personal-share-save-state-title'
                  >
                    <AlertTitle id='personal-share-save-state-title'>
                      {t('The save result is unknown or out of date')}
                    </AlertTitle>
                    <AlertDescription className='space-y-2'>
                      <p>
                        {t(
                          'Reload the current server state before trying again. The save will not be repeated automatically.'
                        )}
                      </p>
                      <Button
                        variant='outline'
                        disabled={props.busy}
                        onClick={props.onReload}
                      >
                        {t('Reload current state')}
                      </Button>
                    </AlertDescription>
                  </Alert>
                )}
                {props.reloadedAfterUnknown && (
                  <Alert>
                    <AlertTitle>
                      {t('Current server state reloaded')}
                    </AlertTitle>
                    <AlertDescription>
                      {t(
                        'Current server state reloaded. Review the window and confirmations before taking another action.'
                      )}
                    </AlertDescription>
                  </Alert>
                )}
                <Button
                  variant='ghost'
                  className='h-auto min-h-10 whitespace-normal'
                  type='button'
                  disabled={props.busy || props.mustReload}
                  onClick={props.onAdvanced}
                >
                  {t('Advanced window controls')}
                </Button>
                <Separator />
                {desktop && actions}
              </CardContent>
            </Card>
          </div>
          <aside
            className='min-w-0 space-y-5'
            aria-label={t('Allocation preview')}
          >
            <Card>
              <CardHeader>
                <CardTitle>{t('Allocation after saving')}</CardTitle>
              </CardHeader>
              <CardContent>
                <dl className='space-y-4 text-sm'>
                  <div className='flex justify-between gap-3'>
                    <dt>{t('This Key share')}</dt>
                    <dd className='font-semibold tabular-nums'>
                      {props.shareBasisPoints === null
                        ? t('Unknown')
                        : `${props.shareBasisPoints / 100}%`}
                    </dd>
                  </div>
                  <div className='flex justify-between gap-3'>
                    <dt className='text-muted-foreground'>{t('Other Keys')}</dt>
                    <dd className='tabular-nums'>
                      {otherBps === null ? t('Unknown') : `${otherBps / 100}%`}
                    </dd>
                  </div>
                  <Separator />
                  <div className='flex justify-between gap-3'>
                    <dt>{t('Total allocated')}</dt>
                    <dd
                      className='font-semibold tabular-nums'
                      aria-live='polite'
                    >
                      {totalBps === null ? t('Unknown') : `${totalBps / 100}%`}
                    </dd>
                  </div>
                  <div className='flex justify-between gap-3'>
                    <dt className='text-muted-foreground'>
                      {t('Unallocated')}
                    </dt>
                    <dd className='tabular-nums'>
                      {totalBps === null || totalBps > 10000
                        ? t('Unknown')
                        : `${(10000 - totalBps) / 100}%`}
                    </dd>
                  </div>
                </dl>
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>
                  {t('Estimated usage of share after saving')}
                </CardTitle>
              </CardHeader>
              <CardContent className='space-y-2'>
                <p className='text-4xl font-bold tracking-tight tabular-nums'>
                  {estimatedPercent === null
                    ? t('Unknown')
                    : `${Number(estimatedPercent.toFixed(2))}%`}
                </p>
                <p className='text-muted-foreground text-sm'>
                  {knownUsage
                    ? t(
                        'Equivalent to {{points}} percentage points of the total window.',
                        {
                          points: Number(
                            (pool.estimated_used_micro / 1_000_000).toFixed(6)
                          ),
                        }
                      )
                    : t('Waiting for a trustworthy usage sample.')}
                </p>
              </CardContent>
            </Card>
            <Alert>
              <Info aria-hidden='true' />
              <AlertTitle>{t('Approximate usage allocation')}</AlertTitle>
              <AlertDescription>
                {t(
                  'Request size, model, failures, external clients, measurement delay and rounding can all cause error. There is no verified numerical error bound.'
                )}
              </AlertDescription>
            </Alert>
            {pool && (
              <details className='bg-card rounded-xl border p-4'>
                <summary className='cursor-pointer text-sm font-medium'>
                  {t('Window evidence and pending usage')}
                </summary>
                <div className='mt-4'>
                  <SubscriptionSharePoolCard
                    pool={pool}
                    selectable={false}
                    showAdminDetails
                  />
                </div>
              </details>
            )}
          </aside>
          {!desktop && actions}
        </form>
      )}
    </section>
  )
}
