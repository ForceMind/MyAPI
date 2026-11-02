/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import type { LoginSession } from '@/stores/auth-store'
import dayjs from '@/lib/dayjs'

import { loginMethodLabel, sessionDevice } from './login-session-utils'

interface LoginSessionDialogsProps {
  revokeTarget: LoginSession | null
  confirmOthers: boolean
  revoking: boolean
  revokingOthers: boolean
  onRevokeTargetChange: (session: LoginSession | null) => void
  onConfirmOthersChange: (open: boolean) => void
  onRevoke: () => void
  onRevokeOthers: () => void
  revokeError?: string | null
  revokeOthersError?: string | null
  returnFocus?: () => HTMLElement | null
}

export function LoginSessionDialogs({
  revokeTarget,
  confirmOthers,
  revoking,
  revokingOthers,
  onRevokeTargetChange,
  onConfirmOthersChange,
  onRevoke,
  onRevokeOthers,
  revokeError,
  revokeOthersError,
  returnFocus,
}: LoginSessionDialogsProps) {
  const { t } = useTranslation()

  return (
    <>
      <ConfirmDialog
        open={Boolean(revokeTarget)}
        onOpenChange={(open) => !open && !revoking && onRevokeTargetChange(null)}
        returnFocus={returnFocus}
        title={
          revokeTarget?.current
            ? t('Sign out this device?')
            : t('Revoke session?')
        }
        desc={<div className='flex min-w-0 flex-col gap-3 [overflow-wrap:anywhere]'>
          <p>{t('This session will lose access immediately and must sign in again.')}</p>
          {revokeTarget && <dl className='grid gap-2 text-sm'>
            <div><dt className='text-muted-foreground'>{t('Device')}</dt><dd>{sessionDevice(revokeTarget.user_agent, t('Unknown device'), t('Browser'), revokeTarget.current && typeof navigator !== 'undefined' ? navigator.maxTouchPoints : 0)}{revokeTarget.current && <span className='ml-2'>({t('Current')})</span>}</dd></div>
            <div><dt className='text-muted-foreground'>{t('IP address')}</dt><dd>{revokeTarget.ip || t('Unknown')}</dd></div>
            <div><dt className='text-muted-foreground'>{t('Login method')}</dt><dd>{loginMethodLabel(revokeTarget.login_method, t)}</dd></div>
            <div><dt className='text-muted-foreground'>{t('Last active')}</dt><dd>{dayjs.unix(revokeTarget.last_active_at).format('YYYY-MM-DD HH:mm')}</dd></div>
          </dl>}
          {revokeError && <p role='alert' className='text-destructive text-sm'>{revokeError}</p>}
        </div>}
        confirmText={revokeTarget?.current ? t('Sign out') : t('Revoke')}
        destructive
        isLoading={revoking}
        handleConfirm={onRevoke}
      />

      <ConfirmDialog
        open={confirmOthers}
        onOpenChange={(open) => { if (open || !revokingOthers) onConfirmOthersChange(open) }}
        returnFocus={returnFocus}
        title={t('Sign out other sessions?')}
        desc={<div className='flex flex-col gap-3'><p>{t('Every other device will lose access immediately. This device will remain signed in.')}</p>{revokeOthersError && <p role='alert' className='text-destructive text-sm'>{revokeOthersError}</p>}</div>}
        confirmText={t('Sign out others')}
        destructive
        isLoading={revokingOthers}
        handleConfirm={onRevokeOthers}
      />
    </>
  )
}
