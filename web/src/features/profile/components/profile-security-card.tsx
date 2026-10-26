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
import { Shield, Key, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { useDialogs } from '@/hooks/use-dialog'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import type { UserProfile } from '../types'
import { AccessTokenDialog } from './dialogs/access-token-dialog'
import { ChangePasswordDialog } from './dialogs/change-password-dialog'
import { DeleteAccountDialog } from './dialogs/delete-account-dialog'

// ============================================================================
// Profile Security Card Component
// ============================================================================

interface ProfileSecurityCardProps {
  profile: UserProfile | null
  loading: boolean
  securityScopeKey: string
}

type DialogKey = 'password' | 'token' | 'delete'

export function ProfileSecurityCard({
  profile,
  loading,
  securityScopeKey,
}: ProfileSecurityCardProps) {
  const { t } = useTranslation()
  const dialogs = useDialogs<DialogKey>()
  const authRole = useAuthStore((state) => state.auth.user?.role)

  if (loading) {
    return (
      <Card data-card-hover='false'>
        <CardHeader>
          <Skeleton className='h-5 w-32' />
          <Skeleton className='h-4 w-48' />
        </CardHeader>
        <CardContent className='flex flex-col gap-3'>
          {['password', 'token', 'delete'].map((key) => (
            <Skeleton key={key} className='h-16 w-full' />
          ))}
        </CardContent>
      </Card>
    )
  }

  if (!profile) return null

  const isRoot = (authRole ?? profile.role) === 100

  const securityActions = [
    {
      icon: Shield,
      title: t('Change Password'),
      description: t('Update your password to keep your account secure'),
      action: () => dialogs.open('password'),
      available: true,
      variant: 'default' as const,
    },
    {
      icon: Key,
      title: t('Access Token'),
      description: t(
        'Regenerate the console access token only when you intend to replace it'
      ),
      action: () => dialogs.open('token'),
      available: true,
      variant: 'default' as const,
    },
    {
      icon: Trash2,
      title: t('Delete Account'),
      description: isRoot
        ? t('Root accounts cannot be deleted')
        : t('Permanently delete your account and all data'),
      action: () => dialogs.open('delete'),
      available: !isRoot,
      variant: 'destructive' as const,
    },
  ]

  return (
    <>
      <Card data-card-hover='false'>
        <CardHeader>
          <CardTitle>
            <h2>{t('Password and account access')}</h2>
          </CardTitle>
          <CardDescription>
            {t(
              'Review each capability before changing access to this account.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='grid grid-cols-1 gap-3'>
          {securityActions.map((item) => (
            <button
              key={item.title}
              type='button'
              onClick={item.action}
              disabled={!item.available}
              className={cn(
                'enabled:hover:bg-muted/50 flex min-h-16 items-center gap-3 rounded-lg border p-3 text-left',
                // Deleting the account is the one irreversible action here.
                item.variant === 'destructive' &&
                  'border-destructive/40 enabled:hover:bg-destructive-muted',
                !item.available && 'cursor-not-allowed opacity-70'
              )}
            >
              <IconBadge
                tone={item.variant === 'destructive' ? 'destructive' : 'neutral'}
                size='sm'
              >
                <item.icon />
              </IconBadge>
              <div className='min-w-0 flex-1'>
                <p
                  className={cn(
                    'text-sm font-medium',
                    item.variant === 'destructive' && 'text-destructive'
                  )}
                >
                  {item.title}
                </p>
                <p className='text-muted-foreground mt-1 text-xs leading-5'>
                  {item.description}
                </p>
              </div>
              <Badge variant='secondary' className='shrink-0'>
                {item.available ? t('Available') : t('Unavailable')}
              </Badge>
            </button>
          ))}
        </CardContent>
      </Card>

      {/* Dialogs */}
      <ChangePasswordDialog
        open={dialogs.isOpen('password')}
        onOpenChange={(open) =>
          open ? dialogs.open('password') : dialogs.close('password')
        }
        username={profile.username}
        securityScopeKey={securityScopeKey}
      />

      <AccessTokenDialog
        open={dialogs.isOpen('token')}
        onOpenChange={(open) =>
          open ? dialogs.open('token') : dialogs.close('token')
        }
        securityScopeKey={securityScopeKey}
      />

      <DeleteAccountDialog
        open={dialogs.isOpen('delete')}
        onOpenChange={(open) =>
          open ? dialogs.open('delete') : dialogs.close('delete')
        }
        username={profile.username}
        securityScopeKey={securityScopeKey}
      />
    </>
  )
}
