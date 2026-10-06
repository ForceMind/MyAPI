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
import { toIntlLocale } from '@/i18n/languages'

import { getChannelRoutingPreview } from '../api'
import { channelsQueryKeys, getChannelTypeLabel } from '../lib'
import {
  createChannelRoutingPercentFormatter,
  resolveChannelRoutingPreviewErrorCode,
} from '../lib/channel-routing'
import type { ChannelRoutingPreviewParams } from '../types'

function AccountHoldExpiry(props: { timestamp?: number }) {
  const { t, i18n } = useTranslation()
  const formatter = useMemo(
    () =>
      new Intl.DateTimeFormat(
        toIntlLocale(i18n.resolvedLanguage || i18n.language),
        {
          year: 'numeric',
          month: 'short',
          day: 'numeric',
          hour: 'numeric',
          minute: '2-digit',
          second: '2-digit',
          timeZoneName: 'short',
        }
      ),
    [i18n.language, i18n.resolvedLanguage]
  )
  if (
    props.timestamp == null ||
    !Number.isFinite(props.timestamp) ||
    props.timestamp <= 0 ||
    props.timestamp > 8_640_000_000_000
  ) {
    return null
  }
  return (
    <p className='text-muted-foreground text-xs wrap-break-word'>
      {t('Next account hold expiry: {{time}}', {
        time: formatter.format(props.timestamp * 1000),
      })}
    </p>
  )
}

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
  const rejected = previewData?.rejected ?? []
  const hasAccountCooldown =
    rejected.some((channel) => channel.reason === 'account_cooling_down') ||
    tiers.some((tier) =>
      tier.channels.some((channel) => (channel.cooldown_until ?? 0) > 0)
    )

  let errorMessage = t('Unable to load routing preview.')
  if (errorCode === 'routing_preview_invalid_params') {
    errorMessage = t('Group and model are required.')
  } else if (errorCode === 'routing_preview_auto_group_unsupported') {
    errorMessage = t('Routing preview does not support the auto group.')
  } else if (errorCode === 'routing_preview_permission_denied') {
    errorMessage = t("You don't have necessary permission")
  } else if (errorCode === 'routing_preview_database_error') {
    errorMessage = t('Routing preview is temporarily unavailable.')
  } else if (errorCode === 'routing_preview_model_route_conflict') {
    errorMessage = t(
      'Conflicting model routes have the same match, endpoint, and priority.'
    )
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

        {previewData?.scheduling ? (
          <section
            className='flex min-w-0 flex-col gap-2 text-sm wrap-break-word'
            aria-labelledby='routing-scheduling-title'
          >
            <h3 id='routing-scheduling-title' className='font-medium'>
              {t('Failover scheduling')}
            </h3>
            <p>
              {previewData.scheduling.failover_timeout_seconds === 0
                ? t('Total failover deadline is disabled (0 seconds).')
                : t('Total failover deadline: {{seconds}} seconds', {
                    seconds: previewData.scheduling.failover_timeout_seconds,
                  })}
            </p>
            <p>
              {previewData.scheduling.failure_cooldown_seconds === 0
                ? t('Failure cooldown is disabled (0 seconds).')
                : t('Failure cooldown: {{seconds}} seconds', {
                    seconds: previewData.scheduling.failure_cooldown_seconds,
                  })}
            </p>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Configurable ranges: deadline 0–3600 seconds, cooldown 0–300 seconds. Zero disables each setting.'
              )}
            </p>
            <p className='text-muted-foreground text-xs'>
              {t(
                'These settings are separate from RELAY_TIMEOUT and do not change its legacy behavior.'
              )}
            </p>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Relay attempts share this deadline. Durable cleanup or settlement may finish after cancellation.'
              )}
            </p>
          </section>
        ) : null}

        {errorCode ? (
          <Alert variant='destructive'>
            <AlertDescription>{errorMessage}</AlertDescription>
          </Alert>
        ) : null}

        {previewQuery.isSuccess && !businessError && tiers.length === 0 ? (
          <p className='text-muted-foreground text-sm'>
            {rejected.length > 0
              ? t('No eligible channels remain in this routing snapshot.')
              : t(
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
                        {channel.upstream_model && (
                          <div className='break-all'>
                            {t('Upstream model')}: {channel.upstream_model}
                          </div>
                        )}
                        {channel.request_path && (
                          <div className='break-all'>
                            {t('Endpoint')}: {channel.request_path}
                          </div>
                        )}
                        {channel.route_reason && (
                          <div className='break-all'>
                            {t('Route reason')}: {channel.route_reason}
                          </div>
                        )}
                        {channel.config_digest && (
                          <div className='text-xs break-all'>
                            {t('Configuration digest')}: {channel.config_digest}
                          </div>
                        )}
                        {channel.cooldown_until != null &&
                        channel.cooldown_until > 0 ? (
                          <div className='mt-1 flex min-w-0 flex-col gap-1 wrap-break-word'>
                            <p>
                              {t('Some accounts are temporarily cooling down.')}
                            </p>
                            <AccountHoldExpiry
                              timestamp={channel.cooldown_until}
                            />
                          </div>
                        ) : null}
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

        {rejected.length > 0 ? (
          <section className='min-w-0' aria-labelledby='routing-excluded-title'>
            <h3
              id='routing-excluded-title'
              className='mb-2 text-sm font-medium'
            >
              {t('Excluded channels')}
            </h3>
            <ul
              aria-label={t('Excluded channels')}
              className='flex min-w-0 flex-col gap-2'
            >
              {rejected.map((channel) => (
                <li
                  key={channel.id}
                  className='bg-muted/30 flex min-w-0 flex-col gap-1 rounded-md px-3 py-2 text-sm'
                >
                  <p className='font-medium break-all'>{channel.name}</p>
                  <p className='text-muted-foreground text-xs'>#{channel.id}</p>
                  <p className='wrap-break-word'>
                    {channel.reason === 'account_cooling_down'
                      ? t(
                          'Remaining eligible accounts are temporarily cooling down.'
                        )
                      : t('Excluded from this routing snapshot.')}
                  </p>
                  {channel.reason === 'account_cooling_down' ? (
                    <AccountHoldExpiry timestamp={channel.cooldown_until} />
                  ) : null}
                </li>
              ))}
            </ul>
          </section>
        ) : null}

        {previewData?.scheduling || hasAccountCooldown ? (
          <p className='text-muted-foreground text-xs wrap-break-word'>
            {t(
              'Account cooldown is a temporary hold. Its expiry does not clear authoritative Codex exhaustion or guarantee eligibility; fresh credentials, permissions, budget, and account windows still apply.'
            )}
          </p>
        ) : null}

        {previewData?.affinity?.account_binding === 'confirmed_success_only' ? (
          <p className='text-muted-foreground text-xs wrap-break-word'>
            {t(
              'Account affinity is saved only after confirmed success. This preview has no session input and does not evaluate or update account affinity. Every request still checks current credentials, permissions, budget, and account windows.'
            )}
          </p>
        ) : null}

        <p className='text-muted-foreground text-xs'>
          {t(
            'This saved channel/group/model preview does not verify API Key permissions, strict budget eligibility, prices, login state, account quota, or request success.'
          )}
        </p>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Affinity is checked before priority and weight routing when a request matches an existing cached affinity mapping. This preview does not evaluate, read, or write affinity state.'
          )}
        </p>
      </CardContent>
    </Card>
  )
}
