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
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { RichContent } from '@/components/rich-content'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useSystemConfig } from '@/hooks/use-system-config'
import {
  MYAPI_LICENSE_URL,
  MYAPI_NOTICES_URL,
  MYAPI_REPOSITORY_URL,
} from '@/lib/build-branding'
import { isHttpUrl, isLikelyHtml } from '@/lib/content-format'

import { getAboutContent } from './api'

function EmptyAboutState() {
  const { t } = useTranslation()
  const { systemName } = useSystemConfig()
  const currentYear = new Date().getFullYear()

  return (
    <Card data-card-hover='false' className='mx-auto max-w-2xl'>
      <CardHeader>
        <CardTitle>
          <h1 className='text-lg'>{t('No About Content Set')}</h1>
        </CardTitle>
        <CardDescription>
          {t(
            'The administrator has not configured any about content yet. You can set it in the settings page, supporting HTML or URL.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className='flex flex-col gap-3 text-sm wrap-anywhere'>
          <p>
            {t('MyAPI Project Repository:')}{' '}
            <a
              href={MYAPI_REPOSITORY_URL}
              target='_blank'
              rel='noopener noreferrer'
              className='text-primary hover:underline'
            >
              {MYAPI_REPOSITORY_URL}
            </a>
          </p>
          <p className='text-muted-foreground'>
            <a
              href={MYAPI_REPOSITORY_URL}
              target='_blank'
              rel='noopener noreferrer'
              className='text-primary hover:underline'
            >
              {systemName || t('MyAPI')}
            </a>{' '}
            © {currentYear} · {t('Maintained by')}{' '}
            <a
              href='https://github.com/ForceMind'
              target='_blank'
              rel='noopener noreferrer'
              className='text-primary hover:underline'
            >
              ForceMind
            </a>
          </p>
          <p className='text-muted-foreground'>
            {t('This project must be used in compliance with the')}{' '}
            <a
              href={MYAPI_LICENSE_URL}
              target='_blank'
              rel='noopener noreferrer'
              className='text-primary hover:underline'
            >
              {t('AGPL v3.0 License')}
            </a>
            .
          </p>
          <p className='text-muted-foreground text-xs'>
            <a
              href={MYAPI_NOTICES_URL}
              target='_blank'
              rel='noopener noreferrer'
              className='text-primary hover:underline'
            >
              {t('View required third-party notices')}
            </a>
          </p>
        </div>
      </CardContent>
    </Card>
  )
}

export function About() {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: ['about-content'],
    queryFn: getAboutContent,
  })

  const rawContent = data?.data?.trim() ?? ''
  const hasContent = rawContent.length > 0
  const isUrl = hasContent && isHttpUrl(rawContent)
  const contentIsHtml = hasContent && isLikelyHtml(rawContent)

  if (isLoading) {
    return (
      <PublicLayout>
        <div className='mx-auto flex max-w-3xl flex-col gap-4'>
          <Skeleton className='h-8 w-[45%]' />
          <Skeleton className='h-4 w-full' />
          <Skeleton className='h-4 w-[90%]' />
          <Skeleton className='h-4 w-[80%]' />
        </div>
      </PublicLayout>
    )
  }

  if (!hasContent) {
    return (
      <PublicLayout>
        <EmptyAboutState />
      </PublicLayout>
    )
  }

  if (isUrl) {
    return (
      <PublicLayout showMainContainer={false}>
        <iframe
          src={rawContent}
          className='h-[calc(100svh-3.5rem)] w-full border-0'
          title={t('About')}
          sandbox='allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts'
        />
      </PublicLayout>
    )
  }

  if (contentIsHtml) {
    return (
      <PublicLayout showMainContainer={false}>
        <RichContent
          mode='html'
          htmlVariant='isolated'
          content={rawContent}
          className='prose-neutral dark:prose-invert max-w-none'
        />
      </PublicLayout>
    )
  }

  return (
    <PublicLayout>
      <div className='min-w-0'>
        <RichContent
          mode='markdown'
          content={rawContent}
          className='prose-neutral dark:prose-invert max-w-none'
        />
      </div>
    </PublicLayout>
  )
}
