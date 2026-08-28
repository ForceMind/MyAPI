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

import { useTranslation } from 'react-i18next'

import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'

interface FullContentLogViewModeToggleProps {
  raw: boolean
  onRawChange: (raw: boolean) => void
}

export function FullContentLogViewModeToggle(
  props: FullContentLogViewModeToggleProps
) {
  const { t } = useTranslation()

  return (
    <div className='bg-background flex items-center gap-2 rounded-lg border px-3 py-2'>
      <span
        className={cn(
          'text-xs',
          props.raw ? 'text-muted-foreground' : 'font-medium'
        )}
      >
        {t('Clean text')}
      </span>
      <Switch
        checked={props.raw}
        aria-label={t('Show raw protocol data')}
        onCheckedChange={(checked) => props.onRawChange(Boolean(checked))}
      />
      <span
        className={cn(
          'text-xs',
          props.raw ? 'font-medium' : 'text-muted-foreground'
        )}
      >
        {t('Raw data')}
      </span>
    </div>
  )
}
