import { useRef, useState } from 'react'
import { useRememberReturnFocus, useReturnFocus } from '@/components/object-page/return-focus'
import { upstreamSearchSchema } from '@/features/personal-upstream/upstream-view-state'
import { createFileRoute, getRouteApi, redirect } from '@tanstack/react-router'

import { PersonalUpstream } from '@/features/personal-upstream/personal-upstream'
import { hasPermission } from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/_authenticated/upstream-accounts/')({
  beforeLoad: () => {
    const user = useAuthStore.getState().auth.user
    if (
      !user ||
      user.role < ROLE.ADMIN ||
      !hasPermission(user, 'channel', 'read')
    ) {
      throw redirect({ to: '/403' })
    }
  },
  validateSearch: upstreamSearchSchema,
  component: UpstreamAccountsPage,
})

const route = getRouteApi('/_authenticated/upstream-accounts/')

function UpstreamAccountsPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const remember = useRememberReturnFocus()
  const fallback = useRef<HTMLHeadingElement>(null)
  const [ready, setReady] = useState(false)
  useReturnFocus({ attribute: 'data-upstream-account', requestedId: search.focusChannelId, settled: ready, rows: search, fallback })
  return <PersonalUpstream view={search} onViewChange={(next, options) => void navigate({ search: { ...next, focusChannelId: options?.preserveFocus ? search.focusChannelId : undefined }, replace: true })} onOpenConnection={(id) => remember('data-upstream-account', id)} onInventoryReady={setReady} inventoryHeadingRef={fallback} />
}
