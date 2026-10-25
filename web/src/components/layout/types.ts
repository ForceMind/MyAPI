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
import type { IconSvgElement } from '@hugeicons/react'
import type { LinkProps } from '@tanstack/react-router'
import type { TFunction } from 'i18next'

import type { ProductCapability } from '@/lib/product-edition'

/**
 * Base navigation item type
 */
type BaseNavItem = {
  title: string
  /** Label for space-constrained places such as the mobile bottom bar. */
  shortTitle?: string
  badge?: string
  icon?: IconSvgElement
  activeUrls?: (LinkProps['to'] | (string & {}))[]
  /** Path prefixes of object pages below this entry, e.g. `/keys/`. */
  activePrefixes?: string[]
  configUrls?: (LinkProps['to'] | (string & {}))[]
  /**
   * Minimum role required to see this item in the sidebar. When set, the item
   * is hidden for users whose role is below this threshold (see
   * `useSidebarView`). Route-level guards still enforce access independently.
   */
  requiredRole?: number
  /** Edition capability that must be enabled for the entry to be shown. */
  capability?: ProductCapability
}

/**
 * Navigation link type - single link item
 */
export type NavLink = BaseNavItem & {
  url: LinkProps['to'] | (string & {})
  items?: never
  type?: never
}

/**
 * Navigation collapsible type - collapsible navigation with sub-items
 */
export type NavCollapsible = BaseNavItem & {
  items: (BaseNavItem & { url: LinkProps['to'] | (string & {}) })[]
  url?: never
  type?: never
}

/**
 * Dynamic chat presets type - dynamically loaded chat preset list from API
 */
export type NavChatPresets = BaseNavItem & {
  type: 'chat-presets'
  url?: never
  items?: never
}

/**
 * Navigation item union type
 */
export type NavItem = NavCollapsible | NavLink | NavChatPresets

/**
 * Where a root navigation group is offered in the console shell:
 * `primary` groups are the top-bar destinations (their items become the
 * section's page tabs), `admin` groups form the Full edition's Admin menu for
 * administrators (and otherwise join More), `more` groups always join More.
 */
type NavPlacement = 'primary' | 'admin' | 'more'

/**
 * Navigation group type - one destination area of the console
 */
export type NavGroup = {
  id?: string
  title: string
  items: NavItem[]
  /** The group heading is announced to assistive technology only. */
  titleHidden?: boolean
  /** Label for the group in the mobile bottom bar, when it has a tab. */
  shortTitle?: string
  placement?: NavPlacement
}

/**
 * Root navigation data: the console's destination areas in display order.
 */
export type SidebarData = {
  navGroups: NavGroup[]
}

/**
 * Top navigation link type
 */
export type TopNavLink = {
  title: string
  href: string
  isActive?: boolean
  disabled?: boolean
  requiresAuth?: boolean
  external?: boolean
}

/**
 * Back-navigation descriptor for a nested sidebar view
 */
export type SidebarViewParent = {
  /** Destination URL for the back button */
  to: LinkProps['to'] | (string & {})
  /** Visible label, e.g. "Back to Dashboard" — already localized */
  label: string
}

/**
 * Nested view configuration
 *
 * A nested view lists the pages of a dedicated workspace (e.g. System
 * Settings): its groups become the workspace's page tabs and are offered in
 * the mobile More sheet while the user is inside it.
 */
export type SidebarView = {
  /** Stable identifier (also drives transition animation keys) */
  id: string
  /** Path matcher that activates this view */
  pathPattern: RegExp
  /** Back-navigation descriptor; required for nested views */
  parent: SidebarViewParent
  /** Nav group builder, called per render with the active translator */
  getNavGroups: (
    t: TFunction,
    capabilities?: Record<ProductCapability, boolean>
  ) => NavGroup[]
}

/**
 * Resolved sidebar view returned by `useSidebarView()`
 *
 * - `view === null`: root navigation (default sidebar)
 * - `view !== null`: nested workspace view (renders header + back button)
 */
export type ResolvedSidebarView = {
  /** Animation/identity key — falls back to a sentinel for the root view */
  key: string
  view: SidebarView | null
  navGroups: NavGroup[]
}
