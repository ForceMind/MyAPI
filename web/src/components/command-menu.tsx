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
import {
  ArrowRight01Icon,
  ComputerIcon,
  FileSearchIcon,
  Moon02Icon,
  Sun03Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useNavigate } from '@tanstack/react-router'
import React from 'react'
import { useTranslation } from 'react-i18next'

import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'
import { useSearch } from '@/context/search-context'
import { useTheme } from '@/context/theme-provider'
import { useSidebarView } from '@/hooks/use-sidebar-view'

import { ScrollArea } from './ui/scroll-area'

const CALL_RECORDS_URL = '/usage-logs/common'

/**
 * A pasted Request ID is one token of at least eight ID characters with a
 * digit, hyphen or underscore, so page names such as "Settings" never match.
 */
const REQUEST_ID_PATTERN = /^(?=.*[\d_-])[A-Za-z0-9][\w.:-]{7,127}$/

export function CommandMenu() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { setTheme } = useTheme()
  const { open, setOpen } = useSearch()
  const root = useSidebarView({ rootOnly: true })
  const context = useSidebarView()
  const [search, setSearch] = React.useState('')

  const navGroups = context.view
    ? [...context.navGroups, ...root.navGroups]
    : root.navGroups
  const canOpenCallRecords = root.navGroups.some((group) =>
    group.items.some((item) => item.url === CALL_RECORDS_URL)
  )
  const requestId = search.trim()
  const offerRequest = canOpenCallRecords && REQUEST_ID_PATTERN.test(requestId)

  const runCommand = React.useCallback(
    (command: () => unknown) => {
      setOpen(false)
      setSearch('')
      command()
    },
    [setOpen]
  )
  const groupKeyCounts = new Map<string, number>()

  return (
    <CommandDialog
      modal
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) setSearch('')
      }}
      title={t('Search')}
      description={t('Search pages or paste a Request ID')}
    >
      <Command>
        <CommandInput
          aria-label={t('Search')}
          placeholder={t('Search pages or paste a Request ID')}
          value={search}
          onValueChange={setSearch}
        />
        <CommandList>
          <ScrollArea className='h-72 pe-1'>
            {/* cmdk does not count force-mounted items, so the lookup hides it. */}
            {!offerRequest && (
              <CommandEmpty>{t('No results found.')}</CommandEmpty>
            )}
            {offerRequest && (
              <CommandGroup heading={t('Request ID')} forceMount>
                <CommandItem
                  value={`request-id:${requestId}`}
                  forceMount
                  onSelect={() =>
                    runCommand(() =>
                      navigate({
                        to: '/usage-logs/$section',
                        params: { section: 'common' },
                        // Opens the request's details directly; the call
                        // records behind it stay filtered to the same ID.
                        search: { requestId, inspect: 'request' },
                      })
                    )
                  }
                >
                  <HugeiconsIcon icon={FileSearchIcon} aria-hidden='true' />
                  <span className='truncate'>
                    {t('Open request {{requestId}}', { requestId })}
                  </span>
                </CommandItem>
              </CommandGroup>
            )}
            {navGroups.map((group) => {
              const groupIdentity = group.id
                ? `id:${group.id}`
                : `title:${group.title}`
              const groupOccurrence = groupKeyCounts.get(groupIdentity) ?? 0
              groupKeyCounts.set(groupIdentity, groupOccurrence + 1)
              const commandKeyCounts = new Map<string, number>()

              return (
                <CommandGroup
                  key={`${groupIdentity}:${groupOccurrence}`}
                  heading={group.title}
                >
                  {group.items.map((navItem) => {
                    if (navItem.url) {
                      const commandIdentity = `url:${navItem.url}`
                      const commandOccurrence =
                        commandKeyCounts.get(commandIdentity) ?? 0
                      commandKeyCounts.set(
                        commandIdentity,
                        commandOccurrence + 1
                      )

                      return (
                        <CommandItem
                          key={`${commandIdentity}:${commandOccurrence}`}
                          value={navItem.title}
                          onSelect={() => {
                            runCommand(() => navigate({ to: navItem.url }))
                          }}
                        >
                          <HugeiconsIcon
                            icon={ArrowRight01Icon}
                            aria-hidden='true'
                            className='text-muted-foreground'
                          />
                          {navItem.title}
                        </CommandItem>
                      )
                    }

                    return navItem.items?.map((subItem) => {
                      const commandIdentity = `url:${subItem.url}`
                      const commandOccurrence =
                        commandKeyCounts.get(commandIdentity) ?? 0
                      commandKeyCounts.set(
                        commandIdentity,
                        commandOccurrence + 1
                      )

                      return (
                        <CommandItem
                          key={`${commandIdentity}:${commandOccurrence}`}
                          value={`${navItem.title}-${subItem.url}`}
                          keywords={[subItem.title]}
                          onSelect={() => {
                            runCommand(() => navigate({ to: subItem.url }))
                          }}
                        >
                          <HugeiconsIcon
                            icon={ArrowRight01Icon}
                            aria-hidden='true'
                            className='text-muted-foreground'
                          />
                          {navItem.title}
                          <span aria-hidden='true'>/</span>
                          {subItem.title}
                        </CommandItem>
                      )
                    })
                  })}
                </CommandGroup>
              )
            })}
            <CommandGroup heading={t('Account')}>
              {/* Profile lives in the account menu, outside the sidebar. */}
              <CommandItem
                value={t('Profile')}
                onSelect={() => runCommand(() => navigate({ to: '/profile' }))}
              >
                <HugeiconsIcon
                  icon={ArrowRight01Icon}
                  aria-hidden='true'
                  className='text-muted-foreground'
                />
                {t('Profile')}
              </CommandItem>
            </CommandGroup>
            <CommandSeparator />
            <CommandGroup heading={t('Theme')}>
              <CommandItem onSelect={() => runCommand(() => setTheme('light'))}>
                <HugeiconsIcon icon={Sun03Icon} aria-hidden='true' />
                <span>{t('Light')}</span>
              </CommandItem>
              <CommandItem onSelect={() => runCommand(() => setTheme('dark'))}>
                <HugeiconsIcon icon={Moon02Icon} aria-hidden='true' />
                <span>{t('Dark')}</span>
              </CommandItem>
              <CommandItem
                onSelect={() => runCommand(() => setTheme('system'))}
              >
                <HugeiconsIcon icon={ComputerIcon} aria-hidden='true' />
                <span>{t('System')}</span>
              </CommandItem>
            </CommandGroup>
          </ScrollArea>
        </CommandList>
      </Command>
    </CommandDialog>
  )
}
