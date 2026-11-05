/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { listSystemTasks } from '@/features/system-settings/api'
import type { SystemTaskStatus } from '@/features/system-settings/types'

import { listSystemInstances } from './api'

export const INSTANCE_POLL_INTERVAL_MS = 30_000
export const TASK_LIMIT = 20
export const ACTIVE_TASK_POLL_INTERVAL_MS = 8000

export function isActiveTaskStatus(status: SystemTaskStatus) {
  return status === 'pending' || status === 'running'
}

/**
 * Instances reporting from this deployment. The page's figures and the
 * instances card read the same query, so the page sends one request.
 */
export function useSystemInstances() {
  const { t } = useTranslation()
  const actor = useAuthStore((state) => state.auth.user)
  const sid = useAuthStore((state) => state.auth.session?.sid ?? null)
  return useQuery({
    queryKey: ['system-info', 'instances', actor?.id ?? null, actor?.role ?? null, sid],
    queryFn: async ({ signal }) => {
      const isCurrentScope = () => {
        const current = useAuthStore.getState().auth
        return !signal.aborted && actor?.id === current.user?.id && actor?.role === current.user?.role && current.user?.role === ROLE.SUPER_ADMIN && sid === (current.session?.sid ?? null)
      }
      const config = { signal, skipErrorHandler: true, skipBusinessError: true, disableDuplicate: true, isCurrentScope }
      const res = await listSystemInstances(config)
      if (!res.success || !Array.isArray(res.data)) {
        throw new Error(res.message || t('We could not load instances.'))
      }
      return res.data
    },
    staleTime: 30 * 1000,
    enabled: actor?.role === ROLE.SUPER_ADMIN,
    retry: false,
    refetchInterval: INSTANCE_POLL_INTERVAL_MS,
  })
}

/**
 * The latest background tasks; polled only while one is pending or running.
 * Shared by the page's figures and the tasks card.
 */
export function useSystemTasks() {
  const { t } = useTranslation()
  const actor = useAuthStore((state) => state.auth.user)
  const sid = useAuthStore((state) => state.auth.session?.sid ?? null)
  return useQuery({
    queryKey: ['system-info', 'system-tasks', actor?.id ?? null, actor?.role ?? null, sid],
    queryFn: async ({ signal }) => {
      const isCurrentScope = () => {
        const current = useAuthStore.getState().auth
        return !signal.aborted && actor?.id === current.user?.id && actor?.role === current.user?.role && current.user?.role === ROLE.SUPER_ADMIN && sid === (current.session?.sid ?? null)
      }
      const config = { signal, skipErrorHandler: true, skipBusinessError: true, disableDuplicate: true, isCurrentScope }
      const res = await listSystemTasks(TASK_LIMIT, config)
      if (!res.success || !Array.isArray(res.data)) {
        throw new Error(res.message || t('We could not load system tasks.'))
      }
      return res.data
    },
    staleTime: 30 * 1000,
    enabled: actor?.role === ROLE.SUPER_ADMIN,
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.some((task) => isActiveTaskStatus(task.status))
        ? ACTIVE_TASK_POLL_INTERVAL_MS
        : false,
  })
}
