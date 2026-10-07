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

import { Skeleton } from '@/components/ui/skeleton'
import { useSystemConfig } from '@/hooks/use-system-config'

type AuthLayoutProps = {
  children: React.ReactNode
}

export function AuthLayout({ children }: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()

  return (
    <div className='myapi-auth-surface flex min-h-svh min-w-0 flex-col px-4 py-4 sm:px-8 sm:py-6'>
      <header className='mx-auto w-full max-w-7xl'>
        <Link
          to='/'
          className='focus-visible:ring-ring inline-flex max-w-full items-center gap-3 rounded-lg outline-none hover:opacity-80 focus-visible:ring-2 focus-visible:ring-offset-4'
        >
          <div className='relative size-9 shrink-0'>
            {loading ? (
              <Skeleton className='absolute inset-0 rounded-lg' />
            ) : (
              <img
                src={logo}
                alt={t('Logo')}
                className='size-9 rounded-lg object-contain'
              />
            )}
          </div>
          {loading ? (
            <Skeleton className='h-6 w-24' />
          ) : (
            <h1 className='min-w-0 text-lg font-semibold tracking-tight wrap-anywhere'>
              {systemName}
            </h1>
          )}
        </Link>
      </header>
      <main
        id='content'
        tabIndex={-1}
        className='flex w-full min-w-0 flex-1 flex-col justify-center py-8 outline-none sm:py-12'
      >
        <div className='myapi-auth-panel mx-auto w-full max-w-md min-w-0 space-y-2 p-5 sm:p-8'>
          {children}
        </div>
      </main>
    </div>
  )
}
