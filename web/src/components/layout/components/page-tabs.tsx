/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Link } from '@tanstack/react-router'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import type { ConsolePageTab } from '@/hooks/use-console-navigation'

/** A category holding the current section is the current location, not the page. */
function currentValue(tab: ConsolePageTab) {
  if (!tab.active) return undefined
  return tab.children && tab.children.length > 1 ? 'true' : 'page'
}

type PageTabsProps = {
  /** Name of the area the pages belong to (the navigation's label). */
  label: string
  tabs: ConsolePageTab[]
}

/**
 * The pages of the current area as underlined tabs (links), and for a
 * settings category its sections as a second row. Every page stays one link
 * away by keyboard and touch; the active tab scrolls into view on narrow
 * screens where the row scrolls sideways.
 */
export function PageTabs(props: PageTabsProps) {
  const { t } = useTranslation()
  const listRef = useRef<HTMLDivElement>(null)
  const active = props.tabs.find((tab) => tab.active)
  const activeUrl = active?.url

  useEffect(() => {
    const list = listRef.current
    const current = list?.querySelector<HTMLElement>('[aria-current]')
    if (!list || !current) return
    // Only the tab row scrolls; never the page.
    const offset =
      current.offsetLeft - (list.clientWidth - current.offsetWidth) / 2
    list.scrollLeft = Math.max(0, offset)
  }, [activeUrl])

  if (props.tabs.length === 0) return null

  return (
    <nav className='pc-page-tabs' aria-label={props.label}>
      <div ref={listRef} className='pc-page-tab-list'>
        {props.tabs.map((tab) => (
          <Link
            key={tab.url}
            to={tab.url}
            preload={false}
            className='pc-page-tab'
            aria-current={currentValue(tab)}
          >
            {tab.title}
          </Link>
        ))}
      </div>
      {active?.children && active.children.length > 1 && (
        <div
          className='pc-page-subtabs'
          role='group'
          aria-label={t('{{section}} sections', { section: active.title })}
        >
          {active.children.map((child) => (
            <Link
              key={child.url}
              to={child.url}
              preload={false}
              className='pc-page-subtab'
              aria-current={child.active ? 'page' : undefined}
            >
              {child.title}
            </Link>
          ))}
        </div>
      )}
    </nav>
  )
}
