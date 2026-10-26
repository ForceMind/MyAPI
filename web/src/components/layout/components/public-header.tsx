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
import { Link, useNavigate, useRouterState } from '@tanstack/react-router'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { LanguageSwitcher } from '@/components/language-switcher'
import { NotificationPopover } from '@/components/notification-popover'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { ThemeSwitch } from '@/components/theme-switch'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useNotifications } from '@/hooks/use-notifications'
import { useSystemConfig } from '@/hooks/use-system-config'
import { useTopNavLinks } from '@/hooks/use-top-nav-links'
import { resolveBrandLogo, resolveBrandName } from '@/lib/build-branding'
import { DEFAULT_LOGO, DEFAULT_SYSTEM_NAME } from '@/lib/constants'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import {
  defaultTopNavLinks,
  withTopNavLinkKeys,
} from '../config/top-nav.config'
import type { TopNavLink } from '../types'
import { HeaderLogo } from './header-logo'

const AUTH_PROMPT_SECONDS = 5

type AuthPromptTarget = {
  title: string
  href: string
}

export interface PublicHeaderProps {
  navLinks?: TopNavLink[]
  mobileLinks?: TopNavLink[]
  navContent?: React.ReactNode
  showThemeSwitch?: boolean
  showLanguageSwitcher?: boolean
  logo?: React.ReactNode
  siteName?: string
  homeUrl?: string
  leftContent?: React.ReactNode
  rightContent?: React.ReactNode
  showNavigation?: boolean
  showAuthButtons?: boolean
  showNotifications?: boolean
  className?: string
}

export function PublicHeader(props: PublicHeaderProps) {
  const {
    navLinks = defaultTopNavLinks,
    showThemeSwitch = true,
    showLanguageSwitcher = true,
    logo: customLogo,
    siteName: customSiteName,
    homeUrl = '/',
    showAuthButtons = true,
    showNotifications = true,
  } = props

  const { t } = useTranslation()
  const navigate = useNavigate()
  const [mobileOpen, setMobileOpen] = useState(false)
  const [authPromptTarget, setAuthPromptTarget] =
    useState<AuthPromptTarget | null>(null)
  const [authPromptSecondsLeft, setAuthPromptSecondsLeft] =
    useState(AUTH_PROMPT_SECONDS)
  const { auth } = useAuthStore()
  const {
    systemName,
    logo: systemLogo,
    loading,
    logoLoaded,
  } = useSystemConfig()
  const dynamicLinks = useTopNavLinks()
  const notifications = useNotifications()
  const routerState = useRouterState()
  const pathname = routerState.location.pathname

  const user = auth.user
  const isAuthenticated = !!user
  const displaySiteName = resolveBrandName(
    customSiteName || systemName,
    DEFAULT_SYSTEM_NAME
  )
  const displayLogo = resolveBrandLogo(systemLogo, DEFAULT_LOGO)
  const links = dynamicLinks.length > 0 ? dynamicLinks : navLinks
  const keyedLinks = withTopNavLinkKeys(links)
  let logoContent: React.ReactNode
  if (loading) {
    logoContent = <Skeleton className='size-full rounded-lg' />
  } else if (customLogo) {
    logoContent = customLogo
  } else {
    logoContent = (
      <HeaderLogo
        src={displayLogo}
        loading={loading}
        logoLoaded={logoLoaded}
        className='size-full rounded-lg object-contain'
      />
    )
  }
  let desktopAuthContent: React.ReactNode = null
  if (showAuthButtons) {
    if (loading) {
      desktopAuthContent = <Skeleton className='h-8 w-20 rounded-lg' />
    } else if (isAuthenticated) {
      desktopAuthContent = <ProfileDropdown />
    } else {
      desktopAuthContent = (
        <Button size='sm' render={<Link to='/sign-in' />}>
          {t('Sign in')}
        </Button>
      )
    }
  }

  useEffect(() => {
    if (!mobileOpen) return
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMobileOpen(false)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('keydown', onKeyDown)
      document.body.style.overflow = previousOverflow
    }
  }, [mobileOpen])

  useEffect(() => {
    if (!authPromptTarget) return

    const intervalId = window.setInterval(() => {
      setAuthPromptSecondsLeft((seconds) => Math.max(seconds - 1, 0))
    }, 1000)

    const timeoutId = window.setTimeout(() => {
      const redirect = authPromptTarget.href
      setAuthPromptTarget(null)
      navigate({ to: '/sign-in', search: { redirect } })
    }, AUTH_PROMPT_SECONDS * 1000)

    return () => {
      window.clearInterval(intervalId)
      window.clearTimeout(timeoutId)
    }
  }, [authPromptTarget, navigate])

  const closeAuthPrompt = useCallback(() => {
    setAuthPromptTarget(null)
    setAuthPromptSecondsLeft(AUTH_PROMPT_SECONDS)
  }, [])

  const navigateToSignIn = useCallback(() => {
    const redirect = authPromptTarget?.href || '/'
    setAuthPromptTarget(null)
    navigate({ to: '/sign-in', search: { redirect } })
  }, [authPromptTarget?.href, navigate])

  const handleNavLinkClick = useCallback(
    (
      event: React.MouseEvent<HTMLAnchorElement>,
      link: TopNavLink,
      closeMobile = false
    ) => {
      if (link.disabled) {
        event.preventDefault()
        return
      }

      if (link.requiresAuth) {
        event.preventDefault()
        if (closeMobile) {
          setMobileOpen(false)
        }
        setAuthPromptSecondsLeft(AUTH_PROMPT_SECONDS)
        setAuthPromptTarget({
          title: t(link.title),
          href: link.href,
        })
        return
      }

      if (closeMobile) {
        setMobileOpen(false)
      }
    },
    [t]
  )

  return (
    <>
      {/* The console's 56px top bar (design direction 3): brand, the public
          links with the primary underline on the current one, then
          language, theme, notifications and the account. It stays in the
          page flow, so content never slides under it. */}
      <header className='bg-background sticky top-0 z-50 border-b'>
        <div className='mx-auto flex h-14 w-full max-w-[1200px] min-w-0 items-center gap-5 px-4 lg:px-6'>
          <Link
            to={homeUrl}
            className='flex min-w-0 items-center gap-2.5'
          >
            <div className='flex size-7 shrink-0 items-center justify-center'>
              {logoContent}
            </div>
            {loading ? (
              <Skeleton className='h-4 w-16' />
            ) : (
              <span className='truncate text-base leading-6 font-bold'>
                {displaySiteName}
              </span>
            )}
          </Link>

          {/* Desktop links */}
          <nav className='hidden self-stretch lg:flex'>
            {keyedLinks.map((link) => {
              const isActive = pathname === link.href
              if (link.external) {
                return (
                  <a
                    key={link.key}
                    href={link.href}
                    target='_blank'
                    rel='noopener noreferrer'
                    aria-disabled={link.disabled}
                    tabIndex={link.disabled ? -1 : undefined}
                    onClick={(event) => handleNavLinkClick(event, link)}
                    className={cn(
                      'text-muted-foreground hover:text-foreground -mb-px flex items-center border-b-2 border-transparent px-3 text-sm font-medium whitespace-nowrap transition-colors',
                      link.disabled && 'pointer-events-none opacity-50'
                    )}
                  >
                    {t(link.title)}
                  </a>
                )
              }
              return (
                <Link
                  key={link.key}
                  to={link.href}
                  disabled={link.disabled}
                  onClick={(event) => handleNavLinkClick(event, link)}
                  aria-current={isActive ? 'page' : undefined}
                  className={cn(
                    '-mb-px flex items-center border-b-2 px-3 text-sm font-medium whitespace-nowrap transition-colors',
                    isActive
                      ? 'border-primary text-foreground'
                      : 'text-muted-foreground hover:text-foreground border-transparent',
                    link.disabled && 'pointer-events-none opacity-50'
                  )}
                >
                  {t(link.title)}
                </Link>
              )
            })}
          </nav>

          {/* Desktop actions */}
          <div className='ms-auto hidden min-w-0 items-center gap-1 lg:flex'>
            {showLanguageSwitcher && <LanguageSwitcher />}
            {showThemeSwitch && <ThemeSwitch />}
            {showNotifications && (
              <NotificationPopover
                open={notifications.popoverOpen}
                onOpenChange={notifications.setPopoverOpen}
                unreadCount={notifications.unreadCount}
                activeTab={notifications.activeTab}
                onTabChange={notifications.setActiveTab}
                notice={notifications.notice}
                announcements={notifications.announcements}
                loading={notifications.loading}
              />
            )}
            {desktopAuthContent ? (
              <div className='ms-2 flex items-center'>{desktopAuthContent}</div>
            ) : null}
          </div>

          {/* Mobile: compact actions + hamburger */}
          <div className='ms-auto flex items-center gap-1 lg:hidden'>
            {showThemeSwitch && <ThemeSwitch />}
            {showAuthButtons && !loading && isAuthenticated && (
              <ProfileDropdown />
            )}
            <Button
              type='button'
              variant='ghost'
              size='icon'
              className='size-11'
              onClick={() => setMobileOpen((v) => !v)}
              aria-label={t('Toggle navigation menu')}
              aria-controls='public-mobile-navigation'
              aria-expanded={mobileOpen}
            >
              <div className='relative size-4'>
                <span
                  className={cn(
                    'absolute inset-x-0 block h-[1.5px] origin-center rounded-full bg-current transition-all duration-300',
                    mobileOpen ? 'top-[7px] rotate-45' : 'top-[3px]'
                  )}
                />
                <span
                  className={cn(
                    'absolute inset-x-0 top-[7px] block h-[1.5px] rounded-full bg-current transition-all duration-300',
                    mobileOpen ? 'scale-x-0 opacity-0' : 'opacity-100'
                  )}
                />
                <span
                  className={cn(
                    'absolute inset-x-0 block h-[1.5px] origin-center rounded-full bg-current transition-all duration-300',
                    mobileOpen ? 'top-[7px] -rotate-45' : 'top-[11px]'
                  )}
                />
              </div>
            </Button>
          </div>
        </div>
      </header>

      {/* Mobile full-screen overlay */}
      <div
        id='public-mobile-navigation'
        aria-hidden={!mobileOpen}
        inert={!mobileOpen}
        className={cn(
          'bg-background/98 fixed inset-0 z-40 backdrop-blur-2xl transition-all duration-500 ease-[cubic-bezier(0.16,1,0.3,1)] lg:pointer-events-none lg:hidden',
          mobileOpen
            ? 'pointer-events-auto opacity-100'
            : 'pointer-events-none opacity-0'
        )}
      >
        <div className='flex h-full flex-col justify-between px-8 pt-20 pb-10'>
          <nav className='flex flex-col gap-1'>
            {keyedLinks.map((link, index) => {
              const isActive = pathname === link.href
              const linkClassName = cn(
                'flex items-center gap-3 py-3 text-base font-medium tracking-tight transition-all duration-500 ease-[cubic-bezier(0.16,1,0.3,1)]',
                mobileOpen
                  ? 'translate-y-0 opacity-100'
                  : 'translate-y-4 opacity-0',
                isActive ? 'text-foreground' : 'text-muted-foreground',
                link.disabled && 'pointer-events-none opacity-50'
              )
              const transitionStyle = {
                transitionDelay: mobileOpen ? `${100 + index * 50}ms` : '0ms',
              }
              if (link.external) {
                return (
                  <a
                    key={link.key}
                    href={link.href}
                    target='_blank'
                    rel='noopener noreferrer'
                    aria-disabled={link.disabled}
                    tabIndex={link.disabled ? -1 : undefined}
                    onClick={(event) => handleNavLinkClick(event, link, true)}
                    className={linkClassName}
                    style={transitionStyle}
                  >
                    {t(link.title)}
                  </a>
                )
              }
              return (
                <Link
                  key={link.key}
                  to={link.href}
                  disabled={link.disabled}
                  onClick={(event) => handleNavLinkClick(event, link, true)}
                  className={linkClassName}
                  style={transitionStyle}
                >
                  {t(link.title)}
                </Link>
              )
            })}
          </nav>

          <div
            className={cn(
              'flex flex-col gap-3 transition-all duration-500',
              mobileOpen
                ? 'translate-y-0 opacity-100'
                : 'translate-y-4 opacity-0'
            )}
            style={{ transitionDelay: mobileOpen ? '250ms' : '0ms' }}
          >
            {showLanguageSwitcher && (
              <div className='flex items-center justify-between'>
                <span className='text-muted-foreground text-sm'>
                  {t('Change language')}
                </span>
                <LanguageSwitcher />
              </div>
            )}
            {showAuthButtons && (
              <Link
                to={isAuthenticated ? '/dashboard' : '/sign-in'}
                onClick={() => setMobileOpen(false)}
                className='bg-primary text-primary-foreground inline-flex h-11 items-center justify-center rounded-lg text-sm font-medium transition-opacity hover:opacity-90 active:opacity-80'
              >
                {isAuthenticated ? t('Go to Dashboard') : t('Sign in')}
              </Link>
            )}
          </div>
        </div>
      </div>

      <Dialog
        open={!!authPromptTarget}
        onOpenChange={(open) => {
          if (!open) {
            closeAuthPrompt()
          }
        }}
        title={t('Sign in required')}
        description={t('Please sign in to view {{module}}.', {
          module: authPromptTarget?.title || '',
        })}
        contentClassName='sm:max-w-md'
        contentHeight='auto'
        footer={
          <>
            <Button variant='outline' onClick={closeAuthPrompt}>
              {t('Cancel')}
            </Button>
            <Button onClick={navigateToSignIn}>{t('Sign in now')}</Button>
          </>
        }
      >
        <div className='bg-muted/40 text-muted-foreground rounded-lg px-3 py-2 text-sm'>
          {t('Redirecting to sign in in {{seconds}} seconds.', {
            seconds: authPromptSecondsLeft,
          })}
        </div>
      </Dialog>
    </>
  )
}
