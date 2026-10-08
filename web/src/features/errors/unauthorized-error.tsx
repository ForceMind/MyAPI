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

import { ErrorPageLayout, type ErrorPageProps } from './error-page-layout'

export function UnauthorisedError(props: ErrorPageProps) {
  const { t } = useTranslation()
  const { history } = useRouter()
  return (
    <ErrorPageLayout
      embedded={props.embedded}
      code={401}
      title={t('Unauthorized Access')}
      actions={
        <>
          <Button variant='outline' onClick={() => history.go(-1)}>
            {t('Go Back')}
          </Button>
          <Button role='link' variant='outline' render={<Link to='/' />}>
            {t('Back to Home')}
          </Button>
          <Button role='link' render={<Link to='/sign-in' />}>
            {t('Sign in')}
          </Button>
        </>
      }
    >
      <p>
        {t('Please log in with the appropriate credentials')}{' '}
        {t('to access this resource.')}
      </p>
    </ErrorPageLayout>
  )
}
