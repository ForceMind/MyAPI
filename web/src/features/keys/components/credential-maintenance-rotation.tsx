/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { AlertCircle, Clock3, RotateCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

import type { CredentialMigrationBlocker } from '../credential-maintenance-api'

export function CredentialMaintenanceRotation(props: {
  blockers: CredentialMigrationBlocker[]
}) {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div className='flex min-w-0 gap-3'>
            <RotateCw
              className='text-primary mt-0.5 shrink-0'
              aria-hidden='true'
            />
            <div className='min-w-0'>
              <CardTitle>{t('Credential rotation')}</CardTitle>
              <CardDescription>
                {t(
                  'Rotating this credential safely still needs compatibility work.'
                )}
              </CardDescription>
            </div>
          </div>
          <Badge variant='outline'>{t('Unavailable')}</Badge>
        </div>
      </CardHeader>
      <CardContent className='flex flex-col gap-4'>
        <Alert>
          <Clock3 aria-hidden='true' />
          <AlertTitle>{t('A short interruption may be required')}</AlertTitle>
          <AlertDescription>
            {t(
              'MyAPI cannot yet prove that every cached credential locator and in-flight request can switch safely. Rotation stays read-only in this release.'
            )}
          </AlertDescription>
        </Alert>
        {props.blockers.length > 0 && (
          <ul className='flex flex-col gap-2 text-sm'>
            {props.blockers.map((blocker) => (
              <li
                key={blocker.code}
                className='bg-muted/30 flex min-w-0 items-start gap-2 rounded-lg border p-3'
              >
                <AlertCircle
                  className='text-muted-foreground mt-0.5 shrink-0'
                  aria-hidden='true'
                />
                <span className='min-w-0 break-words'>
                  {blocker.code === 'rotation_commit_not_supported' &&
                    t('Credential rotation cannot be committed yet.')}
                  {blocker.code ===
                    'token_secret_cache_locator_compatibility_unproven' &&
                    t('Cached credential lookup compatibility is not proven.')}
                  {blocker.code === 'global_inflight_registry_unavailable' &&
                    t('A complete in-flight request registry is unavailable.')}
                  {![
                    'rotation_commit_not_supported',
                    'token_secret_cache_locator_compatibility_unproven',
                    'global_inflight_registry_unavailable',
                  ].includes(blocker.code) &&
                    t('A compatibility check is still blocking rotation.')}
                </span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
