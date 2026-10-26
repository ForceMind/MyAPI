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
import { Copy01Icon, Tick02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { StatTile } from '@/components/stat-tile'
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
import { linkButtonClassName } from '@/lib/link-button'

import { SystemInstancesPanel } from './components/system-instances-panel'
import { SystemTasksPanel } from './components/system-tasks-panel'
import {
  TASK_LIMIT,
  isActiveTaskStatus,
  useSystemInstances,
  useSystemTasks,
} from './hooks'

/**
 * Runtime status (Root): figures for instances and background tasks, the
 * running build, where logging is switched, then the instance and task
 * lists. Figures that could not be read show "Unknown", never 0.
 */
export function SystemInfo() {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const buildRevision = getBuildRevision()
  const instancesQuery = useSystemInstances()
  const tasksQuery = useSystemTasks()

  const instances = instancesQuery.data ?? []
  const tasks = tasksQuery.data ?? []
  const unknown = t('Unknown')
  const onlineCount = instances.filter(
    (instance) => instance.status === 'online'
  ).length
  const staleCount = instances.length - onlineCount
  const activeCount = tasks.filter((task) =>
    isActiveTaskStatus(task.status)
  ).length
  const failedCount = tasks.filter((task) => task.status === 'failed').length

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='inline-flex min-w-0 flex-wrap items-center gap-2'>
          {/* Same name as the navigation entry. */}
          <span className='wrap-break-word'>{t('System status')}</span>
          <Badge variant='outline' className='shrink-0'>
            Root
          </Badge>
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('Runtime identity, instances and background tasks.')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-4'>
          <dl className='grid min-w-0 grid-cols-1 gap-2 min-[360px]:grid-cols-2 sm:gap-3 lg:grid-cols-4'>
            <StatTile
              label={t('Online instances')}
              value={instancesQuery.isError ? unknown : onlineCount}
              loading={instancesQuery.isLoading}
            />
            <StatTile
              label={t('Stale instances')}
              value={instancesQuery.isError ? unknown : staleCount}
              loading={instancesQuery.isLoading}
            />
            <StatTile
              label={t('Active Tasks')}
              value={tasksQuery.isError ? unknown : activeCount}
              loading={tasksQuery.isLoading}
            />
            <StatTile
              label={t('Failed tasks')}
              value={tasksQuery.isError ? unknown : failedCount}
              detail={t('Among the latest {{count}} tasks', {
                count: TASK_LIMIT,
              })}
              loading={tasksQuery.isLoading}
              danger={!tasksQuery.isError && failedCount > 0}
            />
          </dl>

          <Card data-card-hover='false'>
            <CardHeader>
              <CardTitle>
                <h2>{t('Runtime build')}</h2>
              </CardTitle>
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
                  className='shrink-0 self-start sm:self-auto'
                  onClick={() => void copyToClipboard(buildRevision)}
                >
                  <HugeiconsIcon
                    icon={copiedText === buildRevision ? Tick02Icon : Copy01Icon}
                    data-icon='inline-start'
                    aria-hidden='true'
                  />
                  {copiedText === buildRevision
                    ? t('Copied')
                    : t('Copy build revision')}
                </Button>
              </div>
            </CardContent>
          </Card>

          <Card data-card-hover='false'>
            <CardHeader>
              <CardTitle>
                <h2>{t('Log collection')}</h2>
              </CardTitle>
              <CardDescription>
                {t(
                  'Whether calls are recorded is switched in Log Maintenance; whether request bodies are saved is shown on Bodies & SSE.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent className='flex flex-wrap gap-2'>
              <Link
                to='/system-settings/operations/$section'
                params={{ section: 'logs' }}
                preload={false}
                data-myapi-button=''
                className={linkButtonClassName({ variant: 'outline' })}
              >
                {t('Log Maintenance')}
              </Link>
              <Link
                to='/full-content-logs'
                preload={false}
                data-myapi-button=''
                className={linkButtonClassName({ variant: 'outline' })}
              >
                {t('Bodies & SSE')}
              </Link>
            </CardContent>
          </Card>

          <SystemInstancesPanel />
          <SystemTasksPanel />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
