import { Link, linkOptions, useLocation } from '@tanstack/react-router'
import { Database, FileText, Home, KeyRound, Settings } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { AnimatedOutlet } from '@/components/page-transition'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { SkipToMain } from '@/components/skip-to-main'
import { useSidebarConfig } from '@/hooks/use-sidebar-config'
import { hasPermission } from '@/lib/admin-permissions'
import { getBuildRevision } from '@/lib/build-metadata'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

const destinations = [
  {
    title: 'Overview',
    url: '/dashboard/overview',
    icon: Home,
    link: linkOptions({
      to: '/dashboard/$section',
      params: { section: 'overview' },
    }),
  },
  {
    title: 'Upstream accounts',
    url: '/upstream-accounts',
    icon: Database,
    link: linkOptions({ to: '/upstream-accounts' }),
  },
  {
    title: 'API Keys',
    url: '/keys',
    icon: KeyRound,
    link: linkOptions({ to: '/keys' }),
  },
  {
    title: 'Call records',
    url: '/usage-logs/common',
    icon: FileText,
    link: linkOptions({
      to: '/usage-logs/$section',
      params: { section: 'common' },
    }),
  },
] as const

export function PersonalConsoleLayout(props: { children?: ReactNode }) {
  const { t } = useTranslation()
  const path = useLocation({ select: (location) => location.pathname })
  const user = useAuthStore((state) => state.auth.user)
  const canReadChannels = Boolean(
    user && user.role >= ROLE.ADMIN && hasPermission(user, 'channel', 'read')
  )
  const navigation = useSidebarConfig([
    {
      title: 'Personal console',
      items: destinations
        .filter((item) => item.url !== '/upstream-accounts' || canReadChannels)
        .map((item) => ({ title: item.title, url: item.url, icon: item.icon })),
    },
  ])
  const visibleUrls = new Set(
    navigation.flatMap((group) =>
      group.items.flatMap((item) => ('url' in item ? [item.url] : []))
    )
  )
  const items = destinations.filter((item) => visibleUrls.has(item.url))
  const selected = (url: string) =>
    path === url ||
    (url === '/dashboard/overview' && path === '/dashboard') ||
    (url === '/usage-logs/common' && path.startsWith('/usage-logs'))

  return (
    <div className='personal-console' data-myapi-shell='personal'>
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
            <small>{t('Personal console')}</small>
          </span>
        </Link>
        <nav
          className='pc-desktop-nav'
          aria-label={t('Personal console navigation')}
        >
          {items.map((item) => (
            <Link
              key={item.url}
              {...item.link}
              className='pc-nav-link'
              aria-current={selected(item.url) ? 'page' : undefined}
            >
              <item.icon aria-hidden='true' />
              <span>{t(item.title)}</span>
            </Link>
          ))}
        </nav>
        <div className='pc-sidebar-bottom'>
          <Link
            to='/profile'
            className='pc-nav-link'
            aria-current={path === '/profile' ? 'page' : undefined}
          >
            <Settings aria-hidden='true' />
            <span>{t('Settings & profile')}</span>
          </Link>
          <details className='pc-maintenance'>
            <summary>{t('Maintenance tools')}</summary>
            {canReadChannels && (
              <Link to='/channels'>{t('Channel management')}</Link>
            )}
            {user?.role === ROLE.SUPER_ADMIN && (
              <Link to='/system-settings'>{t('System Settings')}</Link>
            )}
            {user?.role === ROLE.SUPER_ADMIN && (
              <Link to='/system-info'>{t('System information')}</Link>
            )}
          </details>
          <small className='pc-build' data-testid='console-build-revision'>
            {getBuildRevision()}
          </small>
        </div>
      </aside>
      <div className='pc-workspace'>
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
                {import.meta.env.VITE_REACT_APP_VERSION || getBuildRevision()}
              </small>
            </span>
          </Link>
          <ProfileDropdown />
        </header>
        <div className='pc-desktop-account'>
          <ProfileDropdown />
        </div>
        <div className='pc-route-content'>
          {props.children ?? <AnimatedOutlet />}
        </div>
        <nav
          className='pc-mobile-nav'
          aria-label={t('Mobile console navigation')}
        >
          {items.map((item) => (
            <Link
              key={item.url}
              {...item.link}
              aria-current={selected(item.url) ? 'page' : undefined}
            >
              <item.icon aria-hidden='true' />
              <span>{t(item.title)}</span>
            </Link>
          ))}
        </nav>
      </div>
    </div>
  )
}
