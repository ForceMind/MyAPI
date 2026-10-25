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
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import {
  CardStaggerContainer,
  CardStaggerItem,
} from '@/components/page-transition'
import { buttonVariants } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { useFundingPresentation } from '@/hooks/use-funding-presentation'
import { useConfirmedFundingMode } from '@/hooks/use-personal-workbench-mode'
import { useProductEdition } from '@/hooks/use-product-edition'
import { useStatus } from '@/hooks/use-status'
import { useAuthStore } from '@/stores/auth-store'

import { CheckinCalendarCard } from './components/checkin-calendar-card'
import { LanguagePreferencesCard } from './components/language-preferences-card'
import { LoginSessionsCard } from './components/login-sessions-card'
import { MySubscriptionCard } from './components/my-subscription-card'
import { PasskeyCard } from './components/passkey-card'
import { ProfileHeader } from './components/profile-header'
import { ProfileSecurityCard } from './components/profile-security-card'
import { ProfileSettingsCard } from './components/profile-settings-card'
import { SidebarModulesCard } from './components/sidebar-modules-card'
import { TwoFACard } from './components/two-fa-card'
import { useProfile } from './hooks'

export function Profile() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const authUserId = useAuthStore((s) => s.auth.user?.id)
  const authRole = useAuthStore((s) => s.auth.user?.role)
  const sessionSid = useAuthStore((s) => s.auth.session?.sid)
  const profileIdentityKey = `${authUserId ?? 'anonymous'}:${authRole ?? 'no-role'}:${sessionSid ?? 'no-session'}`
  const { profile, loading, error, fetchProfile, refreshProfile } = useProfile(
    true,
    profileIdentityKey
  )
  const { commercialEnabled } = useFundingPresentation()
  const edition = useProductEdition()
  const fundingMode = useConfirmedFundingMode()
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
          <CardStaggerContainer className='flex w-full min-w-0 flex-col gap-5'>
            <CardStaggerItem>
              <ProfileHeader
                profile={profile}
                loading={loading}
                commercialEnabled={showCommercialFeatures}
              />
            </CardStaggerItem>

            <CardStaggerItem>
              <div className='grid gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(360px,0.46fr)] xl:items-start'>
                <div className='min-w-0 space-y-5'>
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
                      <Link
                        to='/prompt-learning'
                        className={buttonVariants({ variant: 'outline' })}
                      >
                        {t('Open prompt learning')}
                      </Link>
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

                <div className='min-w-0 space-y-5'>
                  <MySubscriptionCard
                    key={`subscriptions:${securityScopeKey}`}
                    canManage={
                      edition.capabilities.commercial_subscriptions &&
                      showCommercialFeatures
                    }
                  />
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
