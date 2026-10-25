/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { ArrowDown01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import type { ConnectionSummary } from '@/features/connections/connection-state'
import { linkButtonClassName } from '@/lib/link-button'
import { cn } from '@/lib/utils'

import {
  FLOW_NODE_LIMIT,
  visibleNodes,
  type FlowKeyNode,
  type FlowSelection,
  type FlowTraffic,
} from '../../lib/overview-flow'
import { ConnectionNode, KeyNode } from './flow-nodes'

export type FlowColumnState = 'loading' | 'error' | 'ready'

type ColumnProps = {
  title: string
  count: number | null
  state: FlowColumnState
  errorText: string
  onRetry: () => void
  empty: ReactNode
  viewAll: ReactNode
  children: ReactNode
}

function FlowColumn(props: ColumnProps) {
  const { t } = useTranslation()
  let body = props.children
  if (props.state === 'loading') {
    body = (
      <div className='flex flex-col gap-3' aria-busy='true'>
        <span className='sr-only'>{t('Loading...')}</span>
        {[1, 2, 3].map((item) => (
          <Skeleton key={item} className='bg-background h-16 w-full' />
        ))}
      </div>
    )
  } else if (props.state === 'error') {
    body = (
      <div className='bg-background flex flex-col items-start gap-2 rounded-lg border px-3.5 py-3'>
        <p className='text-body'>{props.errorText}</p>
        <Button variant='outline' size='sm' onClick={props.onRetry}>
          {t('Retry')}
        </Button>
      </div>
    )
  } else if (props.count === 0) {
    body = props.empty
  }
  return (
    <div className='flex min-w-0 flex-col gap-3'>
      <h3 className='text-caption text-muted-foreground font-semibold tracking-wider uppercase'>
        {props.title}
        {props.count !== null && props.state === 'ready'
          ? ` · ${props.count}`
          : ''}
      </h3>
      {body}
      {props.state === 'ready' && props.viewAll}
    </div>
  )
}

function ColumnEmpty(props: {
  title: string
  description: string
  action: ReactNode
}) {
  return (
    <Empty className='bg-background gap-3 border p-4'>
      <EmptyHeader>
        <EmptyTitle>{props.title}</EmptyTitle>
        <EmptyDescription>{props.description}</EmptyDescription>
      </EmptyHeader>
      {props.action && <EmptyContent>{props.action}</EmptyContent>}
    </Empty>
  )
}

/** Vertical connector between the stacked sections on a phone. */
function PipeConnector() {
  return (
    <div
      aria-hidden='true'
      className='text-line flex flex-col items-center lg:hidden'
    >
      <span className='bg-line h-3 w-0.5' />
      <HugeiconsIcon icon={ArrowDown01Icon} className='-mt-1 size-4' />
    </div>
  )
}

export type FlowBoardKeys = {
  nodes: FlowKeyNode[]
  state: FlowColumnState
  onRetry: () => void
  /** Create-Key action shown in the empty column; null hides it. */
  createAction: ReactNode
}

export type FlowBoardConnections = {
  items: ConnectionSummary[]
  total: number
  windows: Map<number, ChannelQuotaChangeItem>
  /** The saved account windows could not be read; rings are missing. */
  windowsError: boolean
  onRetryWindows: () => void
  /** Undefined when the call records could not be read. */
  traffic: Map<number, FlowTraffic> | undefined
  state: FlowColumnState
  onRetry: () => void
  addAction: ReactNode
}

/**
 * Flow board (design v2 2.3): who calls (API Keys) -> the gateway core card
 * -> which upstream connection. Selecting a node filters the request table;
 * selecting it again clears the filter. Data comes from the page; this
 * component only renders it and reports selections. On a phone the three
 * columns stack into a vertical pipe.
 */
export function FlowBoard(props: {
  keys: FlowBoardKeys
  /** Null when the viewer may not read connections: no upstream column. */
  connections: FlowBoardConnections | null
  core: ReactNode
  caption: ReactNode
  selection: FlowSelection | null
  onSelect: (selection: FlowSelection | null) => void
}) {
  const { t } = useTranslation()
  const selection = props.selection
  const toggle = (next: FlowSelection) =>
    props.onSelect(
      selection?.kind === next.kind && selection.id === next.id ? null : next
    )
  const keyNodes = visibleNodes(
    props.keys.nodes,
    selection?.kind === 'key' ? selection.id : undefined
  )
  const connections = props.connections

  return (
    <section
      aria-labelledby='overview-flow-title'
      data-testid='flow-board'
      className='bg-muted rounded-card flex min-w-0 flex-col gap-4 border p-4 lg:p-6'
    >
      <div className='flex min-w-0 flex-wrap items-baseline justify-between gap-x-3 gap-y-1'>
        <h2 id='overview-flow-title' className='sr-only'>
          {t('Request flow')}
        </h2>
        <p className='text-caption text-muted-foreground'>{props.caption}</p>
      </div>
      <div
        className={cn(
          'grid min-w-0 gap-3 lg:items-start lg:gap-x-12 lg:gap-y-0',
          connections
            ? 'lg:grid-cols-[minmax(0,1fr)_300px_minmax(0,1fr)]'
            : 'lg:grid-cols-[minmax(0,1fr)_300px]'
        )}
      >
        <FlowColumn
          title={t('API Key')}
          count={props.keys.nodes.length}
          state={props.keys.state}
          errorText={t('API Keys could not be loaded.')}
          onRetry={props.keys.onRetry}
          empty={
            <ColumnEmpty
              title={t('No API Keys yet')}
              description={t(
                'Create a Key for your client; its requests appear here.'
              )}
              action={props.keys.createAction}
            />
          }
          viewAll={
            props.keys.nodes.length > FLOW_NODE_LIMIT && (
              <Link
                to='/keys'
                data-myapi-button=''
                className={linkButtonClassName({
                  variant: 'ghost',
                  className: 'self-start',
                })}
              >
                {t('View all {{count}}', { count: props.keys.nodes.length })}
              </Link>
            )
          }
        >
          <div className='flex flex-col gap-3'>
            {keyNodes.map((node) => (
              <KeyNode
                key={node.id}
                node={node}
                selected={selection?.kind === 'key' && selection.id === node.id}
                onToggle={() => toggle({ kind: 'key', id: node.id })}
              />
            ))}
          </div>
        </FlowColumn>
        <PipeConnector />
        <div className='min-w-0 lg:self-center'>{props.core}</div>
        {connections && (
          <>
            <PipeConnector />
            <FlowColumn
              title={t('Upstream')}
              count={connections.total}
              state={connections.state}
              errorText={t('Connections could not be loaded.')}
              onRetry={connections.onRetry}
              empty={
                <ColumnEmpty
                  title={t('No upstream connections yet')}
                  description={t(
                    'Add a connection so the gateway can forward requests.'
                  )}
                  action={connections.addAction}
                />
              }
              viewAll={
                connections.total > connections.items.length && (
                  <Link
                    to='/connections'
                    data-myapi-button=''
                    className={linkButtonClassName({
                      variant: 'ghost',
                      className: 'self-start',
                    })}
                  >
                    {t('View all {{count}}', { count: connections.total })}
                  </Link>
                )
              }
            >
              <div className='flex flex-col gap-3'>
                {connections.windowsError && (
                  <div
                    data-testid='accounts-error-state'
                    className='bg-background flex flex-wrap items-center gap-2 rounded-lg border px-3.5 py-2'
                  >
                    <p className='text-caption text-warning min-w-0 flex-1'>
                      {t('Saved account windows could not be read.')}
                    </p>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={connections.onRetryWindows}
                    >
                      {t('Retry')}
                    </Button>
                  </div>
                )}
                {connections.items.map((connection) => (
                  <ConnectionNode
                    key={connection.id}
                    connection={connection}
                    window={connections.windows.get(connection.id)}
                    traffic={connections.traffic?.get(connection.id)}
                    trafficKnown={connections.traffic !== undefined}
                    selected={
                      selection?.kind === 'connection' &&
                      selection.id === connection.id
                    }
                    onToggle={() =>
                      toggle({ kind: 'connection', id: connection.id })
                    }
                  />
                ))}
              </div>
            </FlowColumn>
          </>
        )}
      </div>
    </section>
  )
}
