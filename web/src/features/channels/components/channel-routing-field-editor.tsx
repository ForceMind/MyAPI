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
/* Copyright (C) 2026 ForceMind. */
import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { hasPermission } from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'
import { Button } from '@/components/ui/button'

import { getChannel } from '../api'
import { channelsQueryKeys, handleUpdateChannelField, type ChannelFieldUpdateResult } from '../lib/channel-actions'
import { createChannelFieldUpdateScheduler } from '../lib/channel-field-update'
import { NumericSpinnerInput } from './numeric-spinner-input'

type Props = {
  channelId: number
  value: number | null | undefined
  field: 'priority' | 'weight'
  /** A connection page supplies its freshly verified write grant. */
  canWrite?: boolean
  readVersion?: number
}

/** One editor contract for the connection cards, routing page and table. */
export function ChannelRoutingFieldEditor(props: Props) {
  const user = useAuthStore((state) => state.auth.user)
  const sid = useAuthStore((state) => state.auth.session?.sid ?? null)
  return <RoutingFieldControl key={`${user?.id}:${user?.role}:${sid}:${props.channelId}:${props.field}`} {...props} actorId={user?.id} actorRole={user?.role} actorSid={sid} />
}

function RoutingFieldControl(props: Props & { actorId: number | undefined; actorRole: number | undefined; actorSid: string | null }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [result, setResult] = useState<ChannelFieldUpdateResult | null>(null)
  const [phase, setPhase] = useState<'idle' | 'pending' | 'saving' | 'reading'>('idle')
  const [draft, setDraft] = useState<number | null>(null)
  const [readSnapshot, setReadSnapshot] = useState<{ value: number; previousProp: Props['value']; readVersion?: number; revision: number } | null>(null)
  const [confirmed, setConfirmed] = useState<{ value: number; baseValue: Props['value']; readVersion?: number } | null>(null)
  const [readError, setReadError] = useState('')
  const mounted = useRef(true)
  const writeGrant = useRef(props.canWrite)
  writeGrant.current = props.canWrite
  const latestRead = useRef({ value: props.value, readVersion: props.readVersion })
  latestRead.current = { value: props.value, readVersion: props.readVersion }
  const isCurrent = useCallback(() => {
      const current = useAuthStore.getState().auth
      return current.user?.id === props.actorId && current.user?.role === props.actorRole &&
        (current.session?.sid ?? null) === props.actorSid &&
        writeGrant.current !== false && hasPermission(current.user, 'channel', 'write')
    }, [props.actorId, props.actorRole, props.actorSid])
  const scheduler = useMemo(() =>
    createChannelFieldUpdateScheduler((value) => {
      if (!isCurrent()) return
      if (mounted.current) setPhase('saving')
      void handleUpdateChannelField(props.channelId, props.field, value, client,
        () => { void client.invalidateQueries({ queryKey: channelsQueryKeys.detail(props.channelId) }) },
        { isCurrent, onResult: (nextResult) => {
          if (!mounted.current || !isCurrent()) return
          if (nextResult.state === 'saved' || nextResult.state === 'cache-pending') {
            setConfirmed({ value, baseValue: latestRead.current.value, readVersion: latestRead.current.readVersion })
          }
          setResult(nextResult)
          setPhase('idle')
        } })
    }), [isCurrent, props.channelId, props.field, client])
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      scheduler.flush()
    }
  }, [scheduler])
  const currentUser = useAuthStore((state) => state.auth.user)
  const canWrite = props.canWrite !== false && hasPermission(currentUser, 'channel', 'write')
  useEffect(() => {
    if (canWrite || phase === 'idle') return
    // Flush only clears the queued draft when the current write guard is false.
    scheduler.flush()
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setResult({ state: phase === 'pending' ? 'failed' : 'unknown' })
    setPhase('idle')
  }, [canWrite, phase, scheduler])
  let value = props.value
  if (readSnapshot && (props.readVersion === undefined ? props.value === readSnapshot.previousProp : props.readVersion === readSnapshot.readVersion)) value = readSnapshot.value
  if (confirmed && (props.readVersion === undefined ? props.value === confirmed.baseValue : props.readVersion === confirmed.readVersion)) value = confirmed.value
  if (draft !== null && (phase === 'pending' || phase === 'saving' || phase === 'reading' || result?.state === 'failed' || result?.state === 'unknown')) value = draft
  const known = typeof value === 'number' && Number.isFinite(value)
  const unknown = result?.state === 'unknown'
  const readSavedValue = async () => {
    if (!isCurrent() || !mounted.current || phase === 'reading') return
    setReadError('')
    setPhase('reading')
    try {
      const response = await getChannel(props.channelId, { disableDuplicate: true, skipErrorHandler: true, isCurrentScope: isCurrent })
      if (!isCurrent() || !mounted.current) return
      const saved = response.data?.[props.field]
      if (!response.success || response.data?.id !== props.channelId || typeof saved !== 'number' || !Number.isFinite(saved)) {
        throw new Error('Saved routing value unavailable')
      }
      setReadSnapshot((previous) => ({ value: saved, previousProp: latestRead.current.value, readVersion: latestRead.current.readVersion, revision: (previous?.revision ?? 0) + 1 }))
      setConfirmed(null)
      setResult(null)
      void client.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
      void client.invalidateQueries({ queryKey: channelsQueryKeys.detail(props.channelId) })
    } catch {
      if (isCurrent() && mounted.current) setReadError(t('Unable to read the saved routing value. Editing remains locked.'))
    } finally {
      if (isCurrent() && mounted.current) setPhase('idle')
    }
  }
  let feedback = ''
  if (phase === 'pending') feedback = t('Pending save')
  else if (phase === 'saving') feedback = t('Saving...')
  else if (phase === 'reading') feedback = t('Loading...')
  else if (result?.state === 'saved') feedback = t('Saved')
  else if (result?.state === 'cache-pending') feedback = t('Saved; routing cache publication pending')
  else if (result?.state === 'failed') feedback = result.message || t('Failed to update channel')
  else if (result?.state === 'unknown') feedback = t('Save result unknown. Read the saved value before trying again.')
  const failed = phase === 'idle' && (result?.state === 'failed' || result?.state === 'unknown')
  return <div role='group' aria-label={props.field === 'priority' ? t('Priority') : t('Weight')} className='flex min-w-0 flex-col items-start gap-1 [&>div]:max-w-full [&>div>div]:h-auto [&>div>div]:min-h-11 [&>div>div]:max-w-full [&>div>div]:flex-wrap [&_button]:min-h-11 [&_button]:min-w-11 [&_input]:min-h-11'>
    {known ? <NumericSpinnerInput
      key={readSnapshot?.revision ?? 0}
      disabled={!canWrite || phase === 'saving' || phase === 'reading' || unknown}
      value={value}
      onChange={(value) => {
        if (!canWrite || unknown || phase === 'reading') return
        setDraft(value)
        setResult(null)
        setPhase('pending')
        scheduler.schedule(value)
      }}
      onCommit={scheduler.flush}
      min={props.field === 'priority' ? -999 : 0}
    /> : <span>{t('Unknown')}</span>}
    {feedback && <p role={failed ? 'alert' : 'status'} className='text-caption text-muted-foreground wrap-anywhere'>{feedback}</p>}
    {unknown && <Button variant='outline' size='sm' disabled={!canWrite || phase === 'reading'} onClick={() => void readSavedValue()}>{t('Read saved routing value')}</Button>}
    {readError && <p role='alert' className='text-caption text-destructive wrap-anywhere'>{readError}</p>}
    {!result && phase === 'idle' && readSnapshot && draft !== null && draft !== readSnapshot.value && <p className='text-caption text-muted-foreground'>{t('Previous draft: {{value}}', { value: draft })}</p>}
  </div>
}
