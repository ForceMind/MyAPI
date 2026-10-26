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

/**
 * Sign-in, sign-up and recovery pages (design direction 3): the console's
 * 56px brand bar over a light surface, and the task in one centred card of
 * the console card radius. Phones keep the same single column.
 */
export function AuthLayout({ children }: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()

  return (
    <div className='myapi-auth-surface flex min-h-svh min-w-0 flex-col'>
      <header className='myapi-auth-bar'>
        <div className='mx-auto flex min-h-14 w-full max-w-[1200px] min-w-0 items-center px-4 py-2 lg:px-6'>
          <Link
            to='/'
            className='focus-visible:ring-ring inline-flex max-w-full min-w-0 items-center gap-2.5 rounded-lg outline-none hover:opacity-80 focus-visible:ring-2 focus-visible:ring-offset-4'
          >
            <div className='relative size-7 shrink-0'>
              {loading ? (
                <Skeleton className='absolute inset-0 rounded-lg' />
              ) : (
                <img
                  src={logo}
                  alt={t('Logo')}
                  className='size-7 rounded-lg object-contain'
                />
              )}
            </div>
            {loading ? (
              <Skeleton className='h-5 w-24' />
            ) : (
              <h1 className='min-w-0 text-base leading-6 font-bold wrap-anywhere'>
                {systemName}
              </h1>
            )}
          </Link>
        </div>
      </header>
      <main
        id='content'
        tabIndex={-1}
        className='flex w-full min-w-0 flex-1 flex-col justify-center px-4 py-8 outline-none sm:py-12'
      >
        <div className='myapi-auth-panel mx-auto flex w-full max-w-md min-w-0 flex-col gap-2 p-5 sm:p-8'>
          {children}
        </div>
      </main>
    </div>
  )
}
