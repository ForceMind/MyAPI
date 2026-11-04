/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { TFunction } from 'i18next'
import { z } from 'zod'

import { managementKeyScopes, type ManagementKeyScope } from '../management-key-api'

export function managementKeyScopeLabel(scope: ManagementKeyScope, t: TFunction): string {
  const labels = {
    'channels.read': t('Read channels'),
    'channels.operate': t('Operate channels'),
    'tokens.read': t('Read API tokens'),
    'logs.read': t('Read request logs'),
    'system.read': t('Read system status'),
    'operations.read': t('Read upgrade plans'),
    'operations.prepare': t('Prepare upgrades'),
    'operations.execute': t('Execute approved upgrades'),
  }
  return labels[scope]
}

export function parseManagementResourceIDs(value: string): number[] | null {
  if (!value.trim()) return []
  const parts = value.split(',').map((part) => part.trim())
  if (parts.some((part) => !/^[1-9]\d*$/.test(part) || !Number.isSafeInteger(Number(part)))) return null
  return [...new Set(parts.map(Number))]
}

export const managementKeyFormSchema = z.object({
  name: z.string().trim().min(1).max(100),
  scopes: z.array(z.enum(managementKeyScopes)).min(1),
  days: z.string().regex(/^\d+$/).refine((value) => Number(value) >= 1 && Number(value) <= 90),
  channelIDs: z.string().refine((value) => parseManagementResourceIDs(value) !== null),
  tokenIDs: z.string().refine((value) => parseManagementResourceIDs(value) !== null),
}).superRefine((value, context) => {
  if (value.scopes.some((scope) => scope.startsWith('operations.')) && (value.channelIDs.trim() || value.tokenIDs.trim())) {
    context.addIssue({ code: 'custom', path: ['scopes'], message: 'Operations permissions cannot be combined with resource ID limits.' })
  }
})

export type ManagementKeyForm = z.infer<typeof managementKeyFormSchema>
