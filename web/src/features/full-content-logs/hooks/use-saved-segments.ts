/* Copyright (C) 2026 ForceMind. AGPL-3.0-or-later. */
import { useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { useLayoutEffect, useRef, useState } from 'react'

import { useAuthStore } from '@/stores/auth-store'

import { getFullContentLogSegment } from '../api'
import { denySavedContent, savedContentStatus, type SavedContentActor } from '../lib/saved-content-access'
import { savedSegmentPositions } from '../lib/segment-position'
import { savedByteOffset, SAVED_SEGMENT_LIMIT } from '../lib/saved-segment'
import type { SavedContentPart, SavedContentSegment, SavedContentSegmentRequest } from '../types'

type FailureKind = 'position' | 'denied' | 'read' | 'conflict' | 'changed' | 'ambiguous' | 'corrupt' | 'missing' | 'range' | 'invalid'
type SegmentFailure = { kind: FailureKind; conflict: boolean }

export function useSavedSegments(props: { requestId: string; part: SavedContentPart; actor: SavedContentActor; active: boolean }) {
  const client = useQueryClient()
  const key = JSON.stringify([props.actor.id, props.actor.role, props.actor.sid, props.requestId, props.part])
  const [open, setOpen] = useState(false)
  const [page, setPage] = useState<SavedContentSegment | null>(null)
  const [pending, setPending] = useState(false)
  const [failure, setFailure] = useState<SegmentFailure | null>(null)
  const [position, setPosition] = useState('0')
  const controller = useRef<AbortController | null>(null)
  const sequence = useRef(0)
  const mounted = useRef(true)
  const opened = useRef(false)
  const active = useRef(props.active)
  active.current = props.active
  const lastRequest = useRef<SavedContentSegmentRequest | null>(null)
  const backPositions = useRef<Array<{ offset: string; snapshot: string }>>([])
  const [backCount, setBackCount] = useState(0)
  const current = () => {
    const auth = useAuthStore.getState().auth
    return mounted.current && opened.current && active.current && auth.user?.id === props.actor.id &&
      auth.user?.role === props.actor.role && (auth.session?.sid ?? null) === props.actor.sid
  }
  const close = () => {
    opened.current = false
    sequence.current += 1
    controller.current?.abort()
    setOpen(false)
    setPending(false)
    setPage(null)
    setFailure(null)
    backPositions.current = []
    setBackCount(0)
  }
  useLayoutEffect(() => {
    mounted.current = true
    return () => { mounted.current = false; opened.current = false; sequence.current += 1; controller.current?.abort() }
  }, [])
  useLayoutEffect(() => {
    if (props.active) return
    opened.current = false
    sequence.current += 1
    controller.current?.abort()
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setOpen(false)
    setPage(null)
    setPending(false)
  }, [props.active])

  const read = async (offset: string, snapshot?: string, remember = true) => {
    if (!current()) return
    const parsed = savedByteOffset(offset)
    if (parsed === null || (page && parsed > BigInt(page.total_bytes))) {
      setFailure({ kind: 'position', conflict: false })
      return
    }
    if (remember && page && (offset !== page.requested_offset || snapshot !== page.snapshot)) {
      const previous = backPositions.current.at(-1)
      if (!previous || previous.offset !== page.requested_offset || previous.snapshot !== page.snapshot) {
        backPositions.current = [...backPositions.current, { offset: page.requested_offset, snapshot: page.snapshot }].slice(-19)
        setBackCount(backPositions.current.length)
      }
    }
    controller.current?.abort()
    const attempt = ++sequence.current
    const cancel = new AbortController()
    controller.current = cancel
    const request: SavedContentSegmentRequest = { part: props.part, offset, limit: SAVED_SEGMENT_LIMIT, ...(snapshot ? { snapshot } : {}) }
    lastRequest.current = request
    setPending(true)
    setFailure(null)
    setPosition(offset)
    try {
      const result = await getFullContentLogSegment(props.requestId, request, cancel.signal)
      if (!current() || cancel.signal.aborted || attempt !== sequence.current) return
      setPage(result)
      setPosition(result.requested_offset)
      savedSegmentPositions.write(key, { offset: result.requested_offset, snapshot: result.snapshot })
    } catch (error) {
      if (!current() || cancel.signal.aborted || attempt !== sequence.current) return
      setPage(null)
      const status = savedContentStatus(error)
      if (status === 401 || status === 403) {
        backPositions.current = []
        setBackCount(0)
        savedSegmentPositions.remove(key)
        setFailure({ kind: 'denied', conflict: false })
        await denySavedContent(client, props.requestId, props.actor, status)
        return
      }
      let kind: FailureKind = 'read'
      const code: unknown = isAxiosError(error) ? error.response?.data?.code : undefined
      if (status === 409) {
        backPositions.current = []
        setBackCount(0)
        savedSegmentPositions.remove(key)
        kind = 'conflict'
        if (code === 'FULL_CONTENT_SEGMENT_CHANGED') kind = 'changed'
        else if (code === 'FULL_CONTENT_SEGMENT_AMBIGUOUS') kind = 'ambiguous'
        else if (code === 'FULL_CONTENT_SEGMENT_CORRUPT') kind = 'corrupt'
      } else if (status === 404) { savedSegmentPositions.remove(key); kind = 'missing' }
      else if (status === 416) kind = 'range'
      else if (status === 400 || (error instanceof Error && error.message === 'Segment data is invalid.')) kind = 'invalid'
      setFailure({ kind, conflict: status === 409 })
    } finally {
      if (current() && attempt === sequence.current) setPending(false)
    }
  }
  const start = () => {
    if (!props.active) return
    opened.current = true
    setOpen(true)
    backPositions.current = []
    setBackCount(0)
    const previous = savedSegmentPositions.read(key)
    void read(previous?.offset ?? '0', previous?.snapshot, false)
  }
  return {
    open, page, pending, failure, position, setPosition, start, close, canPrevious: backCount > 0,
    isCurrent: current, readVersion: () => sequence.current,
    jump: () => { if (page) void read(position, page.snapshot) },
    next: () => { if (page?.next_offset) void read(page.next_offset, page.snapshot) },
    previous: () => {
      const previous = backPositions.current.pop()
      setBackCount(backPositions.current.length)
      if (previous && page) void read(previous.offset, previous.snapshot, false)
    },
    first: () => { if (page) void read('0', page.snapshot) },
    reload: () => { savedSegmentPositions.remove(key); backPositions.current = []; setBackCount(0); void read('0', undefined, false) },
    retry: () => { if (lastRequest.current) void read(lastRequest.current.offset, lastRequest.current.snapshot) },
  }
}
