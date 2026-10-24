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
import { getRouteApi } from '@tanstack/react-router'
import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import { useInspectorBack } from '@/components/inspector/inspector-history'
import { InspectorLayout } from '@/components/inspector/inspector-layout'
import { SectionPageLayout } from '@/components/layout'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import type { RequestInspectorTab } from '@/features/usage-logs/lib/request-inspector-tabs'
import { useMediaQuery } from '@/hooks/use-media-query'
import { usePersonalWorkbenchMode } from '@/hooks/use-personal-workbench-mode'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { useAuthStore } from '@/stores/auth-store'

import { ApiKeysDialogs } from './components/api-keys-dialogs'
import { ApiKeysPrimaryButtons } from './components/api-keys-primary-buttons'
import { ApiKeysProvider } from './components/api-keys-provider'
import { ApiKeysTable } from './components/api-keys-table'
import { ConnectionKeyWorkspace } from './components/connection-key-workspace'
import { KeyInspector } from './components/inspector/key-inspector'
import type { KeyInspectorTab } from './lib/keys-search'
import type { ApiKey } from './types'

const route = getRouteApi('/_authenticated/keys/')

/**
 * API Keys: one list for every role and edition, with the Key inspector
 * (recent requests, access, window shares) beside it. Everything the
 * inspector shows is in the URL (`inspect`, `tab`, `request`, `requestTab`).
 */
export function ApiKeys() {
  const { t } = useTranslation()
  const personal = usePersonalWorkbenchMode()
  const mobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const userId = useAuthStore((state) => state.auth.user?.id)
  const role = useAuthStore((state) => state.auth.user?.role)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const goBack = useInspectorBack()
  const inspected = search.inspect
  const requestOpen =
    search.request !== undefined || search.requestLog !== undefined

  const openKey = useCallback(
    (key: ApiKey) => {
      const switching = inspected !== undefined
      void navigate({
        search: (previous) => ({
          ...previous,
          inspect: key.id,
          tab: undefined,
          request: undefined,
          requestLog: undefined,
          requestTab: undefined,
        }),
        // Switching Keys replaces the entry, so Back still leaves the
        // inspector instead of walking through every Key looked at.
        replace: switching,
        state: switching ? (previous) => previous : { inspectorBack: true },
      })
    },
    [inspected, navigate]
  )
  const closeKey = useCallback(
    () =>
      goBack(
        () =>
          void navigate({
            search: (previous) => ({
              ...previous,
              inspect: undefined,
              tab: undefined,
              request: undefined,
              requestLog: undefined,
              requestTab: undefined,
            }),
            replace: true,
          })
      ),
    [goBack, navigate]
  )
  const changeTab = useCallback(
    (tab: KeyInspectorTab) =>
      void navigate({
        search: (previous) => ({
          ...previous,
          tab: tab === 'requests' ? undefined : tab,
        }),
        replace: true,
        state: (previous) => previous,
      }),
    [navigate]
  )
  const openRequest = useCallback(
    (log: UsageLog) =>
      void navigate({
        search: (previous) => ({
          ...previous,
          request: log.request_id || undefined,
          requestLog: log.request_id ? undefined : log.id,
          requestTab: undefined,
        }),
        state: { inspectorBack: true },
      }),
    [navigate]
  )
  const closeRequest = useCallback(
    () =>
      goBack(
        () =>
          void navigate({
            search: (previous) => ({
              ...previous,
              request: undefined,
              requestLog: undefined,
              requestTab: undefined,
            }),
            replace: true,
          })
      ),
    [goBack, navigate]
  )
  const changeRequestTab = useCallback(
    (requestTab: RequestInspectorTab) =>
      void navigate({
        search: (previous) => ({
          ...previous,
          requestTab: requestTab === 'diagnosis' ? undefined : requestTab,
        }),
        replace: true,
        state: (previous) => previous,
      }),
    [navigate]
  )

  if (search.connectionContext) {
    return <ConnectionKeyWorkspace context={search.connectionContext} />
  }

  return (
    <ApiKeysProvider
      key={`${userId}:${role}:${sessionId}`}
      viewRequests={openKey}
      openKey={openKey}
      inspectedKeyId={inspected}
      requestFocusId={search.focusKeyId}
      onKeysDeleted={(ids) => {
        if (inspected !== undefined && ids.includes(inspected)) closeKey()
      }}
    >
      <SectionPageLayout fixedContent={!mobile}>
        {/* Same name as the navigation entry (design 5.4). */}
        <SectionPageLayout.Title>
          {t('API Keys', { context: 'nav' })}
        </SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t(
            'Select a Key to see its recent requests, access and window shares.'
          )}
        </SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          <ApiKeysPrimaryButtons personal={personal} />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <InspectorLayout
            open={inspected !== undefined}
            // With a request open, the top bar and Escape step back to the
            // Key; the inspector's close control still closes everything.
            label={
              requestOpen
                ? t('Request {{requestId}}', {
                    requestId: search.request ?? `#${search.requestLog}`,
                  })
                : t('API Key #{{id}}', { id: inspected ?? '' })
            }
            onClose={requestOpen ? closeRequest : closeKey}
            fill={!mobile}
            list={<ApiKeysTable />}
            inspector={
              inspected !== undefined && (
                <KeyInspector
                  key={`${userId}:${sessionId}:${inspected}`}
                  keyId={inspected}
                  tab={search.tab ?? 'requests'}
                  request={search.request}
                  requestLog={search.requestLog}
                  requestTab={search.requestTab ?? 'diagnosis'}
                  personal={personal}
                  returnContext={{
                    page: search.page,
                    pageSize: search.pageSize,
                    filter: search.filter,
                    status: search.status,
                    focusKeyId: inspected,
                  }}
                  onTabChange={changeTab}
                  onOpenRequest={openRequest}
                  onCloseRequest={closeRequest}
                  onRequestTabChange={changeRequestTab}
                  onClose={closeKey}
                />
              )
            }
          />
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ApiKeysDialogs />
    </ApiKeysProvider>
  )
}
