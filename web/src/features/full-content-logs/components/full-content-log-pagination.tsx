/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { useTranslation } from 'react-i18next'

import { LogPagination, type LogPaginationProps } from '@/components/log-pagination'

export function FullContentLogPagination(props: Omit<LogPaginationProps, 'label' | 'requestTotals'>) {
  const { t } = useTranslation()
  return <LogPagination {...props} label={t('Full content log pagination')} requestTotals />
}
