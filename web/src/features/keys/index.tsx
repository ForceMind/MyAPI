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
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { useRememberReturnFocus } from '@/components/object-page/return-focus'
import { useMediaQuery } from '@/hooks/use-media-query'
import { usePersonalWorkbenchMode } from '@/hooks/use-personal-workbench-mode'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { useAuthStore } from '@/stores/auth-store'

import { ApiKeysDialogs } from './components/api-keys-dialogs'
import { ApiKeysPrimaryButtons } from './components/api-keys-primary-buttons'
import { ApiKeysProvider } from './components/api-keys-provider'
import { ApiKeysTable } from './components/api-keys-table'
import { ConnectionKeyWorkspace } from './components/connection-key-workspace'
import { keyListContext } from './lib/keys-search'
import type { ApiKey } from './types'

const route = getRouteApi('/_authenticated/keys/')

/**
 * API Keys: one list for every role and edition. A row opens the Key's own
 * page; Back from there returns here with the same filters and page, and
 * focus on that row.
 */
export function ApiKeys() {
  const { t } = useTranslation()
  const personal = usePersonalWorkbenchMode()
  const mobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const userId = useAuthStore((state) => state.auth.user?.id)
  const role = useAuthStore((state) => state.auth.user?.role)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const search = route.useSearch()
  const navigate = useNavigate()
  const rememberFocus = useRememberReturnFocus()
  const listContext = keyListContext(search)

  const openKey = useCallback(
    (key: ApiKey) => {
      rememberFocus('data-inspect-id', key.id)
      void navigate({
        to: '/keys/$keyId',
        params: { keyId: String(key.id) },
        search: { keyReturn: listContext },
        state: { inspectorBack: true },
      })
    },
    [navigate, rememberFocus, listContext]
  )

  if (search.connectionContext) {
    return <ConnectionKeyWorkspace context={search.connectionContext} />
  }

  return (
    <ApiKeysProvider
      key={`${userId}:${role}:${sessionId}`}
      viewRequests={openKey}
      openKey={openKey}
      requestFocusId={search.focusKeyId}
    >
      <SectionPageLayout fixedContent={!mobile}>
        {/* Same name as the navigation entry (design 5.4). */}
        <SectionPageLayout.Title>
          {t('API Keys', { context: 'nav' })}
        </SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t(
            'Credentials for people and clients; select one to open its page.'
          )}
        </SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          <ApiKeysPrimaryButtons personal={personal} />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <ApiKeysTable />
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ApiKeysDialogs />
    </ApiKeysProvider>
  )
}
