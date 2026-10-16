/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { AlertCircle, Clock3, RotateCw } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

import type { CredentialMaintenance } from '../credential-maintenance-api'
import type { CredentialRotationReceipt } from '../credential-rotation-api'
import { CredentialRotationDialog } from './credential-rotation-dialog'

function RotationBlocker(props: { code: string }) {
  const { t } = useTranslation()
  let message = t(
    'A settlement or credential qualification check is still blocking rotation.'
  )
  if (props.code === 'rotation_commit_not_supported') {
    message = t('Credential rotation cannot be committed yet.')
  }
  if (props.code === 'token_secret_cache_locator_compatibility_unproven') {
    message = t('Cached credential lookup compatibility is not proven.')
  }
  if (props.code === 'global_inflight_registry_unavailable') {
    message = t('A complete in-flight request registry is unavailable.')
  }
  return (
    <li className='bg-muted/30 flex min-w-0 items-start gap-2 rounded-lg border p-3'>
      <AlertCircle
        className='text-muted-foreground mt-0.5 shrink-0'
        aria-hidden='true'
      />
      <span className='min-w-0 break-words'>{message}</span>
    </li>
  )
}

export function CredentialMaintenanceRotation(props: {
  rotation: CredentialMaintenance['rotation']
  token: CredentialMaintenance['token']
  userId: number
  otherWriteLocked: boolean
  onBeforeOpen: () => void
  onActivityChange: (active: boolean) => void
  onCommitted: (receipt: CredentialRotationReceipt) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [writeLocked, setWriteLocked] = useState(false)
  const available =
    props.rotation.status === 'ready' && props.rotation.commit_supported

  useEffect(() => {
    props.onActivityChange(open || writeLocked)
  }, [open, props, writeLocked])

  const openRotation = () => {
    if (
      (!available && !writeLocked) ||
      (props.otherWriteLocked && !writeLocked)
    ) {
      return
    }
    props.onBeforeOpen()
    setOpen(true)
  }

  return (
    <>
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
                  {available
                    ? t(
                        'Rotate this Key value after MyAPI verifies the current settlement path.'
                      )
                    : t(
                        'Rotation stays unavailable until every credential and settlement check is complete.'
                      )}
                </CardDescription>
              </div>
            </div>
            <Badge variant={available ? 'default' : 'outline'}>
              {available ? t('Available') : t('Unavailable')}
            </Badge>
          </div>
        </CardHeader>
        <CardContent className='flex flex-col gap-4'>
          {available ? (
            <Alert>
              <Clock3 aria-hidden='true' />
              <AlertTitle>
                {t('The old Key value stops immediately')}
              </AlertTitle>
              <AlertDescription>
                {t(
                  'A preview and security verification are required. Accepted requests continue settling through the same writer.'
                )}
              </AlertDescription>
            </Alert>
          ) : (
            <Alert>
              <Clock3 aria-hidden='true' />
              <AlertTitle>{t('Rotation remains read-only')}</AlertTitle>
              <AlertDescription>
                {t(
                  'Keep the current Key value while MyAPI resolves the listed qualification checks.'
                )}
              </AlertDescription>
            </Alert>
          )}
          {props.rotation.blockers.length > 0 && (
            <ul className='flex flex-col gap-2 text-sm'>
              {props.rotation.blockers.map((blocker) => (
                <RotationBlocker key={blocker.code} code={blocker.code} />
              ))}
            </ul>
          )}
          {(available || writeLocked) && (
            <Button
              className='h-auto min-h-9 w-full min-w-0 whitespace-normal sm:w-auto'
              type='button'
              variant='outline'
              disabled={props.otherWriteLocked && !writeLocked}
              onClick={openRotation}
            >
              <RotateCw data-icon='inline-start' aria-hidden='true' />
              {writeLocked
                ? t('Review rotation status')
                : t('Check and rotate Key')}
            </Button>
          )}
        </CardContent>
      </Card>

      <CredentialRotationDialog
        open={open}
        tokenId={props.token.id}
        tokenName={props.token.name}
        maskedKey={props.token.masked_key}
        userId={props.userId}
        onOpenChange={setOpen}
        onWriteLockChange={setWriteLocked}
        onCommitted={props.onCommitted}
      />
    </>
  )
}
