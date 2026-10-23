import { Link, useLocation } from '@tanstack/react-router'
import {
  ArrowLeft,
  Ellipsis,
  PanelLeftClose,
  PanelLeftOpen,
} from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { NavigationProgress } from '@/components/navigation-progress'
import { AnimatedOutlet } from '@/components/page-transition'
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
import { useSidebar } from '@/components/ui/sidebar'
import { useLayout } from '@/context/layout-provider'
import { useMediaQuery } from '@/hooks/use-media-query'
import { useSidebarView } from '@/hooks/use-sidebar-view'
import { getBuildRevision } from '@/lib/build-metadata'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'

import { checkIsActive } from '../lib/url-utils'
import type { NavGroup, NavLink } from '../types'
import { AppBreadcrumb } from './app-breadcrumb'
import { ChatPresetsItem } from './chat-presets-item'
import { ConsoleHeaderActions } from './console-header-actions'
import { NavGroup as SettingsNavGroup } from './nav-group'

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
      role='group'
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
  const context = useSidebarView()
  const mobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const href = useLocation({ select: (location) => location.href })
  const [moreOpen, setMoreOpen] = useState(false)
  const { open, setOpen } = useSidebar()
  const { collapsible, variant } = useLayout()
  const collapsed = !open && collapsible !== 'none'
  useEffect(() => setMoreOpen(false), [href])
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
  const version = import.meta.env.VITE_REACT_APP_VERSION || getBuildRevision()

  return (
    <div
      className='personal-console'
      data-myapi-shell='authenticated'
      data-myapi-interface='platform'
      data-collapsed={collapsed}
      data-collapse-mode={collapsible}
      data-sidebar-variant={variant}
    >
      <SkipToMain />
      <aside
        className='pc-sidebar group'
        data-collapsible={collapsed ? collapsible : ''}
      >
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
          data-myapi-sidebar
          tabIndex={0}
        >
          {context.view ? (
            <>
              <Link to={context.view.parent.to} className='pc-nav-link'>
                <ArrowLeft aria-hidden='true' />
                <span>{t(context.view.parent.label)}</span>
              </Link>
              {context.navGroups.map((group) => (
                <SettingsNavGroup key={group.id ?? group.title} {...group} />
              ))}
            </>
          ) : (
            <ModuleNavigation groups={navGroups} path={path} />
          )}
        </nav>
        <div className='pc-sidebar-bottom'>
          <Button
            variant='ghost'
            size='sm'
            className='pc-collapse'
            onClick={() => setOpen(!open)}
            disabled={collapsible === 'none'}
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
        <NavigationProgress scope='workspace' />
        <header className='pc-header' data-myapi-header>
          {!mobile && collapsed && collapsible === 'offcanvas' && (
            <Button
              variant='ghost'
              size='icon'
              aria-label={t('Expand sidebar')}
              onClick={() => setOpen(true)}
            >
              <PanelLeftOpen />
            </Button>
          )}
          {mobile ? (
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
          ) : (
            <div className='min-w-0 flex-1' data-myapi-context>
              <AppBreadcrumb />
            </div>
          )}
          <ConsoleHeaderActions mobile={mobile} />
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
                <SheetDescription className='sr-only'>
                  {t('Console')}
                </SheetDescription>
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
                {context.view && (
                  <>
                    <Link
                      to={context.view.parent.to}
                      className='pc-nav-link'
                      onClick={() => setMoreOpen(false)}
                    >
                      <ArrowLeft aria-hidden='true' />
                      <span>{t(context.view.parent.label)}</span>
                    </Link>
                    <ModuleNavigation
                      groups={context.navGroups}
                      path={path}
                      onNavigate={() => setMoreOpen(false)}
                    />
                  </>
                )}
              </nav>
            </SheetContent>
          </Sheet>
        </nav>
      </div>
    </div>
  )
}
