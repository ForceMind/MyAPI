import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatTimestampToDate } from '@/lib/format'

import type { OpenAIPriceCheckState } from './openai-price-check-api'

export function OpenAIPriceCheckStatus(props: {
  state: OpenAIPriceCheckState
}) {
  const { t } = useTranslation()
  const state = props.state
  const time = (value: number) =>
    value > 0 ? formatTimestampToDate(value) : t('Not available')
  const statuses: Record<string, string> = {
    pending: t('Pending'),
    running: t('Processing...'),
    succeeded: t('Success'),
    failed: t('Failed'),
  }
  const changes = {
    addition: t('Added'),
    change: t('Changed'),
    removal: t('Removed'),
    unqualified: t('Source not qualified'),
    unchanged: t('Unchanged'),
  }
  return (
    <>
      <dl className='grid min-w-0 gap-3 text-sm sm:grid-cols-2'>
        <div>
          <dt>{t('Daily source checks')}</dt>
          <dd>{state.enabled ? t('Enabled') : t('Disabled')}</dd>
        </div>
        <div>
          <dt>{t('Source freshness')}</dt>
          <dd>
            {state.stale
              ? t('Stale source evidence')
              : t('Fresh source evidence')}
          </dd>
        </div>
        <div>
          <dt>{t('Last check attempt')}</dt>
          <dd>
            {time(state.last_attempt_at)} ·{' '}
            {statuses[state.last_attempt_status] ?? t('Not available')}
          </dd>
        </div>
        <div>
          <dt>{t('Last successful check')}</dt>
          <dd>{time(state.last_success_at)}</dd>
        </div>
        <div>
          <dt>{t('Source created at')}</dt>
          <dd>{time(state.source_fetched_at)}</dd>
        </div>
        <div>
          <dt>{t('Price publication revision')}</dt>
          <dd>{state.revision}</dd>
        </div>
        <div className='min-w-0 sm:col-span-2'>
          <dt>{t('Saved source SHA256')}</dt>
          <dd className='font-mono break-all'>
            {state.source_sha256 || t('Not available')}
          </dd>
        </div>
        <div className='min-w-0 sm:col-span-2'>
          <dt>{t('Current price digest')}</dt>
          <dd className='font-mono break-all'>{state.expected_digest}</dd>
        </div>
      </dl>
      {state.last_attempt_status === 'failed' && (
        <Alert role='alert' variant='destructive'>
          <AlertDescription>
            {t(
              'The last source check failed. The last good source and effective prices are retained.'
            )}
          </AlertDescription>
        </Alert>
      )}
      <p className='text-sm'>
        {state.diff_review_required
          ? t(
              'Source differences await Root review. Saved source does not mean published price or budget qualification.'
            )
          : t(
              'No pending source differences. Saved source does not mean budget qualification.'
            )}
      </p>
      <p className='text-muted-foreground text-xs'>
        {t('Compared models')}: {state.diff_total} · {t('Added')}:{' '}
        {state.diff_counts.addition} · {t('Changed')}:{' '}
        {state.diff_counts.change} · {t('Removed')}: {state.diff_counts.removal}{' '}
        · {t('Source not qualified')}: {state.diff_counts.unqualified} ·{' '}
        {t('Unchanged')}: {state.diff_counts.unchanged}
      </p>
      {state.diff_truncated && (
        <p role='status' className='text-sm'>
          {t(
            'This bounded preview is incomplete. Counts cover displayed rows only; review all source models before publication.'
          )}
        </p>
      )}
      {state.diff.length > 0 && (
        <div className='max-w-full min-w-0 overflow-x-auto'>
          <Table
            className='min-w-[640px]'
            tabIndex={0}
            aria-label={t('Source check differences')}
          >
            <TableHeader>
              <TableRow>
                <TableHead>{t('Model')}</TableHead>
                <TableHead>{t('Change')}</TableHead>
                <TableHead>{t('Current price digest')}</TableHead>
                <TableHead>{t('Candidate price digest')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {state.diff.map((row) => (
                <TableRow key={row.model_sha256 ?? row.model}>
                  <TableCell className='max-w-56 break-all whitespace-normal'>
                    {row.model}
                    {row.model_name_truncated && '…'}
                  </TableCell>
                  <TableCell>{changes[row.change]}</TableCell>
                  <TableCell className='max-w-40 font-mono break-all whitespace-normal'>
                    {row.current_expression_sha256 || t('Not available')}
                  </TableCell>
                  <TableCell className='max-w-40 font-mono break-all whitespace-normal'>
                    {row.candidate?.expression_sha256 ?? t('Not quoted')}
                  </TableCell>
                  <TableCell>
                    <div className='space-y-1'>
                      {row.locked && <p>{t('Locked')}</p>}
                      {row.pending_review && <p>{t('Pending review')}</p>}
                      {row.change === 'unqualified' && (
                        <p>{t('Source not qualified')}</p>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </>
  )
}
