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
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'

import { checkIsActive } from '../lib/url-utils'
import type { NavGroup, NavLink } from '../types'
import { ChatPresetLinks } from './chat-presets-item'

type ConsoleNavListProps = {
  groups: NavGroup[]
  href: string
  onNavigate?: () => void
}

/** Grouped console destinations of the mobile More sheet. */
export function ConsoleNavList(props: ConsoleNavListProps) {
  return props.groups.map((group) => {
    const key = group.id ?? group.title

    return (
      <section
        key={key}
        className='pc-nav-group'
        aria-label={group.title}
        role='group'
        data-nav-group={group.id}
      >
        {!group.titleHidden && <h2>{group.title}</h2>}
        <div className='pc-nav-items'>
          {group.items.map((item) => {
            if (item.type === 'chat-presets') {
              return (
                <ChatPresetLinks
                  key={item.title}
                  item={item}
                  onNavigate={props.onNavigate}
                />
              )
            }
            const links = item.items ?? [item as NavLink]
            return links.map((link) => (
              <Link
                key={String(link.url)}
                to={link.url}
                className='pc-nav-link'
                preload={false}
                aria-current={
                  checkIsActive(props.href, link) ? 'page' : undefined
                }
                onClick={props.onNavigate}
                title={link.title}
              >
                {link.icon && (
                  <HugeiconsIcon icon={link.icon} aria-hidden='true' />
                )}
                <span>{link.title}</span>
              </Link>
            ))
          })}
        </div>
      </section>
    )
  })
}
