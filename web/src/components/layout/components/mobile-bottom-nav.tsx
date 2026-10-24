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
import {
  ArrowLeft01Icon,
  MoreHorizontalIcon,
  UserIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon, type IconSvgElement } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'

import { checkIsActive } from '../lib/url-utils'
import type { NavGroup, NavLink, ResolvedSidebarView } from '../types'
import { ConsoleNavList } from './console-nav-list'

type BottomTab = {
  label: string
  url: string
  icon?: IconSvgElement
  active: boolean
}

/**
 * Bottom tabs: Overview, the Upstream group, API Keys and call records, as far
 * as the user can see them, followed by More (design 5.2).
 */
function getBottomTabs(groups: NavGroup[], href: string): BottomTab[] {
  const links = groups.flatMap((group) => group.items)
  const tabs: BottomTab[] = []
  const addLink = (url: string) => {
    const link = links.find((item): item is NavLink => item.url === url)
    if (!link) return
    tabs.push({
      label: link.shortTitle ?? link.title,
      url,
      icon: link.icon,
      active: checkIsActive(href, link),
    })
  }
  addLink('/dashboard/overview')
  const upstream = groups.find((group) => group.id === 'upstream')
  const upstreamLanding = upstream?.items.find(
    (item): item is NavLink => typeof item.url === 'string'
  )
  if (upstream && upstreamLanding) {
    tabs.push({
      label: upstream.shortTitle ?? upstream.title,
      url: String(upstreamLanding.url),
      icon: upstreamLanding.icon,
      active: upstream.items.some((item) => checkIsActive(href, item)),
    })
  }
  addLink('/keys')
  addLink('/usage-logs/common')
  return tabs
}

type MobileBottomNavProps = {
  href: string
  root: ResolvedSidebarView
  context: ResolvedSidebarView
}

/** Mobile composition (<1024px): four tabs and a searchable More drawer. */
export function MobileBottomNav(props: MobileBottomNavProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const modulesRef = useRef<HTMLElement>(null)
  const tabs = getBottomTabs(props.root.navGroups, props.href)
  const tabUrls = new Set(tabs.map((tab) => tab.url))
  const normalizedQuery = query.trim().toLocaleLowerCase()

  useEffect(() => setOpen(false), [props.href])

  const profile: NavLink = {
    title: t('Profile'),
    url: '/profile',
    icon: UserIcon,
  }
  const remaining = [
    ...props.root.navGroups,
    { id: 'account', title: t('Account'), items: [profile] },
  ]
    .map((group) => ({
      ...group,
      titleHidden: false,
      items: group.items.filter(
        (item) =>
          !tabUrls.has(String(item.url)) &&
          item.title.toLocaleLowerCase().includes(normalizedQuery)
      ),
    }))
    .filter((group) => group.items.length > 0)
  const view = props.context.view

  return (
    <nav className='pc-mobile-nav' aria-label={t('Mobile console navigation')}>
      {tabs.map((tab) => (
        <Link
          key={tab.url}
          to={tab.url}
          preload={false}
          aria-current={tab.active ? 'page' : undefined}
          title={tab.label}
        >
          {tab.icon && <HugeiconsIcon icon={tab.icon} aria-hidden='true' />}
          <span>{tab.label}</span>
        </Link>
      ))}
      <Sheet
        open={open}
        onOpenChange={(next) => {
          setOpen(next)
          if (!next) setQuery('')
        }}
      >
        <SheetTrigger
          className='pc-more'
          aria-current={tabs.some((tab) => tab.active) ? undefined : 'page'}
        >
          <HugeiconsIcon icon={MoreHorizontalIcon} aria-hidden='true' />
          <span>{t('More')}</span>
        </SheetTrigger>
        <SheetContent
          side='bottom'
          className='pc-more-sheet max-h-[85dvh] overflow-y-auto rounded-t-2xl'
          // Touch opens on the list so the on-screen keyboard stays closed;
          // keyboard and pointer users start in the search field.
          initialFocus={(openType) =>
            openType === 'touch' ? modulesRef.current : true
          }
        >
          <SheetHeader>
            <SheetTitle>{t('All modules')}</SheetTitle>
            <SheetDescription className='sr-only'>
              {t('Console')}
            </SheetDescription>
          </SheetHeader>
          <div className='px-4'>
            <Input
              type='search'
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              aria-label={t('Search')}
              placeholder={t('Search')}
            />
          </div>
          <nav
            ref={modulesRef}
            className='pc-more-modules px-4 pb-6 outline-none'
            aria-label={t('All modules')}
            tabIndex={-1}
          >
            {view && (
              <>
                <Link
                  to={view.parent.to}
                  className='pc-nav-link'
                  onClick={() => setOpen(false)}
                >
                  <HugeiconsIcon icon={ArrowLeft01Icon} aria-hidden='true' />
                  <span>{t(view.parent.label)}</span>
                </Link>
                <ConsoleNavList
                  groups={props.context.navGroups}
                  href={props.href}
                  onNavigate={() => setOpen(false)}
                />
              </>
            )}
            <ConsoleNavList
              groups={remaining}
              href={props.href}
              onNavigate={() => setOpen(false)}
            />
          </nav>
        </SheetContent>
      </Sheet>
    </nav>
  )
}
