import { createFileRoute, redirect } from '@tanstack/react-router'

import { PersonalUpstream } from '@/features/personal-upstream/personal-upstream'
import { hasPermission } from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/_authenticated/upstream-accounts/')({
  beforeLoad: () => {
    const user = useAuthStore.getState().auth.user
    if (!user || user.role < ROLE.ADMIN || !hasPermission(user, 'channel', 'read')) {
      throw redirect({ to: '/403' })
    }
  },
  component: PersonalUpstream,
})
