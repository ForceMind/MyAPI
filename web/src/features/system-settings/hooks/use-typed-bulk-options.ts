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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import i18next from 'i18next'
import { toast } from 'sonner'
import { useSettingsReloadScope } from '../components/settings-page-context'
import { useLayoutEffect, useRef } from 'react'
import { useAuthStore } from '@/stores/auth-store'
import { markReportedOptionFailure, OptionUpdateError } from './use-update-option'

import { getOptionsTypedBulkRevision, updateOptionsTypedBulk } from '../api'
import type {
  TypedBulkOptionItem,
  TypedBulkOptionType,
  TypedBulkRevisionResponse,
  SystemOptionsResponse,
} from '../types'

export const TYPED_BULK_REVISION_QUERY_KEY = [
  'system-options-typed-bulk-revision',
] as const

// Configuration keys that require status refresh (mirrors use-update-option).
const STATUS_RELATED_KEYS = new Set([
  'HeaderNavModules',
  'SidebarModulesAdmin',
  'Notice',
  'LogConsumeEnabled',
  'QuotaPerUnit',
  'USDExchangeRate',
  'DisplayInCurrencyEnabled',
  'DisplayTokenStatEnabled',
  'general_setting.quota_display_type',
  'general_setting.custom_currency_symbol',
  'general_setting.custom_currency_exchange_rate',
  'oidc.display_name',
])

function inferTypedBulkItemType(value: unknown): TypedBulkOptionType {
  if (typeof value === 'boolean') return 'boolean'
  if (typeof value === 'number') return 'number'
  if (Array.isArray(value) && value.every((el) => typeof el === 'string')) {
    return 'string_list'
  }
  return 'string'
}

/**
 * Build one typed bulk item from a changed form value.
 *
 * Mapping follows docs/OPTION_TYPED_BULK_API.md: booleans stay `boolean`,
 * numbers stay `number`, string arrays become `string_list`, and everything
 * else (plain text, JSON text for map/object fields, non-string arrays) is
 * sent as `string` — non-string values are JSON-encoded first.
 */
export function toTypedBulkItem(
  key: string,
  value: unknown
): TypedBulkOptionItem {
  const type = inferTypedBulkItemType(value)

  if (type === 'string' && typeof value !== 'string') {
    return { key, type, value: JSON.stringify(value ?? null) }
  }

  return { key, type, value: value as TypedBulkOptionItem['value'] }
}

export function toTypedBulkItems(
  changedValues: Record<string, unknown>
): TypedBulkOptionItem[] {
  return Object.entries(changedValues).map(([key, value]) =>
    toTypedBulkItem(key, value)
  )
}

/**
 * Load the persistent typed bulk revision when a merged form mounts, so the
 * submit can compare-and-swap on it. The mutation also refetches through the
 * query cache when the value is stale.
 */
export function useTypedBulkRevision() {
  return useQuery({
    queryKey: TYPED_BULK_REVISION_QUERY_KEY,
    queryFn: getOptionsTypedBulkRevision,
    staleTime: 60 * 1000,
  })
}

function readRevision(data: TypedBulkRevisionResponse | undefined): number {
  return data?.data?.revision ?? 0
}

/**
 * Single-request typed bulk save for merged settings forms.
 *
 * - Sends every changed key in one PUT with `expected_revision` CAS.
 * - 409: the server revision moved; offers a reload action that refreshes the
 *   options and revision caches so the forms re-render with latest values.
 * - 400: surfaces the server-side validation message.
 * - Success: stores the new revision and refreshes the system options cache.
 */
export function useUpdateTypedBulkOptions() {
  const queryClient = useQueryClient()
  const reloadScope = useSettingsReloadScope()
  const initialAuth = useAuthStore.getState().auth
  const actor = useRef(`${initialAuth.user?.id ?? null}:${initialAuth.user?.role ?? 0}:${initialAuth.session?.sid ?? null}`)
  const mounted = useRef(true)
  useLayoutEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  const isCurrent = () => {
    const { auth } = useAuthStore.getState()
    return mounted.current && reloadScope.isActive() && actor.current === `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`
  }

  return useMutation({
    mutationFn: async (items: TypedBulkOptionItem[]) => {
      if (!isCurrent()) throw new OptionUpdateError(i18next.t('Session changed'))
      const revisionResponse = await queryClient.fetchQuery({
        queryKey: TYPED_BULK_REVISION_QUERY_KEY,
        queryFn: getOptionsTypedBulkRevision,
        staleTime: 60 * 1000,
      })
      if (!isCurrent()) throw new OptionUpdateError(i18next.t('Session changed'))
      return updateOptionsTypedBulk({
        expected_revision: readRevision(revisionResponse),
        items,
      })
    },
    onSuccess: (data, items) => {
      if (!data.success) {
        if (isCurrent()) toast.error(data.message || i18next.t('Failed to update setting'))
        return
      }

      if (data.data?.revision != null) {
        queryClient.setQueryData<TypedBulkRevisionResponse>(
          TYPED_BULK_REVISION_QUERY_KEY,
          {
            success: true,
            message: '',
            data: { revision: data.data.revision },
          }
        )
      }

      queryClient.invalidateQueries({ queryKey: ['system-options'] })

      if (items.some((item) => STATUS_RELATED_KEYS.has(item.key))) {
        queryClient.invalidateQueries({ queryKey: ['status'] })
        try {
          window.localStorage.removeItem('status')
        } catch {
          /* empty */
        }
      }

      if (isCurrent()) toast.success(i18next.t('Setting updated successfully'))
    },
    onError: (error: Error) => {
      markReportedOptionFailure(error)
      if (!isCurrent()) return
      if (isAxiosError(error)) {
        const status = error.response?.status
        const message =
          (error.response?.data as { message?: string } | undefined)?.message ??
          ''

        if (status === 409) {
          toast.error(
            message ||
              i18next.t(
                'These settings were changed elsewhere. Reload to get the latest values before saving again.'
              ),
            {
              action: {
                label: i18next.t('Reload'),
                onClick: async () => {
                  try {
                    await Promise.all([
                      queryClient.invalidateQueries({ queryKey: ['system-options'] }, { throwOnError: true }),
                      queryClient.invalidateQueries({ queryKey: TYPED_BULK_REVISION_QUERY_KEY }, { throwOnError: true }),
                    ])
                    if (!reloadScope.isActive() || !reloadScope.hasEditors()) return
                    const options = queryClient.getQueryData<SystemOptionsResponse>(['system-options'])
                    const revision = queryClient.getQueryData<TypedBulkRevisionResponse>(TYPED_BULK_REVISION_QUERY_KEY)
                    if (options?.success !== true || !Array.isArray(options.data) || revision?.success === false) {
                      toast.error(i18next.t('Unable to load settings'))
                      return
                    }
                    reloadScope.acceptReload()
                  } catch {
                    if (reloadScope.isActive() && reloadScope.hasEditors()) toast.error(i18next.t('Unable to load settings'))
                  }
                },
              },
            }
          )
          return
        }

        if (status === 400) {
          toast.error(message || i18next.t('Failed to update setting'))
          return
        }
      }

      toast.error(error.message || i18next.t('Failed to update setting'))
    },
  })
}
