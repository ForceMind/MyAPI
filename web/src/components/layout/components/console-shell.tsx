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
import { useLocation } from '@tanstack/react-router'
import type { ReactNode } from 'react'

import { NavigationProgress } from '@/components/navigation-progress'
import { AnimatedOutlet } from '@/components/page-transition'
import { SkipToMain } from '@/components/skip-to-main'
import { useSidebar } from '@/components/ui/sidebar'
import { useLayout } from '@/context/layout-provider'
import { useMediaQuery } from '@/hooks/use-media-query'
import { useSidebarView } from '@/hooks/use-sidebar-view'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'

import { DesktopHeader, MobileHeader } from './console-header'
import { ConsoleSidebar } from './console-sidebar'
import { MobileBottomNav } from './mobile-bottom-nav'

/**
 * The single authenticated console shell. PC (>=1024px) and mobile are two
 * compositions around one route outlet: the outlet keeps its position in the
 * tree, so crossing the breakpoint never remounts the page or refetches.
 */
export function ConsoleShell(props: { children?: ReactNode }) {
  const href = useLocation({ select: (location) => location.href })
  const root = useSidebarView({ rootOnly: true })
  const context = useSidebarView()
  const mobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const { open, setOpen } = useSidebar()
  const { collapsible, variant } = useLayout()
  const collapsed = !open && collapsible !== 'none'

  return (
    <div
      className='personal-console'
      data-myapi-shell='authenticated'
      data-myapi-interface='platform'
      data-composition={mobile ? 'mobile' : 'desktop'}
      data-collapsed={collapsed}
      data-collapse-mode={collapsible}
      data-sidebar-variant={variant}
    >
      <SkipToMain />
      {!mobile && (
        <ConsoleSidebar
          href={href}
          collapsed={collapsed}
          root={root}
          context={context}
        />
      )}
      <div className='pc-workspace'>
        <NavigationProgress scope='workspace' />
        {mobile ? (
          <MobileHeader />
        ) : (
          <DesktopHeader
            onExpandSidebar={
              collapsed && collapsible === 'offcanvas'
                ? () => setOpen(true)
                : undefined
            }
          />
        )}
        <div className='pc-route-content'>
          {props.children ?? <AnimatedOutlet />}
        </div>
        {mobile && (
          <MobileBottomNav href={href} root={root} context={context} />
        )}
      </div>
    </div>
  )
}
