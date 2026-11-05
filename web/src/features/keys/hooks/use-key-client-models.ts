import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { getUserModels } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

/** Partition directory reads by the whole actor identity; never reuse another session. */
export function useKeyClientModels(enabled: boolean) {
  const { t } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const role = useAuthStore((state) => state.auth.user?.role ?? null)
  const sid = useAuthStore((state) => state.auth.session?.sid ?? null)
  return useQuery({
    queryKey: ['user-models-key-client', userId, role, sid],
    queryFn: async ({ signal }) => {
      const isCurrentScope = () => {
        const current = useAuthStore.getState().auth
        return !signal.aborted && (current.user?.id ?? null) === userId &&
          (current.user?.role ?? null) === role && (current.session?.sid ?? null) === sid
      }
      const response = await getUserModels({ signal, disableDuplicate: true, skipErrorHandler: true, skipBusinessError: true, isCurrentScope })
      if (!response.success || !Array.isArray(response.data) ||
        !response.data.every((model) => typeof model === 'string')) {
        throw new Error(response.message || t('Unable to load available models'))
      }
      return response.data
    },
    retry: false,
    enabled,
    staleTime: 5 * 60 * 1000,
  })
}
