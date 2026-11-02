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
import { Search } from 'lucide-react'
import { useLayoutEffect, useRef, useState } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'

import { useDiscoverEndpoints } from '../hooks/use-custom-oauth-mutations'
import type { CustomOAuthFormValues } from '../types'

type DiscoveryButtonProps = {
  form: UseFormReturn<CustomOAuthFormValues>
  isCurrent?: () => boolean
  disabled?: boolean
}

export function DiscoveryButton(props: DiscoveryButtonProps) {
  const { t } = useTranslation()
  const alive = useRef(true)
  const pending = useRef(false)
  const [error, setError] = useState<string | null>(null)
  useLayoutEffect(() => { alive.current = true; return () => { alive.current = false } }, [])
  const discover = useDiscoverEndpoints({ captureResultGuard: () => {
    const source = props.form.getValues('well_known')
    return () => alive.current && props.isCurrent?.() !== false && props.form.getValues('well_known') === source
  } })

  const handleDiscover = async () => {
    if (pending.current || props.disabled || props.isCurrent?.() === false) return
    const wellKnown = props.form.getValues('well_known')
    if (!wellKnown) {
      toast.error(t('Please enter a Well-Known URL first'))
      return
    }

    if (!wellKnown.startsWith('http://') && !wellKnown.startsWith('https://')) {
      toast.error(t('Well-Known URL must start with http:// or https://'))
      return
    }

    pending.current = true
    setError(null)
    const before = props.form.getValues()
    try {
    const res = await discover.mutateAsync(wellKnown)
    if (!alive.current || props.isCurrent?.() === false || props.form.getValues('well_known') !== wellKnown) return
    if (!res.success) { setError(res.message || t('Failed to discover OIDC endpoints')); return }
    if (res.data?.discovery) {
      const disc = res.data.discovery
      if (disc.authorization_endpoint && props.form.getValues('authorization_endpoint') === before.authorization_endpoint) {
        props.form.setValue(
          'authorization_endpoint',
          disc.authorization_endpoint,
          { shouldDirty: true }
        )
      }
      if (disc.token_endpoint && props.form.getValues('token_endpoint') === before.token_endpoint) {
        props.form.setValue('token_endpoint', disc.token_endpoint, {
          shouldDirty: true,
        })
      }
      if (disc.userinfo_endpoint && props.form.getValues('user_info_endpoint') === before.user_info_endpoint) {
        props.form.setValue('user_info_endpoint', disc.userinfo_endpoint, {
          shouldDirty: true,
        })
      }
      if (disc.scopes_supported && disc.scopes_supported.length > 0) {
        const currentScopes = props.form.getValues('scopes')
        if (!currentScopes) {
          const defaultScopes = disc.scopes_supported
            .filter((s) => ['openid', 'profile', 'email'].includes(s))
            .join(' ')
          if (defaultScopes) {
            props.form.setValue('scopes', defaultScopes, {
              shouldDirty: true,
            })
          }
        }
      }
    }
    } catch (cause) {
      if (alive.current && props.isCurrent?.() !== false) setError(cause instanceof Error ? cause.message : t('Failed to discover OIDC endpoints'))
    } finally { pending.current = false }
  }

  return (
    <div className='space-y-2'>
    <Button
      type='button'
      variant='outline'
      size='sm'
      onClick={handleDiscover}
      disabled={discover.isPending || props.disabled}
    >
      <Search className='mr-1.5 h-3.5 w-3.5' />
      {discover.isPending ? t('Discovering...') : t('Auto-discover')}
    </Button>
    {error && <p role='alert' className='text-destructive text-sm break-words'>{error}</p>}
    </div>
  )
}
