import { useQuery } from '@tanstack/react-query'

import { getChannelQuotaChanges } from '@/features/channels/api'
import { getRecentLogOverview } from '@/features/dashboard/api'
import { getTokenBudget } from '@/features/keys/token-budget-api'
import { getPlaygroundKeys } from '@/features/playground/api'
import { useStatus } from '@/hooks/use-status'
import { hasPermission } from '@/lib/admin-permissions'
import { getSelf } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import type { AuthUser } from '@/stores/auth-store'

import { parseFreshSelfUser } from './workbench-state'

const ACCOUNT_WINDOW_PARAMS = {
  range: '24h' as const,
  rate_window: 60 * 60,
  ewma_half_life: 30 * 60,
  overview_points: 24,
  limit: 12,
  sort: 'observed_desc' as const,
}

export function usePersonalWorkbenchData(props: {
  user: AuthUser
  sessionId: string | null
  selectedKeyId: number | null
}) {
  const status = useStatus()
  const identity = [props.user.id, props.sessionId] as const

  const selfQuery = useQuery({
    queryKey: ['personal-workbench', 'self', ...identity],
    queryFn: async () => parseFreshSelfUser(await getSelf(), props.user.id),
    retry: false,
    refetchOnWindowFocus: false,
  })
  const keysQuery = useQuery({
    queryKey: ['personal-workbench', 'keys', ...identity],
    queryFn: () => getPlaygroundKeys(),
    retry: false,
    refetchOnWindowFocus: false,
  })
  const identityError = selfQuery.isError || selfQuery.isRefetchError
  const identityConfirmed = selfQuery.isSuccess && !identityError
  const freshUser = identityConfirmed ? selfQuery.data : undefined
  const keysError = keysQuery.isError || keysQuery.isRefetchError
  const keysConfirmed = keysQuery.isSuccess && !keysError
  const confirmedKeys = keysConfirmed ? keysQuery.data : undefined
  const selectedKey = confirmedKeys?.find(
    (key) => key.id === props.selectedKeyId
  )
  const budgetTokenId = selectedKey?.id ?? null
  const budgetQuery = useQuery({
    queryKey: ['token-budget', props.user.id, props.user.role, budgetTokenId],
    queryFn: async ({ signal }) => {
      if (budgetTokenId === null) {
        throw new Error('No API Key selected')
      }
      const budget = await getTokenBudget(budgetTokenId, signal)
      if (
        budget.policy.token_id !== budgetTokenId ||
        budget.policy.user_id !== props.user.id
      ) {
        throw new Error('Token budget user identity mismatch')
      }
      return budget
    },
    enabled: budgetTokenId !== null,
    retry: false,
    refetchOnWindowFocus: false,
  })
  const recentQuery = useQuery({
    queryKey: ['personal-workbench', 'recent-records', ...identity],
    queryFn: async () => {
      const response = await getRecentLogOverview(false)
      if (
        !response.success ||
        !response.data ||
        !Array.isArray(response.data.requests) ||
        !Array.isArray(response.data.errors)
      ) {
        throw new Error('Recent records unavailable')
      }
      return response.data
    },
    retry: false,
    refetchOnWindowFocus: false,
  })

  const canReadChannels = Boolean(
    freshUser &&
      freshUser.role >= ROLE.ADMIN &&
      hasPermission(freshUser, 'channel', 'read')
  )
  const accountsQuery = useQuery({
    queryKey: [
      'personal-workbench',
      'account-windows',
      ...identity,
      canReadChannels,
    ],
    queryFn: async ({ signal }) => {
      const response = await getChannelQuotaChanges(ACCOUNT_WINDOW_PARAMS, {
        signal,
      })
      if (
        !response.success ||
        !response.data ||
        !Array.isArray(response.data.items)
      ) {
        throw new Error('Account windows unavailable')
      }
      return response.data
    },
    enabled: canReadChannels,
    retry: false,
    refetchOnWindowFocus: false,
  })

  return {
    status,
    selfQuery,
    keysQuery,
    confirmedKeys,
    keysError,
    budgetQuery,
    recentQuery,
    accountsQuery,
    freshUser,
    identityConfirmed,
    identityError,
    canReadChannels,
    selectedKey,
  }
}
