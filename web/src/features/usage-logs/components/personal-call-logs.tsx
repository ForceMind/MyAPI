/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, ChevronRight, RefreshCw } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { toIntlLocale } from '@/i18n/languages'
import { useAuthStore } from '@/stores/auth-store'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'

import { getUserLogs } from '../api'
import { usageLogSchema } from '../data/schema'
import { getPersonalCallEvidence } from '../lib/personal-call-evidence'
import { PersonalCallDetail } from './personal-call-detail'

export function PersonalCallLogs() {
  const userId = useAuthStore((state) => state.auth.user?.id)
  const role = useAuthStore((state) => state.auth.user?.role)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  return <PersonalCallLogsSession key={`${userId}:${role}:${sessionId}`} userId={userId} role={role} sessionId={sessionId} />
}

function PersonalCallLogsSession(props: { userId?: number; role?: number; sessionId: string | null }) {
  const { t, i18n } = useTranslation()
  const [period, setPeriod] = useState('today')
  const [keyDraft, setKeyDraft] = useState('')
  const [keyFilter, setKeyFilter] = useState('')
  const [result, setResult] = useState('all')
  const [page, setPage] = useState(1)
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [mobileDetail, setMobileDetail] = useState(false)
  const previousDetail = useRef(false)
  const selectedButton = useRef<HTMLButtonElement | null>(null)
  const logs = useQuery({
    queryKey: ['personal-call-logs', props.userId, props.role, props.sessionId, period, keyFilter, page],
    queryFn: async () => {
      const start = new Date()
      start.setHours(0, 0, 0, 0)
      if (period === 'week') start.setDate(start.getDate() - 6)
      if (period === 'month') start.setDate(start.getDate() - 29)
      const response = await getUserLogs({ p: page, page_size: 20, token_name: keyFilter || undefined, start_timestamp: period === 'all' ? undefined : Math.floor(start.getTime() / 1000) })
      if (!response.success || !response.data) throw new Error(response.message || 'Failed to load logs')
      return response.data
    },
    enabled: Boolean(props.userId),
    retry: false,
  })
  const callLogs = (logs.data?.items || []).flatMap((item) => {
    const parsed = usageLogSchema.safeParse(item)
    if (!parsed.success || parsed.data.user_id !== props.userId || (parsed.data.type !== 2 && parsed.data.type !== 5)) return []
    const completed = getPersonalCallEvidence(parsed.data).completed
    if (result === 'success' && !completed) return []
    if (result === 'failed' && completed) return []
    return [parsed.data]
  })
  const selected = callLogs.find((log) => log.id === selectedId) || callLogs[0]
  const hasDetail = mobileDetail && Boolean(selected)
  useEffect(() => {
    if (previousDetail.current && !hasDetail) selectedButton.current?.focus()
    previousDetail.current = hasDetail
  }, [hasDetail])

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Call logs')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>{t('View the result and details of each API call.')}</SectionPageLayout.Description>
      <SectionPageLayout.Actions><Button variant='outline' onClick={() => void logs.refetch()} disabled={logs.isFetching}><RefreshCw aria-hidden='true' className='size-4' />{t('Refresh')}</Button></SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='mx-auto w-full max-w-6xl space-y-5'>
          <form className={hasDetail ? 'hidden md:block' : ''} onSubmit={(event) => { event.preventDefault(); setKeyFilter(keyDraft.trim()); setPage(1); setMobileDetail(false) }}>
            <FieldGroup className='grid items-end gap-3 sm:grid-cols-3'>
              <Field><FieldLabel htmlFor='personal-call-period'>{t('Time range')}</FieldLabel><select id='personal-call-period' className='h-11 min-w-0 rounded-lg border border-border bg-card px-3 text-sm' value={period} onChange={(event) => { setPeriod(event.target.value); setPage(1); setMobileDetail(false) }}><option value='today'>{t('Today')}</option><option value='week'>{t('Last 7 days')}</option><option value='month'>{t('Last 30 days')}</option><option value='all'>{t('All time')}</option></select></Field>
              <Field><FieldLabel htmlFor='personal-call-key'>{t('API Key')}</FieldLabel><div className='flex min-w-0 gap-2'><Input id='personal-call-key' className='h-11 bg-card' placeholder={t('All Keys')} value={keyDraft} onChange={(event) => setKeyDraft(event.target.value)} /><Button type='submit' variant='outline' className='h-11'>{t('Filter')}</Button></div></Field>
              <Field><FieldLabel htmlFor='personal-call-result'>{t('Results on this page')}</FieldLabel><select id='personal-call-result' className='h-11 min-w-0 rounded-lg border border-border bg-card px-3 text-sm' value={result} onChange={(event) => { setResult(event.target.value); setMobileDetail(false) }}><option value='all'>{t('All results')}</option><option value='success'>{t('Success')}</option><option value='failed'>{t('Failed')}</option></select></Field>
            </FieldGroup>
          </form>
          {logs.isPending && <div role='status' aria-label={t('Loading call logs')} className='grid gap-4 md:grid-cols-2'><Skeleton className='h-80 rounded-xl' /><Skeleton className='hidden h-80 rounded-xl md:block' /><span className='sr-only'>{t('Loading call logs')}</span></div>}
          {logs.isError && <ErrorState title={t('Failed to load logs')} onRetry={() => void logs.refetch()} />}
          {!logs.isPending && !logs.isError && (
            <div className='grid items-start gap-4 md:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]'>
              <Card className={`min-w-0 border border-border bg-card py-5 shadow-none ring-0 ${hasDetail ? 'hidden md:flex' : ''}`}>
                <CardHeader className='px-4 sm:px-5'><CardTitle><h2 className='text-lg'>{t('Call list')}</h2></CardTitle><p className='text-muted-foreground text-sm'>{t('{{count}} calls on this page', { count: callLogs.length })}</p></CardHeader>
                <CardContent className='px-2 sm:px-3'>
                  {callLogs.length === 0 && <Empty><EmptyHeader><EmptyTitle>{t('No calls found')}</EmptyTitle><EmptyDescription>{t('Try another time range, Key or page.')}</EmptyDescription></EmptyHeader></Empty>}
                  <ul className='divide-y divide-border'>
                    {callLogs.map((log) => {
                      const evidence = getPersonalCallEvidence(log)
                      const createdAt = new Date(log.created_at * 1000)
                      const validDate = Number.isFinite(createdAt.getTime())
                      const active = selected?.id === log.id
                      return <li key={log.id}><button type='button' ref={active ? selectedButton : undefined} aria-pressed={active} aria-label={t('View call {{id}}', { id: log.request_id || log.id })} onClick={() => { setSelectedId(log.id); setMobileDetail(true) }} className={`flex w-full min-w-0 items-center gap-3 rounded-md border-l-2 px-3 py-5 text-left focus-visible:outline-2 focus-visible:outline-primary ${active ? 'border-primary bg-accent/60' : 'border-transparent hover:bg-muted/50'}`}><span className='w-16 shrink-0 space-y-1'><span className='block text-sm font-medium'>{validDate ? createdAt.toLocaleTimeString(toIntlLocale(i18n.language), { hour: '2-digit', minute: '2-digit' }) : t('Unknown')}</span><span className='text-muted-foreground block text-xs'>{validDate ? createdAt.toLocaleDateString(toIntlLocale(i18n.language)) : t('Unknown')}</span></span><span className='min-w-0 flex-1 space-y-2'><span className='block break-words text-sm font-medium'>{log.token_name || log.model_name || t('Unknown')}</span><span className='text-muted-foreground block break-words text-xs'>{log.model_name}</span></span><span className='min-w-0 flex-1 space-y-2'><span className={`block text-sm font-medium ${evidence.completed ? 'text-emerald-700 dark:text-emerald-300' : 'text-red-700 dark:text-red-300'}`}>{evidence.completed ? t('Success') : t('Failed')}</span><span className='text-muted-foreground block text-xs'>{evidence.usageKnown ? t('Usage reported') : t('Usage not provided')}</span></span><ChevronRight aria-hidden='true' className='size-4 shrink-0 text-primary' /></button></li>
                    })}
                  </ul>
                  <div className='mt-5 flex flex-wrap items-center justify-between gap-2 px-2'><Button variant='outline' onClick={() => { setPage(page - 1); setMobileDetail(false) }} disabled={page <= 1}>{t('Previous')}</Button><span className='text-muted-foreground text-sm'>{t('Page {{page}}', { page })}</span><Button variant='outline' onClick={() => { setPage(page + 1); setMobileDetail(false) }} disabled={page * 20 >= (logs.data?.total || 0)}>{t('Next')}</Button></div>
                </CardContent>
              </Card>
              {selected && <div className={`min-w-0 ${hasDetail ? '' : 'hidden md:block'}`}><Button variant='ghost' className='mb-3 md:hidden' onClick={() => setMobileDetail(false)}><ArrowLeft aria-hidden='true' className='size-4' />{t('Back to call list')}</Button><PersonalCallDetail log={selected} /></div>}
            </div>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
