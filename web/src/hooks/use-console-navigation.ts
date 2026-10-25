/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { IconSvgElement } from '@hugeicons/react'
import { useLocation } from '@tanstack/react-router'

import { checkIsActive } from '@/components/layout/lib/url-utils'
import type {
  NavGroup,
  NavItem,
  NavLink,
  SidebarView,
} from '@/components/layout/types'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { useProductEdition } from './use-product-edition'
import { useSidebarView } from './use-sidebar-view'

/** One top-bar destination: a primary area and the page it opens. */
type ConsoleSection = {
  id: string
  title: string
  /** Label for the mobile bottom bar. */
  shortTitle: string
  url: string
  icon?: IconSvgElement
  active: boolean
  group: NavGroup
}

/** A page of the current area, shown as a tab under the top bar. */
export type ConsolePageTab = {
  title: string
  url: string
  active: boolean
  /** Second-level pages (settings sections of a settings category). */
  children?: ConsolePageTab[]
}

export type ConsoleNavigation = {
  href: string
  sections: ConsoleSection[]
  /** Full edition administrators: Users, Commerce and System. */
  admin: NavGroup[]
  /** Everything else, including the admin areas outside the Admin menu. */
  more: NavGroup[]
  adminActive: boolean
  moreActive: boolean
  /** The root area holding the current page, if any. */
  currentGroup?: NavGroup
  /** Pages of the current area; empty when it has a single page. */
  tabs: ConsolePageTab[]
  /** Title of the area the tabs belong to (their accessible name). */
  tabsLabel?: string
  view: SidebarView | null
  viewGroups: NavGroup[]
}

function isLink(item: NavItem): item is NavLink {
  return typeof item.url === 'string'
}

function groupIsActive(href: string, group: NavGroup) {
  return group.items.some((item) => checkIsActive(href, item))
}

/**
 * The console's navigation model for design direction 3: five primary
 * areas in the top bar (each opening its first visible page), an Admin menu
 * for Full edition administrators, More for the remaining areas, and the
 * page tabs of the current area. It only rearranges what `useSidebarView`
 * already narrowed by role, permission, capability and `sidebar_modules`, so
 * it never widens access.
 */
export function useConsoleNavigation(): ConsoleNavigation {
  const href = useLocation({ select: (location) => location.href })
  const role = useAuthStore((state) => state.auth.user?.role ?? ROLE.GUEST)
  const { edition } = useProductEdition()
  const root = useSidebarView({ rootOnly: true })
  const context = useSidebarView()
  const view = context.view
  const viewGroups = view ? context.navGroups : []
  const groups = root.navGroups

  const sections: ConsoleSection[] = []
  for (const group of groups) {
    if (group.placement !== 'primary') continue
    const landing = group.items.find(isLink)
    if (!landing) continue
    const active =
      group.id === 'settings' ? view !== null : groupIsActive(href, group)
    sections.push({
      id: group.id ?? group.title,
      title: group.title,
      shortTitle: group.shortTitle ?? group.title,
      url: String(landing.url),
      icon: landing.icon,
      active,
      group,
    })
  }

  const adminMenu = edition === 'full' && role >= ROLE.ADMIN
  const admin = adminMenu
    ? groups.filter((group) => group.placement === 'admin')
    : []
  const more = groups.filter(
    (group) =>
      group.placement === 'more' || (!adminMenu && group.placement === 'admin')
  )

  const currentGroup = groups.find((group) => groupIsActive(href, group))
  const settingsSection = sections.find((section) => section.id === 'settings')

  let tabs: ConsolePageTab[] = []
  let tabsLabel: string | undefined
  if (view) {
    tabsLabel = settingsSection?.title
    tabs = viewGroups
      .flatMap((group) => group.items)
      .flatMap((category) => {
        const children = (category.items ?? []).map((child) => ({
          title: child.title,
          url: String(child.url),
          active: checkIsActive(href, child),
        }))
        if (children.length === 0) return []
        return [
          {
            title: category.title,
            url: children[0].url,
            active: children.some((child) => child.active),
            children,
          },
        ]
      })
  } else if (currentGroup?.placement === 'primary') {
    // Only the top-bar areas get page tabs; Admin and More list their pages.
    const links = currentGroup.items.filter(isLink)
    if (links.length > 1) {
      tabsLabel = currentGroup.title
      tabs = links.map((link) => ({
        title: link.title,
        url: String(link.url),
        active: checkIsActive(href, link),
      }))
    }
  }

  return {
    href,
    sections,
    admin,
    more,
    adminActive: admin.some((group) => groupIsActive(href, group)),
    moreActive: more.some((group) => groupIsActive(href, group)),
    currentGroup,
    tabs,
    tabsLabel,
    view,
    viewGroups,
  }
}
