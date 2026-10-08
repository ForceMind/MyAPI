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
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Settings2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { PageSectionLinks } from '@/components/layout/components/page-section-links'
import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import { getChannelOps } from './api'
import { ChannelQuotaChangesPanel } from './components/channel-quota-changes-panel'
import { ChannelQuotaEvents } from './components/channel-quota-events'
import { ChannelRoutingPreview } from './components/channel-routing-preview'
import { ChannelsDialogs } from './components/channels-dialogs'
import { ChannelsPrimaryButtons } from './components/channels-primary-buttons'
import { ChannelsProvider } from './components/channels-provider'
import { ChannelsTable } from './components/channels-table'

function canViewChannelRoutingPreview(
  user: AuthUser | null | undefined
): boolean {
  return hasPermission(
    user,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.READ
  )
}

export function Channels() {
  const { t } = useTranslation()
  const currentUser = useAuthStore((state) => state.auth.user)
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const channelOpsQuery = useQuery({
    queryKey: ['channel-ops'],
    queryFn: getChannelOps,
    retry: false,
    staleTime: 5 * 60 * 1000,
  })
  const retryTimes = channelOpsQuery.data?.data?.retry_times
  const canReadChannels = canViewChannelRoutingPreview(currentUser)
  const retryLabel =
    typeof retryTimes === 'number' ? `${t('Max Retries')}: ${retryTimes}` : null
  let retryBadge = null
  if (retryLabel) {
    retryBadge = isRoot ? (
      <Tooltip>
        <TooltipTrigger
          render={
            <Badge
              variant='outline'
              className='shrink-0 cursor-pointer'
              aria-label={t('Retry Settings')}
              render={
                <Link
                  to='/system-settings/models/$section'
                  params={{ section: 'routing-reliability' }}
                />
              }
            />
          }
        >
          <span>{retryLabel}</span>
          <Settings2 data-icon='inline-end' />
        </TooltipTrigger>
        <TooltipContent>
          <p>{t('Retry Settings')}</p>
        </TooltipContent>
      </Tooltip>
    ) : (
      <Badge variant='outline' className='shrink-0'>
        {retryLabel}
      </Badge>
    )
  }

  return (
    <ChannelsProvider>
      {/* Charts and the table share the page scroll on every viewport height. */}
      <SectionPageLayout fixedContent={false}>
        <SectionPageLayout.Title>
          <span className='flex min-w-0 flex-wrap items-center gap-2'>
            <span className='wrap-break-word'>{t('Channels')}</span>
            {retryBadge}
          </span>
        </SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('Connect upstream accounts and inspect routing evidence.')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          <ChannelsPrimaryButtons />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='min-w-0 space-y-6'>
            <PageSectionLinks
              sections={[
                { id: 'channel-inventory', label: t('Channels') },
                { id: 'channel-evidence', label: t('Quota consumption') },
                ...(canReadChannels
                  ? [
                      { id: 'channel-events', label: t('Quota events') },
                      { id: 'channel-routing', label: t('Routing Preview') },
                    ]
                  : []),
              ]}
            />
            <section
              id='channel-inventory'
              tabIndex={-1}
              aria-label={t('Channels')}
              className='min-w-0 outline-none'
            >
              <ChannelsTable />
            </section>
            <section
              id='channel-evidence'
              tabIndex={-1}
              aria-label={t('Quota consumption')}
              className='min-w-0 outline-none'
            >
              <ChannelQuotaChangesPanel />
            </section>
            {canReadChannels ? (
              <section
                id='channel-events'
                tabIndex={-1}
                aria-label={t('Quota events')}
                className='min-w-0 outline-none'
              >
                <ChannelQuotaEvents />
              </section>
            ) : null}
            {canReadChannels ? (
              <section
                id='channel-routing'
                tabIndex={-1}
                aria-label={t('Routing Preview')}
                className='min-w-0 outline-none'
              >
                <ChannelRoutingPreview />
              </section>
            ) : null}
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ChannelsDialogs />
    </ChannelsProvider>
  )
}
