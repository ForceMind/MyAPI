/* Copyright (C) 2026 ForceMind. AGPL-3.0-or-later. */
import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { copyToClipboard } from '@/lib/copy-to-clipboard'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { useSavedSegments } from '../hooks/use-saved-segments'
import type { SavedContentActor } from '../lib/saved-content-access'
import type { SavedContentPart } from '../types'

type Props = { requestId: string; part: SavedContentPart; active?: boolean; available?: boolean; preview?: ReactNode }

export function SavedContentSegments(props: Props) {
  const user = useAuthStore((state) => state.auth.user)
  const sid = useAuthStore((state) => state.auth.session?.sid ?? null)
  const scope = JSON.stringify([user?.id, user?.role, sid, props.requestId, props.part])
  const offered = useRef({ scope, available: props.available !== false })
  if (offered.current.scope !== scope) offered.current = { scope, available: props.available !== false }
  else if (props.available !== false) offered.current.available = true
  if (!user || user.role < ROLE.ADMIN || !offered.current.available) return props.preview ?? null
  return <SegmentReader key={scope} {...props} actor={{ id: user.id, role: user.role, sid }} />
}

function SegmentReader(props: Props & { actor: SavedContentActor }) {
  const { t } = useTranslation()
  const segment = useSavedSegments({ ...props, active: props.active !== false })
  const [wrap, setWrap] = useState(true)
  const [copied, setCopied] = useState<string | null>(null)
  const [copyFailure, setCopyFailure] = useState<string | null>(null)
  const mounted = useRef(true)
  const entry = useRef<HTMLButtonElement>(null)
  const heading = useRef<HTMLHeadingElement>(null)
  const previousOpen = useRef(false)
  useLayoutEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  useLayoutEffect(() => {
    if (segment.open) heading.current?.focus()
    else if (previousOpen.current && props.active !== false) entry.current?.focus()
    previousOpen.current = segment.open
  }, [segment.open, props.active])
  const page = segment.page
  const pageKey = page ? `${page.snapshot}:${page.offset}` : ''
  const feedbackKey = `${segment.readVersion()}:${pageKey}`
  const currentPageKey = useRef(pageKey)
  currentPageKey.current = pageKey
  const title = props.part === 'request' ? t('Saved request segments') : t('Saved response segments')
  const messages = {
    position: t('Enter a saved byte position within the available range.'),
    denied: t('Request content is available to administrators only.'),
    read: t('Failed to read the saved segment.'),
    conflict: t('Saved content cannot be read consistently. Reload to try again.'),
    changed: t('Saved content changed. Reload to start a new read.'),
    ambiguous: t('Saved request identity is ambiguous. Reload to try again.'),
    corrupt: t('Saved content is corrupt. Reload to try again.'),
    missing: t('No saved content for this request part.'),
    range: t('The saved byte position is outside the available content.'),
    invalid: t('Segment data is invalid.'),
  }
  let completeness = t('Unknown')
  if (page?.source_complete === true) completeness = t('Complete saved capture')
  else if (page?.source_complete === false) completeness = t('Incomplete saved capture')
  const copy = async () => {
    if (!page || !segment.isCurrent()) return
    const version = segment.readVersion()
    const success = await copyToClipboard(page.body)
    const auth = useAuthStore.getState().auth
    if (!mounted.current || !segment.isCurrent() || segment.readVersion() !== version || currentPageKey.current !== pageKey || auth.user?.id !== props.actor.id || auth.user.role !== props.actor.role || (auth.session?.sid ?? null) !== props.actor.sid) return
    setCopied(success ? feedbackKey : null)
    setCopyFailure(success ? null : feedbackKey)
  }
  return <div className='min-w-0 space-y-3'>
    <div hidden={segment.open} className='min-w-0 space-y-3'>
      {props.preview}
    <Button ref={entry} type='button' variant='outline' className='min-h-11' aria-expanded={segment.open} disabled={props.active === false} onClick={segment.start}>
      {props.part === 'request' ? t('Read saved request in segments') : t('Read saved response in segments')}
    </Button>
    </div>
    {segment.open && <section aria-label={title} className='bg-muted/20 min-w-0 space-y-3 rounded-lg border p-3'>
      <div className='flex min-w-0 flex-wrap items-center justify-between gap-2'>
        <h3 ref={heading} tabIndex={-1} className='font-semibold'>{title}</h3>
        <Button type='button' variant='outline' className='min-h-11' onClick={segment.close}>{t('Close segment reader')}</Button>
      </div>
      <p className='text-caption text-muted-foreground'>{t('Byte positions refer to saved content, not request success.')}</p>
      <p className='text-caption text-muted-foreground'>{t('Segments contain exact saved bytes. SSE event search still uses the bounded preview.')}</p>
      {segment.pending && <p role='status'>{t('Loading...')}</p>}
      {segment.failure && <Alert variant='destructive'>
        <AlertTitle>{messages[segment.failure.kind]}</AlertTitle>
        {segment.failure.kind !== 'position' && <AlertDescription>
          <Button type='button' variant='outline' className='mt-2 min-h-11' disabled={segment.pending} onClick={segment.failure.conflict || segment.failure.kind === 'range' ? segment.reload : segment.retry}>
            {segment.failure.conflict || segment.failure.kind === 'range' ? t('Reload saved content') : t('Retry')}
          </Button>
        </AlertDescription>}
      </Alert>}
      {page && <>
        <dl className='grid min-w-0 grid-cols-1 gap-2 text-sm sm:grid-cols-2'>
          <div><dt className='text-muted-foreground'>{t('Requested byte position')}</dt><dd className='font-mono wrap-anywhere'>{page.requested_offset}</dd></div>
          <div><dt className='text-muted-foreground'>{t('Actual byte position')}</dt><dd className='font-mono wrap-anywhere'>{page.offset}</dd></div>
          <div><dt className='text-muted-foreground'>{t('Saved bytes')}</dt><dd className='font-mono wrap-anywhere'>{page.total_bytes}</dd></div>
          <div><dt className='text-muted-foreground'>{t('Expected bytes')}</dt><dd className='font-mono wrap-anywhere'>{page.expected_bytes ?? t('Unknown')}</dd></div>
          <div><dt className='text-muted-foreground'>{t('Capture completeness')}</dt><dd>{completeness}</dd></div>
          <div><dt className='text-muted-foreground'>{t('Encoding')}</dt><dd className='font-mono wrap-anywhere'>{page.encoding}{page.content_type ? ` · ${page.content_type}` : ''}</dd></div>
        </dl>
        <div className='flex flex-wrap gap-2'>
          <Button type='button' variant='outline' className='min-h-11' aria-pressed={wrap} onClick={() => setWrap((previous) => !previous)}>{t('Wrap lines')}</Button>
          <Button type='button' variant='outline' className='min-h-11' disabled={!page.body} onClick={() => void copy()}>{t('Copy')}</Button>
          {copied === feedbackKey && <span role='status'>{t('Copied to clipboard')}</span>}
        </div>
        {copyFailure === feedbackKey && <p role='alert'>{t('Failed to copy to clipboard')}</p>}
        <pre tabIndex={0} className={`bg-background max-h-[48vh] max-w-full overflow-auto rounded-lg border p-3 font-mono text-xs ${wrap ? 'break-all whitespace-pre-wrap' : 'whitespace-pre'}`}>{page.body || t('Empty saved segment')}</pre>
      </>}
      <div className='flex flex-wrap gap-2'>
        <Button type='button' variant='outline' className='min-h-11' disabled={segment.pending || !page || !segment.canPrevious} onClick={segment.previous}>{t('Previous segment')}</Button>
        <Button type='button' variant='outline' className='min-h-11' disabled={segment.pending || !page?.has_more} onClick={segment.next}>{t('Next segment')}</Button>
        <Button type='button' variant='outline' className='min-h-11' disabled={segment.pending || !page} onClick={segment.first}>{t('First segment')}</Button>
      </div>
      <form className='flex flex-wrap items-end gap-2' onSubmit={(event) => { event.preventDefault(); segment.jump() }}>
        <label className='flex min-w-0 flex-1 flex-col gap-1'><span className='text-caption'>{t('Saved byte position')}</span><Input type='text' inputMode='numeric' className='min-h-11 font-mono' maxLength={20} value={segment.position} disabled={segment.pending} onChange={(event) => segment.setPosition(event.target.value)} /></label>
        <Button type='submit' variant='outline' className='min-h-11' disabled={segment.pending || !page}>{t('Go to saved byte')}</Button>
      </form>
    </section>}
  </div>
}
