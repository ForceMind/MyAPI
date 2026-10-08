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
import { Link, useRouter } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { MYAPI_ISSUES_URL } from '@/lib/build-branding'

import { ErrorPageLayout, type ErrorPageProps } from './error-page-layout'

const FEEDBACK_URL = MYAPI_ISSUES_URL

type GeneralErrorProps = React.HTMLAttributes<HTMLDivElement> &
  ErrorPageProps & {
    minimal?: boolean
    error?: unknown
  }

function getHttpStatus(error: unknown): number | undefined {
  if (typeof error !== 'object' || error === null) return undefined
  const response = (error as Record<string, unknown>).response
  if (typeof response !== 'object' || response === null) return undefined
  const status = (response as Record<string, unknown>).status
  return typeof status === 'number' ? status : undefined
}

export function GeneralError({
  className,
  minimal = false,
  embedded,
  error,
}: GeneralErrorProps) {
  const { t } = useTranslation()
  const { history } = useRouter()
  const status = getHttpStatus(error)
  const isRateLimited = status === 429
  const title = isRateLimited
    ? t('Too many requests')
    : t('Oops! Something went wrong')
  const description = isRateLimited
    ? t('Please wait a moment before trying again.')
    : t('Please try again later.')

  return (
    <ErrorPageLayout
      code={status ?? 500}
      title={title}
      minimal={minimal}
      embedded={embedded}
      className={className}
      actions={
        !minimal && (
          <>
            <Button variant='outline' onClick={() => history.go(-1)}>
              {t('Go Back')}
            </Button>
            <Button
              role='link'
              variant='outline'
              render={
                <a
                  href={FEEDBACK_URL}
                  target='_blank'
                  rel='noopener noreferrer'
                />
              }
            >
              {t('Report an issue')}
            </Button>
            <Button role='link' render={<Link to='/' />}>
              {t('Back to Home')}
            </Button>
          </>
        )
      }
    >
      <p>
        {t('We apologize for the inconvenience.')} {description}
      </p>
      {!minimal && (
        <p>
          {t('If this keeps happening, please report it on GitHub Issues.')}
        </p>
      )}
    </ErrorPageLayout>
  )
}
