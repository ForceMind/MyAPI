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
import { Loading03Icon, SentIcon, Shield01Icon, UserIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { IconGithub, IconLinuxDo, IconWeChat } from '@/assets/brand-icons'

import { AuthLayout } from '../auth-layout'

type OAuthCallbackScreenProps = {
  provider: string
  mode: 'login' | 'bind'
}

type ProviderMeta = {
  label: string
  icon: ReactNode
}

const providerDictionary: Record<string, ProviderMeta> = {
  github: {
    label: 'GitHub',
    icon: <IconGithub className='size-7' focusable='false' />,
  },
  oidc: { label: 'OIDC', icon: <HugeiconsIcon icon={Shield01Icon} className='size-7' aria-hidden='true' /> },
  linuxdo: {
    label: 'LinuxDO',
    icon: <IconLinuxDo className='size-7' focusable='false' />,
  },
  telegram: { label: 'Telegram', icon: <HugeiconsIcon icon={SentIcon} className='size-7' aria-hidden='true' /> },
  wechat: {
    label: 'WeChat',
    icon: <IconWeChat className='size-7' focusable='false' />,
  },
}

export function OAuthCallbackScreen({
  provider,
  mode,
}: OAuthCallbackScreenProps) {
  const { t } = useTranslation()
  const { label, icon } = useMemo(() => {
    const normalized = provider?.toLowerCase() ?? ''
    return (
      providerDictionary[normalized] || {
        label: 'account',
        icon: <HugeiconsIcon icon={UserIcon} className='size-7' aria-hidden='true' />,
      }
    )
  }, [provider])

  const providerLabel = t(label)
  const isBindMode = mode === 'bind'

  const headline = isBindMode
    ? t('Binding your {{provider}} account', { provider: providerLabel })
    : t('Signing you in with {{provider}}', { provider: providerLabel })

  const description = isBindMode
    ? t('Hang tight while we securely link this account to your profile.')
    : t('Hang tight while we finish connecting your account.')

  const secondaryNote = isBindMode
    ? t(
        'You can close this tab once the binding completes or a success message appears in the original window.'
      )
    : t(
        "You'll be redirected automatically. You can return to the previous page if nothing happens after a few seconds."
      )

  return (
    <AuthLayout>
      <div className='w-full space-y-8'>
        <div className='flex flex-col items-center space-y-4 text-center'>
          <div className='bg-muted flex size-14 items-center justify-center rounded-lg'>
            {icon}
          </div>
          <div className='space-y-2'>
            <h2 className='text-2xl leading-8 font-semibold tracking-tight wrap-break-word'>
              {headline}
            </h2>
            <p className='text-muted-foreground text-sm leading-relaxed'>
              {description}
            </p>
          </div>
        </div>

        <div className='space-y-4 text-center'>
          <div role='status' aria-live='polite' className='flex items-center justify-center gap-2 text-sm font-medium'>
            <HugeiconsIcon icon={Loading03Icon} aria-hidden='true' className='h-4 w-4 animate-spin' />
            <span>{t('Processing OAuth response...')}</span>
          </div>
          <p className='text-muted-foreground text-sm'>{secondaryNote}</p>
          <p className='text-muted-foreground text-xs'>
            {t(
              'This may take a few moments while we validate the request and update your session.'
            )}
          </p>
        </div>
      </div>
    </AuthLayout>
  )
}
