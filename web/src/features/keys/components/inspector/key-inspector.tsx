/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import {
  Copy01Icon,
  PauseIcon,
  PlayIcon,
  SourceCodeIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { InspectorHeader } from '@/components/inspector/inspector-layout'
import { useInspectorPresentation } from '@/components/inspector/use-inspector-presentation'
import { StatusBadge } from '@/components/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { createPlaygroundKeyHandoff } from '@/features/playground/lib/playground-key-handoff'
import { RequestInspector } from '@/features/usage-logs/components/request-inspector'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import { useRequestLookup } from '@/features/usage-logs/hooks/use-request-lookup'
import type { RequestInspectorTab } from '@/features/usage-logs/lib/request-inspector-tabs'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { INSPECTOR_TABS_LIST_CLASS } from '@/lib/console-layout'
import { copyToClipboard } from '@/lib/copy-to-clipboard'
import { formatTimestampToDate } from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { fetchTokenKey, getApiKey, updateApiKeyStatus } from '../../api'
import { API_KEY_STATUS, API_KEY_STATUSES } from '../../constants'
import { useKeyMaintenanceSession } from '../../hooks/use-key-maintenance-session'
import { useKeyRecentRequests } from '../../hooks/use-key-recent-requests'
import type { KeyRequestContext } from '../../key-request-context'
import { getKeyCondition } from '../../lib/key-condition'
import type { KeyInspectorTab } from '../../lib/keys-search'
import type { ApiKey } from '../../types'
import { useApiKeys } from '../api-keys-provider'
import { CredentialMaintenanceDialog } from '../credential-maintenance-dialog'
import { DataTableRowActions } from '../data-table-row-actions'
import { KeyAccessPanel } from './key-access-panel'
import { KeyRecentRequests } from './key-recent-requests'
import { KeySharesPanel } from './key-shares-panel'

export type KeyInspectorProps = {
  keyId: number
  tab: KeyInspectorTab
  /** Request ID of an open request, or a row ID for legacy records. */
  request?: string
  requestLog?: number
  requestTab: RequestInspectorTab
  /** Keys carry no financial quota (personal workbench mode). */
  personal: boolean
  returnContext: KeyRequestContext
  onTabChange: (tab: KeyInspectorTab) => void
  onOpenRequest: (log: UsageLog) => void
  onCloseRequest: () => void
  onRequestTabChange: (tab: RequestInspectorTab) => void
  onClose: () => void
}

/**
 * One Key: its state and actions in the header, then recent requests,
 * access and window shares. A request opened from the list replaces the Key
 * view with diagnosis, body and SSE tabs; Back returns here with focus on
 * that request (design 8.2).
 */
export function KeyInspector(props: KeyInspectorProps) {
  const { t } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const role = useAuthStore((state) => state.auth.user?.role ?? 0)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const { refreshTrigger } = useApiKeys()
  const isAdmin = role >= ROLE.ADMIN
  const recent = useKeyRecentRequests(props.keyId, isAdmin)
  const detail = useQuery({
    queryKey: [
      'api-key-inspector',
      userId,
      role,
      sessionId,
      props.keyId,
      refreshTrigger,
    ],
    queryFn: async ({ signal }) => {
      const response = await getApiKey(props.keyId, signal)
      if (!response.success || response.data?.id !== props.keyId) {
        throw new Error('Key unavailable')
      }
      return response.data
    },
    enabled: userId !== null,
    retry: false,
    placeholderData: (previous) =>
      previous?.id === props.keyId ? previous : undefined,
  })

  const requestOpen =
    props.request !== undefined || props.requestLog !== undefined
  const recentRows = recent.isError ? undefined : recent.data
  const fromRecent = recentRows?.find((row) =>
    props.request !== undefined
      ? row.request_id === props.request
      : row.id === props.requestLog
  )
  const lookup = useRequestLookup({
    requestId: props.request,
    isAdmin,
    enabled: props.request !== undefined && !fromRecent && !recent.isPending,
  })
  const openedLog = fromRecent ?? lookup.data?.log

  // Returning from a request puts focus back on its row in the list.
  const lastRequestRow = useRef<number | undefined>(undefined)
  const panelRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (requestOpen) {
      if (openedLog) lastRequestRow.current = openedLog.id
      return
    }
    const rowId = lastRequestRow.current
    if (rowId === undefined) return
    const row = panelRef.current?.querySelector<HTMLElement>(
      `[data-request-row="${rowId}"]`
    )
    if (row) {
      lastRequestRow.current = undefined
      row.focus()
    }
  }, [requestOpen, openedLog, recentRows])

  const apiKey = detail.isError ? undefined : detail.data
  const keyTitle = apiKey?.name ?? t('API Key #{{id}}', { id: props.keyId })

  if (requestOpen) {
    return (
      <RequestView
        {...props}
        isAdmin={isAdmin}
        keyTitle={keyTitle}
        log={openedLog}
        pending={
          !openedLog &&
          (recent.isPending ||
            (props.request !== undefined && lookup.isPending))
        }
        failed={
          !openedLog && (recent.isError || lookup.isError) && !recent.isPending
        }
        onRetry={() => {
          void recent.refetch()
          if (props.request !== undefined) void lookup.refetch()
        }}
      />
    )
  }

  return (
    <div ref={panelRef} className='flex min-w-0 flex-col'>
      <InspectorHeader
        focusKey={`key:${props.keyId}`}
        onClose={props.onClose}
        title={keyTitle}
        subtitle={apiKey ? <KeySubtitle apiKey={apiKey} /> : `#${props.keyId}`}
        actions={
          apiKey ? (
            <KeyActions apiKey={apiKey} onRefetch={() => detail.refetch()} />
          ) : undefined
        }
        menu={
          apiKey ? <DataTableRowActions apiKey={apiKey} menuOnly /> : undefined
        }
      />
      <div className='p-card flex min-w-0 flex-col gap-4'>
        {detail.isPending && (
          <div className='flex flex-col gap-2' aria-busy='true'>
            <span className='sr-only'>{t('Loading...')}</span>
            <Skeleton className='h-8 w-full' />
            <Skeleton className='h-40 w-full' />
          </div>
        )}
        {detail.isError && (
          <Alert variant='destructive'>
            <AlertTitle>
              {t('This Key is unavailable or belongs to another account.')}
            </AlertTitle>
            <AlertDescription>
              <Button
                variant='outline'
                size='sm'
                disabled={detail.isFetching}
                onClick={() => void detail.refetch()}
              >
                {t('Retry')}
              </Button>
            </AlertDescription>
          </Alert>
        )}
        {apiKey && (
          <Tabs
            value={props.tab}
            onValueChange={(value) =>
              props.onTabChange(value as KeyInspectorTab)
            }
          >
            <TabsList className={INSPECTOR_TABS_LIST_CLASS}>
              <TabsTrigger value='requests'>{t('Recent requests')}</TabsTrigger>
              <TabsTrigger value='access'>{t('Access')}</TabsTrigger>
              <TabsTrigger value='shares'>{t('Window shares')}</TabsTrigger>
            </TabsList>
            <TabsContent value='requests' className='min-w-0 pt-2'>
              <KeyRecentRequests
                keyId={props.keyId}
                query={recent}
                returnContext={props.returnContext}
                onOpen={props.onOpenRequest}
              />
            </TabsContent>
            <TabsContent value='access' className='min-w-0 pt-2'>
              <KeyAccessSection apiKey={apiKey} personal={props.personal} />
            </TabsContent>
            <TabsContent value='shares' className='min-w-0 pt-2'>
              {props.tab === 'shares' && <KeySharesPanel apiKey={apiKey} />}
            </TabsContent>
          </Tabs>
        )}
      </div>
    </div>
  )
}

function KeyAccessSection(props: { apiKey: ApiKey; personal: boolean }) {
  const role = useAuthStore((state) => state.auth.user?.role)
  const [maintaining, setMaintaining] = useState(false)
  return (
    <>
      <KeyAccessPanel
        apiKey={props.apiKey}
        showQuota={!props.personal}
        onCredentialMaintenance={
          role === ROLE.SUPER_ADMIN ? () => setMaintaining(true) : undefined
        }
      />
      {maintaining && (
        <CredentialMaintenanceDialog
          tokenId={props.apiKey.id}
          onClose={() => setMaintaining(false)}
        />
      )}
    </>
  )
}

function KeySubtitle(props: { apiKey: ApiKey }) {
  const { t } = useTranslation()
  const status = API_KEY_STATUSES[props.apiKey.status]
  return (
    <span className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1'>
      <span className='font-mono'>#{props.apiKey.id}</span>
      <StatusBadge
        variant={status?.variant ?? 'neutral'}
        label={status ? t(status.label) : t('Unknown')}
        showDot
        copyable={false}
      />
      <span>
        {t('Last used')}:{' '}
        {props.apiKey.accessed_time > 0
          ? formatTimestampToDate(props.apiKey.accessed_time)
          : t('Never')}
      </span>
    </span>
  )
}

/**
 * Pause/resume, copy and Playground act on this Key only after a click. An
 * unknown status write locks the toggle until the current state is reloaded.
 */
function KeyActions(props: { apiKey: ApiKey; onRefetch: () => unknown }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isCurrentSession = useKeyMaintenanceSession()
  const { triggerRefresh } = useApiKeys()
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const showPlayground = useIsSidebarModuleVisible('/playground')
  const [copying, setCopying] = useState(false)
  const active = props.apiKey.status === API_KEY_STATUS.ENABLED
  const toggleable =
    props.apiKey.status === API_KEY_STATUS.ENABLED ||
    props.apiKey.status === API_KEY_STATUS.DISABLED
  const toggle = useMutation({
    mutationFn: async () => {
      if (!isCurrentSession()) throw new Error('Session changed')
      const response = await updateApiKeyStatus(
        props.apiKey.id,
        active ? API_KEY_STATUS.DISABLED : API_KEY_STATUS.ENABLED
      )
      if (!response.success) throw new Error('Status update failed')
    },
    retry: false,
    onSuccess: () => {
      if (isCurrentSession()) triggerRefresh()
    },
  })
  const copy = async () => {
    if (copying || !isCurrentSession()) return
    setCopying(true)
    try {
      const response = await fetchTokenKey(props.apiKey.id)
      if (!isCurrentSession()) return
      if (!response.success || !response.data?.key) {
        throw new Error('Key unavailable')
      }
      const copied = await copyToClipboard(`sk-${response.data.key}`)
      if (!isCurrentSession()) return
      if (copied) toast.success(t('Copied successfully'))
      else toast.error(t('Failed to copy'))
    } catch {
      if (isCurrentSession()) {
        toast.error(t('Could not copy this Key. Please try again.'))
      }
    } finally {
      setCopying(false)
    }
  }
  const playgroundReady =
    showPlayground &&
    user !== null &&
    sessionId !== null &&
    getKeyCondition(props.apiKey) === 'available'

  return (
    <div className='flex min-w-0 flex-col gap-2'>
      <div className='flex min-w-0 flex-wrap items-center gap-2'>
        {toggleable && (
          <Button
            variant='outline'
            disabled={toggle.isPending || toggle.isError}
            onClick={() => toggle.mutate()}
          >
            <HugeiconsIcon
              icon={active ? PauseIcon : PlayIcon}
              data-icon='inline-start'
              aria-hidden='true'
            />
            {active ? t('Pause Key') : t('Resume Key')}
          </Button>
        )}
        <Button variant='ghost' disabled={copying} onClick={() => void copy()}>
          <HugeiconsIcon
            icon={Copy01Icon}
            data-icon='inline-start'
            aria-hidden='true'
          />
          {t('Copy')}
        </Button>
        {showPlayground && (
          <Button
            variant='ghost'
            disabled={!playgroundReady}
            onClick={() => {
              if (!playgroundReady || !user || !isCurrentSession()) return
              createPlaygroundKeyHandoff({
                userId: user.id,
                sessionId,
                keyId: props.apiKey.id,
              })
              void navigate({ to: '/playground' })
            }}
          >
            <HugeiconsIcon
              icon={SourceCodeIcon}
              data-icon='inline-start'
              aria-hidden='true'
            />
            {t('Open Playground')}
          </Button>
        )}
      </div>
      {toggle.isError && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Failed to update API key status')}</AlertTitle>
          <AlertDescription>
            <Button
              variant='outline'
              size='sm'
              onClick={async () => {
                await props.onRefetch()
                if (isCurrentSession()) toggle.reset()
              }}
            >
              {t('Reload current state')}
            </Button>
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}

function RequestView(
  props: KeyInspectorProps & {
    isAdmin: boolean
    keyTitle: string
    log: UsageLog | undefined
    pending: boolean
    failed: boolean
    onRetry: () => void
  }
) {
  const { t } = useTranslation()
  const presentation = useInspectorPresentation()
  const back = {
    label: t('Back to {{name}}', { name: props.keyTitle }),
    onBack: props.onCloseRequest,
  }
  if (props.log) {
    return (
      <>
        <RequestInspector
          log={props.log}
          isAdmin={props.isAdmin}
          tab={props.requestTab}
          onTabChange={props.onRequestTabChange}
          onClose={props.onClose}
          back={back}
        />
        {props.log.token_id !== props.keyId && (
          <p className='text-caption text-warning px-card pb-card'>
            {t('This request was made with a different Key.')}
          </p>
        )}
      </>
    )
  }
  let body
  if (props.pending) {
    body = (
      <div className='flex flex-col gap-2' aria-busy='true'>
        <span className='sr-only'>{t('Loading...')}</span>
        <Skeleton className='h-20 w-full' />
        <Skeleton className='h-48 w-full' />
      </div>
    )
  } else if (props.failed) {
    body = (
      <Alert variant='destructive'>
        <AlertTitle>{t('Could not load this request')}</AlertTitle>
        <AlertDescription>
          <Button variant='outline' size='sm' onClick={props.onRetry}>
            {t('Retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  } else {
    body = (
      <Empty className='border'>
        <EmptyHeader>
          <EmptyTitle>
            {t('This request is not among the latest ones')}
          </EmptyTitle>
          <EmptyDescription>
            {t('Open the call records to search older requests of this Key.')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }
  return (
    <div className='flex min-w-0 flex-col'>
      {presentation !== 'page' && (
        <div className='border-b px-2 py-1'>
          <Button variant='ghost' onClick={back.onBack}>
            {back.label}
          </Button>
        </div>
      )}
      <InspectorHeader
        focusKey={`request-missing:${props.request ?? props.requestLog}`}
        onClose={props.onClose}
        title={t('Request details')}
      />
      <div className='p-card'>{body}</div>
    </div>
  )
}
