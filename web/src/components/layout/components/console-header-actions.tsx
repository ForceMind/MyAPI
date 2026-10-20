/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { ConfigDrawer } from '@/components/config-drawer'
import { LanguageSwitcher } from '@/components/language-switcher'
import { NotificationPopover } from '@/components/notification-popover'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { useNotifications } from '@/hooks/use-notifications'

export function ConsoleHeaderActions(props: { mobile: boolean }) {
  const notifications = useNotifications()
  return (
    <div className='pc-header-actions flex shrink-0 items-center gap-0.5'>
      <Search className='size-11 flex-none justify-center p-0 sm:w-11 sm:pe-0 [&>kbd]:hidden [&>span]:hidden [&>svg]:start-1/2 [&>svg]:-translate-x-1/2' />
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
      {!props.mobile && <LanguageSwitcher />}
      {props.mobile ? <ThemeSwitch /> : <ConfigDrawer />}
      <ProfileDropdown />
    </div>
  )
}
