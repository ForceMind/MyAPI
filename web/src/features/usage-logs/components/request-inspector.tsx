/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { ArrowLeft01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { InspectorHeader } from '@/components/inspector/inspector-layout'
import { useInspectorPresentation } from '@/components/inspector/use-inspector-presentation'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  RequestBodyPanel,
  RequestStreamPanel,
} from '@/features/full-content-logs/components/request-content-panels'
import { INSPECTOR_TABS_LIST_CLASS } from '@/lib/console-layout'

import type { UsageLog } from '../data/schema'
import type { RequestInspectorTab } from '../lib/request-inspector-tabs'
import { LogDetailsBadges, LogDetailsContent } from './dialogs/details-dialog'
import { RequestFacts, RequestTime } from './request-summary'

/**
 * One request in an inspector: the four facts, then diagnosis, saved body
 * and the saved SSE stream as tabs. The tab lives in the URL with the
 * request so refresh and shared links reopen the same view. Body tabs read
 * saved, redacted content only and never resend the request.
 */
export function RequestInspector(props: {
  log: UsageLog
  isAdmin: boolean
  tab: RequestInspectorTab
  onTabChange: (tab: RequestInspectorTab) => void
  onClose: () => void
  /** Returns to the object the request was opened from. */
  back?: { label: string; onBack: () => void }
}) {
  const { t } = useTranslation()
  const presentation = useInspectorPresentation()
  const requestId = props.log.request_id || null
  return (
    <div className='flex min-w-0 flex-col' data-testid='request-inspector'>
      {/* On a phone the top bar's Back does this. */}
      {props.back && presentation !== 'page' && (
        <div className='border-b px-2 py-1'>
          <Button variant='ghost' onClick={props.back.onBack}>
            <HugeiconsIcon
              icon={ArrowLeft01Icon}
              data-icon='inline-start'
              aria-hidden='true'
            />
            {props.back.label}
          </Button>
        </div>
      )}
      <InspectorHeader
        focusKey={`request:${props.log.id}`}
        onClose={props.onClose}
        title={
          <span className='flex min-w-0 flex-wrap items-center gap-2'>
            {t('Request details')}
            <LogDetailsBadges log={props.log} />
          </span>
        }
        subtitle={
          <span className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1'>
            <RequestTime seconds={props.log.created_at} withDate />
            {props.log.model_name && <span>{props.log.model_name}</span>}
            {requestId ? (
              <span className='flex min-w-0 items-center gap-1'>
                <code className='font-mono wrap-anywhere'>{requestId}</code>
                <CopyButton
                  value={requestId}
                  size='icon'
                  variant='ghost'
                  tooltip={t('Copy Request ID')}
                  aria-label={t('Copy Request ID')}
                />
              </span>
            ) : (
              <span>{t('No Request ID on this record')}</span>
            )}
          </span>
        }
      />
      <div className='p-card flex min-w-0 flex-col gap-4'>
        <RequestFacts log={props.log} />
        <Tabs
          value={props.tab}
          onValueChange={(value) =>
            props.onTabChange(value as RequestInspectorTab)
          }
        >
          <TabsList className={INSPECTOR_TABS_LIST_CLASS}>
            <TabsTrigger value='diagnosis'>{t('Diagnosis')}</TabsTrigger>
            <TabsTrigger value='body'>{t('Body')}</TabsTrigger>
            <TabsTrigger value='sse'>SSE</TabsTrigger>
          </TabsList>
          <TabsContent value='diagnosis' className='min-w-0 pt-2'>
            <LogDetailsContent
              log={props.log}
              isAdmin={props.isAdmin}
              active={props.tab === 'diagnosis'}
              embedded
            />
          </TabsContent>
          <TabsContent value='body' className='min-w-0 pt-2'>
            <RequestBodyPanel
              requestId={requestId}
              active={props.tab === 'body'}
            />
          </TabsContent>
          <TabsContent value='sse' className='min-w-0 pt-2'>
            <RequestStreamPanel
              requestId={requestId}
              active={props.tab === 'sse'}
            />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  )
}
