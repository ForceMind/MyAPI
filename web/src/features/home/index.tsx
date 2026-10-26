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
import { Link, useNavigate } from '@tanstack/react-router'
import {
  ArrowRight,
  ArrowUpRight,
  FileSearch,
  KeyRound,
  Radio,
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
import { resolveDocsLink } from '@/lib/build-branding'
import { isLikelyHtml } from '@/lib/content-format'
import { useAuthStore } from '@/stores/auth-store'

import { useHomePageContent } from './hooks'

export function Home() {
  const { i18n, t } = useTranslation()
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const { resolvedTheme } = useTheme()
  const navigate = useNavigate()
  const isAuthenticated = useAuthStore((state) => Boolean(state.auth.user))
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

  useEffect(() => {
    if (isLoaded && !content && !failed && isAuthenticated) {
      void navigate({
        to: '/dashboard/$section',
        params: { section: 'overview' },
        replace: true,
      })
    }
  }, [content, failed, isAuthenticated, isLoaded, navigate])

  if (!isLoaded) {
    return (
      <PublicLayout showMainContainer={false}>
        <main className='flex min-h-[calc(100svh-3.5rem)] items-center justify-center'>
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
            className='h-[calc(100svh-3.5rem)] w-full border-none'
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
        <div className='min-w-0'>
          <RichContent
            mode='markdown'
            content={content}
            className='custom-home-content'
          />
        </div>
      </PublicLayout>
    )
  }

  if (isAuthenticated && !failed) return null

  const capabilities = [
    {
      icon: Radio,
      title: t('Connections & access'),
      description: t('Manage upstream connections, model routes and API keys.'),
    },
    {
      icon: KeyRound,
      title: t('Access controls'),
      description: t(
        'Keep model access, limits and expiry under your control.'
      ),
    },
    {
      icon: FileSearch,
      title: t('Records & diagnostics'),
      description: t(
        'Trace requests, inspect saved content and follow background tasks.'
      ),
    },
  ]

  return (
    <PublicLayout showMainContainer={false}>
      <main className='mx-auto w-full max-w-[1200px] min-w-0 px-4 py-10 sm:py-16 lg:px-6'>
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
        <div className='grid items-center gap-10 lg:grid-cols-[1.25fr_1fr] lg:gap-16'>
          <section className='min-w-0'>
            <div className='text-primary mb-5 flex items-center gap-3 text-sm font-medium'>
              <img src='/myapi-logo-v1.png' alt='' className='size-9' />
              MyAPI · {t('Self-hosted gateway')}
            </div>
            <h1 className='max-w-2xl text-4xl leading-tight font-semibold tracking-tight sm:text-5xl'>
              {t('Your gateway. Every request in view.')}
            </h1>
            <p className='text-muted-foreground mt-5 max-w-xl text-base leading-relaxed'>
              {t(
                'Connect your AI tools through one API. Manage access and diagnose every request from one console.'
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
                <ArrowRight aria-hidden='true' />
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
                <ArrowUpRight aria-hidden='true' />
              </Button>
            </div>
          </section>
          <section
            className='bg-card rounded-card min-w-0 overflow-hidden border'
            aria-label={t('Platform workflow')}
          >
            <div className='bg-muted flex items-center justify-between border-b px-5 py-3.5'>
              <span className='font-semibold'>MyAPI</span>
              <span className='text-muted-foreground text-xs'>
                {t('Console')}
              </span>
            </div>
            <div className='divide-y px-5'>
              {capabilities.map((item, index) => (
                <div
                  key={item.title}
                  className='flex min-w-0 items-start gap-4 py-5'
                >
                  <span className='bg-accent text-accent-foreground flex size-10 shrink-0 items-center justify-center rounded-lg'>
                    <item.icon className='size-5' aria-hidden='true' />
                  </span>
                  <div className='min-w-0'>
                    <h2 className='text-sm font-semibold'>
                      <span className='text-muted-foreground mr-2 text-xs tabular-nums'>
                        0{index + 1}
                      </span>
                      {item.title}
                    </h2>
                    <p className='text-muted-foreground mt-1.5 text-sm leading-relaxed'>
                      {item.description}
                    </p>
                  </div>
                </div>
              ))}
            </div>
          </section>
        </div>
        <section
          className='rounded-card bg-card mt-10 border p-5 sm:mt-14'
          aria-labelledby='home-entry-title'
        >
          <h2 id='home-entry-title' className='text-sm font-semibold'>
            {t('One endpoint for your AI tools')}
          </h2>
          <p className='text-muted-foreground mt-2 text-sm'>
            {t(
              'Sign in to manage connections, issue a key and inspect call records. Existing API clients keep their configured endpoint.'
            )}
          </p>
        </section>
      </main>
      <Footer />
    </PublicLayout>
  )
}
