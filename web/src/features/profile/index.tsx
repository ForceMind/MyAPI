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
import { Link } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import {
  CardStaggerContainer,
  CardStaggerItem,
} from '@/components/page-transition'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { useFundingPresentation } from '@/hooks/use-funding-presentation'
import { useConfirmedFundingMode } from '@/hooks/use-personal-workbench-mode'
import { useStatus } from '@/hooks/use-status'
import { useAuthStore } from '@/stores/auth-store'

import { CheckinCalendarCard } from './components/checkin-calendar-card'
import { LanguagePreferencesCard } from './components/language-preferences-card'
import { LoginSessionsCard } from './components/login-sessions-card'
import { PasskeyCard } from './components/passkey-card'
import { PersonalProfile } from './components/personal-profile'
import { ProfileHeader } from './components/profile-header'
import { ProfileSecurityCard } from './components/profile-security-card'
import { ProfileSettingsCard } from './components/profile-settings-card'
import { SidebarModulesCard } from './components/sidebar-modules-card'
import { TwoFACard } from './components/two-fa-card'
import { useProfile } from './hooks'

export function Profile() {
  const { t } = useTranslation()
  const { status, confirmed: statusConfirmed, error: statusError } = useStatus()
  const [profileReadEnabled, setProfileReadEnabled] = useState(
    statusConfirmed || Boolean(statusError)
  )
  // Status observers mounted by loaded settings may retry a failed status
  // query. That background retry must not restart an admitted profile read.
  useEffect(() => {
    if (statusConfirmed || statusError) setProfileReadEnabled(true)
  }, [statusConfirmed, statusError])
  const authUserId = useAuthStore((s) => s.auth.user?.id)
  const authRole = useAuthStore((s) => s.auth.user?.role)
  const sessionSid = useAuthStore((s) => s.auth.session?.sid)
  const profileIdentityKey = `${authUserId ?? 'anonymous'}:${sessionSid ?? 'no-session'}`
  const { profile, loading, error, fetchProfile, refreshProfile } = useProfile(
    profileReadEnabled,
    profileIdentityKey
  )
  const { commercialEnabled } = useFundingPresentation()
  const fundingMode = useConfirmedFundingMode()
  const personalMode = fundingMode === 'disabled'
  const permissions = useAuthStore((s) => s.auth.user?.permissions)
  const securityScopeKey = `${authUserId ?? profile?.id ?? 'anonymous'}:${authRole ?? profile?.role ?? 'no-role'}:${
    sessionSid ?? 'no-session'
  }`

  const showCommercialFeatures = commercialEnabled && fundingMode === 'enabled'
  const checkinEnabled =
    showCommercialFeatures && status?.checkin_enabled === true
  const turnstileEnabled = !!(
    status?.turnstile_check && status?.turnstile_site_key
  )
  const turnstileSiteKey = status?.turnstile_site_key || ''
  const canConfigureSidebar = permissions?.sidebar_settings !== false

  if (personalMode) {
    return (
      <PersonalProfile
        profile={profile}
        loading={loading}
        error={error}
        onRetry={() => void fetchProfile()}
        onRefresh={() => void refreshProfile()}
      />
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Profile')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('Personal settings and profile management.')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        {error && !loading && (
          <ErrorState
            title={t('Failed to load profile')}
            onRetry={() => void fetchProfile()}
          />
        )}
        {(!error || loading) && (
          <CardStaggerContainer className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-6'>
            <CardStaggerItem>
              <ProfileHeader
                profile={profile}
                loading={loading}
                commercialEnabled={showCommercialFeatures}
              />
            </CardStaggerItem>

            <CardStaggerItem>
              <div className='grid gap-4 sm:gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(360px,0.46fr)] xl:items-start'>
                <div className='min-w-0 space-y-4 sm:space-y-6'>
                  <ProfileSettingsCard
                    key={`settings:${securityScopeKey}`}
                    profile={profile}
                    loading={loading}
                    onProfileUpdate={refreshProfile}
                    securityScopeKey={securityScopeKey}
                    showBalanceThreshold={
                      fundingMode === 'enabled' || fundingMode === 'retirement'
                    }
                  />
                  <Card>
                    <CardHeader>
                      <CardTitle>{t('Prompt learning workspace')}</CardTitle>
                      <CardDescription>
                        {t(
                          'Review authorized learning settings and instruction versions.'
                        )}
                      </CardDescription>
                    </CardHeader>
                    <CardContent>
                      <Button
                        render={<Link to='/prompt-learning' />}
                        nativeButton={false}
                        variant='outline'
                      >
                        {t('Open prompt learning')}
                      </Button>
                    </CardContent>
                  </Card>
                  <LanguagePreferencesCard
                    profile={profile}
                    onProfileUpdate={refreshProfile}
                  />
                  <ProfileSecurityCard
                    key={`access:${securityScopeKey}`}
                    profile={profile}
                    loading={loading}
                    securityScopeKey={securityScopeKey}
                  />
                  <LoginSessionsCard
                    key={`sessions:${securityScopeKey}`}
                    securityScopeKey={securityScopeKey}
                  />
                </div>

                <div className='min-w-0 space-y-4 sm:space-y-6'>
                  {checkinEnabled && (
                    <CheckinCalendarCard
                      checkinEnabled={checkinEnabled}
                      turnstileEnabled={turnstileEnabled}
                      turnstileSiteKey={turnstileSiteKey}
                    />
                  )}
                  {canConfigureSidebar && !loading && profile && (
                    <SidebarModulesCard
                      sidebarModules={profile.sidebar_modules}
                    />
                  )}
                  <PasskeyCard
                    key={`passkey:${securityScopeKey}`}
                    loading={loading}
                    securityScopeKey={securityScopeKey}
                  />
                  <TwoFACard
                    key={`two-fa:${securityScopeKey}`}
                    loading={loading}
                    securityScopeKey={securityScopeKey}
                  />
                </div>
              </div>
            </CardStaggerItem>
          </CardStaggerContainer>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
