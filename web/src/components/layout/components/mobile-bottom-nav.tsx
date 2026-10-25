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
import { HugeiconsIcon } from '@hugeicons/react'
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
import type { ConsoleNavigation } from '@/hooks/use-console-navigation'

import type { NavGroup, NavLink } from '../types'
import { ConsoleNavList } from './console-nav-list'

/** Areas with a bottom tab; Settings and every other area live in More. */
const BOTTOM_SECTIONS = new Set(['overview', 'upstream', 'keys', 'requests'])

/**
 * Mobile composition (<1024px): Overview, Upstream, Key and Requests as far
 * as the user can see them, followed by More, a searchable sheet with every
 * other destination.
 */
export function MobileBottomNav(props: { navigation: ConsoleNavigation }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const modulesRef = useRef<HTMLElement>(null)
  const navigation = props.navigation
  const tabs = navigation.sections.filter((section) =>
    BOTTOM_SECTIONS.has(section.id)
  )
  const tabUrls = new Set(tabs.map((tab) => tab.url))
  const normalizedQuery = query.trim().toLocaleLowerCase()

  useEffect(() => setOpen(false), [navigation.href])

  const profile: NavLink = {
    title: t('Profile'),
    url: '/profile',
    icon: UserIcon,
  }
  const groups: NavGroup[] = [
    ...navigation.sections.map((section) => section.group),
    ...navigation.admin,
    ...navigation.more,
    { id: 'account', title: t('Account'), items: [profile] },
  ]
  const remaining = groups
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
  const view = navigation.view

  return (
    <nav className='pc-mobile-nav' aria-label={t('Mobile console navigation')}>
      {tabs.map((tab) => (
        <Link
          key={tab.id}
          to={tab.url}
          preload={false}
          aria-current={tab.active ? 'page' : undefined}
          title={tab.shortTitle}
        >
          {tab.icon && <HugeiconsIcon icon={tab.icon} aria-hidden='true' />}
          <span>{tab.shortTitle}</span>
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
                  groups={navigation.viewGroups}
                  href={navigation.href}
                  onNavigate={() => setOpen(false)}
                />
              </>
            )}
            <ConsoleNavList
              groups={remaining}
              href={navigation.href}
              onNavigate={() => setOpen(false)}
            />
          </nav>
        </SheetContent>
      </Sheet>
    </nav>
  )
}
