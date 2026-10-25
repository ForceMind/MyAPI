/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Key01Icon, PlusSignIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { getRouteApi, Link } from '@tanstack/react-router'
import { useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { getRequestFacts } from '@/features/usage-logs/lib/request-facts'
import { useMediaQuery } from '@/hooks/use-media-query'
import { useConfirmedFundingMode } from '@/hooks/use-personal-workbench-mode'
import { useProductEdition } from '@/hooks/use-product-edition'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { useStatus } from '@/hooks/use-status'
import { hasPermission } from '@/lib/admin-permissions'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { linkButtonClassName } from '@/lib/link-button'
import { useAuthStore } from '@/stores/auth-store'

import { useDashboardContentVisibility } from '../../hooks/use-status-data'
import {
  buildKeyNodes,
  hourlyBuckets,
  medianSeconds,
  selectionFromSearch,
  tightestWindowByConnection,
  trafficBy,
  type FlowSelection,
} from '../../lib/overview-flow'
import { FlowBoard, type FlowColumnState } from '../flow-board/flow-board'
import {
  GatewayCoreCard,
  type GatewayAddress,
} from '../flow-board/gateway-core-card'
import { AnnouncementsPanel } from './announcements-panel'
import { ApiInfoPanel } from './api-info-panel'
import { FAQPanel } from './faq-panel'
import { OverviewAttention } from './overview-attention'
import { OverviewRecentRequests } from './overview-recent-requests'
import { PerformanceHealthPanel } from './performance-health-panel'
import { SummaryCards } from './summary-cards'
import { UptimePanel } from './uptime-panel'
import { useOverviewData } from './use-overview-data'

const route = getRouteApi('/_authenticated/dashboard/$section')
const LOOPBACK_HOSTS = new Set(['localhost', '127.0.0.1', '[::1]', '::1'])

function columnState(query: {
  isPending: boolean
  isError: boolean
}): FlowColumnState {
  if (query.isError) return 'error'
  return query.isPending ? 'loading' : 'ready'
}

/**
 * Overview (design v2 2.3, direction 3 "flow"): what needs attention, the
 * flow board (API Keys -> gateway -> upstream connections) and the latest
 * requests, filtered by the selected node. The selection lives in the URL
 * (`?key=` or `?connection=`). Funds appear only when the server has
 * confirmed a commercial funding mode; site content only where the edition
 * provides it.
 */
export function OverviewPage() {
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  if (!user) return null
  // A different account or session starts from empty state.
  return <OverviewSession key={`${user.id}:${user.role}:${sessionId ?? ''}`} />
}

function OverviewSession() {
  const { t } = useTranslation()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const data = useOverviewData()
  const edition = useProductEdition()
  const { status } = useStatus()
  const fundingMode = useConfirmedFundingMode()
  const visibility = useDashboardContentVisibility()
  const mobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const showKeys = useIsSidebarModuleVisible('/keys')
  const showConnections = useIsSidebarModuleVisible('/connections')
  const requestsRef = useRef<HTMLElement>(null)
  const selection = selectionFromSearch(search)
  const isAdmin = data.isAdmin
  const canAddConnection =
    showConnections &&
    isAdmin &&
    hasPermission(data.user, 'channel', 'read') &&
    hasPermission(data.user, 'channel', 'write')
  const siteContent = edition.capabilities.site_content
  const showFunds = fundingMode === 'enabled' || fundingMode === 'retirement'
  const contentPanels = [
    siteContent && visibility.apiInfo && <ApiInfoPanel key='api-info' />,
    siteContent && visibility.announcements && (
      <AnnouncementsPanel key='announcements' />
    ),
    siteContent && visibility.faq && <FAQPanel key='faq' />,
    siteContent && visibility.uptimeKuma && <UptimePanel key='uptime' />,
  ].filter(Boolean)

  const sample = data.sample.isError ? undefined : data.sample.data
  const rows = sample?.rows
  const viewerId = data.user?.id
  const keyNodes = useMemo(
    () => buildKeyNodes(data.keys.data ?? [], rows, viewerId),
    [data.keys.data, rows, viewerId]
  )
  const connectionTraffic = useMemo(
    () => (rows ? trafficBy(rows, (row) => row.channel) : undefined),
    [rows]
  )
  const windows = useMemo(
    () =>
      tightestWindowByConnection(
        data.upstream.accountsQuery.isError
          ? []
          : (data.upstream.accountsQuery.data?.items ?? [])
      ),
    [data.upstream.accountsQuery.isError, data.upstream.accountsQuery.data]
  )
  const buckets = useMemo(() => {
    if (!sample) return null
    // A truncated sample only reaches back to its oldest row.
    const oldest = sample.rows.at(-1)?.created_at
    const coverageStart =
      sample.truncated && oldest !== undefined ? oldest : data.since
    return hourlyBuckets(sample.rows, Date.now() / 1000, coverageStart)
  }, [sample, data.since])

  const failures = rows?.filter((row) => !getRequestFacts(row).completed)
  const usageMissing = rows?.filter((row) => {
    const facts = getRequestFacts(row)
    return facts.completed && !facts.usageKnown
  }).length
  const serverAddress =
    typeof status?.server_address === 'string'
      ? status.server_address.trim().replace(/\/+$/, '')
      : ''
  const address: GatewayAddress = serverAddress
    ? { baseUrl: `${serverAddress}/v1`, source: 'server' }
    : { baseUrl: `${window.location.origin}/v1`, source: 'current' }
  const version =
    typeof status?.version === 'string' && status.version
      ? status.version
      : t('Version not provided')
  const meta = [edition.edition === 'lite' ? 'Lite' : 'Full', version]
  if (edition.edition === 'lite') {
    meta.push(
      LOOPBACK_HOSTS.has(window.location.hostname)
        ? t('Opened on this computer')
        : t('Opened over the network')
    )
  }
  if (usageMissing) {
    meta.push(t('Usage not provided: {{count}}', { count: usageMissing }))
  }

  let caption = t('Node counts come from the latest requests in 24 hours.')
  if (rows) {
    caption = sample?.truncated
      ? t(
          'Node counts come from the latest {{count}} requests; older ones in 24 hours are not counted.',
          { count: rows.length }
        )
      : t('Node counts cover all {{count}} requests in 24 hours.', {
          count: rows.length,
        })
  } else if (data.sample.isError) {
    caption = t('Request counts are unknown: call records could not be read.')
  }

  const select = (next: FlowSelection | null) => {
    void navigate({
      search: (previous) => ({
        ...previous,
        key: next?.kind === 'key' ? next.id : undefined,
        connection: next?.kind === 'connection' ? next.id : undefined,
      }),
      resetScroll: false,
    })
    // On a phone the table is below the whole pipe; bring it into view.
    if (next && mobile) {
      requestsRef.current?.scrollIntoView({ block: 'start' })
    }
  }

  let selectionLabel = ''
  if (selection?.kind === 'key') {
    const name =
      keyNodes.find((node) => node.id === selection.id)?.name ||
      `#${selection.id}`
    selectionLabel = t('Key: {{name}}', { name })
  } else if (selection) {
    const name =
      data.connections.data?.items.find((item) => item.id === selection.id)
        ?.name ||
      rows?.find((row) => row.channel === selection.id)?.channel_name ||
      `#${selection.id}`
    selectionLabel = t('Upstream: {{name}}', { name })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Overview')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t(
          'Who calls, through the gateway, to which upstream. Select a node to filter the requests below.'
        )}
      </SectionPageLayout.Description>
      <SectionPageLayout.Actions>
        {canAddConnection && (
          <Link
            to='/connections'
            data-myapi-button=''
            className={linkButtonClassName({ variant: 'outline' })}
          >
            <HugeiconsIcon
              icon={PlusSignIcon}
              data-icon='inline-start'
              aria-hidden='true'
            />
            {t('Add connection')}
          </Link>
        )}
        {showKeys && (
          <Link
            to='/keys'
            data-myapi-button=''
            className={linkButtonClassName()}
          >
            <HugeiconsIcon
              icon={Key01Icon}
              data-icon='inline-start'
              aria-hidden='true'
            />
            {t('Create API Key')}
          </Link>
        )}
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex min-w-0 flex-col gap-5' data-testid='overview'>
          <OverviewAttention data={data} />
          <FlowBoard
            caption={caption}
            selection={selection}
            onSelect={select}
            keys={{
              nodes: keyNodes,
              state: columnState(data.keys),
              onRetry: () => void data.keys.refetch(),
              createAction: showKeys && (
                <Link
                  to='/keys'
                  data-myapi-button=''
                  className={linkButtonClassName({ size: 'sm' })}
                >
                  {t('Create API Key')}
                </Link>
              ),
            }}
            connections={
              data.canReadChannels && showConnections
                ? {
                    items: data.connections.data?.items ?? [],
                    total: data.connections.data?.total ?? 0,
                    windows,
                    windowsError: data.upstream.accountsQuery.isError,
                    onRetryWindows: () =>
                      void data.upstream.accountsQuery.refetch(),
                    traffic: connectionTraffic,
                    state: columnState(data.connections),
                    onRetry: () => void data.connections.refetch(),
                    addAction: canAddConnection && (
                      <Link
                        to='/connections'
                        data-myapi-button=''
                        className={linkButtonClassName({ size: 'sm' })}
                      >
                        {t('Add connection')}
                      </Link>
                    ),
                  }
                : null
            }
            core={
              <GatewayCoreCard
                address={address}
                requests={
                  data.counts.isError ? null : (data.counts.data ?? null)
                }
                requestsLoading={data.counts.isPending}
                failureRate={
                  rows && rows.length > 0 && failures
                    ? failures.length / rows.length
                    : null
                }
                medianSeconds={
                  rows
                    ? medianSeconds(rows.map((row) => row.use_time))
                    : undefined
                }
                sampleLoading={data.sample.isPending}
                buckets={buckets}
                meta={meta}
              />
            }
          />
          <OverviewRecentRequests
            ref={requestsRef}
            data={data}
            selection={selection}
            selectionLabel={selectionLabel}
            onClearSelection={() => select(null)}
          />
          {showFunds && <SummaryCards />}
          {isAdmin && edition.capabilities.perf_metrics && (
            <PerformanceHealthPanel />
          )}
          {contentPanels.length > 0 && (
            <div className='grid min-w-0 gap-4 lg:grid-cols-2'>
              {contentPanels}
            </div>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
