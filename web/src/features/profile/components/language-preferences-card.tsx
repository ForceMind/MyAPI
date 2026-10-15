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
import { Languages, Loader2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldLabel } from '@/components/ui/field'
import { TitledCard } from '@/components/ui/titled-card'
import {
  AVAILABLE_INTERFACE_LANGUAGE_OPTIONS,
  normalizeInterfaceLanguage,
} from '@/i18n/languages'
import { useAuthStore } from '@/stores/auth-store'

import { updateUserLanguage } from '../api'
import { parseUserSettings } from '../lib'
import type { UserProfile } from '../types'

type LanguagePreferencesCardProps = {
  profile: UserProfile | null
  onProfileUpdate: () => void
  compact?: boolean
}

export function LanguagePreferencesCard(props: LanguagePreferencesCardProps) {
  const { t, i18n } = useTranslation()
  const { auth } = useAuthStore()
  const [saving, setSaving] = useState(false)

  const savedLanguage = useMemo(() => {
    const settings = parseUserSettings(props.profile?.setting)
    return normalizeInterfaceLanguage(settings.language || i18n.language)
  }, [props.profile?.setting, i18n.language])

  const [currentLanguage, setCurrentLanguage] = useState(savedLanguage)

  useEffect(() => {
    setCurrentLanguage(savedLanguage)
  }, [savedLanguage])

  const handleLanguageChange = async (language: string | null) => {
    if (!language) return
    const nextLanguage = normalizeInterfaceLanguage(language)
    if (nextLanguage === currentLanguage) return

    const previousLanguage = currentLanguage
    setCurrentLanguage(nextLanguage)
    setSaving(true)
    await i18n.changeLanguage(nextLanguage)

    try {
      const response = await updateUserLanguage(nextLanguage)
      if (!response.success) {
        throw new Error(response.message || t('Failed to update settings'))
      }

      if (auth.user) {
        const existingSetting =
          typeof auth.user.setting === 'string'
            ? parseUserSettings(auth.user.setting)
            : (auth.user.setting ?? {})
        auth.setUser({
          ...auth.user,
          setting: JSON.stringify({
            ...existingSetting,
            language: nextLanguage,
          }),
        })
      }

      props.onProfileUpdate()
      toast.success(t('Language preference saved'))
    } catch {
      setCurrentLanguage(previousLanguage)
      await i18n.changeLanguage(previousLanguage)
      toast.error(t('Failed to update settings'))
    } finally {
      setSaving(false)
    }
  }

  const languageSelect = (
    <div className='flex min-w-0 items-center gap-2'>
      <Select
        items={AVAILABLE_INTERFACE_LANGUAGE_OPTIONS.map((language) => ({ value: language.code, label: language.label }))}
        value={currentLanguage}
        onValueChange={handleLanguageChange}
        disabled={saving}
      >
        <SelectTrigger id='profile-interface-language' aria-label={t('Interface Language')} className='w-full min-w-0 sm:w-48'>
          <SelectValue placeholder={t('Select language')} />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {AVAILABLE_INTERFACE_LANGUAGE_OPTIONS.map((language) => (
              <SelectItem key={language.code} value={language.code}>{language.label}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      {saving && <Loader2 aria-label={t('Saving...')} className='size-4 shrink-0 animate-spin' />}
    </div>
  )

  if (props.compact) {
    return (
      <Field orientation='horizontal' className='flex-wrap justify-between gap-3 py-4'>
        <FieldLabel htmlFor='profile-interface-language' className='flex shrink-0 items-center gap-3'>
          <Languages aria-hidden='true' className='size-5 text-primary' />
          {t('Language')}
        </FieldLabel>
        {languageSelect}
      </Field>
    )
  }

  return (
    <TitledCard
      title={t('Language Preferences')}
      description={t('Set the language used across the interface')}
      icon={<Languages className='h-4 w-4' />}
      iconTone='chart-4'
      disableHoverEffect
    >
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4'>
        <div className='space-y-1'>
          <div className='text-sm font-medium'>{t('Interface Language')}</div>
          <p className='text-muted-foreground line-clamp-2 text-xs sm:text-sm'>
            {t(
              'Language preferences sync across your signed-in devices and affect API error messages.'
            )}
          </p>
        </div>
        {languageSelect}
      </div>
    </TitledCard>
  )
}
