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
import { Link, getRouteApi } from '@tanstack/react-router'
import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import { useRememberReturnFocus } from '@/components/object-page/return-focus'
import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { connectionListContext } from '@/features/connections/connection-state'
import { CacheStatsDialog } from '@/features/system-settings/general/channel-affinity/cache-stats-dialog'
import { useMediaQuery } from '@/hooks'
import { useFundingPresentation } from '@/hooks/use-funding-presentation'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { linkButtonClassName } from '@/lib/link-button'
import { useAuthStore } from '@/stores/auth-store'

import { UserInfoDialog } from './components/dialogs/user-info-dialog'
import { PendingUsageReviews } from './components/pending-usage-reviews'
import {
  type LogsViewScope,
  UsageLogsProvider,
  useLogsViewScope,
  useUsageLogsContext,
} from './components/usage-logs-provider'
import { UsageLogsTable } from './components/usage-logs-table'
import {
  isUsageLogsSectionId,
  USAGE_LOGS_DEFAULT_SECTION,
  type UsageLogsSectionId,
} from './section-registry'

const route = getRouteApi('/_authenticated/usage-logs/$section')

const SECTION_META: Record<UsageLogsSectionId, { titleKey: string }> = {
  common: {
    titleKey: 'Call records',
  },
  drawing: {
    titleKey: 'Drawing Logs',
  },
  task: {
    titleKey: 'Task Logs',
  },
}

function UsageLogsContent() {
  const { t } = useTranslation()
  const isMobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const params = route.useParams()
  const contextSearch = route.useSearch()
  const connectionContext = contextSearch.connectionContext
  const activeCategory: UsageLogsSectionId =
    params.section && isUsageLogsSectionId(params.section)
      ? params.section
      : USAGE_LOGS_DEFAULT_SECTION
  const {
    selectedUserId,
    userInfoDialogOpen,
    setUserInfoDialogOpen,
    affinityTarget,
    affinityDialogOpen,
    setAffinityDialogOpen,
  } = useUsageLogsContext()
  const { canManageScope, viewScope, setViewScope } = useLogsViewScope()

  const handleViewScopeChange = useCallback(
    (scope: string) => {
      if (scope === 'all' || scope === 'self') {
        setViewScope(scope as LogsViewScope)
      }
    },
    [setViewScope]
  )

  // The area's page tabs (call records, bodies, tasks, drawing, analytics)
  // come from the console shell; each page keeps its own title.
  const pageMeta = SECTION_META[activeCategory]

  return (
    <>
      <SectionPageLayout fixedContent={!isMobile}>
        <SectionPageLayout.Title>
          {t(pageMeta.titleKey)}
        </SectionPageLayout.Title>
        <SectionPageLayout.Breadcrumb>
          {contextSearch.keyReturn && !connectionContext && (
            <Link
              to='/keys'
              search={contextSearch.keyReturn}
              className='text-primary underline'
            >
              {t('Back to Keys')}
            </Link>
          )}
          {connectionContext && contextSearch.returnToKeys && (
            <Link
              to='/keys'
              search={{ connectionContext }}
              className='text-primary underline'
            >
              {t('Associated Key shares')}
            </Link>
          )}
          {connectionContext && !contextSearch.returnToKeys && (
            <Link
              to='/connections/$connectionId'
              params={{
                connectionId: String(connectionContext.connection.connection),
              }}
              search={{
                connectionReturn: connectionListContext(
                  connectionContext.connection
                ),
              }}
              className='text-primary underline'
            >
              {t('Back to connection')}
            </Link>
          )}
        </SectionPageLayout.Breadcrumb>
        <SectionPageLayout.Description>
          {t('Inspect requests, routing and usage evidence.')}
        </SectionPageLayout.Description>
        {canManageScope && (
          <SectionPageLayout.Actions>
            {activeCategory === 'common' && <PendingUsageReviews />}
            <Tabs value={viewScope} onValueChange={handleViewScopeChange}>
              <TabsList>
                <TabsTrigger value='all'>{t('All')}</TabsTrigger>
                <TabsTrigger value='self'>{t('Only Mine')}</TabsTrigger>
              </TabsList>
            </Tabs>
          </SectionPageLayout.Actions>
        )}
        <SectionPageLayout.Content>
          <div
            className={
              isMobile
                ? 'flex min-h-full flex-col gap-3'
                : 'flex h-full min-h-0 flex-col gap-4'
            }
          >
            {contextSearch.tokenId && (
              <div
                className='bg-muted/30 space-y-1 rounded-lg border p-3 text-sm'
                role='status'
              >
                <p className='font-medium'>
                  {t('Requests for Key #{{id}}', {
                    id: contextSearch.tokenId,
                  })}
                </p>
                <p className='text-muted-foreground'>
                  {t(
                    'Requests are matched by the saved Key ID, including before a rename.'
                  )}
                </p>
              </div>
            )}
            {activeCategory === 'common' &&
              contextSearch.result === 'failed' && (
                <div
                  className='bg-muted/30 flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border p-3 text-sm'
                  role='status'
                  data-testid='failed-requests-filter'
                >
                  <p className='min-w-0 flex-1 basis-60'>
                    <span className='font-medium'>
                      {t('Request failures only')}
                    </span>{' '}
                    <span className='text-muted-foreground'>
                      {t(
                        'Settlement reviews and requests rejected before sending are not listed.'
                      )}
                    </span>
                  </p>
                  <Link
                    to='/usage-logs/$section'
                    params={{ section: 'common' }}
                    search={{ ...contextSearch, result: undefined, page: 1 }}
                    data-myapi-button=''
                    className={linkButtonClassName({
                      variant: 'outline',
                      size: 'sm',
                    })}
                  >
                    {t('Show all error records')}
                  </Link>
                </div>
              )}
            <div className='min-h-0 flex-1'>
              <UsageLogsTable logCategory={activeCategory} />
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <UserInfoDialog
        userId={selectedUserId}
        open={userInfoDialogOpen}
        onOpenChange={setUserInfoDialogOpen}
      />

      <CacheStatsDialog
        open={affinityDialogOpen}
        onOpenChange={setAffinityDialogOpen}
        target={
          affinityTarget
            ? {
                rule_name: affinityTarget.rule_name || '',
                using_group:
                  affinityTarget.using_group ||
                  affinityTarget.selected_group ||
                  '',
                key_hint: affinityTarget.key_hint || '',
                key_fp: affinityTarget.key_fp || '',
              }
            : null
        }
      />
    </>
  )
}

export function UsageLogs() {
  const actorKey = useAuthStore(
    (state) =>
      `${state.auth.user?.id ?? ''}:${state.auth.user?.role ?? ''}:${state.auth.session?.sid ?? ''}`
  )
  const funding = useFundingPresentation()
  const rememberFocus = useRememberReturnFocus()
  return (
    <UsageLogsProvider key={actorKey} showCosts={funding.commercialEnabled} onRequestOpen={(logId) => rememberFocus('data-usage-request-link', logId)}>
      <UsageLogsContent />
    </UsageLogsProvider>
  )
}
