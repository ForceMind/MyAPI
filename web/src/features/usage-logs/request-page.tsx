/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { FileSearchIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { getRouteApi, Link, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { useInspectorBack } from '@/components/inspector/inspector-history'
import { ObjectPage } from '@/components/object-page/object-page'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ContentPanel } from '@/features/full-content-logs/components/full-content-log-details-dialog'
import {
  RequestBodyPanel,
  RequestStreamPanel,
  RequestOriginalPanel,
} from '@/features/full-content-logs/components/request-content-panels'
import { useKeyDetail } from '@/features/keys/hooks/use-key-detail'
import { useFundingPresentation } from '@/hooks/use-funding-presentation'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { OBJECT_PAGE_TABS_LIST_CLASS } from '@/lib/console-layout'
import { linkButtonClassName } from '@/lib/link-button'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import type { OpenInspector } from '@/stores/inspector-store'

import { DiagnosisTimeline } from './components/diagnosis-timeline'
import { LogDetailsContent } from './components/dialogs/details-dialog'
import { ResultGrid } from './components/request-result-grid'
import { RequestIdentity } from './components/request-identity'
import { UsageLogsProvider } from './components/usage-logs-provider'
import type { UsageLog } from './data/schema'
import { useRequestLookup } from './hooks/use-request-lookup'
import { parseLogOther } from './lib/format'
import type { RequestPageTab } from './lib/request-page-search'

const keyRoute = getRouteApi('/_authenticated/keys/$keyId/requests/$requestId')
const recordRoute = getRouteApi('/_authenticated/usage-logs/request/$requestId')

/** A long Request ID shortened for a heading; the full ID is shown below it. */
function shortRequestId(requestId: string) {
  return requestId.length > 18
    ? `${requestId.slice(0, 8)}…${requestId.slice(-6)}`
    : requestId
}

/** A request opened from a Key's page: API Key › Key › request. */
export function KeyRequestPage() {
  const actor = useAuthStore((state) => JSON.stringify([
    state.auth.user?.id ?? null, state.auth.session?.sid ?? null, state.auth.user?.role ?? 0,
  ]))
  return <KeyRequestPageContent key={actor} />
}

function KeyRequestPageContent() {
  const { t } = useTranslation()
  const params = keyRoute.useParams()
  const search = keyRoute.useSearch()
  const navigate = keyRoute.useNavigate()
  const goBack = useInspectorBack()
  const keyId = Number(params.keyId)
  const detail = useKeyDetail(keyId)
  const keyTitle =
    (detail.isError ? undefined : detail.data?.name) ??
    t('API Key #{{id}}', { id: keyId })
  const toKey = () =>
    goBack(
      () =>
        void navigate({
          to: '/keys/$keyId',
          params: { keyId: String(keyId) },
          replace: true,
        })
    )
  return (
    <RequestPage
      key={params.requestId}
      requestId={params.requestId}
      keyId={keyId}
      tab={search.tab ?? 'body'}
      onTabChange={(tab) =>
        void navigate({
          search: (previous) => ({
            ...previous,
            tab: tab === 'body' ? undefined : tab,
          }),
          replace: true,
          state: (previous) => previous,
        })
      }
      navigation={{
        close: toKey,
        parents: [{ title: keyTitle, open: toKey }],
        closeAll: () => void navigate({ to: '/keys' }),
      }}
    />
  )
}

/** A request opened from the call records, command search or overview. */
export function CallRecordRequestPage() {
  const params = recordRoute.useParams()
  const search = recordRoute.useSearch()
  const navigate = useNavigate()
  const goBack = useInspectorBack()
  return (
    <RequestPage
      key={params.requestId}
      requestId={params.requestId}
      tab={search.tab ?? 'body'}
      onTabChange={(tab) =>
        void navigate({
          to: '/usage-logs/request/$requestId',
          params: { requestId: params.requestId },
          search: { tab: tab === 'body' ? undefined : tab },
          replace: true,
          state: (previous) => previous,
        })
      }
      navigation={{
        // Without an in-app entry to return to, the call records stay
        // filtered to the same Request ID.
        close: () =>
          goBack(
            () =>
              void navigate({
                to: '/usage-logs/$section',
                params: { section: 'common' },
                search: { requestId: params.requestId },
                replace: true,
              })
          ),
      }}
    />
  )
}

/**
 * One request as a page (design direction 3): its four results, the
 * diagnosis timeline and grouped record, saved body, SSE event inspection
 * and original evidence in separate tabs. Everything is read from saved records; nothing is
 * resent and nothing is polled.
 */
type RequestPageProps = {
  requestId: string
  /** The Key the page was opened from; a request of another Key says so. */
  keyId?: number
  tab: RequestPageTab
  onTabChange: (tab: RequestPageTab) => void
  navigation: Omit<OpenInspector, 'title' | 'page'>
}

function RequestPage(props: RequestPageProps) {
  const actor = useAuthStore((state) => JSON.stringify([
    state.auth.user?.id ?? null, state.auth.session?.sid ?? null, state.auth.user?.role ?? 0,
  ]))
  return <RequestPageContent key={actor} {...props} />
}

function RequestPageContent(props: RequestPageProps) {
  const { t } = useTranslation()
  const role = useAuthStore((state) => state.auth.user?.role ?? 0)
  const isAdmin = role >= ROLE.ADMIN
  const funding = useFundingPresentation()
  const showLogs = useIsSidebarModuleVisible('/usage-logs/common')
  const lookup = useRequestLookup({
    requestId: props.requestId,
    isAdmin,
    enabled: true,
  })
  const log = lookup.isError ? undefined : lookup.data?.log
  const shortId = shortRequestId(props.requestId)

  let body
  if (lookup.isPending) {
    body = (
      <div className='flex flex-col gap-3' aria-busy='true'>
        <span className='sr-only'>{t('Loading...')}</span>
        <Skeleton className='h-16 w-full' />
        <Skeleton className='h-72 w-full' />
      </div>
    )
  } else if (lookup.isError) {
    body = (
      <Alert variant='destructive'>
        <AlertTitle>{t('Could not look up this Request ID')}</AlertTitle>
        <AlertDescription className='flex flex-col items-start gap-2'>
          <span>{t('The call records could not be read. Try again.')}</span>
          <Button
            variant='outline'
            size='sm'
            disabled={lookup.isFetching}
            onClick={() => void lookup.refetch()}
          >
            {t('Retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  } else if (!log) {
    body = (
      <Empty className='border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={FileSearchIcon} aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('No call record has this Request ID')}</EmptyTitle>
          <EmptyDescription>
            {isAdmin
              ? t(
                  'Check the ID. Records removed by log cleanup cannot be found.'
                )
              : t(
                  'Check the ID. Only your own call records can be found from this account.'
                )}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  } else {
    body = (
      <RequestDetails
        log={log}
        isAdmin={isAdmin}
        count={lookup.data?.count ?? 1}
        otherKey={props.keyId !== undefined && log.token_id !== props.keyId}
        tab={props.tab}
        onTabChange={props.onTabChange}
      />
    )
  }

  return (
    <UsageLogsProvider showCosts={funding.commercialEnabled}>
      <ObjectPage
        data-testid='request-page'
        title={<span className='font-mono'>{shortId}</span>}
        label={t('Request {{requestId}}', { requestId: shortId })}
        meta={log
          ? t('Saved request evidence')
          : <code className='font-mono wrap-anywhere'>{props.requestId}</code>}
        actions={
          <>
            <CopyButton
              value={props.requestId}
              variant='outline'
              size='default'
              aria-label={t('Copy Request ID')}
              className='min-h-11'
            >
              {t('Copy ID')}
            </CopyButton>
            {showLogs && (
              <Link
                to='/usage-logs/$section'
                params={{ section: 'common' }}
                search={{ requestId: props.requestId }}
                data-myapi-button=''
                className={linkButtonClassName({ variant: 'ghost', className: 'min-h-11' })}
              >
                {t('Show in call records')}
              </Link>
            )}
          </>
        }
        navigation={props.navigation}
      >
        {body}
      </ObjectPage>
    </UsageLogsProvider>
  )
}

function RequestDetails(props: {
  log: UsageLog
  isAdmin: boolean
  count: number
  otherKey: boolean
  tab: RequestPageTab
  onTabChange: (tab: RequestPageTab) => void
}) {
  const { t } = useTranslation()
  const log = props.log
  const other = parseLogOther(log.other)
  const requestId = log.request_id || null
  // The record as this account may read it, with `other` decoded.
  const rawRecord = props.tab === 'raw' ? JSON.stringify(
    { ...log, other: other ?? log.other },
    null,
    2
  ) : ''

  return (
    <div className='flex min-w-0 flex-col gap-4'>
      {props.otherKey && (
        <Alert>
          <AlertTitle>
            {t('This request was made with a different Key.')}
          </AlertTitle>
        </Alert>
      )}
      {props.count > 1 && (
        <p className='text-caption text-muted-foreground'>
          {t(
            'This Request ID has {{count}} call records; the newest request record is shown. All of them are listed in the table.',
            { count: props.count }
          )}
        </p>
      )}
      <RequestIdentity log={log} isAdmin={props.isAdmin} />
      <ResultGrid log={log} />
      <Tabs
        value={props.tab}
        onValueChange={(value) => props.onTabChange(value as RequestPageTab)}
        className='min-w-0 gap-4'
      >
        <TabsList variant='line' className={OBJECT_PAGE_TABS_LIST_CLASS}>
          <TabsTrigger value='detail' className='min-h-11'>{t('Diagnosis')}</TabsTrigger>
          <TabsTrigger value='body' className='min-h-11'>{t('Body')}</TabsTrigger>
          <TabsTrigger value='sse' className='min-h-11'>{t('SSE')}</TabsTrigger>
          <TabsTrigger value='raw' className='min-h-11'>{t('Raw evidence')}</TabsTrigger>
        </TabsList>
        <TabsContent value='detail' className='min-w-0'>
          <div className='grid min-w-0 items-start gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.5fr)]'>
            <section aria-labelledby='request-diagnosis-title' className='bg-card min-w-0 rounded-xl border p-4'>
              <h2 id='request-diagnosis-title' className='text-title mb-4'>{t('Diagnosis')}</h2>
              <DiagnosisTimeline log={log} isAdmin={props.isAdmin} />
            </section>
            <section aria-label={t('Saved record')} className='bg-card min-w-0 rounded-xl border p-4'>
              <LogDetailsContent log={log} isAdmin={props.isAdmin} active={props.tab === 'detail'} embedded presentation='request' />
            </section>
          </div>
        </TabsContent>
        <TabsContent value='body' className='bg-card min-w-0 rounded-xl border p-4'>
          <RequestBodyPanel requestId={requestId} active={props.tab === 'body'} />
        </TabsContent>
        <TabsContent value='sse' className='bg-card min-w-0 rounded-xl border p-4'>
          <RequestStreamPanel requestId={requestId} active={props.tab === 'sse'} />
        </TabsContent>
        <TabsContent value='raw' className='min-w-0 space-y-4'>
          <section className='bg-card min-w-0 space-y-3 rounded-xl border p-4'>
            <p className='text-caption text-muted-foreground'>
              {t('The call record as saved for this account; request and response bodies are under Body and SSE.')}
            </p>
            <ContentPanel title={t('Saved call record')} body={rawRecord} encoding='json' contentType='application/json' emptyText={t('Empty response body')} />
          </section>
          <section aria-label={t('Saved upstream request and response')} className='bg-card min-w-0 space-y-3 rounded-xl border p-4'>
            <p className='text-caption text-muted-foreground'>
              {t('Saved upstream request and response are separate from the call record. Missing content is not reconstructed.')}
            </p>
            <RequestOriginalPanel requestId={requestId} active={props.tab === 'raw'} />
          </section>
        </TabsContent>
      </Tabs>
    </div>
  )
}
