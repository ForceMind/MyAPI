/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Link } from '@tanstack/react-router'
import { ChevronRight, Clock3, CloudOff, ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuthStore } from '@/stores/auth-store'

import type { UserProfile } from '../types'
import { LanguagePreferencesCard } from './language-preferences-card'
import { LoginSessionsCard } from './login-sessions-card'
import { PasskeyCard } from './passkey-card'
import { ProfileSecurityCard } from './profile-security-card'
import { ProfileSettingsCard } from './profile-settings-card'
import { TwoFACard } from './two-fa-card'

type PersonalProfileProps = {
  profile: UserProfile | null
  loading: boolean
  error: boolean
  onRetry: () => void
  onRefresh: () => void
}

export function PersonalProfile(props: PersonalProfileProps) {
  const { t } = useTranslation()
  const authUserId = useAuthStore((state) => state.auth.user?.id)
  const authRole = useAuthStore((state) => state.auth.user?.role)
  const sessionSid = useAuthStore((state) => state.auth.session?.sid)
  const securityScopeKey = `${authUserId ?? props.profile?.id ?? 'anonymous'}:${authRole ?? props.profile?.role ?? 'no-role'}:${
    sessionSid ?? 'no-session'
  }`
  const [openSecurityScope, setOpenSecurityScope] = useState<string | null>(
    null
  )
  const securityOpen = openSecurityScope === securityScopeKey
  const name = props.profile?.display_name || props.profile?.username || ''

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Profile')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('Manage your personal information and preferences.')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <div className='personal-profile mx-auto w-full max-w-6xl space-y-5'>
          {props.error && !props.loading && (
            <Card className='border-border bg-card min-h-[420px] border shadow-none ring-0 sm:min-h-[520px]'>
              <CardContent
                className='flex flex-1 flex-col items-center justify-center gap-5 px-5 py-12 text-center'
                role='alert'
              >
                <CloudOff
                  aria-hidden='true'
                  className='mb-3 size-16 text-slate-400'
                />
                <h2 className='text-xl font-semibold sm:text-2xl'>
                  {t('Profile is temporarily unavailable')}
                </h2>
                <p className='text-muted-foreground'>
                  {t('The connection failed or timed out. Please retry.')}
                </p>
                <div className='mt-3 flex w-full max-w-sm flex-col gap-3'>
                  <Button onClick={props.onRetry} className='h-11'>
                    {t('Reload profile')}
                  </Button>
                  <Button
                    variant='outline'
                    className='h-11'
                    render={
                      <Link
                        to='/dashboard/$section'
                        params={{ section: 'overview' }}
                      />
                    }
                    nativeButton={false}
                  >
                    {t('Back to overview')}
                  </Button>
                </div>
                <p className='text-muted-foreground mt-2 text-sm'>
                  {t('Other pages are still available.')}
                </p>
              </CardContent>
            </Card>
          )}
          {props.loading && (
            <div
              role='status'
              aria-label={t('Loading profile')}
              className='grid gap-5 md:grid-cols-[1.4fr_1fr]'
            >
              <Skeleton className='h-80 rounded-xl' />
              <Skeleton className='h-80 rounded-xl' />
              <span className='sr-only'>{t('Loading profile')}</span>
            </div>
          )}
          {!props.loading && !props.error && props.profile && (
            <>
              <div className='grid gap-5 lg:grid-cols-[1.45fr_1fr]'>
                <Card className='border-border bg-card min-w-0 border py-6 shadow-none ring-0'>
                  <CardHeader className='px-5 sm:px-6'>
                    <CardTitle>
                      <h2 className='text-lg'>{t('Identity')}</h2>
                    </CardTitle>
                  </CardHeader>
                  <CardContent className='px-5 sm:px-6'>
                    <div className='mb-6 flex min-w-0 items-center gap-4'>
                      <div
                        aria-hidden='true'
                        className='bg-accent text-primary flex size-16 shrink-0 items-center justify-center rounded-full text-2xl font-semibold sm:size-20'
                      >
                        {name.slice(0, 1).toUpperCase()}
                      </div>
                      <div className='min-w-0 space-y-2'>
                        <div className='flex flex-wrap items-center gap-2'>
                          <h3 className='text-lg font-semibold break-words'>
                            {name}
                          </h3>
                          <Badge
                            variant='secondary'
                            className='h-auto max-w-full text-left break-words whitespace-normal'
                          >
                            {t('Console maintainer')}
                          </Badge>
                        </div>
                        <p className='text-muted-foreground text-sm leading-6'>
                          {t(
                            'Only you manage the console. Others use API Keys.'
                          )}
                        </p>
                      </div>
                    </div>
                    <Separator />
                    <dl className='text-sm sm:text-base'>
                      <div className='grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-4 py-5'>
                        <dt>{t('Display Name')}</dt>
                        <dd className='text-muted-foreground break-words'>
                          {name}
                        </dd>
                      </div>
                      <Separator />
                      <div className='grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-4 pt-5'>
                        <dt>{t('Management scope')}</dt>
                        <dd className='text-muted-foreground'>
                          {t('Only me')}
                        </dd>
                      </div>
                    </dl>
                  </CardContent>
                </Card>
                <Card className='border-border bg-card min-w-0 border py-6 shadow-none ring-0'>
                  <CardHeader className='px-5 sm:px-6'>
                    <CardTitle>
                      <h2 className='text-lg'>{t('Interface preferences')}</h2>
                    </CardTitle>
                  </CardHeader>
                  <CardContent className='px-5 sm:px-6'>
                    <Separator />
                    <LanguagePreferencesCard
                      profile={props.profile}
                      onProfileUpdate={props.onRefresh}
                      compact
                    />
                    <Separator />
                    <div className='flex flex-wrap items-center justify-between gap-3 py-5 text-sm'>
                      <span className='flex items-center gap-3'>
                        <Clock3
                          aria-hidden='true'
                          className='text-primary size-5'
                        />
                        {t('Time')}
                      </span>
                      <span className='text-muted-foreground'>
                        {t('Local time')}
                      </span>
                    </div>
                  </CardContent>
                </Card>
              </div>
              <Card className='border-border bg-card min-w-0 border py-6 shadow-none ring-0'>
                <CardHeader className='px-5 sm:px-6'>
                  <CardTitle>
                    <h2 className='text-lg'>{t('Login and security')}</h2>
                  </CardTitle>
                </CardHeader>
                <CardContent className='px-5 sm:px-6'>
                  <Separator />
                  <button
                    type='button'
                    aria-expanded={securityOpen}
                    aria-controls='personal-security-settings'
                    onClick={() =>
                      setOpenSecurityScope(
                        securityOpen ? null : securityScopeKey
                      )
                    }
                    className='focus-visible:outline-primary flex w-full items-center gap-4 rounded-md pt-5 text-left focus-visible:outline-2 focus-visible:outline-offset-4'
                  >
                    <ShieldCheck
                      aria-hidden='true'
                      className='text-primary size-9 shrink-0'
                    />
                    <span className='min-w-0 flex-1'>
                      <span className='block font-medium'>
                        {t('Manage login methods')}
                      </span>
                      <span className='text-muted-foreground mt-1 block text-sm'>
                        {t(
                          'Configure your login methods and keep your account secure.'
                        )}
                      </span>
                    </span>
                    <ChevronRight
                      aria-hidden='true'
                      className='text-muted-foreground size-5 shrink-0'
                    />
                  </button>
                </CardContent>
              </Card>
              {securityOpen && (
                <div
                  key={securityScopeKey}
                  id='personal-security-settings'
                  className='space-y-5'
                >
                  <ProfileSettingsCard
                    showBalanceThreshold={false}
                    profile={props.profile}
                    loading={false}
                    onProfileUpdate={props.onRefresh}
                    securityScopeKey={securityScopeKey}
                  />
                  <ProfileSecurityCard
                    profile={props.profile}
                    loading={false}
                    securityScopeKey={securityScopeKey}
                  />
                  <div className='grid items-start gap-5 lg:grid-cols-2'>
                    <PasskeyCard
                      loading={false}
                      securityScopeKey={securityScopeKey}
                    />
                    <TwoFACard
                      loading={false}
                      securityScopeKey={securityScopeKey}
                    />
                  </div>
                  <LoginSessionsCard securityScopeKey={securityScopeKey} />
                </div>
              )}
            </>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
