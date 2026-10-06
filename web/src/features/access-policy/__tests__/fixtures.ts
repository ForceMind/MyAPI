import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement, type ReactNode } from 'react'

import { useAuthStore } from '@/stores/auth-store'

import type { AssignedPolicy } from '../types'

export const userPolicy: AssignedPolicy = {
  subject: 'user',
  subject_id: 2,
  owner_user_id: 2,
  revision: 0,
  assigned: false,
  enabled: true,
  public_models: null,
  upstream_models: null,
  channel_ids: null,
}
export const tokenPolicy: AssignedPolicy = {
  ...userPolicy,
  subject: 'token',
  subject_id: 7,
  owner_user_id: 1,
}
export function envelope(data: unknown) {
  return { data: { success: true, data } }
}
export function authenticate(role = 100, id = 1, sid = 'test-session'): void {
  useAuthStore.getState().auth.setBundle({
    access_token: 'test-access',
    token_type: 'Bearer',
    access_expires_at: 9999999999,
    user: { id, username: `fixture-${id}`, role },
    session: {
      sid,
      current: true,
      login_method: 'password',
      ip: '',
      user_agent: '',
      created_at: 0,
      last_active_at: 0,
      expires_at: 9999999999,
    },
  })
}
export function queryWrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return {
    client,
    wrapper: (props: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client }, props.children),
  }
}
export function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}
