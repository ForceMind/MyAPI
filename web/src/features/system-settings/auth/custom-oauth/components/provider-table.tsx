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
import { Plus } from 'lucide-react'
import { useLayoutEffect, useRef, useState } from 'react'
import { useAuthStore } from '@/stores/auth-store'
import { isCurrentSettingsFrame, useSettingsReloadScope } from '../../../components/settings-page-scope'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { BadgeCell } from '@/components/data-table/core/badge-cell'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'

import { useDeleteProvider } from '../hooks/use-custom-oauth-mutations'
import type { CustomOAuthProvider } from '../types'

type ProviderTableProps = {
  providers: CustomOAuthProvider[]
  onEdit: (provider: CustomOAuthProvider) => void
  onCreate: () => void
}

export function ProviderTable(props: ProviderTableProps) {
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  return <ProviderTableTask key={identity} {...props} identity={identity} />
}

function ProviderTableTask(props: ProviderTableProps & { identity: string }) {
  const { t } = useTranslation()
  const scope = useSettingsReloadScope()
  const alive = useRef(true)
  const pending = useRef(false)
  const focusTarget = useRef<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  useLayoutEffect(() => { alive.current = true; return () => { alive.current = false } }, [])
  const isCurrent = () => alive.current && isCurrentSettingsFrame(scope, props.identity)
  const deleteProvider = useDeleteProvider({ captureResultGuard: () => isCurrent })
  const [deleteTarget, setDeleteTarget] = useState<CustomOAuthProvider | null>(
    null
  )

  const handleDelete = async () => {
    if (!deleteTarget || pending.current || !isCurrent()) return
    pending.current = true
    setError(null)
    try {
      const res = await deleteProvider.mutateAsync(deleteTarget.id)
      if (!isCurrent()) return
      if (res.success) setDeleteTarget(null)
      else setError(res.message || t('Failed to delete provider'))
    } catch (cause) {
      if (isCurrent()) setError(cause instanceof Error ? cause.message : t('Failed to delete provider'))
    } finally { pending.current = false }
  }

  return (
    <>
      <div className='flex items-center justify-between'>
        <p className='text-muted-foreground text-sm'>
          {t('Manage custom OAuth providers for user authentication')}
        </p>
        <Button size='sm' data-provider-create onClick={props.onCreate}>
          <Plus className='mr-1.5 h-4 w-4' />
          {t('Add Provider')}
        </Button>
      </div>

      <StaticDataTable
        data={props.providers}
        getRowKey={(provider) => provider.id}
        emptyClassName='text-sm'
        emptyContent={t('No custom OAuth providers configured yet.')}
        columns={[
          {
            id: 'icon',
            header: t('Icon'),
            cell: (provider) =>
              provider.icon ? (
                <span className='text-lg'>{provider.icon}</span>
              ) : (
                <span className='text-muted-foreground text-sm'>--</span>
              ),
          },
          {
            id: 'name',
            header: t('Name'),
            cellClassName: 'font-medium whitespace-normal break-all',
            cell: (provider) => provider.name,
          },
          {
            id: 'slug',
            header: t('Slug'),
            cell: (provider) => (
              <BadgeCell>
                <StatusBadge
                  label={provider.slug}
                  variant='neutral'
                  copyable={false}
                />
              </BadgeCell>
            ),
          },
          {
            id: 'status',
            header: t('Status'),
            cell: (provider) => (
              <BadgeCell>
                <StatusBadge
                  label={provider.enabled ? t('Enabled') : t('Disabled')}
                  variant={provider.enabled ? 'success' : 'neutral'}
                  copyable={false}
                />
              </BadgeCell>
            ),
          },
          {
            id: 'client-id',
            header: t('Client ID'),
            cellClassName:
              'text-muted-foreground whitespace-normal break-all font-mono',
            cell: (provider) => provider.client_id,
          },
          {
            id: 'actions',
            header: t('Actions'),
            className: 'text-right',
            cellClassName: 'text-right',
            cell: (provider) => (
              <span data-provider-actions={provider.id}><StaticRowActions
                editLabel={t('Edit')}
                deleteLabel={t('Delete')}
                menuLabel={t('Open menu')}
                onEdit={() => props.onEdit(provider)}
                onDelete={() => { focusTarget.current = provider.id; setError(null); setDeleteTarget(provider) }}
              /></span>
            ),
          },
        ]}
      />

      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(open) => { if (!open && !pending.current) setDeleteTarget(null) }}
        returnFocus={() => document.querySelector<HTMLElement>(`[data-provider-actions="${focusTarget.current}"] button[aria-haspopup="menu"]`) ?? document.querySelector<HTMLElement>('[data-provider-create]')}
        title={t('Delete Provider')}
        desc={t(
          'Are you sure you want to delete "{{name}}"? Users who authenticated with this provider will no longer be able to log in.',
          { name: deleteTarget?.name || '' }
        )}
        confirmText={t('Delete')}
        destructive
        handleConfirm={handleDelete}
        isLoading={deleteProvider.isPending}
      >{error && <p role='alert' className='text-destructive text-sm break-words'>{error}</p>}</ConfirmDialog>
    </>
  )
}
