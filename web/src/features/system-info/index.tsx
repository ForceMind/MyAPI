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
import { Check, Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { getBuildRevision } from '@/lib/build-metadata'

import { SystemInstancesPanel } from './components/system-instances-panel'
import { SystemTasksPanel } from './components/system-tasks-panel'

export function SystemInfo() {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const buildRevision = getBuildRevision()

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='inline-flex min-w-0 items-center gap-2'>
          <span className='truncate'>{t('System Info')}</span>
          <Badge variant='outline' className='shrink-0'>
            Root
          </Badge>
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <Card>
            <CardHeader>
              <CardTitle>{t('Runtime build')}</CardTitle>
              <CardDescription>
                {t(
                  'Use this read-only identifier to confirm which MyAPI frontend build is running.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
                <code
                  data-testid='runtime-build-revision'
                  className='bg-muted min-w-0 overflow-x-auto rounded-md px-3 py-2 text-xs whitespace-nowrap'
                >
                  {buildRevision}
                </code>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  className='shrink-0'
                  onClick={() => void copyToClipboard(buildRevision)}
                >
                  {copiedText === buildRevision ? (
                    <Check className='mr-2 size-4' aria-hidden='true' />
                  ) : (
                    <Copy className='mr-2 size-4' aria-hidden='true' />
                  )}
                  {copiedText === buildRevision
                    ? t('Copied')
                    : t('Copy build revision')}
                </Button>
              </div>
            </CardContent>
          </Card>
          <SystemInstancesPanel />
          <SystemTasksPanel />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
