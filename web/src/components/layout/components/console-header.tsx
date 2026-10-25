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
import { ArrowDown01Icon, ArrowLeft01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link, useRouter } from '@tanstack/react-router'
import { Fragment } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfigDrawer } from '@/components/config-drawer'
import { LanguageSwitcher } from '@/components/language-switcher'
import { NotificationPopover } from '@/components/notification-popover'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { ConsoleNavigation } from '@/hooks/use-console-navigation'
import { useConsolePage } from '@/hooks/use-console-page'
import { useNotifications } from '@/hooks/use-notifications'
import { useInspectorStore } from '@/stores/inspector-store'

import { checkIsActive } from '../lib/url-utils'
import type { NavGroup } from '../types'
import { ChatPresetMenuItems } from './chat-presets-item'

function Notifications() {
  const notifications = useNotifications()
  return (
    <NotificationPopover
      open={notifications.popoverOpen}
      onOpenChange={notifications.setPopoverOpen}
      unreadCount={notifications.unreadCount}
      activeTab={notifications.activeTab}
      onTabChange={notifications.setActiveTab}
      notice={notifications.notice}
      announcements={notifications.announcements}
      loading={notifications.loading}
    />
  )
}

type NavMenuProps = {
  label: string
  groups: NavGroup[]
  active: boolean
  href: string
}

/**
 * A top-bar menu (Admin or More): one labelled group per area, its pages as
 * link items. Base UI's menu gives arrow-key, Home/End, typeahead and Escape
 * handling and returns focus to the trigger.
 */
function NavMenu(props: NavMenuProps) {
  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        className='pc-topnav-link pc-topnav-menu-trigger'
        data-active={props.active || undefined}
      >
        <span>{props.label}</span>
        <HugeiconsIcon icon={ArrowDown01Icon} aria-hidden='true' />
      </DropdownMenuTrigger>
      <DropdownMenuContent className='pc-topnav-menu w-60' sideOffset={6}>
        {props.groups.map((group, index) => (
          <Fragment key={group.id ?? group.title}>
            {index > 0 && <DropdownMenuSeparator />}
            <DropdownMenuGroup>
              {group.items.length > 1 && (
                <DropdownMenuLabel>{group.title}</DropdownMenuLabel>
              )}
              {group.items.map((item) => {
                if (item.type === 'chat-presets') {
                  return <ChatPresetMenuItems key={item.title} item={item} />
                }
                if (typeof item.url !== 'string') return null
                const current = checkIsActive(props.href, item)
                return (
                  <DropdownMenuItem
                    key={item.url}
                    render={
                      <Link
                        to={item.url}
                        preload={false}
                        aria-current={current ? 'page' : undefined}
                      />
                    }
                  >
                    {item.icon && (
                      <HugeiconsIcon icon={item.icon} aria-hidden='true' />
                    )}
                    {item.title}
                  </DropdownMenuItem>
                )
              })}
            </DropdownMenuGroup>
          </Fragment>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

/** The primary areas, Admin and More, as the top bar's navigation. */
function TopNavigation(props: { navigation: ConsoleNavigation }) {
  const { t } = useTranslation()
  const navigation = props.navigation
  return (
    <nav
      className='pc-topnav'
      aria-label={t('Personal console navigation')}
      data-myapi-navigation
    >
      {navigation.sections.map((section) => (
        <Link
          key={section.id}
          to={section.url}
          preload={false}
          className='pc-topnav-link'
          aria-current={section.active ? 'page' : undefined}
        >
          {section.title}
        </Link>
      ))}
      {navigation.admin.length > 0 && (
        <NavMenu
          label={t('Admin', { context: 'nav' })}
          groups={navigation.admin}
          active={navigation.adminActive}
          href={navigation.href}
        />
      )}
      {navigation.more.length > 0 && (
        <NavMenu
          label={t('More')}
          groups={navigation.more}
          active={navigation.moreActive}
          href={navigation.href}
        />
      )}
    </nav>
  )
}

/**
 * PC top bar (56px): brand, the five primary areas with Admin and More,
 * global search, notifications, language, appearance and the account menu.
 */
export function DesktopHeader(props: { navigation: ConsoleNavigation }) {
  const { t } = useTranslation()
  const navigation = props.navigation
  return (
    <header className='pc-topbar' data-myapi-header>
      <div className='pc-topbar-inner'>
        <Link
          className='pc-brand'
          to='/dashboard/$section'
          params={{ section: 'overview' }}
          aria-label={t('Overview')}
        >
          <img src='/myapi-logo-v1.png' alt='' />
          <strong>MyAPI</strong>
        </Link>
        <TopNavigation navigation={navigation} />
        <div className='pc-header-actions'>
          <Search variant='bar' />
          <Notifications />
          <LanguageSwitcher />
          <ConfigDrawer />
          <ProfileDropdown />
        </div>
      </div>
    </header>
  )
}

/**
 * Mobile top bar: brand mark on navigation destinations, a back control
 * elsewhere, the page title, search, notifications and account.
 */
export function MobileHeader() {
  const { t } = useTranslation()
  const router = useRouter()
  const page = useConsolePage()
  // An open inspector fills the screen like a second-level page.
  const inspector = useInspectorStore((state) => state.current)

  const goBack = () => {
    if (inspector) {
      inspector.close()
      return
    }
    if (window.history.length > 1) {
      router.history.back()
      return
    }
    void router.navigate({ to: page.parent?.url ?? '/dashboard/overview' })
  }
  const title = inspector?.title ?? page.title

  return (
    <header className='pc-header-mobile' data-myapi-header>
      {page.topLevel && !inspector ? (
        <Link
          to='/dashboard/$section'
          params={{ section: 'overview' }}
          className='pc-mobile-brand'
          aria-label={t('Overview')}
        >
          <img src='/myapi-logo-v1.png' alt='' />
        </Link>
      ) : (
        <Button
          variant='ghost'
          size='icon'
          aria-label={t('Back')}
          onClick={goBack}
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} aria-hidden='true' />
        </Button>
      )}
      {/* The page keeps its own h1; this repeats it as a compact label. */}
      <span className='pc-mobile-title' title={title}>
        {title}
      </span>
      <div className='pc-header-actions'>
        <Search variant='icon' />
        <Notifications />
        <ProfileDropdown />
      </div>
    </header>
  )
}
