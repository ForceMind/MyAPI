/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Key01Icon, PlusSignIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { useConfirmedFundingMode } from '@/hooks/use-personal-workbench-mode'
import { useProductEdition } from '@/hooks/use-product-edition'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { hasPermission } from '@/lib/admin-permissions'
import { linkButtonClassName } from '@/lib/link-button'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { useDashboardContentVisibility } from '../../hooks/use-status-data'
import { AnnouncementsPanel } from './announcements-panel'
import { ApiInfoPanel } from './api-info-panel'
import { FAQPanel } from './faq-panel'
import { OverviewAttention } from './overview-attention'
import { OverviewHealth } from './overview-health'
import { OverviewNextSteps } from './overview-next-steps'
import { OverviewRecentRequests } from './overview-recent-requests'
import { OverviewWindows } from './overview-windows'
import { PerformanceHealthPanel } from './performance-health-panel'
import { SummaryCards } from './summary-cards'
import { UptimePanel } from './uptime-panel'
import { useOverviewData } from './use-overview-data'

/**
 * Overview (design 8.1), one page for every role and edition: health strip,
 * what needs attention, the most used account windows, recent requests and,
 * for a new instance, the next steps. Funds appear only when the server has
 * confirmed a commercial funding mode; site content only where the edition
 * provides it.
 */
export function OverviewPage() {
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  if (!user) return null
  // A different account or session starts from empty state.
  return <OverviewSession key={`${user.id}:${user.role}:${sessionId ?? ''}`} />
}

function OverviewSession() {
  const { t } = useTranslation()
  const data = useOverviewData()
  const edition = useProductEdition()
  const fundingMode = useConfirmedFundingMode()
  const visibility = useDashboardContentVisibility()
  const showKeys = useIsSidebarModuleVisible('/keys')
  const showConnections = useIsSidebarModuleVisible('/connections')
  const isAdmin = data.isAdmin
  const canAddConnection =
    showConnections &&
    isAdmin &&
    hasPermission(data.user, 'channel', 'read') &&
    hasPermission(data.user, 'channel', 'write')
  const siteContent = edition.capabilities.site_content
  const showFunds = fundingMode === 'enabled' || fundingMode === 'retirement'
  const contentPanels = [
    siteContent && visibility.apiInfo && <ApiInfoPanel key='api-info' />,
    siteContent && visibility.announcements && (
      <AnnouncementsPanel key='announcements' />
    ),
    siteContent && visibility.faq && <FAQPanel key='faq' />,
    siteContent && visibility.uptimeKuma && <UptimePanel key='uptime' />,
  ].filter(Boolean)

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Overview')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        {showKeys && (
          <Link
            to='/keys'
            data-myapi-button=''
            className={linkButtonClassName()}
          >
            <HugeiconsIcon
              icon={Key01Icon}
              data-icon='inline-start'
              aria-hidden='true'
            />
            {t('Create API Key')}
          </Link>
        )}
        {canAddConnection && (
          <Link
            to='/connections'
            data-myapi-button=''
            className={linkButtonClassName({ variant: 'outline' })}
          >
            <HugeiconsIcon
              icon={PlusSignIcon}
              data-icon='inline-start'
              aria-hidden='true'
            />
            {t('Add connection')}
          </Link>
        )}
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex min-w-0 flex-col gap-3' data-testid='overview'>
          <OverviewHealth data={data} />
          <div
            className={cn(
              'grid min-w-0 items-start gap-3',
              isAdmin && 'lg:grid-cols-2'
            )}
          >
            <OverviewAttention data={data} />
            {isAdmin && <OverviewWindows data={data} />}
          </div>
          <OverviewRecentRequests data={data} />
          <OverviewNextSteps
            data={data}
            showKeys={showKeys}
            showConnections={showConnections}
          />
          {showFunds && <SummaryCards />}
          {isAdmin && edition.capabilities.perf_metrics && (
            <PerformanceHealthPanel />
          )}
          {contentPanels.length > 0 && (
            <div className='grid min-w-0 gap-4 lg:grid-cols-2'>
              {contentPanels}
            </div>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
