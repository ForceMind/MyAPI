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
import { useNavigate } from '@tanstack/react-router'
import { Alert02Icon, Loading03Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { logout } from '@/features/auth/api'
import { clearAuthentication } from '@/lib/api'

import { deleteUserAccount } from '../../api'
import { useSecurityRequestScope } from '../../hooks'

// ============================================================================
// Delete Account Dialog Component
// ============================================================================

interface DeleteAccountDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  username: string
  securityScopeKey: string
}

export function DeleteAccountDialog({
  open,
  onOpenChange,
  username,
  securityScopeKey,
}: DeleteAccountDialogProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [confirmation, setConfirmation] = useState('')
  const requestScope = useSecurityRequestScope(securityScopeKey)

  const handleDelete = async () => {
    if (confirmation !== username) {
      toast.error(t('Username confirmation does not match'))
      return
    }

    let requestCurrent = true
    try {
      setLoading(true)
      const result = await requestScope.run((signal) =>
        deleteUserAccount(undefined, signal)
      )
      requestCurrent = result.current
      if (!result.current) return
      const response = result.value

      if (response.success) {
        toast.success(t('Account deleted successfully'))

        // Logout and redirect
        try {
          await logout()
        } catch {
          // Ignore logout errors
        }

        clearAuthentication()
        navigate({ to: '/sign-in' })
      } else {
        toast.error(response.message || t('Failed to delete account'))
      }
    } catch {
      toast.error(
        t(
          'The result could not be confirmed. Verify the current account state before trying again.'
        )
      )
    } finally {
      if (requestCurrent) setLoading(false)
    }
  }

  const handleOpenChange = (open: boolean) => {
    if (!open && loading) return
    if (!open) {
      requestScope.cancel()
      setLoading(false)
      setConfirmation('')
    }
    onOpenChange(open)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={handleOpenChange}
      title={
        <>
          <HugeiconsIcon icon={Alert02Icon} className='h-5 w-5' />
          {t('Delete Account')}
        </>
      }
      description={t(
        'This action cannot be undone. This will permanently delete your account and remove all your data from our servers.'
      )}
      contentClassName='sm:max-w-md [&_[data-slot=button]]:min-h-11 [&_[data-slot=button]]:min-w-11 [&_[data-slot=input]]:min-h-11 [&_[data-slot=select-trigger]]:min-h-11 [&_[data-slot=dialog-close]]:min-h-11 [&_[data-slot=dialog-close]]:min-w-11'
      titleClassName='text-destructive flex items-center gap-2'
      contentHeight='auto'
      showCloseButton={!loading}
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => handleOpenChange(false)}
            disabled={loading}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            variant='destructive'
            onClick={handleDelete}
            disabled={loading || confirmation !== username}
          >
            {loading && <HugeiconsIcon icon={Loading03Icon} className='mr-2 h-4 w-4 animate-spin' />}
            {loading ? t('Deleting...') : t('Delete Account')}
          </Button>
        </>
      }
    >
      <section role='region' tabIndex={0} aria-label={t('Account security details')} className='min-w-0 [overflow-wrap:anywhere] [&_code]:break-all [&_code]:whitespace-pre-wrap'>
      <div className='my-6 space-y-4'>
        <Alert variant='destructive'>
          <HugeiconsIcon icon={Alert02Icon} className='h-4 w-4' />
          <AlertDescription>
            {t('Warning: This action is permanent and irreversible!')}
          </AlertDescription>
        </Alert>

        <div className='space-y-2'>
          <Label htmlFor='confirmation'>
            {t('Type')} <strong>{username}</strong> {t('to confirm')}
          </Label>
          <Input
            id='confirmation'
            type='text'
            value={confirmation}
            onChange={(e) => setConfirmation(e.target.value)}
            disabled={loading}
            placeholder={username}
            autoComplete='off'
          />
        </div>
      </div>
      </section>
    </Dialog>
  )
}
