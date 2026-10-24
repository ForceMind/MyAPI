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
import { useState } from 'react'

import { SidebarProvider } from '@/components/ui/sidebar'
import { LayoutProvider } from '@/context/layout-provider'
import { SearchProvider } from '@/context/search-provider'
import { getCookie } from '@/lib/cookies'

import { ConsoleShell } from './console-shell'

type AuthenticatedLayoutProps = {
  children?: React.ReactNode
}

/**
 * A saved `sidebar_state` wins; otherwise the sidebar starts as an icon rail
 * between 1024 and 1279px and expanded from 1280px (design 5.1).
 */
function getInitialSidebarOpen() {
  const saved = getCookie('sidebar_state')
  if (saved === 'true' || saved === 'false') return saved === 'true'
  return window.matchMedia('(min-width: 1280px)').matches
}

export function AuthenticatedLayout(props: AuthenticatedLayoutProps) {
  const [defaultOpen] = useState(getInitialSidebarOpen)
  return (
    <LayoutProvider>
      <SearchProvider>
        <SidebarProvider defaultOpen={defaultOpen} className='contents'>
          <ConsoleShell>{props.children}</ConsoleShell>
        </SidebarProvider>
      </SearchProvider>
    </LayoutProvider>
  )
}
