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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import {
  Copy,
  KeyRound,
  Pause,
  Play,
  Search,
  ShieldCheck,
  SlidersHorizontal,
  ListFilter,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import { Skeleton } from '@/components/ui/skeleton'
import { quotaWindowLabel } from '@/features/channels/lib/quota-history'
import { copyToClipboard } from '@/lib/copy-to-clipboard'
import { formatTimestampToDate } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import {
  fetchTokenKey,
  getApiKeys,
  searchApiKeys,
  updateApiKeyStatus,
} from '../api'
import { API_KEY_STATUS, API_KEY_STATUSES } from '../constants'
import { getSubscriptionShare } from '../subscription-share-api'
import type { ApiKey } from '../types'
import { ApiKeysPrimaryButtons } from './api-keys-primary-buttons'
import { useApiKeys } from './api-keys-provider'
import { CredentialMaintenanceDialog } from './credential-maintenance-dialog'
import { DataTableRowActions } from './data-table-row-actions'
import { SubscriptionShareDialog } from './subscription-share-dialog'

const route = getRouteApi('/_authenticated/keys/')

export function PersonalApiKeys() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const role = useAuthStore((state) => state.auth.user?.role)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  return (
    <PersonalApiKeysSession
      key={`${userId}:${role}:${sessionId}:${search.filter ?? ''}`}
      userId={userId}
      role={role}
      sessionId={sessionId}
      filter={search.filter ?? ''}
      page={search.page ?? 1}
      onPageChange={(page) =>
        void navigate({ search: (previous) => ({ ...previous, page }) })
      }
      onSearch={(filter) =>
        void navigate({
          search: (previous) => ({
            ...previous,
            filter: filter || undefined,
            page: 1,
          }),
        })
      }
    />
  )
}

function PersonalApiKeysSession(props: {
  userId?: number
  role?: number
  sessionId: string | null
  filter: string
  page: number
  onPageChange: (page: number) => void
  onSearch: (filter: string) => void
}) {
  const { t } = useTranslation()
  const page = props.page
  const [searchInput, setSearchInput] = useState(props.filter)
  const keyword = props.filter
  const [editingKey, setEditingKey] = useState<ApiKey | null>(null)
  const [maintenanceKey, setMaintenanceKey] = useState<ApiKey | null>(null)
  const client = useQueryClient()
  const { refreshTrigger } = useApiKeys()
  const query = useQuery({
    queryKey: [
      'personal-api-keys',
      props.userId,
      props.role,
      props.sessionId,
      refreshTrigger,
      page,
      keyword,
    ],
    queryFn: async () => {
      const response = keyword
        ? await searchApiKeys({ keyword, p: page, size: 10 })
        : await getApiKeys({ p: page, size: 10 })
      if (!response.success || !response.data) {
        throw new Error('Key list unavailable')
      }
      return response.data
    },
    enabled: !!props.userId,
    retry: false,
  })
  if (editingKey) {
    return (
      <SubscriptionShareDialog
        personalKey={editingKey}
        tokenId={editingKey.id}
        onClose={() => {
          setEditingKey(null)
          void client.invalidateQueries({ queryKey: ['subscription-share'] })
        }}
      />
    )
  }
  return (
    <main
      id='content'
      tabIndex={-1}
      className='pc-page min-w-0 space-y-6'
      aria-labelledby='personal-keys-title'
    >
      <header className='flex min-w-0 flex-wrap items-start justify-between gap-4'>
        <div className='min-w-0 space-y-2'>
          <h1
            id='personal-keys-title'
            className='text-2xl font-bold tracking-tight sm:text-[32px]'
          >
            {t('API Keys')}
          </h1>
          <p className='text-muted-foreground'>
            {t(
              'Manage existing Keys and their approximate subscription-window shares.'
            )}
          </p>
        </div>
        <ApiKeysPrimaryButtons personal />
      </header>
      <Alert>
        <KeyRound aria-hidden='true' />
        <AlertTitle>{t('One native window, one shared pool')}</AlertTitle>
        <AlertDescription>
          {t(
            'Shares are approximate request limits, not reserved upstream capacity. Each native window is managed separately.'
          )}
        </AlertDescription>
      </Alert>
      <form
        className='flex max-w-lg gap-2'
        onSubmit={(event) => {
          event.preventDefault()
          props.onSearch(searchInput.trim())
        }}
      >
        <InputGroup>
          <InputGroupAddon>
            <Search className='size-4' aria-hidden='true' />
          </InputGroupAddon>
          <InputGroupInput
            aria-label={t('Search keys')}
            placeholder={t('Search keys')}
            value={searchInput}
            onChange={(event) => setSearchInput(event.target.value)}
          />
        </InputGroup>
        <Button type='submit' variant='outline'>
          {t('Search')}
        </Button>
      </form>
      {query.isPending && (
        <div role='status' className='grid gap-4 md:grid-cols-2'>
          <span className='sr-only'>{t('Loading...')}</span>
          <Skeleton className='h-64 rounded-xl' />
          <Skeleton className='h-64 rounded-xl' />
        </div>
      )}
      {query.isError && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Failed to load API keys')}</AlertTitle>
          <AlertDescription>
            <Button
              variant='outline'
              disabled={query.isFetching}
              onClick={() => void query.refetch()}
            >
              {t('Retry')}
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {query.data && !query.isError && query.data.items.length === 0 && (
        <Empty className='bg-card rounded-xl border'>
          <EmptyHeader>
            <EmptyTitle>{t('No API keys found')}</EmptyTitle>
            <EmptyDescription>
              {t(
                'Your existing Keys will appear here. No replacement Key is created automatically.'
              )}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
      {query.data && !query.isError && (
        <div className='grid min-w-0 gap-5 lg:grid-cols-2'>
          {query.data.items.map((item) => (
            <PersonalApiKeyCard
              key={item.id}
              apiKey={item}
              userId={props.userId}
              role={props.role}
              sessionId={props.sessionId}
              onEdit={() => setEditingKey(item)}
              onMaintain={() => setMaintenanceKey(item)}
            />
          ))}
        </div>
      )}
      {query.data && query.data.total > 10 && (
        <nav
          aria-label={t('Pagination')}
          className='flex flex-wrap items-center justify-between gap-3'
        >
          <Button
            variant='outline'
            disabled={page === 1 || query.isFetching}
            onClick={() => props.onPageChange(page - 1)}
          >
            {t('Previous')}
          </Button>
          <span className='text-muted-foreground text-sm'>
            {t('Page {{page}}', { page })}
          </span>
          <Button
            variant='outline'
            disabled={page * 10 >= query.data.total || query.isFetching}
            onClick={() => props.onPageChange(page + 1)}
          >
            {t('Next')}
          </Button>
        </nav>
      )}
      {maintenanceKey && (
        <CredentialMaintenanceDialog
          tokenId={maintenanceKey.id}
          onClose={() => setMaintenanceKey(null)}
        />
      )}
    </main>
  )
}

function PersonalApiKeyCard(props: {
  apiKey: ApiKey
  userId?: number
  role?: number
  sessionId: string | null
  onEdit: () => void
  onMaintain: () => void
}) {
  const { t } = useTranslation()
  const { setCurrentRow, setOpen, viewRequests, requestFocusId } = useApiKeys()
  const client = useQueryClient()
  const [copying, setCopying] = useState(false)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const share = useQuery({
    queryKey: [
      'subscription-share',
      props.userId,
      props.role,
      props.sessionId,
      props.apiKey.id,
    ],
    queryFn: ({ signal }) => getSubscriptionShare(props.apiKey.id, signal),
    retry: false,
  })
  const active = props.apiKey.status === API_KEY_STATUS.ENABLED
  const config = API_KEY_STATUSES[props.apiKey.status]
  const configuredPools =
    share.data?.pools.filter(
      (pool) =>
        pool.pool_id > 0 &&
        pool.state !== 'candidate' &&
        pool.state !== 'closed'
    ) ?? []
  const sameSession = () => {
    const auth = useAuthStore.getState().auth
    return (
      auth.user?.id === props.userId &&
      auth.user?.role === props.role &&
      (auth.session?.sid ?? null) === props.sessionId
    )
  }
  const toggle = useMutation({
    mutationFn: async () => {
      if (!sameSession()) throw new Error('Session changed')
      const response = await updateApiKeyStatus(
        props.apiKey.id,
        active ? API_KEY_STATUS.DISABLED : API_KEY_STATUS.ENABLED
      )
      if (!response.success) throw new Error('Status update failed')
    },
    retry: false,
    onSuccess: () => {
      if (sameSession()) {
        void client.invalidateQueries({
          queryKey: [
            'personal-api-keys',
            props.userId,
            props.role,
            props.sessionId,
          ],
        })
      }
    },
  })
  const copy = async () => {
    if (copying || !sameSession()) return
    setCopying(true)
    try {
      const response = await fetchTokenKey(props.apiKey.id)
      if (!sameSession()) return
      if (!response.success || !response.data?.key) {
        throw new Error('Key unavailable')
      }
      const copied = await copyToClipboard(`sk-${response.data.key}`)
      if (!sameSession()) return
      if (copied) toast.success(t('Copied successfully'))
      else toast.error(t('Failed to copy'))
    } catch {
      if (sameSession()) {
        toast.error(t('Could not copy this Key. Please try again.'))
      }
    } finally {
      setCopying(false)
    }
  }
  return (
    <Card className='min-w-0'>
      <CardHeader>
        <div className='flex min-w-0 items-start justify-between gap-3'>
          <div className='min-w-0 space-y-2'>
            <div className='flex min-w-0 items-start gap-2'>
              <KeyRound
                className='text-primary size-5 shrink-0'
                aria-hidden='true'
              />
              <CardTitle className='min-w-0 text-base leading-6 break-words'>
                {props.apiKey.name}
              </CardTitle>
            </div>
            <p className='text-muted-foreground font-mono text-xs'>
              #{props.apiKey.id} · sk-••••••••
            </p>
          </div>
          <Badge
            variant={active ? 'secondary' : 'outline'}
            className='max-w-36 whitespace-normal'
          >
            {config ? t(config.label) : t('Unknown')}
          </Badge>
        </div>
      </CardHeader>
      <CardContent className='min-w-0 space-y-4'>
        {share.isPending && (
          <p role='status' className='text-muted-foreground text-sm'>
            {t('Loading subscription windows...')}
          </p>
        )}
        {share.isError && (
          <div role='alert' className='space-y-2 text-sm'>
            <p>{t('Could not load subscription shares.')}</p>
            <Button
              variant='outline'
              size='sm'
              onClick={() => void share.refetch()}
            >
              {t('Retry')}
            </Button>
          </div>
        )}
        {share.data && !share.isError && configuredPools.length === 0 && (
          <p className='text-muted-foreground text-sm'>
            {t('No subscription share is configured for this Key.')}
          </p>
        )}
        {!share.isError &&
          configuredPools.map((pool) => {
            const knownUsage = pool.baseline_time > 0
            const used =
              knownUsage && pool.share_bps > 0
                ? pool.estimated_used_micro / (pool.share_bps * 100)
                : null
            const paused =
              pool.share_bps === 0 ||
              pool.state !== 'active' ||
              (knownUsage &&
                pool.estimated_used_micro >= pool.share_bps * 10_000)
            return (
              <section
                key={`${pool.pool_id}:${pool.snapshot_id}`}
                className='bg-muted/20 rounded-lg border p-3 text-sm'
                aria-label={`${pool.channel_name} ${quotaWindowLabel(pool.window_slot, t)}`}
              >
                <div className='mb-3 flex flex-wrap justify-between gap-2'>
                  <span className='font-medium break-words'>
                    {pool.channel_name || t('Native window')} ·{' '}
                    {quotaWindowLabel(pool.window_slot, t)} ·{' '}
                    {quotaWindowLabel(pool.window_type, t)}
                  </span>
                  <Badge variant='outline'>
                    {paused ? t('Requests paused') : t('Approximate')}
                  </Badge>
                </div>
                <dl className='grid grid-cols-2 gap-3'>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('This Key share')}
                    </dt>
                    <dd className='mt-1 text-xl font-semibold'>
                      {pool.share_bps / 100}%
                    </dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('Estimated usage of share')}
                    </dt>
                    <dd className='mt-1 text-xl font-semibold'>
                      {used === null
                        ? t('Unknown')
                        : `${Number(used.toFixed(2))}%`}
                    </dd>
                  </div>
                </dl>
                <p className='text-muted-foreground mt-3 text-xs break-words'>
                  {t('Observed at')}: {formatTimestampToDate(pool.observed_at)}
                </p>
                {(pool.pending_attempts > 0 || pool.unsampled_attempts > 0) && (
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {t('Usage is still awaiting attribution.')}
                  </p>
                )}
                {pool.state !== 'active' && (
                  <p className='text-warning mt-1 text-xs'>
                    {t(
                      'Window unavailable or stale; later requests are paused.'
                    )}
                  </p>
                )}
              </section>
            )
          })}
        <p className='text-muted-foreground text-xs'>
          {t('Last used')}:{' '}
          {props.apiKey.accessed_time > 0
            ? formatTimestampToDate(props.apiKey.accessed_time)
            : t('Never')}
        </p>
        {toggle.isError && (
          <Alert variant='destructive'>
            <AlertDescription>
              {t('Failed to update API key status')}
              <Button
                variant='outline'
                size='sm'
                onClick={async () => {
                  await client.invalidateQueries({
                    queryKey: [
                      'personal-api-keys',
                      props.userId,
                      props.role,
                      props.sessionId,
                    ],
                  })
                  if (sameSession()) toggle.reset()
                }}
              >
                {t('Reload current state')}
              </Button>
            </AlertDescription>
          </Alert>
        )}
      </CardContent>
      <CardFooter className='flex min-w-0 flex-wrap gap-2'>
        {viewRequests && (
          <Button
            className='h-auto min-h-10 whitespace-normal'
            variant='outline'
            autoFocus={requestFocusId === props.apiKey.id}
            onClick={() => viewRequests(props.apiKey)}
          >
            {t('View Key requests')}
          </Button>
        )}
        <Button
          className='h-auto min-h-10 whitespace-normal'
          variant='outline'
          onClick={props.onEdit}
        >
          <SlidersHorizontal className='size-4' aria-hidden='true' />
          {t('Window share')}
        </Button>
        <Button
          className='h-auto min-h-10 whitespace-normal'
          variant='outline'
          onClick={() => {
            setCurrentRow(props.apiKey)
            setOpen('model-scope')
          }}
        >
          <ListFilter aria-hidden='true' />
          {t('Model scope')}
        </Button>
        {props.role === 100 && (
          <Button
            className='h-auto min-h-10 whitespace-normal'
            variant='outline'
            onClick={props.onMaintain}
          >
            <ShieldCheck data-icon='inline-start' aria-hidden='true' />
            {t('Credential maintenance')}
          </Button>
        )}
        <Button
          className='h-auto min-h-10 whitespace-normal'
          variant='ghost'
          disabled={copying}
          onClick={() => void copy()}
        >
          <Copy className='size-4' aria-hidden='true' />
          {t('Copy')}
        </Button>
        {(props.apiKey.status === API_KEY_STATUS.ENABLED ||
          props.apiKey.status === API_KEY_STATUS.DISABLED) && (
          <Button
            className='h-auto min-h-10 whitespace-normal'
            variant='ghost'
            disabled={toggle.isPending || toggle.isError}
            onClick={() => toggle.mutate()}
          >
            {active ? (
              <Pause className='size-4' aria-hidden='true' />
            ) : (
              <Play className='size-4' aria-hidden='true' />
            )}
            {active ? t('Pause Key') : t('Resume Key')}
          </Button>
        )}
        <details
          className='w-full border-t pt-3'
          onToggle={(event) => setAdvancedOpen(event.currentTarget.open)}
        >
          <summary className='cursor-pointer text-sm font-medium'>
            {t('Advanced options')}
          </summary>
          {advancedOpen && (
            <div className='mt-3 min-w-0'>
              <DataTableRowActions
                apiKey={props.apiKey}
                onCredentialMaintenance={
                  props.role === 100 ? props.onMaintain : undefined
                }
              />
            </div>
          )}
        </details>
      </CardFooter>
    </Card>
  )
}
