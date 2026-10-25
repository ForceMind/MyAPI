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
import { useLocation } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { checkIsActive, normalizeHref } from '@/components/layout/lib/url-utils'
import { getDashboardSectionNavItems } from '@/features/dashboard/section-registry'
import { getModelsSectionNavItems } from '@/features/models/section-registry'
import { getUsageLogsSectionNavItems } from '@/features/usage-logs/section-registry'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { useConsoleNavigation } from './use-console-navigation'

export type ConsolePage = {
  /** Area of the page, when it adds information to the page name. */
  group?: string
  /** Parent page that the breadcrumb links back to. */
  parent?: { title: string; url: string }
  title: string
  /** The page is one of the navigation destinations (or a tab of one). */
  topLevel: boolean
}

/**
 * Names the current page from the same navigation data the shell shows, so
 * breadcrumb, mobile title and top bar always use one name for one page.
 * Labels never introduce another permission path.
 */
export function useConsolePage(): ConsolePage {
  const { t } = useTranslation()
  const href = useLocation({ select: (location) => location.href })
  const pathname = useLocation({ select: (location) => location.pathname })
  const role = useAuthStore((state) => state.auth.user?.role ?? ROLE.GUEST)
  const navigation = useConsoleNavigation()

  if (navigation.view) {
    const categories = navigation.viewGroups.flatMap((group) => group.items)
    const category = categories.find((item) => checkIsActive(href, item))
    const child = category?.items?.find((item) => checkIsActive(href, item))
    const firstChild = category?.items?.[0]
    return {
      group: t('Settings'),
      parent:
        category && child && firstChild
          ? { title: category.title, url: String(firstChild.url) }
          : undefined,
      title: child?.title ?? category?.title ?? t('Settings'),
      topLevel: false,
    }
  }

  const group = navigation.currentGroup
  const item = group?.items.find((candidate) => checkIsActive(href, candidate))
  const section = [
    ...getDashboardSectionNavItems(t, { isAdmin: role >= ROLE.ADMIN }),
    ...getModelsSectionNavItems(t),
    ...getUsageLogsSectionNavItems(t),
  ].find((candidate) => candidate.url === pathname)
  const itemUrl = typeof item?.url === 'string' ? item.url : undefined
  const isItemPage =
    itemUrl !== undefined && normalizeHref(itemUrl) === normalizeHref(pathname)

  let title = isItemPage ? item?.title : undefined
  title ??= section?.title ?? item?.title
  if (!title && pathname.startsWith('/chat/')) title = t('Chat')
  if (!title && pathname === '/profile') title = t('Profile')
  title ??= t('Console')

  const parent =
    item && itemUrl && !isItemPage && item.title !== title
      ? { title: item.title, url: itemUrl }
      : undefined
  // An area name adds nothing when it repeats the page (Overview, API Keys)
  // or the area has a single page.
  const groupName =
    group &&
    !group.titleHidden &&
    group.items.length > 1 &&
    group.title !== (parent?.title ?? title)
      ? group.title
      : undefined

  return {
    group: groupName,
    parent,
    title,
    topLevel: Boolean(item),
  }
}
