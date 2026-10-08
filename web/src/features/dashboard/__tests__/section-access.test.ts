import { afterEach, describe, expect, it } from 'vitest'

import { ROLE } from '@/lib/roles'
import { Route } from '@/routes/_authenticated/dashboard/$section'
import { useAuthStore } from '@/stores/auth-store'

afterEach(() => useAuthStore.getState().auth.setUser(null))

describe('dashboard direct section access', () => {
  it('rejects an ordinary user before the administrator analytics component mounts', () => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 2, username: 'owner', role: ROLE.USER })
    expect(() =>
      Route.options.beforeLoad?.({ params: { section: 'users' } } as never)
    ).toThrow()
  })

  it('retains administrator access to the user analytics section', () => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'admin', role: ROLE.ADMIN })
    expect(() =>
      Route.options.beforeLoad?.({ params: { section: 'users' } } as never)
    ).not.toThrow()
  })
})
