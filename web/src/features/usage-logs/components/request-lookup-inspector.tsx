/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { FileSearchIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { InspectorHeader } from '@/components/inspector/inspector-layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'

import type { useRequestLookup } from '../hooks/use-request-lookup'
import type { RequestInspectorTab } from '../lib/request-inspector-tabs'
import { RequestInspector } from './request-inspector'

/**
 * Inspector content for a Request ID looked up by the call records page.
 * Loading, failure and "no matching record" each have their own state; a
 * lookup failure never reads as "not found".
 */
export function RequestLookupInspector(props: {
  requestId: string
  lookup: ReturnType<typeof useRequestLookup>
  isAdmin: boolean
  tab: RequestInspectorTab
  onTabChange: (tab: RequestInspectorTab) => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  const log = props.lookup.data?.log
  if (log) {
    return (
      <>
        <RequestInspector
          log={log}
          isAdmin={props.isAdmin}
          tab={props.tab}
          onTabChange={props.onTabChange}
          onClose={props.onClose}
        />
        {(props.lookup.data?.count ?? 0) > 1 && (
          <p className='text-caption text-muted-foreground px-card pb-card'>
            {t(
              'This Request ID has {{count}} call records; the newest request record is shown. All of them are listed in the table.',
              { count: props.lookup.data?.count }
            )}
          </p>
        )}
      </>
    )
  }

  let body
  if (props.lookup.isPending) {
    body = (
      <div className='flex flex-col gap-3' aria-busy='true'>
        <span className='sr-only'>{t('Loading...')}</span>
        <Skeleton className='h-20 w-full' />
        <Skeleton className='h-48 w-full' />
      </div>
    )
  } else if (props.lookup.isError) {
    body = (
      <Alert variant='destructive'>
        <AlertTitle>{t('Could not look up this Request ID')}</AlertTitle>
        <AlertDescription className='flex flex-col items-start gap-2'>
          <span>{t('The call records could not be read. Try again.')}</span>
          <Button
            variant='outline'
            size='sm'
            disabled={props.lookup.isFetching}
            onClick={() => void props.lookup.refetch()}
          >
            {t('Retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  } else {
    body = (
      <Empty className='border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={FileSearchIcon} aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('No call record has this Request ID')}</EmptyTitle>
          <EmptyDescription>
            {props.isAdmin
              ? t(
                  'Check the ID. Records removed by log cleanup cannot be found.'
                )
              : t(
                  'Check the ID. Only your own call records can be found from this account.'
                )}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }
  return (
    <div className='flex min-w-0 flex-col'>
      <InspectorHeader
        focusKey={`lookup:${props.requestId}`}
        onClose={props.onClose}
        title={t('Request details')}
        subtitle={
          <code className='font-mono wrap-anywhere'>{props.requestId}</code>
        }
      />
      <div className='p-card'>{body}</div>
    </div>
  )
}
