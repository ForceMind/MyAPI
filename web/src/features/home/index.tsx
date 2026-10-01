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
import {
  Activity,
  ArrowRight,
  ArrowUpRight,
  ChartNoAxesCombined,
  KeyRound,
  Layers3,
  Server,
  Wallet,
} from 'lucide-react'
import { useCallback, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { RichContent } from '@/components/rich-content'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useTheme } from '@/context/theme-provider'
import { useStatus } from '@/hooks/use-status'
import { hasPermission } from '@/lib/admin-permissions'
import { resolveDocsLink } from '@/lib/build-branding'
import { isLikelyHtml } from '@/lib/content-format'
import { useAuthStore } from '@/stores/auth-store'

import { useHomePageContent } from './hooks'

export function Home() {
  const { i18n, t } = useTranslation()
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const { resolvedTheme } = useTheme()
  const { auth } = useAuthStore()
  const isAuthenticated = !!auth.user
  const canReadChannels = hasPermission(auth.user, 'channel', 'read')
  const ordinaryUser = isAuthenticated && !canReadChannels
  const { content, isLoaded, isUrl, failed, retrying, retry } =
    useHomePageContent()
  const { status } = useStatus()
  const docsLink = resolveDocsLink(status?.docs_link)

  const syncIframePreferences = useCallback(() => {
    try {
      iframeRef.current?.contentWindow?.postMessage(
        { themeMode: resolvedTheme },
        '*'
      )
      iframeRef.current?.contentWindow?.postMessage(
        { lang: i18n.language },
        '*'
      )
    } catch {
      // Cross-origin frames may reject access while navigating.
    }
  }, [i18n.language, resolvedTheme])

  useEffect(() => {
    if (isUrl) {
      syncIframePreferences()
    }
  }, [isUrl, syncIframePreferences])

  if (!isLoaded) {
    return (
      <PublicLayout showMainContainer={false}>
        <main className='flex min-h-screen items-center justify-center'>
          <div className='text-muted-foreground'>{t('Loading...')}</div>
        </main>
      </PublicLayout>
    )
  }

  if (content) {
    if (isUrl) {
      return (
        <PublicLayout showMainContainer={false}>
          {/*
            allow-top-navigation-by-user-activation: the custom home page URL is
            admin-configured (trusted); this lets its target="_top" nav/menu links
            navigate the top-level window on user click. The default sandbox blocks
            this on desktop, while some mobile browsers allow it via allow-popups,
            causing inconsistent behavior. This token only permits user-activated
            top-level navigation and does NOT grant same-origin access.
          */}
          <iframe
            ref={iframeRef}
            src={content}
            className='h-screen w-full border-none'
            title={t('Custom Home Page')}
            sandbox='allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts allow-top-navigation-by-user-activation'
            onLoad={syncIframePreferences}
          />
        </PublicLayout>
      )
    }

    const contentIsHtml = isLikelyHtml(content)

    if (contentIsHtml) {
      return (
        <PublicLayout showMainContainer={false}>
          <RichContent
            mode='html'
            htmlVariant='isolated'
            content={content}
            className='custom-home-content'
          />
        </PublicLayout>
      )
    }

    return (
      <PublicLayout>
        <div className='mx-auto max-w-6xl px-4 py-8'>
          <RichContent
            mode='markdown'
            content={content}
            className='custom-home-content'
          />
        </div>
      </PublicLayout>
    )
  }

  const steps = ordinaryUser
    ? ([
        {
          icon: KeyRound,
          title: t('API Keys'),
          description: t('Create a key for your app or service'),
          to: '/keys',
          available: true,
        },
        {
          icon: Activity,
          title: t('Usage logs'),
          description: t(
            'Recent requests and errors without message contents.'
          ),
          to: '/usage-logs',
          available: true,
        },
        {
          icon: Wallet,
          title: t('Quota & history'),
          description: t('Review remaining credit and past transactions.'),
          to: '/wallet',
          available: true,
        },
      ] as const)
    : ([
        {
          icon: Layers3,
          title: t('Channels'),
          description: t('Connect provider accounts and review their status.'),
          to: '/channels',
          available: canReadChannels,
        },
        {
          icon: KeyRound,
          title: t('API Keys'),
          description: t('Create a key for your app or service'),
          to: '/keys',
          available: isAuthenticated,
        },
        {
          icon: ChartNoAxesCombined,
          title: t('Remaining quota'),
          description: t(
            'Compare remaining quota across channels in one chart.'
          ),
          to: '/dashboard',
          available: canReadChannels,
        },
      ] as const)

  return (
    <PublicLayout showMainContainer={false}>
      <main className='relative overflow-hidden px-5 py-16 sm:px-8 sm:py-24'>
        <div
          className='pointer-events-none absolute inset-x-0 top-0 h-[32rem] bg-[radial-gradient(ellipse_at_20%_0%,color-mix(in_oklch,var(--chart-1)_13%,transparent),transparent_60%)]'
          aria-hidden='true'
        />
        <div className='relative mx-auto max-w-6xl'>
          {failed && (
            <Alert
              aria-label={t('Failed to load home page content')}
              className='mb-8'
            >
              <AlertTitle>{t('Failed to load home page content')}</AlertTitle>
              <AlertDescription>
                {t(
                  'Showing the default home because live settings are unavailable.'
                )}
              </AlertDescription>
              <Button
                type='button'
                size='sm'
                variant='outline'
                className='mt-2 w-fit'
                disabled={retrying}
                onClick={retry}
              >
                {retrying ? t('Loading...') : t('Retry')}
              </Button>
            </Alert>
          )}
          <div className='text-muted-foreground mb-6 flex items-center gap-2 text-xs font-medium tracking-wider uppercase'>
            <span
              className='bg-primary size-2 rounded-full'
              aria-hidden='true'
            />
            My API · {t('Self-hosted gateway')}
          </div>
          <div
            className={
              ordinaryUser
                ? 'grid items-end gap-8'
                : 'grid items-end gap-8 lg:grid-cols-[minmax(0,1fr)_17rem]'
            }
          >
            <div>
              <h1 className='max-w-3xl text-4xl leading-tight font-semibold tracking-tight sm:text-5xl'>
                {t('Your AI gateway, in one place')}
              </h1>
              <p className='text-muted-foreground mt-5 max-w-2xl text-base leading-relaxed'>
                {ordinaryUser
                  ? t(
                      'Use your API key, review requests, and track remaining credit.'
                    )
                  : t(
                      'Connect providers, route requests through one API, and compare remaining quota across accounts.'
                    )}
              </p>
              <div className='mt-8 flex flex-wrap gap-3'>
                <Button
                  size='lg'
                  render={
                    <Link to={isAuthenticated ? '/dashboard' : '/sign-in'} />
                  }
                >
                  {isAuthenticated ? t('Go to Dashboard') : t('Sign in')}
                  <ArrowRight data-icon='inline-end' aria-hidden='true' />
                </Button>
                <Button
                  variant='outline'
                  size='lg'
                  render={
                    docsLink.external ? (
                      <a
                        href={docsLink.href}
                        target='_blank'
                        rel='noopener noreferrer'
                      />
                    ) : (
                      <Link to={docsLink.href} />
                    )
                  }
                >
                  {t('Docs')}
                  <ArrowUpRight data-icon='inline-end' aria-hidden='true' />
                </Button>
              </div>
            </div>
            {!ordinaryUser && (
              <div className='bg-card/80 rounded-2xl border p-5 shadow-xs'>
                <Server className='text-primary size-5' aria-hidden='true' />
                <h2 className='mt-4 text-sm font-semibold'>
                  {t('Container or native process')}
                </h2>
                <p className='text-muted-foreground mt-2 text-sm leading-relaxed'>
                  {t(
                    'In a container, connect Codex with ChatGPT sign-in. Local login import works only in a native process with access to that login.'
                  )}
                </p>
              </div>
            )}
          </div>
          <section className='mt-14' aria-labelledby='home-start-title'>
            <p className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
              {t('Get started')}
            </p>
            <h2
              id='home-start-title'
              className='mt-2 text-2xl font-semibold tracking-tight'
            >
              {t('Three steps to get started')}
            </h2>
            <ol className='mt-5 grid gap-3 md:grid-cols-3'>
              {steps.map((item, index) => {
                const Icon = item.icon
                return (
                  <li
                    key={item.title}
                    className='bg-card flex min-w-0 flex-col rounded-2xl border p-5 shadow-xs'
                  >
                    <div className='flex items-center justify-between'>
                      <span
                        className='text-muted-foreground text-xs font-medium tabular-nums'
                        aria-hidden='true'
                      >
                        {String(index + 1).padStart(2, '0')}
                      </span>
                      <Icon
                        className='text-primary size-5'
                        aria-hidden='true'
                      />
                    </div>
                    <h3 className='mt-5 text-base font-semibold'>
                      {item.title}
                    </h3>
                    <p className='text-muted-foreground mt-2 text-sm leading-relaxed'>
                      {item.description}
                    </p>
                    {item.available && (
                      <Link
                        to={item.to}
                        aria-label={`${t('Open')} ${item.title}`}
                        className='text-primary focus-visible:ring-ring mt-5 inline-flex w-fit items-center gap-1 rounded-md text-sm font-medium outline-none hover:underline focus-visible:ring-2'
                      >
                        {t('Open')}
                        <ArrowUpRight className='size-4' aria-hidden='true' />
                      </Link>
                    )}
                  </li>
                )
              })}
            </ol>
          </section>
        </div>
      </main>
      <Footer />
    </PublicLayout>
  )
}
