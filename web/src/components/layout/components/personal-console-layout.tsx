import { Link, useLocation } from '@tanstack/react-router'
import { Ellipsis, PanelLeftClose, PanelLeftOpen } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { AnimatedOutlet } from '@/components/page-transition'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { SkipToMain } from '@/components/skip-to-main'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  SheetTrigger,
} from '@/components/ui/sheet'
import { useSidebarView } from '@/hooks/use-sidebar-view'
import { getBuildRevision } from '@/lib/build-metadata'

import { checkIsActive } from '../lib/url-utils'
import type { NavGroup, NavLink } from '../types'
import { ChatPresetsItem } from './chat-presets-item'

function ModuleNavigation(props: {
  groups: NavGroup[]
  path: string
  onNavigate?: () => void
}) {
  return props.groups.map((group) => (
    <section
      key={group.id ?? group.title}
      className='pc-nav-group'
      aria-label={group.title}
    >
      <h2>{group.title}</h2>
      {group.items.map((item) => {
        if (item.type === 'chat-presets') {
          return <ChatPresetsItem key={item.title} item={item} />
        }
        const links = item.items ?? [item as NavLink]
        return links.map((link) => (
          <Link
            key={link.url}
            to={link.url}
            className='pc-nav-link'
            preload={false}
            aria-current={checkIsActive(props.path, link) ? 'page' : undefined}
            onClick={props.onNavigate}
            title={link.title}
          >
            {link.icon && <link.icon aria-hidden='true' />}
            <span>{link.title}</span>
          </Link>
        ))
      })}
    </section>
  ))
}

export function PersonalConsoleLayout(props: { children?: ReactNode }) {
  const { t } = useTranslation()
  const path = useLocation({ select: (location) => location.pathname })
  const { navGroups } = useSidebarView({ rootOnly: true })
  const [moreOpen, setMoreOpen] = useState(false)
  const [collapsed, setCollapsed] = useState(false)
  const primaryUrls = [
    '/dashboard/overview',
    '/upstream-accounts',
    '/keys',
    '/usage-logs/common',
  ]
  const items = navGroups
    .flatMap((group) => group.items)
    .filter((item): item is NavLink => Boolean(item.url))
  const primary = primaryUrls.flatMap((url) =>
    items.filter((item) => item.url === url)
  )
  const active = items.find((item) => checkIsActive(path, item))
  const version = import.meta.env.VITE_REACT_APP_VERSION || getBuildRevision()

  return (
    <div
      className='personal-console'
      data-myapi-shell='personal'
      data-collapsed={collapsed}
    >
      <SkipToMain />
      <aside className='pc-sidebar'>
        <Link
          className='pc-brand'
          to='/dashboard/$section'
          params={{ section: 'overview' }}
          aria-label={t('Overview')}
        >
          <span className='pc-logo-tile'>
            <img src='/myapi-logo-v1.png' alt='' />
          </span>
          <span>
            <strong>MyAPI</strong>
            <small>{t('Console')}</small>
          </span>
        </Link>
        <nav
          className='pc-desktop-nav'
          aria-label={t('Personal console navigation')}
        >
          <ModuleNavigation groups={navGroups} path={path} />
        </nav>
        <div className='pc-sidebar-bottom'>
          <Button
            variant='ghost'
            size='sm'
            className='pc-collapse'
            onClick={() => setCollapsed(!collapsed)}
            aria-label={collapsed ? t('Expand sidebar') : t('Collapse sidebar')}
          >
            {collapsed ? <PanelLeftOpen /> : <PanelLeftClose />}
            <span>{t('Collapse sidebar')}</span>
          </Button>
          <small
            className='pc-build'
            data-testid='console-build-revision'
            title={getBuildRevision()}
          >
            {version}
          </small>
        </div>
      </aside>
      <div className='pc-workspace'>
        <header className='pc-desktop-header'>
          <span className='text-muted-foreground text-sm'>
            MyAPI{' '}
            <span aria-hidden='true' className='px-2'>
              /
            </span>{' '}
            <span className='text-foreground font-medium'>
              {active?.title ?? t('Console')}
            </span>
          </span>
          <ProfileDropdown />
        </header>
        <header className='pc-mobile-header'>
          <Link
            to='/dashboard/$section'
            params={{ section: 'overview' }}
            className='pc-brand'
          >
            <img src='/myapi-logo-v1.png' alt='' />
            <span>
              <strong>MyAPI</strong>
              <small
                className='pc-mobile-version'
                data-testid='console-mobile-build-version'
                title={getBuildRevision()}
              >
                {version}
              </small>
            </span>
          </Link>
          <ProfileDropdown />
        </header>
        <div className='pc-route-content'>
          {props.children ?? <AnimatedOutlet />}
        </div>
        <nav
          className='pc-mobile-nav'
          aria-label={t('Mobile console navigation')}
        >
          {primary.map((item) => (
            <Link
              key={item.url}
              to={item.url}
              preload={false}
              aria-current={checkIsActive(path, item) ? 'page' : undefined}
            >
              {item.icon && <item.icon aria-hidden='true' />}
              <span>{item.title}</span>
            </Link>
          ))}
          <Sheet open={moreOpen} onOpenChange={setMoreOpen}>
            <SheetTrigger
              className='pc-more'
              aria-current={
                !primary.some((item) => checkIsActive(path, item))
                  ? 'page'
                  : undefined
              }
            >
              <Ellipsis aria-hidden='true' />
              <span>{t('More')}</span>
            </SheetTrigger>
            <SheetContent
              side='bottom'
              className='pc-more-sheet max-h-[85dvh] overflow-y-auto rounded-t-2xl'
            >
              <SheetHeader>
                <SheetTitle>{t('All modules')}</SheetTitle>
                <SheetDescription>{t('Console')}</SheetDescription>
              </SheetHeader>
              <nav
                className='pc-more-modules px-4 pb-6'
                aria-label={t('All modules')}
              >
                <ModuleNavigation
                  groups={navGroups}
                  path={path}
                  onNavigate={() => setMoreOpen(false)}
                />
              </nav>
            </SheetContent>
          </Sheet>
        </nav>
      </div>
    </div>
  )
}
