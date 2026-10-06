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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { QUOTA_WRITER_DRAIN_BUDGET_MAX } from '../constants'
import { useDriveQuotaWriterDrains } from '../hooks/use-quota-writer'
import type { QuotaWriterDrainReport } from '../types'

export function DrainCard() {
  const { t } = useTranslation()
  const [budget, setBudget] = useState(10)
  const [report, setReport] = useState<QuotaWriterDrainReport | null>(null)
  const drainMutation = useDriveQuotaWriterDrains()

  const handleRun = () => {
    const normalized = Math.min(
      QUOTA_WRITER_DRAIN_BUDGET_MAX,
      Math.max(1, Math.trunc(budget) || 1)
    )
    setBudget(normalized)
    drainMutation.mutate(normalized, {
      onSuccess: (res) => {
        setReport(res.data ?? null)
        toast.success(t('Drain run finished'))
      },
      onError: () => {
        toast.error(t('Failed to drive drains'))
      },
    })
  }

  return (
    <div className='flex flex-col gap-4 rounded-lg border p-4'>
      <div className='flex flex-col gap-1'>
        <h4 className='text-sm font-semibold'>
          {t('Drive drains (recovery)')}
        </h4>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Recovery operation: drives the batch queue, balance drain generations and the projection backlog towards zero within the budgeted rounds, then reports what remains.'
          )}
        </p>
      </div>

      <div className='flex flex-wrap items-end gap-3'>
        <div className='flex flex-col gap-2'>
          <Label htmlFor='quota-writer-drain-budget'>
            {t('Budget (rounds)')}
          </Label>
          <Input
            id='quota-writer-drain-budget'
            type='number'
            min={1}
            max={QUOTA_WRITER_DRAIN_BUDGET_MAX}
            className='w-32'
            value={budget}
            onChange={(event) =>
              setBudget(
                Number.isNaN(event.target.valueAsNumber)
                  ? 1
                  : event.target.valueAsNumber
              )
            }
          />
        </div>
        <Button
          variant='outline'
          onClick={handleRun}
          disabled={drainMutation.isPending}
        >
          {drainMutation.isPending ? t('Running...') : t('Run drain')}
        </Button>
      </div>

      {report && (
        <div className='flex flex-col gap-3'>
          <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-3'>
            <div className='bg-muted/40 rounded-md p-3'>
              <div className='text-muted-foreground text-xs'>
                {t('Rounds executed')}
              </div>
              <div className='text-lg font-semibold'>{report.rounds}</div>
            </div>
            <div className='bg-muted/40 rounded-md p-3'>
              <div className='text-muted-foreground text-xs'>
                {t('Batch queue remaining')}
              </div>
              <div className='text-lg font-semibold'>
                {report.batch_queue_remaining}
              </div>
            </div>
            <div className='bg-muted/40 rounded-md p-3'>
              <div className='text-muted-foreground text-xs'>
                {t('Balance drain pending')}
              </div>
              <div className='text-lg font-semibold'>
                {report.balance_drain_pending}
              </div>
            </div>
            <div className='bg-muted/40 rounded-md p-3'>
              <div className='text-muted-foreground text-xs'>
                {t('Balance drain in flight')}
              </div>
              <div className='text-lg font-semibold'>
                {report.balance_drain_inflight ? t('Yes') : t('No')}
              </div>
            </div>
            <div className='bg-muted/40 rounded-md p-3'>
              <div className='text-muted-foreground text-xs'>
                {t('Projection pending')}
              </div>
              <div className='text-lg font-semibold'>
                {report.projection_pending}
              </div>
            </div>
          </div>
          <div>
            <StatusBadge
              label={
                report.complete ? t('All queues drained') : t('Backlog remains')
              }
              variant={report.complete ? 'success' : 'warning'}
              copyable={false}
            />
          </div>
        </div>
      )}
    </div>
  )
}
