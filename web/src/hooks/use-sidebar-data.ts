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
  Activity,
  Box,
  CreditCard,
  FileText,
  FileSearch,
  FlaskConical,
  Key,
  LayoutDashboard,
  ListTodo,
  MessageSquare,
  Radio,
  ServerCog,
  Settings,
  Ticket,
  User,
  Users,
  Wallet,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { SidebarData } from '@/components/layout/types'
import { ROLE } from '@/lib/roles'
import { SELF_USE_MINIMAL } from '@/lib/self-use-build'

import { useFundingPresentation } from './use-funding-presentation'

/**
 * Root navigation groups for the application sidebar.
 *
 * These are shown when the URL does not match any nested sidebar view
 * registered in `layout/lib/sidebar-view-registry.ts`.
 */
export function useSidebarData(): SidebarData {
  const { t } = useTranslation()
  const { commercialEnabled } = useFundingPresentation()

  const hiddenSelfUseItems = new Set(['/redemption-codes'])

  const navGroups: SidebarData['navGroups'] = [
    {
      id: 'general',
      title: t('Console'),
      items: [
        {
          title: t('Overview'),
          url: '/dashboard/overview',
          icon: Activity,
        },
        {
          title: t('Channels'),
          url: '/channels',
          icon: Radio,
          requiredRole: ROLE.ADMIN,
        },
        {
          title: t('Models'),
          url: '/models/metadata',
          activeUrls: ['/models/deployments'],
          icon: Box,
          requiredRole: ROLE.ADMIN,
        },
        {
          title: t('API Keys'),
          url: '/keys',
          icon: Key,
        },
        {
          title: t('Usage Logs'),
          url: '/usage-logs/common',
          icon: FileText,
        },
        {
          title: t('System Settings'),
          url: '/system-settings/site',
          activeUrls: ['/system-settings'],
          icon: Settings,
          requiredRole: ROLE.SUPER_ADMIN,
        },
      ],
    },
    {
      id: 'admin',
      title: t('Admin'),
      items: [
        {
          title: t('Users'),
          url: '/users',
          icon: Users,
        },
        {
          title: t('Redemption Codes'),
          url: '/redemption-codes',
          icon: Ticket,
        },
        {
          title: t('Subscriptions'),
          url: '/subscriptions',
          icon: CreditCard,
        },
        {
          title: t('API Request Logs'),
          url: '/full-content-logs',
          icon: FileSearch,
          requiredRole: ROLE.ADMIN,
        },
        {
          title: t('System Info'),
          url: '/system-info',
          icon: ServerCog,
          requiredRole: ROLE.SUPER_ADMIN,
        },
      ],
    },
    {
      id: 'chat',
      title: t('Tools'),
      items: [
        {
          title: t('Model Call Analytics'),
          url: '/dashboard/models',
          icon: LayoutDashboard,
          activeUrls: ['/dashboard/flow', '/dashboard/users'],
        },
        {
          title: t('Task Logs'),
          url: '/usage-logs/task',
          activeUrls: ['/usage-logs/drawing'],
          configUrls: ['/usage-logs/drawing', '/usage-logs/task'],
          icon: ListTodo,
        },
        {
          title: t('Playground'),
          url: '/playground',
          icon: FlaskConical,
        },
        {
          title: t('Chat'),
          icon: MessageSquare,
          type: 'chat-presets',
        },
      ],
    },
    {
      id: 'personal',
      title: t('Personal'),
      items: [
        {
          title: commercialEnabled ? t('Wallet') : t('Funding history'),
          url: '/wallet',
          icon: Wallet,
        },
        {
          title: t('Profile'),
          url: '/profile',
          icon: User,
        },
      ],
    },
  ]

  if (!commercialEnabled) {
    // Keep guarded historical routes available by URL, outside daily navigation.
    for (const group of navGroups) {
      group.items = group.items.filter(
        (item) =>
          !['/wallet', '/redemption-codes', '/subscriptions'].includes(
            item.url ?? ''
          )
      )
    }
  }

  if (!SELF_USE_MINIMAL) {
    return { navGroups }
  }

  return {
    navGroups: navGroups
      .map((group) => ({
        ...group,
        items: group.items.filter((item) => {
          if ('type' in item && item.type === 'chat-presets') return false
          if (!('url' in item) || !item.url) return true
          return !hiddenSelfUseItems.has(item.url)
        }),
      }))
      .filter((group) => group.items.length > 0),
  }
}
