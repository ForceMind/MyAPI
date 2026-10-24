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
import { Link, useRouter } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ConfigDrawer } from '@/components/config-drawer'
import { LanguageSwitcher } from '@/components/language-switcher'
import { NotificationPopover } from '@/components/notification-popover'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { Button } from '@/components/ui/button'
import { useConsolePage } from '@/hooks/use-console-page'
import { useNotifications } from '@/hooks/use-notifications'
import { useInspectorStore } from '@/stores/inspector-store'

import { AppBreadcrumb } from './app-breadcrumb'

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

/** PC top bar: breadcrumb, global search, notifications, language, theme, account. */
export function DesktopHeader(props: { onExpandSidebar?: () => void }) {
  const { t } = useTranslation()
  return (
    <header className='pc-header' data-myapi-header>
      {props.onExpandSidebar && (
        <Button
          variant='ghost'
          size='icon'
          aria-label={t('Expand sidebar')}
          onClick={props.onExpandSidebar}
        >
          <HugeiconsIcon icon={SidebarLeft01Icon} aria-hidden='true' />
        </Button>
      )}
      <div className='pc-header-context min-w-0 flex-1' data-myapi-context>
        <AppBreadcrumb />
      </div>
      <Search variant='bar' />
      <div className='pc-header-actions'>
        <Notifications />
        <LanguageSwitcher />
        <ConfigDrawer />
        <ProfileDropdown />
      </div>
    </header>
  )
}

/**
 * Mobile top bar: brand mark on main destinations, a back control elsewhere,
 * the page title, search and account. No build number (design 5.2, M2).
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
    <header className='pc-header pc-header-mobile' data-myapi-header>
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
