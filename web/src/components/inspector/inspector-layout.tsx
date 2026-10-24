/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Cancel01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  useEffect,
  useLayoutEffect,
  useRef,
  type KeyboardEvent,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet'
import { cn } from '@/lib/utils'
import { useInspectorStore } from '@/stores/inspector-store'

import { useInspectorPresentation } from './use-inspector-presentation'

/**
 * List plus inspector (direction A). The open object always lives in the
 * URL, so refresh, back and shared links restore it; this component only
 * decides the presentation:
 *
 * - below 1024px the inspector replaces the list as a full-screen page and
 *   the top bar offers Back (the list stays mounted, hidden, so its filters,
 *   scroll position and focus return survive);
 * - 1024-1279px it overlays the list from the right;
 * - from 1280px it sits beside the list.
 */
export function InspectorLayout(props: {
  list: ReactNode
  inspector: ReactNode
  open: boolean
  /** Accessible name of the open object, also used by the shell. */
  label: string
  onClose: () => void
  /**
   * The page bounds its content height (SectionPageLayout `fixedContent`)
   * and the list scrolls inside; the side inspector then scrolls on its own
   * instead of sticking to the page scroll.
   */
  fill?: boolean
  className?: string
}) {
  const presentation = useInspectorPresentation()
  const setCurrent = useInspectorStore((state) => state.setCurrent)
  const closeRef = useRef(props.onClose)
  closeRef.current = props.onClose
  const listScroll = useRef<number | null>(null)
  const rootRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!props.open) {
      setCurrent(null)
      return
    }
    setCurrent({ title: props.label, close: () => closeRef.current() })
    return () => setCurrent(null)
  }, [props.open, props.label, setCurrent])

  // The mobile page scrolls in the shell's route container. Remember where
  // the list was so returning from the full-screen inspector lands there.
  useLayoutEffect(() => {
    if (presentation !== 'page') return
    const scroller = rootRef.current?.closest('.pc-route-content')
    if (!(scroller instanceof HTMLElement)) return
    if (props.open) {
      if (listScroll.current === null) listScroll.current = scroller.scrollTop
      scroller.scrollTop = 0
    } else if (listScroll.current !== null) {
      scroller.scrollTop = listScroll.current
      listScroll.current = null
    }
  }, [props.open, presentation])

  const closeOnEscape = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key !== 'Escape' || event.defaultPrevented) return
    // Menus and dialogs inside the inspector close themselves first.
    if (
      (event.target as HTMLElement).closest('[role="menu"],[role="dialog"]')
    ) {
      return
    }
    event.preventDefault()
    props.onClose()
  }

  const inspector = props.open ? props.inspector : null

  if (presentation === 'page') {
    return (
      <div ref={rootRef} className={cn('min-w-0', props.className)}>
        <div hidden={props.open} className='min-w-0'>
          {props.list}
        </div>
        {inspector && (
          <section
            aria-label={props.label}
            data-inspector='page'
            className='min-w-0'
          >
            {inspector}
          </section>
        )}
      </div>
    )
  }

  if (presentation === 'overlay') {
    return (
      <div
        ref={rootRef}
        className={cn(
          'min-w-0',
          props.fill && 'h-full min-h-0',
          props.className
        )}
      >
        {props.list}
        <Sheet
          open={props.open}
          onOpenChange={(open) => {
            if (!open) props.onClose()
          }}
        >
          <SheetContent
            side='right'
            data-inspector='overlay'
            className='shadow-popover w-[400px] max-w-[calc(100vw-4rem)] gap-0 p-0 sm:max-w-[400px]'
            showCloseButton={false}
          >
            {/* Names the dialog without repeating the inspector's heading. */}
            <SheetTitle className='sr-only' render={<span />}>
              {props.label}
            </SheetTitle>
            {inspector && (
              <div className='flex min-h-0 flex-1 flex-col overflow-y-auto'>
                {inspector}
              </div>
            )}
          </SheetContent>
        </Sheet>
      </div>
    )
  }

  return (
    <div
      ref={rootRef}
      className={cn(
        'grid min-w-0 gap-4',
        props.fill ? 'h-full min-h-0 items-stretch' : 'items-start',
        props.open && 'grid-cols-[minmax(0,1fr)_440px]',
        props.className
      )}
    >
      <div className={cn('min-w-0', props.fill && 'h-full min-h-0')}>
        {props.list}
      </div>
      {inspector && (
        <aside
          aria-label={props.label}
          data-inspector='side'
          onKeyDown={closeOnEscape}
          className={cn(
            'bg-card flex min-w-0 flex-col overflow-y-auto rounded-xl border',
            props.fill
              ? 'h-full min-h-0'
              : 'sticky top-0 max-h-[calc(100svh-var(--pc-header-height)-2rem)]'
          )}
        >
          {inspector}
        </aside>
      )}
    </div>
  )
}

/**
 * Inspector header: object title, an optional subtitle, actions and a close
 * control. The heading takes programmatic focus when the object changes, so
 * screen readers announce it; it is not a tab stop.
 */
export function InspectorHeader(props: {
  title: ReactNode
  subtitle?: ReactNode
  actions?: ReactNode
  /** A "more" menu kept beside the close control. */
  menu?: ReactNode
  onClose: () => void
  /** Changes when a different object opens; refocuses the heading. */
  focusKey: string
}) {
  const { t } = useTranslation()
  const presentation = useInspectorPresentation()
  const headingRef = useRef<HTMLHeadingElement>(null)

  useEffect(() => {
    headingRef.current?.focus({ preventScroll: true })
  }, [props.focusKey])

  return (
    <header className='p-card flex min-w-0 flex-col gap-3 border-b'>
      <div className='flex min-w-0 items-start gap-2'>
        <div className='flex min-w-0 flex-1 flex-col gap-1'>
          <h2
            ref={headingRef}
            tabIndex={-1}
            className='text-title min-w-0 wrap-anywhere outline-none'
          >
            {props.title}
          </h2>
          {props.subtitle && (
            <div className='text-caption text-muted-foreground min-w-0 wrap-anywhere'>
              {props.subtitle}
            </div>
          )}
        </div>
        {props.menu}
        {presentation !== 'page' && (
          <Button
            variant='ghost'
            size='icon'
            aria-label={t('Close details')}
            onClick={props.onClose}
          >
            <HugeiconsIcon icon={Cancel01Icon} aria-hidden='true' />
          </Button>
        )}
      </div>
      {props.actions && (
        <div className='flex min-w-0 flex-wrap items-center gap-2'>
          {props.actions}
        </div>
      )}
    </header>
  )
}
