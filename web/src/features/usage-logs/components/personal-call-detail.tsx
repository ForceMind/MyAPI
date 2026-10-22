/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Link } from '@tanstack/react-router'
import { CheckCircle2, Info, KeyRound, XCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toIntlLocale } from '@/i18n/languages'

import type { UsageLog } from '../data/schema'
import { getPersonalCallEvidence } from '../lib/personal-call-evidence'

export function PersonalCallDetail(props: { log: UsageLog }) {
  const { t, i18n } = useTranslation()
  const evidence = getPersonalCallEvidence(props.log)
  const createdAt = new Date(props.log.created_at * 1000)
  const rows = [
    [
      t('Request time'),
      Number.isFinite(createdAt.getTime())
        ? createdAt.toLocaleString(toIntlLocale(i18n.language))
        : t('Unknown'),
    ],
    [
      t('Duration'),
      Number.isFinite(props.log.use_time) && props.log.use_time >= 0
        ? t('{{seconds}} seconds', { seconds: props.log.use_time.toFixed(1) })
        : t('Unknown'),
    ],
    [t('Model'), props.log.model_name || t('Unknown')],
    [t('Request path'), evidence.other?.request_path || t('Unknown')],
    [t('API Key'), props.log.token_name || t('Unknown')],
    [t('HTTP status'), t('Unknown')],
  ]
  return (
    <div className='min-w-0 space-y-4' aria-label={t('Call details')}>
      <Card className='border-border bg-card border py-5 shadow-none ring-0'>
        <CardHeader className='gap-2 px-4 sm:px-5'>
          <CardTitle>
            <h2 className='text-lg'>{t('Call details')}</h2>
          </CardTitle>
          {props.log.request_id && (
            <p className='text-muted-foreground font-mono text-xs break-all'>
              {props.log.request_id}
            </p>
          )}
        </CardHeader>
        <CardContent className='px-4 sm:px-5'>
          <div
            className={`mb-3 flex items-center gap-3 rounded-lg p-4 ${evidence.completed ? 'bg-emerald-50 text-emerald-800 dark:bg-emerald-950/50 dark:text-emerald-200' : 'bg-red-50 text-red-800 dark:bg-red-950/50 dark:text-red-200'}`}
          >
            {evidence.completed ? (
              <CheckCircle2 aria-hidden='true' className='size-9 shrink-0' />
            ) : (
              <XCircle aria-hidden='true' className='size-9 shrink-0' />
            )}
            <div>
              <h3 className='text-lg font-semibold'>
                {evidence.completed
                  ? t('Request succeeded')
                  : t('Request failed')}
              </h3>
              <p className='mt-1 text-sm'>
                {evidence.completed
                  ? t('The response was completed.')
                  : t('Inspect the error details below.')}
              </p>
            </div>
          </div>
          <dl className='divide-border divide-y'>
            {rows.map(([label, value]) => (
              <div
                key={label}
                className='grid grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)] gap-3 py-3 text-sm'
              >
                <dt className='text-muted-foreground'>{label}</dt>
                <dd className='text-right break-words'>{value}</dd>
              </div>
            ))}
          </dl>
          {!evidence.completed && props.log.content && (
            <p className='mt-3 rounded-lg bg-red-50 p-3 text-sm break-words whitespace-pre-wrap text-red-800 dark:bg-red-950/50 dark:text-red-200'>
              {props.log.content}
            </p>
          )}
        </CardContent>
      </Card>
      <Card className='border-border bg-card border py-5 shadow-none ring-0'>
        <CardHeader className='px-4 sm:px-5'>
          <div className='bg-muted flex items-center gap-3 rounded-lg p-3'>
            <Info
              aria-hidden='true'
              className='text-muted-foreground size-6 shrink-0'
            />
            <div>
              <CardTitle>
                <h3>
                  {evidence.usageKnown
                    ? t('Usage reported')
                    : t('Usage not provided')}
                </h3>
              </CardTitle>
              <p className='text-muted-foreground mt-1 text-sm'>
                {evidence.usageKnown
                  ? t('Token counts returned by the upstream.')
                  : t('Reliable token usage is unavailable.')}
              </p>
            </div>
          </div>
        </CardHeader>
        <CardContent className='space-y-3 px-4 sm:px-5'>
          <dl className='divide-border divide-y'>
            <div className='flex justify-between gap-3 py-2 text-sm'>
              <dt className='text-muted-foreground'>{t('Input Tokens')}</dt>
              <dd>
                {evidence.usageKnown
                  ? props.log.prompt_tokens.toLocaleString(
                      toIntlLocale(i18n.language)
                    )
                  : t('Unknown')}
              </dd>
            </div>
            <div className='flex justify-between gap-3 py-2 text-sm'>
              <dt className='text-muted-foreground'>{t('Output Tokens')}</dt>
              <dd>
                {evidence.usageKnown
                  ? props.log.completion_tokens.toLocaleString(
                      toIntlLocale(i18n.language)
                    )
                  : t('Unknown')}
              </dd>
            </div>
          </dl>
          {!evidence.usageKnown && (
            <Alert className='bg-accent text-accent-foreground border-0'>
              <Info aria-hidden='true' />
              <AlertDescription>
                {t(
                  'Missing usage does not change a successful request. Window shares are observed separately.'
                )}
              </AlertDescription>
            </Alert>
          )}
          {props.log.token_id > 0 && (
            <Button
              className='h-11 w-full whitespace-normal'
              variant='outline'
              render={
                <Link
                  to='/keys'
                  search={{
                    keyId: props.log.token_id,
                  }}
                />
              }
              nativeButton={false}
            >
              <KeyRound aria-hidden='true' className='size-4 shrink-0' />
              {t('View this Key')}
            </Button>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
