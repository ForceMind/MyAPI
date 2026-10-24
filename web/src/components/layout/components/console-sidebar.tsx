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
import { ArrowLeft01Icon, SidebarLeft01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useSidebar } from '@/components/ui/sidebar'
import { useLayout } from '@/context/layout-provider'

import type { ResolvedSidebarView } from '../types'
import { ConsoleNavList } from './console-nav-list'
import { NavGroup as SettingsNavGroup } from './nav-group'

type ConsoleSidebarProps = {
  href: string
  collapsed: boolean
  root: ResolvedSidebarView
  context: ResolvedSidebarView
}

/** PC composition (>=1024px): brand, grouped destinations, collapse control. */
export function ConsoleSidebar(props: ConsoleSidebarProps) {
  const { t } = useTranslation()
  const { open, setOpen } = useSidebar()
  const { collapsible } = useLayout()
  const view = props.context.view

  return (
    <aside
      className='pc-sidebar group'
      data-collapsible={props.collapsed ? collapsible : ''}
    >
      <Link
        className='pc-brand'
        to='/dashboard/$section'
        params={{ section: 'overview' }}
        aria-label={t('Overview')}
      >
        <span className='pc-logo-tile'>
          <img src='/myapi-logo-v1.png' alt='' />
        </span>
        <span>
          <strong>MyAPI</strong>
          <small>{t('Console')}</small>
        </span>
      </Link>
      <nav
        className='pc-desktop-nav'
        aria-label={t('Personal console navigation')}
        data-myapi-sidebar
        tabIndex={0}
      >
        {view ? (
          <>
            <Link to={view.parent.to} className='pc-nav-link'>
              <HugeiconsIcon icon={ArrowLeft01Icon} aria-hidden='true' />
              <span>{t(view.parent.label)}</span>
            </Link>
            {props.context.navGroups.map((group) => (
              <SettingsNavGroup key={group.id ?? group.title} {...group} />
            ))}
          </>
        ) : (
          <ConsoleNavList
            groups={props.root.navGroups}
            href={props.href}
            foldable={!props.collapsed}
          />
        )}
      </nav>
      <div className='pc-sidebar-bottom'>
        <Button
          variant='ghost'
          size='sm'
          className='pc-collapse'
          onClick={() => setOpen(!open)}
          disabled={collapsible === 'none'}
          aria-label={
            props.collapsed ? t('Expand sidebar') : t('Collapse sidebar')
          }
          aria-expanded={!props.collapsed}
        >
          <HugeiconsIcon
            icon={SidebarLeft01Icon}
            data-icon='inline-start'
            aria-hidden='true'
          />
          <span>{t('Collapse sidebar')}</span>
        </Button>
      </div>
    </aside>
  )
}
