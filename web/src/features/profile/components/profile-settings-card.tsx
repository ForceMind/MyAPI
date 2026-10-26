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
import { Link2, Settings } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import type { UserProfile } from '../types'
import { AccountBindingsTab } from './tabs/account-bindings-tab'
import { NotificationTab } from './tabs/notification-tab'

// ============================================================================
// Profile Settings Card Component
// ============================================================================

interface ProfileSettingsCardProps {
  profile: UserProfile | null
  loading: boolean
  onProfileUpdate: () => void
  showBalanceThreshold?: boolean
  securityScopeKey?: string
}

export function ProfileSettingsCard({
  profile,
  loading,
  onProfileUpdate,
  showBalanceThreshold = true,
  securityScopeKey = 'profile-security',
}: ProfileSettingsCardProps) {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState('bindings')

  if (loading) {
    return (
      <Card data-card-hover='false'>
        <CardHeader>
          <Skeleton className='h-5 w-32' />
          <Skeleton className='h-4 w-48' />
        </CardHeader>
        <CardContent className='flex flex-col gap-4'>
          <Skeleton className='h-10 w-full' />
          {['bindings', 'preferences', 'notifications'].map((key) => (
            <Skeleton key={key} className='h-20 w-full' />
          ))}
        </CardContent>
      </Card>
    )
  }

  return (
    <Card data-card-hover='false'>
      <CardHeader>
        <CardTitle>
          <h2>{t('Settings')}</h2>
        </CardTitle>
        <CardDescription>
          {t('Configure your account preferences and integrations')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Tabs value={activeTab} onValueChange={setActiveTab}>
          <TabsList className='grid w-full grid-cols-2 items-stretch gap-1 rounded-xl p-1 group-data-horizontal/tabs:h-10'>
            <TabsTrigger
              value='bindings'
              className='h-full gap-2 rounded-lg px-3 py-0 leading-none'
            >
              <Link2 className='h-4 w-4' />
              <span className='hidden sm:inline'>{t('Account Bindings')}</span>
              <span className='sm:hidden'>{t('Bindings')}</span>
            </TabsTrigger>
            <TabsTrigger
              value='settings'
              className='h-full gap-2 rounded-lg px-3 py-0 leading-none'
            >
              <Settings className='h-4 w-4' />
              <span className='hidden sm:inline'>
                {t('Settings & Preferences')}
              </span>
              <span className='sm:hidden'>{t('Settings')}</span>
            </TabsTrigger>
          </TabsList>

          <TabsContent value='bindings' className='mt-4 sm:mt-6'>
            <AccountBindingsTab
              profile={profile}
              onUpdate={onProfileUpdate}
              securityScopeKey={securityScopeKey}
            />
          </TabsContent>

          <TabsContent value='settings' className='mt-4 sm:mt-6'>
            <NotificationTab
              profile={profile}
              onUpdate={onProfileUpdate}
              showBalanceThreshold={showBalanceThreshold}
            />
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  )
}
