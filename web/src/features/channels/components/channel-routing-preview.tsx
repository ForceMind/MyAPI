/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { Route } from 'lucide-react'
import { useMemo, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'

import { getChannelRoutingPreview } from '../api'
import { channelsQueryKeys, getChannelTypeLabel } from '../lib'
import {
  createChannelRoutingPercentFormatter,
  resolveChannelRoutingPreviewErrorCode,
} from '../lib/channel-routing'
import type { ChannelRoutingPreviewParams } from '../types'

export function ChannelRoutingPreview() {
  const { t, i18n } = useTranslation()
  const [group, setGroup] = useState('default')
  const [model, setModel] = useState('')
  const [requestPath, setRequestPath] = useState('')
  const [submittedParams, setSubmittedParams] =
    useState<ChannelRoutingPreviewParams | null>(null)

  const previewQuery = useQuery({
    queryKey: channelsQueryKeys.routingPreview(submittedParams ?? {}),
    queryFn: () => {
      if (!submittedParams) {
        throw new Error('routing preview parameters are unavailable')
      }
      return getChannelRoutingPreview(submittedParams)
    },
    enabled: submittedParams !== null,
    retry: false,
  })

  const percentFormatter = useMemo(
    () =>
      createChannelRoutingPercentFormatter(
        i18n.resolvedLanguage || i18n.language
      ),
    [i18n.language, i18n.resolvedLanguage]
  )

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const normalizedGroup = group.trim()
    const normalizedModel = model.trim()
    if (!normalizedGroup || !normalizedModel) {
      return
    }
    const params = {
      group: normalizedGroup,
      model: normalizedModel,
      request_path: requestPath.trim() || undefined,
    }
    if (
      submittedParams?.group === params.group &&
      submittedParams.model === params.model &&
      submittedParams.request_path === params.request_path
    ) {
      void previewQuery.refetch()
      return
    }
    setSubmittedParams(params)
  }

  const errorCode = resolveChannelRoutingPreviewErrorCode(
    previewQuery.error,
    previewQuery.data
  )
  const businessError =
    previewQuery.data !== undefined && !previewQuery.data.success
  const previewData =
    !errorCode && previewQuery.data?.success
      ? previewQuery.data.data
      : undefined
  const tiers = previewData?.tiers ?? []

  let errorMessage = t('Unable to load routing preview.')
  if (errorCode === 'routing_preview_invalid_params') {
    errorMessage = t('Group and model are required.')
  } else if (errorCode === 'routing_preview_auto_group_unsupported') {
    errorMessage = t('Routing preview does not support the auto group.')
  } else if (errorCode === 'routing_preview_permission_denied') {
    errorMessage = t("You don't have necessary permission")
  } else if (errorCode === 'routing_preview_database_error') {
    errorMessage = t('Routing preview is temporarily unavailable.')
  } else if (errorCode === 'routing_preview_network_error') {
    errorMessage = t('Unable to reach the server. Try again.')
  }

  return (
    <Card className='mb-4' size='sm'>
      <CardHeader>
        <CardTitle className='flex items-center gap-2'>
          <Route className='size-4' aria-hidden='true' />
          {t('Routing Preview')}
        </CardTitle>
        <CardDescription>
          {t(
            'Preview the read-only priority tiers and expected weight shares for one explicit group and model.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-4'>
        <div
          className='border-border bg-card rounded-lg border px-3 py-2 text-sm'
          aria-labelledby='routing-contract-title'
        >
          <h3 id='routing-contract-title' className='font-medium'>
            {t('Runtime routing contract')}
          </h3>
          <div className='text-muted-foreground mt-1 space-y-1'>
            <p>
              {t(
                'Higher priority values are attempted first. Retries fall back to lower priority tiers.'
              )}
            </p>
            <p>
              {t(
                'Within a tier, positive weights split traffic proportionally. If every weight is zero, traffic is split evenly; a zero weight receives no traffic when any positive weight exists.'
              )}
            </p>
            <p>
              {t(
                'Channel list sorting is display-only and does not change runtime routing.'
              )}
            </p>
          </div>
        </div>

        <form
          className='grid grid-cols-1 gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1.2fr)_auto]'
          onSubmit={handleSubmit}
          aria-label={t('Routing Preview')}
        >
          <label className='space-y-1 text-sm'>
            <span className='font-medium'>{t('Group')}</span>
            <Input
              value={group}
              onChange={(event) => {
                setGroup(event.target.value)
                setSubmittedParams(null)
              }}
              placeholder={t('Explicit group name')}
              aria-label={t('Routing preview group')}
            />
          </label>
          <label className='space-y-1 text-sm'>
            <span className='font-medium'>{t('Model')}</span>
            <Input
              value={model}
              onChange={(event) => {
                setModel(event.target.value)
                setSubmittedParams(null)
              }}
              placeholder={t('Model name')}
              aria-label={t('Routing preview model')}
            />
          </label>
          <label className='space-y-1 text-sm'>
            <span className='font-medium'>{t('Request Path')}</span>
            <Input
              value={requestPath}
              onChange={(event) => {
                setRequestPath(event.target.value)
                setSubmittedParams(null)
              }}
              placeholder={t('Optional request path')}
              aria-label={t('Routing preview request path')}
            />
          </label>
          <Button
            type='submit'
            className='self-end'
            disabled={!group.trim() || !model.trim() || previewQuery.isFetching}
          >
            {previewQuery.isFetching ? t('Loading...') : t('Preview routing')}
          </Button>
        </form>

        {previewData ? (
          <div className='flex flex-wrap items-center gap-2 text-sm'>
            <Badge variant='outline'>
              {previewData.source === 'cache'
                ? t('Runtime cache snapshot')
                : t('Database snapshot')}
            </Badge>
            <span className='text-muted-foreground'>
              {t('Snapshot epoch {{epoch}}', {
                epoch: previewData.generation,
              })}
            </span>
            {previewData.cache_enabled ? (
              <span className='text-muted-foreground'>
                {t(
                  'Local published epoch {{local}} · cluster committed epoch {{cluster}}',
                  {
                    local: previewData.local_published_epoch,
                    cluster: previewData.cluster_committed_epoch,
                  }
                )}
              </span>
            ) : null}
          </div>
        ) : null}

        {previewData?.cache_pending ? (
          <Alert>
            <AlertDescription>
              {t(
                'The runtime cache is pending publication. This preview uses local epoch {{published}}, while the cluster is committed at epoch {{data}}.',
                {
                  published: previewData.local_published_epoch,
                  data: previewData.cluster_committed_epoch,
                }
              )}
            </AlertDescription>
          </Alert>
        ) : null}

        {errorCode ? (
          <Alert variant='destructive'>
            <AlertDescription>{errorMessage}</AlertDescription>
          </Alert>
        ) : null}

        {previewQuery.isSuccess && !businessError && tiers.length === 0 ? (
          <p className='text-muted-foreground text-sm'>
            {t(
              'No matching enabled channels were found for this routing preview.'
            )}
          </p>
        ) : null}

        {tiers.length > 0 ? (
          <div className='space-y-3' aria-label={t('Routing preview tiers')}>
            {tiers.map((tier) => (
              <section
                key={`${tier.priority}-${tier.fallback_index}`}
                className='border-border/60 rounded-lg border p-3'
                aria-labelledby={`routing-tier-${tier.fallback_index}`}
              >
                <div className='mb-3 flex flex-wrap items-center gap-2'>
                  <h3
                    id={`routing-tier-${tier.fallback_index}`}
                    className='font-medium'
                  >
                    {t('Priority {{priority}}', { priority: tier.priority })}
                  </h3>
                  <Badge variant='outline'>
                    {t('Fallback tier {{index}}', {
                      index: tier.fallback_index + 1,
                    })}
                  </Badge>
                </div>
                <div className='space-y-2'>
                  {tier.channels.map((channel) => (
                    <div
                      key={channel.id}
                      className='bg-muted/30 grid gap-2 rounded-md px-3 py-2 text-sm sm:grid-cols-[minmax(0,1fr)_auto_auto_auto] sm:items-center'
                    >
                      <div className='min-w-0'>
                        <div className='truncate font-medium'>
                          {channel.name}
                        </div>
                        <div className='text-muted-foreground text-xs'>
                          #{channel.id} · {t(getChannelTypeLabel(channel.type))}
                        </div>
                      </div>
                      <span>
                        {t('Weight')}: {channel.weight}
                      </span>
                      <span>
                        {t('Effective weight')}: {channel.effective_weight}
                      </span>
                      <span className='font-medium'>
                        {t('Expected share')}:{' '}
                        {percentFormatter.format(channel.expected_share)}
                      </span>
                    </div>
                  ))}
                </div>
              </section>
            ))}
          </div>
        ) : null}

        <p className='text-muted-foreground text-xs'>
          {t(
            'Affinity is checked before priority and weight routing when a request matches an existing cached affinity mapping. This preview does not evaluate, read, or write affinity state.'
          )}
        </p>
      </CardContent>
    </Card>
  )
}
