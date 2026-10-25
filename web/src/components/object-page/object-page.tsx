/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useEffect, useId, useRef, type ReactNode } from 'react'

import { Main } from '@/components/layout/components/main'
import { useInspectorStore, type OpenInspector } from '@/stores/inspector-store'

/**
 * A full page for one object (design direction 3): a heading with its state
 * and actions, then the object's content, scrolling as one column. The page
 * registers itself with the shell, so the breadcrumb leads back up on PC and
 * the mobile top bar shows its name and a Back control.
 */
export function ObjectPage(props: {
  /** Heading content; `label` is its plain text for the shell. */
  title: ReactNode
  label: string
  meta?: ReactNode
  actions?: ReactNode
  /** Back one level, the levels above, and the way to the list. */
  navigation: Omit<OpenInspector, 'title' | 'page'>
  children: ReactNode
  'data-testid'?: string
}) {
  const titleId = useId()
  const headingRef = useRef<HTMLHeadingElement>(null)
  const setCurrent = useInspectorStore((state) => state.setCurrent)
  const navigationRef = useRef(props.navigation)
  navigationRef.current = props.navigation
  const parentTitles = JSON.stringify(
    props.navigation.parents?.map((parent) => parent.title) ?? []
  )

  useEffect(() => {
    const parents = navigationRef.current.parents
    setCurrent({
      title: props.label,
      page: true,
      close: () => navigationRef.current.close(),
      closeAll: navigationRef.current.closeAll
        ? () => navigationRef.current.closeAll?.()
        : undefined,
      parents: parents?.map((parent, index) => ({
        title: parent.title,
        open: () => navigationRef.current.parents?.[index]?.open(),
      })),
    })
    return () => setCurrent(null)
  }, [props.label, parentTitles, setCurrent])

  // Announce the new object, unless focus already went somewhere on purpose
  // (an element restored on Back).
  useEffect(() => {
    const active = document.activeElement
    if (!active || active === document.body) {
      headingRef.current?.focus({ preventScroll: true })
    }
  }, [props.label])

  return (
    <Main
      className='myapi-page'
      aria-labelledby={titleId}
      data-object-page=''
      data-testid={props['data-testid']}
    >
      <div className='myapi-page-header shrink-0 px-4 py-4 sm:px-6 sm:py-5'>
        <div className='flex flex-wrap items-start justify-between gap-x-5 gap-y-3'>
          <div className='flex min-w-0 flex-1 basis-60 flex-col gap-1.5'>
            <h1
              ref={headingRef}
              id={titleId}
              tabIndex={-1}
              className='text-xl leading-tight font-semibold tracking-tight wrap-anywhere outline-none sm:text-2xl'
            >
              {props.title}
            </h1>
            {props.meta != null && (
              <div className='text-muted-foreground flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-sm'>
                {props.meta}
              </div>
            )}
          </div>
          {props.actions != null && (
            <div className='myapi-page-actions flex max-w-full flex-wrap items-center gap-2 sm:ms-auto sm:justify-end'>
              {props.actions}
            </div>
          )}
        </div>
      </div>
      <div
        data-page-scroll='page'
        className='myapi-page-content min-h-0 min-w-0 flex-1 overflow-auto px-4 py-4 sm:px-6 sm:py-5'
      >
        {props.children}
      </div>
    </Main>
  )
}
