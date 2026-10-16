/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useTranslation } from 'react-i18next'

import { toIntlLocale } from '@/i18n/languages'

export function WindowPoints(props: { value: number | null }) {
  const { t, i18n } = useTranslation()
  if (props.value === null) return <>{t('Unknown')}</>
  return (
    <>
      {t('{{value}} percentage points', {
        value: new Intl.NumberFormat(toIntlLocale(i18n.language), {
          maximumFractionDigits: 6,
        }).format(props.value / 1000000),
      })}
    </>
  )
}
