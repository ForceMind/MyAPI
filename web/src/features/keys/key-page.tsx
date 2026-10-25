/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQueryClient } from '@tanstack/react-query'
import { getRouteApi, Link, useNavigate } from '@tanstack/react-router'
import { useCallback, useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { useInspectorBack } from '@/components/inspector/inspector-history'
import { ObjectPage } from '@/components/object-page/object-page'
import {
  useRememberReturnFocus,
  useReturnFocus,
} from '@/components/object-page/return-focus'
import { StatusBadge } from '@/components/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { DetailsDialog } from '@/features/usage-logs/components/dialogs/details-dialog'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import { usePersonalWorkbenchMode } from '@/hooks/use-personal-workbench-mode'
import { OBJECT_PAGE_TABS_LIST_CLASS } from '@/lib/console-layout'
import { formatTimestampToDate } from '@/lib/format'
import { linkButtonClassName } from '@/lib/link-button'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { ApiKeysDialogs } from './components/api-keys-dialogs'
import { ApiKeysProvider, useApiKeys } from './components/api-keys-provider'
import { DataTableRowActions } from './components/data-table-row-actions'
import { KeyAccessPanel } from './components/inspector/key-access-panel'
import { KeyRecentRequests } from './components/inspector/key-recent-requests'
import { KeySharesPanel } from './components/inspector/key-shares-panel'
import { KeyActions } from './components/key-page/key-actions'
import { KeyDeliveryPanel } from './components/key-page/key-delivery-panel'
import { KeyStats } from './components/key-page/key-stats'
import { API_KEY_STATUSES } from './constants'
import { useKeyDetail } from './hooks/use-key-detail'
import { useKeyRecentRequests } from './hooks/use-key-recent-requests'
import type { KeyPageTab } from './lib/keys-search'
import type { ApiKey } from './types'

const route = getRouteApi('/_authenticated/keys/$keyId/')

/**
 * One Key as a page (design direction 3): name, state and actions, three
 * figures, then its requests, scope and shares, and delivery as tabs. The
 * tab is in the URL; Back returns to the list with its filters, page and
 * focus on the Key's row.
 */
export function KeyPage() {
  const params = route.useParams()
  const keyId = Number(params.keyId)
  const search = route.useSearch()
  const navigate = useNavigate()
  const goBack = useInspectorBack()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const role = useAuthStore((state) => state.auth.user?.role)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const queryClient = useQueryClient()
  const keyReturn = search.keyReturn
  const backToList = useCallback(
    () =>
      goBack(
        () =>
          void navigate({
            to: '/keys',
            search: { ...keyReturn, focusKeyId: keyId },
            replace: true,
          })
      ),
    [goBack, navigate, keyReturn, keyId]
  )

  return (
    <ApiKeysProvider
      key={`${userId}:${role}:${sessionId}`}
      onKeysDeleted={(ids) => {
        // The list is shown again from its cache; it must not list the Key.
        void queryClient.invalidateQueries({ queryKey: ['keys'] })
        if (ids.includes(keyId)) backToList()
      }}
    >
      <KeyPageContent
        key={`${userId}:${sessionId}:${keyId}`}
        keyId={keyId}
        tab={search.tab ?? 'requests'}
        requestLog={search.requestLog}
        onBack={backToList}
      />
      <ApiKeysDialogs />
    </ApiKeysProvider>
  )
}

function KeyPageContent(props: {
  keyId: number
  tab: KeyPageTab
  requestLog: number | undefined
  onBack: () => void
}) {
  const { t } = useTranslation()
  const navigate = route.useNavigate()
  const search = route.useSearch()
  const goBack = useInspectorBack()
  const rememberFocus = useRememberReturnFocus()
  const personal = usePersonalWorkbenchMode()
  const role = useAuthStore((state) => state.auth.user?.role ?? 0)
  const isAdmin = role >= ROLE.ADMIN
  const { refreshTrigger } = useApiKeys()
  const queryClient = useQueryClient()
  // A change made here (status, edit, scope) also makes the list's cached
  // rows stale, so Back shows the current state.
  useEffect(() => {
    if (refreshTrigger > 0) {
      void queryClient.invalidateQueries({ queryKey: ['keys'] })
    }
  }, [refreshTrigger, queryClient])
  const detail = useKeyDetail(props.keyId, refreshTrigger)
  const recent = useKeyRecentRequests(props.keyId, isAdmin)
  // Back from a request puts focus on its row.
  useReturnFocus({
    attribute: 'data-request-row',
    settled: !recent.isFetching && !detail.isFetching,
    rows: detail.data ? recent.data : undefined,
  })
  const apiKey = detail.isError ? undefined : detail.data
  const title = apiKey?.name ?? t('API Key #{{id}}', { id: props.keyId })
  const recentRows = recent.isError ? undefined : recent.data
  const legacyLog =
    props.requestLog === undefined
      ? undefined
      : recentRows?.find((row) => row.id === props.requestLog)

  const openRequest = (log: UsageLog) => {
    rememberFocus('data-request-row', log.id)
    if (log.request_id) {
      void navigate({
        to: '/keys/$keyId/requests/$requestId',
        params: { keyId: String(props.keyId), requestId: log.request_id },
        state: { inspectorBack: true },
      })
      return
    }
    // A record without a Request ID has no page of its own.
    void navigate({
      search: (previous) => ({ ...previous, requestLog: log.id }),
      state: { inspectorBack: true },
    })
  }
  const closeLegacy = () =>
    goBack(
      () =>
        void navigate({
          search: (previous) => ({ ...previous, requestLog: undefined }),
          replace: true,
        })
    )

  return (
    <ObjectPage
      data-testid='key-page'
      title={title}
      label={title}
      meta={apiKey ? <KeyMeta apiKey={apiKey} /> : <span>#{props.keyId}</span>}
      actions={
        apiKey ? (
          <KeyActions
            apiKey={apiKey}
            onRefetch={() => detail.refetch()}
            menu={
              // The shared menu button is 36px and the page actions clear its
              // minimum width; a touch target needs 44px.
              <span className='flex pointer-coarse:[&_button]:min-h-11 pointer-coarse:[&_button]:min-w-11!'>
                <DataTableRowActions apiKey={apiKey} menuOnly />
              </span>
            }
          />
        ) : undefined
      }
      navigation={{ close: props.onBack }}
    >
      <div className='flex min-w-0 flex-col gap-5'>
        {detail.isPending && (
          <div className='flex flex-col gap-3' aria-busy='true'>
            <span className='sr-only'>{t('Loading...')}</span>
            <Skeleton className='h-24 w-full' />
            <Skeleton className='h-64 w-full' />
          </div>
        )}
        {detail.isError && (
          <Alert variant='destructive'>
            <AlertTitle>
              {t('This Key is unavailable or belongs to another account.')}
            </AlertTitle>
            <AlertDescription className='flex flex-wrap gap-2'>
              <Button
                variant='outline'
                size='sm'
                disabled={detail.isFetching}
                onClick={() => void detail.refetch()}
              >
                {t('Retry')}
              </Button>
              <Link
                to='/keys'
                search={search.keyReturn ?? {}}
                data-myapi-button=''
                className={linkButtonClassName({
                  variant: 'ghost',
                  size: 'sm',
                })}
              >
                {t('Back to Keys')}
              </Link>
            </AlertDescription>
          </Alert>
        )}
        {apiKey && (
          <>
            <KeyStats
              apiKey={apiKey}
              isAdmin={isAdmin}
              recent={recentRows}
              recentPending={recent.isPending}
              showQuota={!personal}
            />
            <Tabs
              value={props.tab}
              onValueChange={(value) =>
                void navigate({
                  search: (previous) => ({
                    ...previous,
                    tab:
                      value === 'requests' ? undefined : (value as KeyPageTab),
                  }),
                  replace: true,
                  state: (previous) => previous,
                })
              }
              className='min-w-0 gap-4'
            >
              <TabsList variant='line' className={OBJECT_PAGE_TABS_LIST_CLASS}>
                <TabsTrigger value='requests'>
                  {t('Requests', { context: 'section' })}
                </TabsTrigger>
                <TabsTrigger value='scope'>{t('Scope and shares')}</TabsTrigger>
                <TabsTrigger value='delivery'>{t('Delivery')}</TabsTrigger>
              </TabsList>
              <TabsContent value='requests' className='min-w-0'>
                <KeyRecentRequests
                  keyId={props.keyId}
                  query={recent}
                  showConnection={isAdmin}
                  returnContext={{
                    ...search.keyReturn,
                    focusKeyId: props.keyId,
                  }}
                  onOpen={openRequest}
                />
              </TabsContent>
              <TabsContent value='scope' className='min-w-0'>
                <div className='flex min-w-0 flex-col gap-4'>
                  <section
                    aria-labelledby='key-access-title'
                    className='bg-card p-card flex min-w-0 flex-col gap-3 rounded-xl border'
                  >
                    <h2 id='key-access-title' className='text-title'>
                      {t('Access')}
                    </h2>
                    <KeyAccessPanel apiKey={apiKey} showQuota={!personal} />
                  </section>
                  <section
                    aria-labelledby='key-shares-title'
                    className='bg-card p-card flex min-w-0 flex-col gap-3 rounded-xl border'
                  >
                    <h2 id='key-shares-title' className='text-title'>
                      {t('Window shares')}
                    </h2>
                    {props.tab === 'scope' && (
                      <KeySharesPanel apiKey={apiKey} />
                    )}
                  </section>
                </div>
              </TabsContent>
              <TabsContent value='delivery' className='min-w-0'>
                <KeyDeliveryPanel apiKey={apiKey} />
              </TabsContent>
            </Tabs>
          </>
        )}
      </div>
      {legacyLog && (
        <DetailsDialog
          log={legacyLog}
          isAdmin={isAdmin}
          open
          onOpenChange={(open) => {
            if (!open) closeLegacy()
          }}
        />
      )}
    </ObjectPage>
  )
}

/** State, masked Key, ID and dates under the Key name. */
function KeyMeta(props: { apiKey: ApiKey }) {
  const { t } = useTranslation()
  const key = props.apiKey
  const status = API_KEY_STATUSES[key.status]
  return (
    <>
      <StatusBadge
        variant={status?.variant ?? 'neutral'}
        label={status ? t(status.label) : t('Unknown')}
        showDot
        copyable={false}
      />
      <span className='font-mono'>#{key.id}</span>
      <span className='font-mono wrap-anywhere'>sk-{key.key}</span>
      <span>
        {t('Created')}: {formatTimestampToDate(key.created_time)}
      </span>
      <span>
        {t('Expires')}:{' '}
        {key.expired_time === -1
          ? t('Never')
          : formatTimestampToDate(key.expired_time)}
      </span>
      <span>
        {t('Last used')}:{' '}
        {key.accessed_time > 0
          ? formatTimestampToDate(key.accessed_time)
          : t('Never')}
      </span>
    </>
  )
}
