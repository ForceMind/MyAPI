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
import { useQuery } from '@tanstack/react-query'
import { getRouteApi, Link, useLocation } from '@tanstack/react-router'
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import { hasPermission } from '@/lib/admin-permissions'
import { formatTimestampToDate } from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { getChannelOps } from './api'
import type { ChannelTaskReadState } from './channel-task-state'
import { ChannelQuotaChangesPanel } from './components/channel-quota-changes-panel'
import { ChannelQuotaEvents } from './components/channel-quota-events'
import { ChannelRoutingPreview } from './components/channel-routing-preview'
import { ChannelsDialogs } from './components/channels-dialogs'
import { ChannelsPrimaryButtons } from './components/channels-primary-buttons'
import { ChannelsProvider } from './components/channels-provider'
import { ChannelsTable } from './components/channels-table'

const route = getRouteApi('/_authenticated/channels/')
const WORKSPACES = [
  { mode: 'inventory', anchor: 'channel-inventory', title: 'Channel inventory' },
  { mode: 'quota', anchor: 'channel-evidence', title: 'Quota evidence' },
  { mode: 'events', anchor: 'channel-events', title: 'All channel events' },
  { mode: 'routing', anchor: 'channel-routing', title: 'Routing Preview' },
] as const

type WorkspaceMode = typeof WORKSPACES[number]['mode']
type ReadFeedback = ChannelTaskReadState & { lastFailureAt?: number; unresolvedFailure?: boolean }
type WorkspaceAuth = ReturnType<typeof useAuthStore.getState>['auth']

function workspaceActor(auth: WorkspaceAuth): string {
  return [auth.user?.id ?? null, auth.user?.role ?? 0, auth.session?.sid ?? null,
    hasPermission(auth.user, 'channel', 'read'), hasPermission(auth.user, 'channel', 'write'),
    hasPermission(auth.user, 'channel', 'operate'), hasPermission(auth.user, 'channel', 'sensitive_write')].join(':')
}

function ReadSummary(props: { state: ReadFeedback; routing?: boolean; compact?: boolean }) {
  const { t } = useTranslation()
  const state = props.state
  let label = props.routing ? t('Not submitted') : t('Not loaded')
  if (state.phase === 'loading') label = t('Loading...')
  if (state.phase === 'ready') label = t('Saved data loaded')
  if (state.phase === 'error') label = t('Read failed')
  return <span className={cn('text-caption min-w-0 [overflow-wrap:anywhere]', (state.phase === 'error' || state.unresolvedFailure) && 'text-destructive', (state.incomplete || state.failedObservations) && 'text-warning')}>
    {label}
    {state.incomplete && <> · {t('Data incomplete')}</>}
    {state.failedObservations && <> · {props.compact ? t('Failed observations') : t('Saved quota evidence includes failed observations.')}</>}
    {state.samplingEnabled === false && <> · {props.compact ? t('Sampling off') : t('Background sampling disabled')}</>}
    {state.unresolvedFailure && state.phase !== 'error' && <> · {props.compact ? t('Read failure unresolved') : t('Earlier read failure is not yet confirmed recovered')}</>}
  </span>
}

export function Channels() {
  const identity = useAuthStore((state) => workspaceActor(state.auth))
  return <ChannelsProvider key={identity}><ChannelWorkspace identity={identity} /></ChannelsProvider>
}

function ChannelWorkspace(props: { identity: string }) {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const canRead = hasPermission(user, 'channel', 'read')
  const location = useLocation()
  const navigate = route.useNavigate()
  const search = route.useSearch()
  const hash = location.hash.replace(/^#/, '')
  const explicit = WORKSPACES.find((item) => item.anchor === hash)
  const active: WorkspaceMode = canRead ? (explicit?.mode ?? (search.quotaChannelId != null ? 'quota' : 'inventory')) : 'inventory'
  const [visited, setVisited] = useState<Record<WorkspaceMode, boolean>>({ inventory: true, quota: active === 'quota', events: active === 'events', routing: active === 'routing' })
  const [reads, setReads] = useState<Record<WorkspaceMode, ReadFeedback>>({ inventory: { phase: 'unread' }, quota: { phase: 'unread' }, events: { phase: 'unread' }, routing: { phase: 'unread' } })
  const mounted = useRef(true)
  const inventory = useRef<HTMLElement | null>(null)
  const headings = useRef<Partial<Record<WorkspaceMode, HTMLHeadingElement | null>>>({})
  const returnPoint = useRef<{ scroller: HTMLElement; top: number; left: number; focus: HTMLElement | null } | null>(null)
  const previousTask = useRef<WorkspaceMode | null>(null)

  useEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  useEffect(() => { setVisited((previous) => previous[active] ? previous : { ...previous, [active]: true }) }, [active])

  const report = useCallback((mode: WorkspaceMode, next: ChannelTaskReadState) => {
    if (!mounted.current || workspaceActor(useAuthStore.getState().auth) !== props.identity) return
    setReads((previous) => {
      const before = previous[mode]
      const failureAt = next.phase === 'error' ? (next.readAt ?? (before.phase === 'error' ? before.lastFailureAt : Date.now() / 1000)) : before.lastFailureAt
      const newerSuccess = next.phase === 'ready' && (next.readAt ?? 0) > (failureAt ?? 0)
      const canReplaceKnown = newerSuccess && (next.readAt ?? 0) >= (before.readAt ?? 0)
      const state: ReadFeedback = {
        ...next,
        readAt: Math.max(next.readAt ?? 0, before.readAt ?? 0) || undefined,
        incomplete: canReplaceKnown ? (next.incomplete ?? before.incomplete) : Boolean(next.incomplete || before.incomplete),
        failedObservations: canReplaceKnown ? (next.failedObservations ?? before.failedObservations) : Boolean(next.failedObservations || before.failedObservations),
        samplingEnabled: next.samplingEnabled ?? before.samplingEnabled,
        lastFailureAt: failureAt,
        unresolvedFailure: next.phase === 'error' || Boolean(before.unresolvedFailure && !newerSuccess),
      }
      if (before.phase === state.phase && before.readAt === state.readAt && before.incomplete === state.incomplete && before.failedObservations === state.failedObservations && before.samplingEnabled === state.samplingEnabled && before.lastFailureAt === state.lastFailureAt && before.unresolvedFailure === state.unresolvedFailure) return previous
      return { ...previous, [mode]: state }
    })
  }, [props.identity])
  const reportInventory = useCallback((next: ChannelTaskReadState) => report('inventory', next), [report])
  const reportQuota = useCallback((next: ChannelTaskReadState) => report('quota', next), [report])
  const reportEvents = useCallback((next: ChannelTaskReadState) => report('events', next), [report])
  const reportRouting = useCallback((next: ChannelTaskReadState) => report('routing', next), [report])

  const openTask = (mode: WorkspaceMode) => {
    if (mode !== 'inventory' && !canRead) return
    if (active === 'inventory' && mode !== 'inventory') {
      const scroller = inventory.current?.closest<HTMLElement>('[data-page-scroll]') ?? document.documentElement
      returnPoint.current = { scroller, top: scroller.scrollTop, left: scroller.scrollLeft, focus: document.activeElement instanceof HTMLElement ? document.activeElement : null }
    }
    const destination = WORKSPACES.find((item) => item.mode === mode)
    if (destination) void navigate({ hash: destination.anchor, search: (previous) => previous, hashScrollIntoView: false, resetScroll: false })
  }

  useLayoutEffect(() => {
    const previous = previousTask.current
    previousTask.current = active
    if (previous === active) return
    if (active === 'inventory') {
      if (previous !== null) {
        const point = returnPoint.current
        if (point) {
          point.scroller.scrollTop = point.top
          point.scroller.scrollLeft = point.left
          if (point.focus?.isConnected && !point.focus.closest('[hidden]')) {
            point.focus.focus({ preventScroll: true })
            return
          }
        }
        inventory.current?.focus({ preventScroll: true })
        inventory.current?.scrollIntoView({ block: 'start' })
      }
      return
    }
    const heading = headings.current[active]
    heading?.focus({ preventScroll: true })
    heading?.scrollIntoView({ block: 'start' })
  }, [active])

  const channelOps = useQuery({ queryKey: ['channel-ops', props.identity], queryFn: getChannelOps, enabled: canRead && active === 'inventory', retry: false, staleTime: 5 * 60 * 1000 })
  const retryTimes = channelOps.data?.data?.retry_times
  const opsFailed = channelOps.isError || channelOps.data?.success === false
  const retryUnavailable = channelOps.isSuccess && channelOps.data?.success === true && (typeof retryTimes !== 'number' || !Number.isFinite(retryTimes))
  const isRoot = user?.role === ROLE.SUPER_ADMIN
  const read = reads[active]

  return <>
    <SectionPageLayout fixedContent={false}>
      <SectionPageLayout.Title>{t('Channels')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>{t('Manage channel inventory and open saved evidence explicitly.')}</SectionPageLayout.Description>
      <SectionPageLayout.Actions>{active === 'inventory' && <ChannelsPrimaryButtons />}</SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='min-w-0 space-y-4'>
          <nav aria-label={t('Channel workspace')} className='flex min-w-0 gap-1 overflow-x-auto border-b'>
            {WORKSPACES.filter((item) => canRead || item.mode === 'inventory').map((item) => <Link key={item.mode} to='/channels' hash={item.anchor} search={(previous) => previous} hashScrollIntoView={false} resetScroll={false} activeOptions={{ includeHash: true }} disabled={item.mode !== 'inventory' && !canRead} aria-current={active === item.mode ? 'location' : undefined} aria-label={t(item.title)} onClick={(event) => {
              if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
              event.preventDefault()
              openTask(item.mode)
            }} className={cn(buttonVariants({ variant: 'ghost' }), 'h-auto min-h-11 shrink-0 rounded-none border-b-2 border-transparent px-3 py-2', active === item.mode && 'border-primary text-primary')}>
              <span className='flex min-w-0 flex-col items-start gap-0.5'>
                <span>{t(item.title)}</span>
                <ReadSummary state={reads[item.mode]} routing={item.mode === 'routing'} compact />
              </span>
            </Link>)}
          </nav>
          {WORKSPACES.filter((item) => item.mode !== active && (reads[item.mode].unresolvedFailure || reads[item.mode].incomplete || reads[item.mode].failedObservations)).map((item) => <Alert key={item.mode} variant={reads[item.mode].unresolvedFailure ? 'destructive' : 'default'}>
            <AlertDescription className='flex flex-wrap items-center justify-between gap-2'>
              <span>{t(item.title)}: <ReadSummary state={reads[item.mode]} routing={item.mode === 'routing'} /></span>
              <Button type='button' variant='outline' className='min-h-11' onClick={() => openTask(item.mode)}>{t('Open task')}</Button>
            </AlertDescription>
          </Alert>)}
          {(opsFailed || retryUnavailable) && <Alert variant={opsFailed ? 'destructive' : 'default'}>
            <AlertDescription className='flex flex-wrap items-center justify-between gap-2'>
              <span>{opsFailed ? t('Retry settings could not be read') : t('Retry settings unavailable')}</span>
              <Button type='button' variant='outline' disabled={!canRead || channelOps.isFetching} className='min-h-11' onClick={() => { void channelOps.refetch() }}>{t('Refresh saved settings')}</Button>
            </AlertDescription>
          </Alert>}
          {active === 'inventory' && typeof retryTimes === 'number' && Number.isFinite(retryTimes) && <div className='flex flex-wrap items-center gap-2 text-caption'>
            <Badge variant='outline'>{t('Max Retries')}: {retryTimes}</Badge>
            {isRoot && <Button nativeButton={false} render={<Link to='/system-settings/models/$section' params={{ section: 'routing-reliability' }} />} variant='link' className='min-h-11'>{t('Retry Settings')}</Button>}
          </div>}
          <section ref={inventory} id='channel-inventory' hidden={active !== 'inventory'} tabIndex={-1} aria-label={t('Channel inventory')} className='min-w-0 outline-none'>
            <ChannelsTable active={active === 'inventory'} onReadStateChange={reportInventory} />
          </section>
          {canRead && WORKSPACES.filter((item) => item.mode !== 'inventory').map((item) => <section key={item.mode} id={item.anchor} hidden={active !== item.mode} aria-label={t(item.title)} className='min-w-0 space-y-4 outline-none'>
            <header className='flex min-w-0 flex-wrap items-start justify-between gap-3 border-b pb-3'>
              <div className='min-w-0 space-y-1'>
                <h2 ref={(node) => { headings.current[item.mode] = node }} tabIndex={-1} className='text-lg font-semibold outline-none'>{t(item.title)}</h2>
                <p className='text-caption text-muted-foreground'>
                  {item.mode === 'quota' && (search.quotaChannelId != null ? t('Channel #{{id}}', { id: search.quotaChannelId }) : t('All channels'))}
                  {item.mode === 'events' && t('Events cover all channels; the current API does not filter them by channel.')}
                  {item.mode === 'routing' && t('Read-only local routing snapshot')}
                </p>
                {item.mode === active && <div className='flex flex-wrap gap-x-3 gap-y-1 text-caption'><ReadSummary state={read} routing={item.mode === 'routing'} />{read.readAt != null && Number.isFinite(read.readAt) && read.readAt > 0 && <span>{t('Last saved-data read: {{time}}', { time: formatTimestampToDate(read.readAt) })}</span>}</div>}
              </div>
              <Button type='button' variant='outline' className='min-h-11' data-channel-workspace-return={active === item.mode ? 'active' : undefined} onClick={() => openTask('inventory')}>{t('Back to inventory')}</Button>
            </header>
            {canRead && visited[item.mode] && <>
              {item.mode === 'quota' && <ChannelQuotaChangesPanel active={active === 'quota'} onReadStateChange={reportQuota} />}
              {item.mode === 'events' && <ChannelQuotaEvents active={active === 'events'} onReadStateChange={reportEvents} />}
              {item.mode === 'routing' && <ChannelRoutingPreview active={active === 'routing'} onReadStateChange={reportRouting} />}
            </>}
          </section>)}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
    <ChannelsDialogs />
  </>
}
