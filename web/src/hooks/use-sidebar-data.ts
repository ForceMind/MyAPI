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
  Analytics01Icon,
  CreditCardIcon,
  CubeIcon,
  DashboardSquare01Icon,
  FileSearchIcon,
  Image01Icon,
  Key01Icon,
  Message01Icon,
  Mortarboard01Icon,
  Note01Icon,
  PlugSocketIcon,
  ServerStack01Icon,
  Settings01Icon,
  Task01Icon,
  TestTube01Icon,
  TicketIcon,
  UserAccountIcon,
  UserGroupIcon,
  Wallet01Icon,
} from '@hugeicons/core-free-icons'
import { useTranslation } from 'react-i18next'

import type { NavItem, SidebarData } from '@/components/layout/types'
import { ROLE } from '@/lib/roles'
import { SELF_USE_MINIMAL } from '@/lib/self-use-build'

import { useFundingPresentation } from './use-funding-presentation'
import { useConfirmedFundingMode } from './use-personal-workbench-mode'
import { useProductEdition } from './use-product-edition'

/**
 * Root navigation groups for the console shell, in the fixed order of the
 * design direction (5.1): overview and analytics, then Upstream, Access,
 * Records, Tools, System and Commerce.
 *
 * These are shown when the URL does not match any nested sidebar view
 * registered in `layout/lib/sidebar-view-registry.ts`. Role and
 * `sidebar_modules` narrowing happen in `useSidebarView`.
 */
export function useSidebarData(): SidebarData {
  const { t } = useTranslation()
  const funding = useFundingPresentation()
  const serverFundingMode = useConfirmedFundingMode()
  const { capabilities } = useProductEdition()
  const commercialEnabled =
    funding.commercialEnabled && serverFundingMode === 'enabled'

  const navGroups: SidebarData['navGroups'] = [
    {
      id: 'general',
      title: t('Console'),
      titleHidden: true,
      items: [
        {
          title: t('Overview'),
          url: '/dashboard/overview',
          icon: DashboardSquare01Icon,
        },
        {
          title: t('Analytics'),
          url: '/dashboard/models',
          icon: Analytics01Icon,
          activeUrls: ['/dashboard/flow', '/dashboard/users'],
        },
      ],
    },
    {
      id: 'upstream',
      title: t('Upstream'),
      shortTitle: t('Upstream'),
      items: [
        {
          title: t('Connections'),
          url: '/connections',
          activeUrls: ['/channels'],
          icon: PlugSocketIcon,
          requiredRole: ROLE.ADMIN,
        },
        {
          title: t('Account windows'),
          url: '/upstream-accounts',
          icon: UserAccountIcon,
          requiredRole: ROLE.ADMIN,
        },
        {
          title: t('Model catalog'),
          url: '/models/metadata',
          activeUrls: ['/models/deployments'],
          icon: CubeIcon,
          requiredRole: ROLE.ADMIN,
        },
      ],
    },
    {
      id: 'access',
      title: t('Access'),
      items: [
        {
          title: t('API Keys', { context: 'nav' }),
          shortTitle: t('Keys'),
          url: '/keys',
          icon: Key01Icon,
        },
        {
          title: t('Users'),
          url: '/users',
          icon: UserGroupIcon,
          requiredRole: ROLE.ADMIN,
        },
      ],
    },
    {
      id: 'records',
      title: t('Records'),
      items: [
        {
          title: t('Requests', { context: 'nav' }),
          shortTitle: t('Records'),
          url: '/usage-logs/common',
          icon: Note01Icon,
        },
        {
          title: t('Bodies & SSE'),
          url: '/full-content-logs',
          icon: FileSearchIcon,
          requiredRole: ROLE.ADMIN,
        },
        { title: t('Tasks'), url: '/usage-logs/task', icon: Task01Icon },
        {
          title: t('Drawing'),
          url: '/usage-logs/drawing',
          icon: Image01Icon,
          capability: 'midjourney',
        },
      ],
    },
    {
      id: 'tools',
      title: t('Tools'),
      defaultCollapsed: true,
      items: [
        { title: t('Playground'), url: '/playground', icon: TestTube01Icon },
        {
          title: t('Prompt learning'),
          url: '/prompt-learning',
          icon: Mortarboard01Icon,
        },
        { title: t('Chat'), icon: Message01Icon, type: 'chat-presets' },
      ],
    },
    {
      id: 'system',
      title: t('System'),
      defaultCollapsed: true,
      items: [
        {
          title: t('System status'),
          url: '/system-info',
          icon: ServerStack01Icon,
          requiredRole: ROLE.SUPER_ADMIN,
        },
        {
          title: t('Settings'),
          url: '/system-settings/site',
          activeUrls: ['/system-settings'],
          icon: Settings01Icon,
          requiredRole: ROLE.SUPER_ADMIN,
        },
      ],
    },
    {
      id: 'commerce',
      title: t('Commerce'),
      defaultCollapsed: true,
      items: [
        {
          title: commercialEnabled ? t('Wallet') : t('Funding history'),
          url: '/wallet',
          icon: Wallet01Icon,
          capability: 'payments',
        },
        {
          title: t('Subscriptions'),
          url: '/subscriptions',
          icon: CreditCardIcon,
          requiredRole: ROLE.ADMIN,
          capability: 'commercial_subscriptions',
        },
        {
          title: t('Redemption Codes'),
          url: '/redemption-codes',
          icon: TicketIcon,
          requiredRole: ROLE.ADMIN,
          capability: 'redemption_codes',
        },
      ],
    },
  ]

  // Keep guarded historical routes available by URL, outside daily navigation.
  let hiddenUrls: string[] = []
  if (!commercialEnabled) {
    hiddenUrls =
      serverFundingMode === 'retirement'
        ? ['/redemption-codes', '/subscriptions']
        : ['/wallet', '/redemption-codes', '/subscriptions']
  }
  // The self-use build removes these routes and the chat presets entirely.
  if (SELF_USE_MINIMAL) hiddenUrls.push('/redemption-codes')

  const isVisible = (item: NavItem) => {
    if (item.capability && !capabilities[item.capability]) return false
    if (SELF_USE_MINIMAL && item.type === 'chat-presets') return false
    return !hiddenUrls.includes(item.url ?? '')
  }

  return {
    navGroups: navGroups
      .map((group) => ({ ...group, items: group.items.filter(isVisible) }))
      .filter((group) => group.items.length > 0),
  }
}
