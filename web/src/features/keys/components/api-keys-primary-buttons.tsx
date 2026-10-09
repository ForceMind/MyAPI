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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { UserUsagePolicyDialog } from '@/features/users/components/user-usage-policy-dialog'
import { useAuthStore } from '@/stores/auth-store'

import { useApiKeys } from './api-keys-provider'

export function ApiKeysPrimaryButtons() {
  const actor = useAuthStore((state) => state.auth.user)
  return (
    <ApiKeysPrimaryButtonSession
      key={`${actor?.id}:${actor?.role}`}
      userId={actor?.id}
    />
  )
}

function ApiKeysPrimaryButtonSession(props: { userId?: number }) {
  const [policyOpen, setPolicyOpen] = useState(false)
  const { t } = useTranslation()
  const { setOpen } = useApiKeys()
  return (
    <>
      <div className='flex min-w-0 flex-wrap gap-2'>
        {props.userId && (
          <Button
            variant='outline'
            size='sm'
            className='h-auto min-h-8 whitespace-normal pointer-coarse:min-h-11'
            onClick={() => setPolicyOpen(true)}
          >
            {t('My usage policy')}
          </Button>
        )}
        <Button size='sm' onClick={() => setOpen('create')}>
          <Plus className='h-4 w-4' />
          {t('Create API Key')}
        </Button>
      </div>
      {policyOpen && props.userId && (
        <UserUsagePolicyDialog
          userId={props.userId}
          onClose={() => setPolicyOpen(false)}
        />
      )}
    </>
  )
}
