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
import { AlertTriangle, CheckCircle2, XCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { cn } from '@/lib/utils'

import { buildAuditChecklist } from '../constants'
import type { QuotaWriterAudit } from '../types'

type AuditChecklistProps = {
  audit: QuotaWriterAudit
}

export function AuditChecklist(props: AuditChecklistProps) {
  const { t } = useTranslation()
  const checks = buildAuditChecklist(props.audit)
  const unmigratedWriters = (props.audit.writers ?? []).filter(
    (writer) => !writer.migrated
  )

  return (
    <div className='flex flex-col gap-3'>
      <ul className='divide-y rounded-lg border'>
        {checks.map((check) => {
          const showCount = check.count != null && check.count > 0
          return (
            <li
              key={check.id}
              data-check-id={check.id}
              data-passed={check.passed}
              className={cn(
                'flex items-center justify-between gap-3 px-4 py-2.5',
                !check.passed && 'bg-destructive/5'
              )}
            >
              <span className='flex min-w-0 items-center gap-2'>
                {check.passed ? (
                  <CheckCircle2
                    className='text-success size-4 shrink-0'
                    aria-hidden='true'
                  />
                ) : (
                  <XCircle
                    className='text-destructive size-4 shrink-0'
                    aria-hidden='true'
                  />
                )}
                <span
                  className={cn(
                    'truncate text-sm',
                    !check.passed && 'text-destructive font-medium'
                  )}
                >
                  {t(check.labelKey)}
                </span>
                {showCount && check.countLabelKey && (
                  <span className='text-muted-foreground shrink-0 text-xs'>
                    {t(check.countLabelKey, { count: check.count })}
                  </span>
                )}
              </span>
              <StatusBadge
                label={check.passed ? t('Passed') : t('Not met')}
                variant={check.passed ? 'success' : 'danger'}
                copyable={false}
                className='shrink-0'
              />
            </li>
          )
        })}
      </ul>

      {unmigratedWriters.length > 0 && (
        <div className='border-destructive/50 bg-destructive/5 flex flex-col gap-2 rounded-lg border p-3'>
          <span className='text-destructive flex items-center gap-1.5 text-sm font-medium'>
            <AlertTriangle className='size-4' aria-hidden='true' />
            {t('Unmigrated writers')}
          </span>
          <div className='flex flex-wrap gap-1.5'>
            {unmigratedWriters.map((writer) => (
              <StatusBadge
                key={writer.name}
                label={writer.name}
                variant='danger'
                copyable={false}
              />
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
