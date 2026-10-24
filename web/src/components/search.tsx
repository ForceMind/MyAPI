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
import { Search01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { useSearch } from '@/context/search-provider'

import { Button } from './ui/button'
import { Kbd } from './ui/kbd'

type SearchProps = {
  /** `bar` shows the hint and shortcut (PC); `icon` is a 44px touch target. */
  variant?: 'bar' | 'icon'
}

const isApplePlatform =
  typeof navigator !== 'undefined' &&
  /Mac|iPhone|iPad/.test(navigator.userAgent)

/** Opens the global command search (`Ctrl/⌘ K`). */
export function Search(props: SearchProps) {
  const { t } = useTranslation()
  const { setOpen } = useSearch()

  if (props.variant === 'icon') {
    return (
      <Button
        variant='ghost'
        size='icon'
        className='pc-search-icon'
        onClick={() => setOpen(true)}
        aria-label={t('Search')}
      >
        <HugeiconsIcon icon={Search01Icon} aria-hidden='true' />
      </Button>
    )
  }

  return (
    <Button
      variant='outline'
      className='pc-search-bar'
      onClick={() => setOpen(true)}
      aria-label={t('Search')}
      aria-keyshortcuts={isApplePlatform ? 'Meta+K' : 'Control+K'}
    >
      <HugeiconsIcon
        icon={Search01Icon}
        data-icon='inline-start'
        aria-hidden='true'
      />
      <span className='pc-search-hint'>
        {t('Search pages or paste a Request ID')}
      </span>
      <Kbd aria-hidden='true'>{isApplePlatform ? '⌘ K' : 'Ctrl K'}</Kbd>
    </Button>
  )
}
