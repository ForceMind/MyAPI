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
import { getRouteApi, Link } from '@tanstack/react-router'
import { Settings2 } from 'lucide-react'
import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getChannelOps } from './api'
import { ChannelQuotaChangesPanel } from './components/channel-quota-changes-panel'
import { ChannelsDialogs } from './components/channels-dialogs'
import { ChannelsPrimaryButtons } from './components/channels-primary-buttons'
import { ChannelsProvider } from './components/channels-provider'
import { ChannelsTable } from './components/channels-table'

const RoutingPanel = lazy(() =>
  import('@/features/channel-routing/channel-routing-dialog').then(
    (module) => ({ default: module.ChannelRoutingPanel })
  )
)
const QuotaComparison = lazy(() =>
  import('./components/channel-quota-comparison').then((module) => ({
    default: module.ChannelQuotaComparison,
  }))
)
const route = getRouteApi('/_authenticated/channels/')

export function Channels() {
  const { t } = useTranslation()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const tab =
    search.tab ?? (search.quotaChannelId != null ? 'quota' : 'channels')
  const [showDetails, setShowDetails] = useState(false)
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
      <SectionPageLayout fixedContent={false}>
        <SectionPageLayout.Title>
          <span className='flex min-w-0 items-center gap-2'>
            <span className='truncate'>{t('Channels')}</span>
            {retryBadge}
          </span>
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          {tab === 'channels' ? <ChannelsPrimaryButtons /> : null}
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <Tabs
            value={tab}
            onValueChange={(value) => {
              if (
                value === 'channels' ||
                value === 'quota' ||
                value === 'routing'
              ) {
                void navigate({
                  search: (previous) => ({ ...previous, tab: value }),
                })
              }
            }}
            className='min-w-0 gap-5'
          >
            <TabsList
              className='w-full sm:w-fit'
              aria-label={t('Channel sections')}
            >
              <TabsTrigger value='channels'>
                {t('Channel management')}
              </TabsTrigger>
              <TabsTrigger value='quota'>{t('Quota analysis')}</TabsTrigger>
              <TabsTrigger value='routing'>
                {t('Traffic Allocation')}
              </TabsTrigger>
            </TabsList>
            <TabsContent value='channels'>
              <ChannelsTable />
            </TabsContent>
            <TabsContent value='quota'>
              <Suspense fallback={<p>{t('Loading')}</p>}>
                <QuotaComparison initialChannelId={search.quotaChannelId} />
              </Suspense>
              <details
                className='mt-6 rounded-lg border p-3'
                onToggle={(event) => setShowDetails(event.currentTarget.open)}
              >
                <summary className='cursor-pointer text-sm'>
                  {t('Sampling details and diagnostics')}
                </summary>
                {showDetails ? (
                  <div className='mt-4'>
                    <ChannelQuotaChangesPanel />
                  </div>
                ) : null}
              </details>
            </TabsContent>
            <TabsContent value='routing'>
              <Suspense fallback={<p>{t('Loading')}</p>}>
                <RoutingPanel />
              </Suspense>
            </TabsContent>
          </Tabs>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ChannelsDialogs />
    </ChannelsProvider>
  )
}
