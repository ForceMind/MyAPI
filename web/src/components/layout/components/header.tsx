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

import { SidebarTrigger, useSidebar } from '@/components/ui/sidebar'
import { cn } from '@/lib/utils'

type HeaderProps = React.HTMLAttributes<HTMLElement> & {
  contentClassName?: string
}

export function Header({
  className,
  children,
  contentClassName,
  ...props
}: HeaderProps) {
  const { t } = useTranslation()
  const { isMobile, open, openMobile } = useSidebar()
  return (
    <header
      data-myapi-header
      className={cn(
        'bg-card sticky top-0 z-40 h-[var(--app-header-height,3rem)] w-full shrink-0 border-b',
        className
      )}
      {...props}
    >
      <div
        className={cn(
          'flex h-full min-w-0 items-center gap-1.5 px-3 sm:gap-2 sm:px-4',
          contentClassName
        )}
      >
        <SidebarTrigger
          variant='ghost'
          className='col-start-1 row-start-1 size-9 shrink-0'
          aria-label={t('Toggle sidebar')}
          aria-expanded={isMobile ? openMobile : open}
        />
        {children}
      </div>
    </header>
  )
}
