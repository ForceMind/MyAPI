/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

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
  return useQuery({
    queryKey: ['system-info', 'instances'],
    queryFn: async () => {
      const res = await listSystemInstances()
      if (!res.success || !Array.isArray(res.data)) {
        throw new Error(res.message || t('We could not load instances.'))
      }
      return res.data
    },
    staleTime: 30 * 1000,
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
  return useQuery({
    queryKey: ['system-info', 'system-tasks'],
    queryFn: async () => {
      const res = await listSystemTasks(TASK_LIMIT)
      if (!res.success || !Array.isArray(res.data)) {
        throw new Error(res.message || t('We could not load system tasks.'))
      }
      return res.data
    },
    staleTime: 30 * 1000,
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.some((task) => isActiveTaskStatus(task.status))
        ? ACTIVE_TASK_POLL_INTERVAL_MS
        : false,
  })
}
