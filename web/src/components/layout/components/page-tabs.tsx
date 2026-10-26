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
  const sectionsRef = useRef<HTMLDivElement>(null)
  const active = props.tabs.find((tab) => tab.active)
  const activeUrl = active?.url
  const activeSectionUrl = active?.children?.find((child) => child.active)?.url
  const sectionCount = active?.children?.length ?? 0

  useEffect(() => {
    // Only the tab and section rows scroll; never the page.
    for (const row of [listRef.current, sectionsRef.current]) {
      const current = row?.querySelector<HTMLElement>('[aria-current]')
      if (!row || !current) continue
      const offset =
        current.offsetLeft - (row.clientWidth - current.offsetWidth) / 2
      row.scrollLeft = Math.max(0, offset)
    }
  }, [activeUrl, activeSectionUrl, sectionCount])

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
          ref={sectionsRef}
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
