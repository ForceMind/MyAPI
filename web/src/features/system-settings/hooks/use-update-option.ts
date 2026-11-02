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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { toast } from 'sonner'
import { useLayoutEffect, useRef } from 'react'

import { useAuthStore } from '@/stores/auth-store'
import { useSettingsReloadScope } from '../components/settings-page-scope'

import { updateSystemOption } from '../api'
import type { UpdateOptionRequest } from '../types'
import { markReportedOptionFailure, OptionUpdateError } from './option-update-error'

// Configuration keys that require status refresh
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

export function useUpdateOption() {
  const queryClient = useQueryClient()
  const reloadScope = useSettingsReloadScope()
  const initialAuth = useAuthStore.getState().auth
  const actor = useRef(`${initialAuth.user?.id ?? null}:${initialAuth.user?.role ?? 0}:${initialAuth.session?.sid ?? null}`)
  const mounted = useRef(true)
  useLayoutEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])

  return useMutation({
    mutationFn: async (request: UpdateOptionRequest) => {
      const { auth } = useAuthStore.getState()
      if (!mounted.current || !reloadScope.isActive() || actor.current !== `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`) throw new OptionUpdateError(i18next.t('Session changed'))
      const data = await updateSystemOption(request)
      if (data?.success !== true) throw new OptionUpdateError(data?.message || i18next.t('Failed to update setting'))
      return data
    },
    retry: false,
    onSuccess: (data, variables) => {
      if (data.success) {
        // Always refresh system-options
        queryClient.invalidateQueries({ queryKey: ['system-options'] })

        // If updating frontend-display-related config, also refresh status
        if (STATUS_RELATED_KEYS.has(variables.key)) {
          queryClient.invalidateQueries({ queryKey: ['status'] })
          try {
            window.localStorage.removeItem('status')
          } catch {
            /* empty */
          }
        }

        const { auth } = useAuthStore.getState()
        if (mounted.current && reloadScope.isActive() && actor.current === `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`) toast.success(i18next.t('Setting updated successfully'))
      } else {
        toast.error(data.message || i18next.t('Failed to update setting'))
      }
    },
    onError: (error: Error) => {
      markReportedOptionFailure(error)
      const { auth } = useAuthStore.getState()
      if (!mounted.current || !reloadScope.isActive() || actor.current !== `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`) return
      toast.error(error.message || i18next.t('Failed to update setting'))
    },
  })
}
