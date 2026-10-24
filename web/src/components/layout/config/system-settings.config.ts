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
  CreditCardIcon,
  CubeIcon,
  Layout01Icon,
  SecurityIcon,
  Settings01Icon,
  Shield01Icon,
  Wrench01Icon,
} from '@hugeicons/core-free-icons'
import type { TFunction } from 'i18next'

import {
  resolveProductEdition,
  type ProductCapability,
} from '@/lib/product-edition'
import { SELF_USE_MINIMAL } from '@/lib/self-use-build'

import type { NavGroup, SidebarView } from '../types'

type SettingsNavSection = {
  id: string
  titleKey: string
  selfUseHidden?: boolean
  capability?: ProductCapability
}

const SITE_SECTIONS = [
  { id: 'system-info', titleKey: 'System Information' },
  { id: 'notice', titleKey: 'System Notice' },
  { id: 'header-navigation', titleKey: 'Header navigation' },
  { id: 'sidebar-modules', titleKey: 'Sidebar modules' },
] as const

const AUTH_SECTIONS = [
  { id: 'basic-auth', titleKey: 'Basic Authentication' },
  { id: 'oauth', titleKey: 'OAuth Integrations', capability: 'external_oauth' },
  { id: 'passkey', titleKey: 'Passkey Authentication' },
  { id: 'bot-protection', titleKey: 'Bot Protection' },
  {
    id: 'custom-oauth',
    titleKey: 'Custom OAuth',
    capability: 'external_oauth',
  },
] as const

const BILLING_SECTIONS = [
  { id: 'quota', titleKey: 'Quota Settings' },
  { id: 'currency', titleKey: 'Currency & Display' },
  { id: 'model-pricing', titleKey: 'Model Pricing' },
  { id: 'group-pricing', titleKey: 'Group Pricing' },
  { id: 'quota-writer', titleKey: 'Quota Writer Mode' },
  {
    id: 'payment',
    titleKey: 'Payment Gateway',
    selfUseHidden: true,
    capability: 'payments',
  },
  {
    id: 'checkin',
    titleKey: 'Check-in Rewards',
    selfUseHidden: true,
    capability: 'checkin',
  },
] as const

const MODEL_SECTIONS = [
  { id: 'global', titleKey: 'Global Model Configuration' },
  { id: 'routing-reliability', titleKey: 'Routing Reliability' },
  { id: 'openai-pricing-source', titleKey: 'OpenAI official pricing source' },
  { id: 'gemini', titleKey: 'Gemini' },
  { id: 'claude', titleKey: 'Claude' },
  { id: 'grok', titleKey: 'Grok' },
  { id: 'channel-affinity', titleKey: 'Channel Affinity' },
  {
    id: 'model-deployment',
    titleKey: 'Model Deployment',
    selfUseHidden: true,
    capability: 'model_deployments',
  },
] as const

const SECURITY_SECTIONS = [
  { id: 'rate-limit', titleKey: 'Rate Limiting' },
  { id: 'sensitive-words', titleKey: 'Sensitive Words' },
  { id: 'ssrf', titleKey: 'SSRF Protection' },
  { id: 'token-limits', titleKey: 'Token Limits' },
] as const

const CONTENT_SECTIONS = [
  { id: 'dashboard', titleKey: 'Data Dashboard' },
  { id: 'announcements', titleKey: 'Announcements' },
  { id: 'api-info', titleKey: 'API Addresses' },
  { id: 'faq', titleKey: 'FAQ' },
  { id: 'uptime-kuma', titleKey: 'Uptime Kuma' },
  { id: 'chat', titleKey: 'Chat Presets' },
  { id: 'drawing', titleKey: 'Drawing' },
] as const

const OPERATION_SECTIONS = [
  { id: 'behavior', titleKey: 'System Behavior' },
  { id: 'alerts', titleKey: 'Monitoring & Alerts' },
  { id: 'email', titleKey: 'SMTP Email', selfUseHidden: true },
  { id: 'worker', titleKey: 'Worker Proxy', selfUseHidden: true },
  { id: 'logs', titleKey: 'Log Maintenance' },
  { id: 'performance', titleKey: 'Performance' },
  {
    id: 'update-checker',
    titleKey: 'System maintenance',
    selfUseHidden: true,
  },
] as const

function getSettingsSectionNavItems(
  t: TFunction,
  basePath: string,
  sections: readonly SettingsNavSection[],
  capabilities: Record<ProductCapability, boolean>
) {
  return sections
    .filter((section) => !SELF_USE_MINIMAL || !section.selfUseHidden)
    .filter(
      (section) => !section.capability || capabilities[section.capability]
    )
    .map((section) => ({
      title: t(section.titleKey),
      url: `${basePath}/${section.id}`,
    }))
}

/**
 * Sidebar nav groups for the System Settings nested view.
 *
 * Kept as a single group because the workspace title in the sidebar
 * header already provides top-level context — the inner group label
 * scopes the items as "administration" actions.
 */
function getSystemSettingsNavGroups(
  t: TFunction,
  capabilities = resolveProductEdition(null).capabilities
): NavGroup[] {
  return [
    {
      id: 'system-administration',
      title: t('System Administration'),
      items: [
        {
          title: t('Site & Branding'),
          icon: Settings01Icon,
          items: getSettingsSectionNavItems(
            t,
            '/system-settings/site',
            SITE_SECTIONS,
            capabilities
          ),
        },
        {
          title: t('Authentication'),
          icon: Shield01Icon,
          items: getSettingsSectionNavItems(
            t,
            '/system-settings/auth',
            AUTH_SECTIONS,
            capabilities
          ),
        },
        {
          title: t('Billing & Payment'),
          icon: CreditCardIcon,
          items: getSettingsSectionNavItems(
            t,
            '/system-settings/billing',
            BILLING_SECTIONS,
            capabilities
          ),
        },
        {
          title: t('Models & Routing'),
          icon: CubeIcon,
          items: getSettingsSectionNavItems(
            t,
            '/system-settings/models',
            MODEL_SECTIONS,
            capabilities
          ),
        },
        {
          title: t('Security & Limits'),
          icon: SecurityIcon,
          items: getSettingsSectionNavItems(
            t,
            '/system-settings/security',
            SECURITY_SECTIONS,
            capabilities
          ),
        },
        ...(SELF_USE_MINIMAL || !capabilities.site_content
          ? []
          : [
              {
                title: t('Console Content'),
                icon: Layout01Icon,
                items: getSettingsSectionNavItems(
                  t,
                  '/system-settings/content',
                  CONTENT_SECTIONS,
                  capabilities
                ),
              },
            ]),
        {
          title: t('Operations'),
          icon: Wrench01Icon,
          items: getSettingsSectionNavItems(
            t,
            '/system-settings/operations',
            OPERATION_SECTIONS,
            capabilities
          ),
        },
      ],
    },
  ]
}

/**
 * Nested sidebar view for `/system-settings/*`.
 *
 * Activates the Vercel / Cloudflare-style drill-in sidebar:
 * the root navigation is replaced by the system administration
 * groups, with a "Back to Dashboard" affordance in the header.
 */
export const SYSTEM_SETTINGS_VIEW: SidebarView = {
  id: 'system-settings',
  pathPattern: /^\/system-settings(\/|$)/,
  parent: {
    to: '/dashboard/overview',
    label: 'Back to Dashboard',
  },
  getNavGroups: getSystemSettingsNavGroups,
}
