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
  Link,
  useRouter,
  type NotFoundRouteProps,
} from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { ErrorPageLayout, type ErrorPageProps } from './error-page-layout'

export function NotFoundError(
  props: ErrorPageProps & Pick<NotFoundRouteProps, 'data'>
) {
  const { t } = useTranslation()
  const { history } = useRouter()
  return (
    <ErrorPageLayout
      embedded={props.embedded}
      code={404}
      title={t('Oops! Page Not Found!')}
      actions={
        <>
          <Button variant='outline' onClick={() => history.go(-1)}>
            {t('Go Back')}
          </Button>

          <Button role='link' render={<Link to='/' />}>
            {t('Back to Home')}
          </Button>
        </>
      }
    >
      <p>
        {t("It seems like the page you're looking for")}{' '}
        {t('does not exist or might have been removed.')}
      </p>
    </ErrorPageLayout>
  )
}
