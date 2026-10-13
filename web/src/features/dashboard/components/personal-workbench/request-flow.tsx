import {
  Activity02Icon,
  ArrowRight01Icon,
  LinkSquare02Icon,
  TerminalIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusBadgeProps } from '@/components/status-badge'
import type { PlaygroundKey } from '@/features/playground/types'

import type { PlaygroundKeyCondition } from './workbench-state'

export function RequestFlow(props: {
  selectedKey: PlaygroundKey | undefined
  condition: PlaygroundKeyCondition | null
  statusLabel: string | undefined
  statusVariant: StatusBadgeProps['variant']
}) {
  const { t } = useTranslation()
  return (
    <div className='min-w-0'>
      <ol className='grid min-w-0 gap-2 sm:grid-cols-3'>
        <li className='bg-muted/35 min-w-0 rounded-lg border p-3'>
          <HugeiconsIcon
            icon={TerminalIcon}
            strokeWidth={2}
            className='text-primary mb-4 size-5'
            aria-hidden='true'
          />
          <p className='text-xs font-medium tracking-wide uppercase'>
            {t('Selected Key')}
          </p>
          <div className='mt-1 flex min-w-0 flex-wrap items-center gap-2'>
            <span
              data-testid='request-flow-selected-key'
              className='min-w-0 font-medium [overflow-wrap:anywhere]'
            >
              {props.selectedKey?.name ?? t('No Key selected')}
            </span>
            {props.statusLabel ? (
              <StatusBadge variant={props.statusVariant} copyable={false}>
                {props.statusLabel}
              </StatusBadge>
            ) : null}
          </div>
        </li>
        <li className='bg-muted/35 min-w-0 rounded-lg border p-3'>
          <HugeiconsIcon
            icon={Activity02Icon}
            strokeWidth={2}
            className='text-primary mb-4 size-5'
            aria-hidden='true'
          />
          <p className='font-medium'>{t('MyAPI request admission')}</p>
          <p className='text-muted-foreground mt-1 text-xs leading-relaxed'>
            {t(
              'The server checks the Key, route, and limits when a request starts.'
            )}
          </p>
        </li>
        <li className='bg-muted/35 min-w-0 rounded-lg border p-3'>
          <HugeiconsIcon
            icon={LinkSquare02Icon}
            strokeWidth={2}
            className='text-primary mb-4 size-5'
            aria-hidden='true'
          />
          <p className='font-medium'>{t('Upstream routing')}</p>
          <p className='text-muted-foreground mt-1 text-xs leading-relaxed'>
            {t(
              'The server selects an eligible upstream account at request time.'
            )}
          </p>
        </li>
      </ol>
      <div className='text-muted-foreground mt-4 flex items-start gap-2 border-t pt-3 text-xs'>
        <HugeiconsIcon
          icon={ArrowRight01Icon}
          strokeWidth={2}
          className='mt-0.5 size-3.5 shrink-0'
          aria-hidden='true'
        />
        <p>
          {props.condition && props.condition !== 'available'
            ? t(
                'This Key cannot start a request until its status or limits change.'
              )
            : t('Playground opens without sending a request.')}
        </p>
      </div>
    </div>
  )
}
