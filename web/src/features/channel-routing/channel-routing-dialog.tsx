import { Dialog as DialogPrimitive } from '@base-ui/react/dialog'
/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import { Network, RefreshCw } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogDescription,
  DialogHeader,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  getChannelRouting,
  previewChannelRouting,
  updateChannelRoutingPolicy,
  updateRoutingChannel,
} from './api'
import {
  getQuotaStateLabel,
  getRoutingReasonLabel,
  getWorkloadLabel,
} from './labels'
import type {
  ChannelRoutingPolicy,
  RoutingChannel,
  RoutingChannelUpdate,
  RoutingPreviewRequest,
} from './types'

type ChannelDraft = RoutingChannelUpdate

const CHAT_PATH = '/v1/chat/completions'
const WORK_PATH = '/v1/responses'

function getErrorMessage(error: unknown, fallback: string): string {
  if (axios.isAxiosError<{ message?: string }>(error)) {
    return error.response?.data?.message || error.message || fallback
  }
  return error instanceof Error && error.message ? error.message : fallback
}

function ensureSuccess(
  response: { success: boolean; message?: string },
  fallback: string
): void {
  if (!response.success) throw new Error(response.message || fallback)
}

function isConflictError(error: unknown): boolean {
  if (axios.isAxiosError<{ code?: string; message?: string }>(error)) {
    return (
      error.response?.status === 409 ||
      /conflict|changed|version/i.test(
        `${error.response?.data?.code ?? ''} ${error.response?.data?.message ?? ''}`
      )
    )
  }
  return (
    error instanceof Error && /conflict|changed|version/i.test(error.message)
  )
}

function formatAvailable(available?: number, unit?: string): string | null {
  if (typeof available !== 'number') return null
  if (unit === 'percent') return `${available.toLocaleString()}%`
  return `${available.toLocaleString()}${unit ? ` ${unit}` : ''}`
}

function formatQuota(channel: RoutingChannel): string | null {
  return formatAvailable(channel.quota.available, channel.quota.unit)
}

function formatShare(share: number): string {
  const formatter = new Intl.NumberFormat(undefined, {
    style: 'percent',
    maximumFractionDigits: share > 0 && share < 0.01 ? 2 : 1,
    minimumFractionDigits: share > 0 && share < 0.01 ? 2 : 0,
  })
  if (share > 0 && share < 0.0001) {
    return `<${formatter.format(0.0001)}`
  }
  return formatter.format(share)
}

function quotaVariant(state: RoutingChannel['quota']['state']) {
  if (state === 'exhausted') return 'destructive' as const
  if (state === 'stale') return 'warning' as const
  return 'outline' as const
}

function localPolicy(policy: ChannelRoutingPolicy): ChannelRoutingPolicy {
  return { ...policy }
}

function policiesMatch(
  first: ChannelRoutingPolicy,
  second: ChannelRoutingPolicy
): boolean {
  return (
    first.enabled === second.enabled &&
    first.sticky_enabled === second.sticky_enabled &&
    first.session_ttl_seconds === second.session_ttl_seconds &&
    first.quota_max_age_seconds === second.quota_max_age_seconds
  )
}

const routingPriorityOptions = [
  { value: 10, label: 'Preferred' },
  { value: 0, label: 'Standard' },
  { value: -10, label: 'Backup' },
] as const

function hasStandardPriority(priority: number): boolean {
  return routingPriorityOptions.some((option) => option.value === priority)
}

function getTrafficShare(
  channels: RoutingChannel[],
  drafts: Record<number, ChannelDraft>,
  channel: RoutingChannel
): number | null {
  const priorityValue = (drafts[channel.id] ?? channel).priority
  const priority =
    priorityValue === channel.priority
      ? (channel.legacy_priority ?? String(priorityValue))
      : String(priorityValue)
  const weight = (drafts[channel.id] ?? channel).weight
  if (channel.status !== 1 || !Number.isFinite(weight) || weight <= 0) {
    return null
  }

  const totalWeight = channels.reduce((total, currentChannel) => {
    const currentDraft = drafts[currentChannel.id] ?? currentChannel
    if (
      currentChannel.status !== 1 ||
      (currentDraft.priority === currentChannel.priority
        ? (currentChannel.legacy_priority ?? String(currentDraft.priority))
        : String(currentDraft.priority)) !== priority ||
      !Number.isFinite(currentDraft.weight) ||
      currentDraft.weight <= 0
    ) {
      return total
    }
    return total + currentDraft.weight
  }, 0)

  return totalWeight > 0 ? weight / totalWeight : null
}

export interface ChannelRoutingPanelProps {
  active?: boolean
  onClose?: () => void
}

export function ChannelRoutingPanel(props: ChannelRoutingPanelProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid)
  const isRoot = currentUser?.role === ROLE.SUPER_ADMIN
  const canEditChannels = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.WRITE
  )
  const queryKey = useMemo(
    () => ['channel-routing', currentUser?.id ?? null, sessionId ?? null],
    [currentUser?.id, sessionId]
  )
  const sessionIdentity = `${currentUser?.id ?? ''}:${sessionId ?? ''}`
  const routingQuery = useQuery({
    queryKey,
    queryFn: async () => {
      const response = await getChannelRouting()
      ensureSuccess(response, t('Unable to load traffic allocation settings'))
      if (!response.data) {
        throw new Error(t('Traffic allocation settings are unavailable'))
      }
      return response.data
    },
    enabled: (props.active ?? true) && Boolean(currentUser && sessionId),
    retry: false,
  })
  const [policy, setPolicy] = useState<ChannelRoutingPolicy | null>(null)
  const policyRef = useRef<ChannelRoutingPolicy | null>(null)
  const policyBaselineRef = useRef<ChannelRoutingPolicy | null>(null)
  const sessionIdentityRef = useRef(sessionIdentity)
  const [channelDrafts, setChannelDrafts] = useState<
    Record<number, ChannelDraft>
  >({})
  const [previewRequest, setPreviewRequest] = useState<RoutingPreviewRequest>({
    model: '',
    group: '',
    path: CHAT_PATH,
  })
  const [policyError, setPolicyError] = useState<string | null>(null)
  const [channelError, setChannelError] = useState<string | null>(null)
  const [previewIsStale, setPreviewIsStale] = useState(false)

  useEffect(() => {
    if (sessionIdentityRef.current === sessionIdentity) return
    sessionIdentityRef.current = sessionIdentity
    policyRef.current = null
    policyBaselineRef.current = null
    setPolicy(null)
    setChannelDrafts({})
    setPolicyError(null)
    setChannelError(null)
    setPreviewIsStale(false)
    setPreviewRequest({ model: '', group: '', path: CHAT_PATH })
  }, [sessionIdentity])

  useEffect(() => {
    if (!routingQuery.data) return
    const currentPolicy = policyRef.current
    const currentBaseline = policyBaselineRef.current
    if (
      !currentPolicy ||
      !currentBaseline ||
      policiesMatch(currentPolicy, currentBaseline)
    ) {
      const nextPolicy = localPolicy(routingQuery.data.policy)
      policyRef.current = nextPolicy
      policyBaselineRef.current = localPolicy(nextPolicy)
      setPolicy(nextPolicy)
    }
    setChannelDrafts((current) => {
      const currentChannels = new Map(
        routingQuery.data.channels.map((channel) => [channel.id, channel])
      )
      const next: Record<number, ChannelDraft> = {}
      for (const [channelId, draft] of Object.entries(current)) {
        if (!currentChannels.has(Number(channelId))) continue
        const isDirty =
          draft.priority !== draft.expected_priority ||
          draft.weight !== draft.expected_weight ||
          !!draft.expected_legacy_priority ||
          !!draft.expected_legacy_weight
        if (!isDirty) continue
        next[Number(channelId)] = draft
      }
      return next
    })
  }, [routingQuery.data])

  const policyMutation = useMutation({
    mutationFn: async (nextPolicy: ChannelRoutingPolicy) => {
      const response = await updateChannelRoutingPolicy(nextPolicy)
      ensureSuccess(response, t('Unable to save traffic allocation settings'))
    },
    onSuccess: (_, savedPolicy) => {
      setPolicyError(null)
      setPreviewIsStale(true)
      const nextBaseline = localPolicy(savedPolicy)
      policyBaselineRef.current = nextBaseline
      void queryClient.invalidateQueries({ queryKey })
    },
    onError: (error) =>
      setPolicyError(getErrorMessage(error, t('Save failed'))),
  })
  const channelMutation = useMutation({
    mutationFn: async (values: {
      channel: RoutingChannel
      draft: ChannelDraft
    }) => {
      const response = await updateRoutingChannel(values.channel.id, {
        priority: values.draft.priority,
        weight: values.draft.weight,
        expected_priority: values.draft.expected_priority,
        expected_weight: values.draft.expected_weight,
        ...(values.draft.expected_legacy_priority
          ? { expected_legacy_priority: values.draft.expected_legacy_priority }
          : {}),
        ...(values.draft.expected_legacy_weight
          ? { expected_legacy_weight: values.draft.expected_legacy_weight }
          : {}),
      })
      ensureSuccess(response, t('Unable to save channel allocation'))
    },
    onSuccess: (_, values) => {
      setChannelError(null)
      setPreviewIsStale(true)
      setChannelDrafts((current) => {
        const next = { ...current }
        delete next[values.channel.id]
        return next
      })
      void queryClient.invalidateQueries({ queryKey })
    },
    onError: async (error, values) => {
      if (isConflictError(error)) {
        setPreviewIsStale(true)
        setChannelError(
          t(
            'This channel changed elsewhere. Current values were reloaded; your edits are still shown.'
          )
        )
        const result = await routingQuery.refetch()
        const freshChannel = result.data?.channels.find(
          (channel) => channel.id === values.channel.id
        )
        if (freshChannel) {
          setChannelDrafts((current) => ({
            ...current,
            [freshChannel.id]: {
              ...values.draft,
              expected_priority: freshChannel.priority,
              expected_weight: freshChannel.weight,
              expected_legacy_priority: freshChannel.legacy_priority,
              expected_legacy_weight: freshChannel.legacy_weight,
            },
          }))
        }
        return
      }
      setChannelError(getErrorMessage(error, t('Save failed')))
    },
  })
  const previewMutation = useMutation({
    mutationFn: async (request: RoutingPreviewRequest) => {
      const response = await previewChannelRouting(request)
      ensureSuccess(response, t('Unable to preview traffic allocation'))
      if (!response.data) {
        throw new Error(t('Traffic allocation preview is unavailable'))
      }
      return response.data
    },
  })

  const updatePolicy = <K extends keyof ChannelRoutingPolicy>(
    key: K,
    value: ChannelRoutingPolicy[K]
  ) => {
    setPreviewIsStale(true)
    setPolicy((current) => {
      if (!current) return current
      const nextPolicy = { ...current, [key]: value }
      policyRef.current = nextPolicy
      return nextPolicy
    })
  }
  const updateChannelDraft = (
    channel: RoutingChannel,
    key: 'priority' | 'weight',
    value: number
  ) => {
    setPreviewIsStale(true)
    setChannelDrafts((current) => {
      const draft = current[channel.id] ?? {
        priority: channel.priority,
        weight: channel.weight,
        expected_priority: channel.priority,
        expected_weight: channel.weight,
        ...(channel.legacy_weight
          ? { expected_legacy_weight: channel.legacy_weight }
          : {}),
        ...(channel.legacy_priority
          ? { expected_legacy_priority: channel.legacy_priority }
          : {}),
      }
      return { ...current, [channel.id]: { ...draft, [key]: value } }
    })
  }
  const handlePolicySave = () => {
    if (!policy) return
    if (
      !Number.isSafeInteger(policy.session_ttl_seconds) ||
      !Number.isSafeInteger(policy.quota_max_age_seconds) ||
      policy.session_ttl_seconds < 3600 ||
      policy.session_ttl_seconds > 2592000 ||
      policy.quota_max_age_seconds < 60 ||
      policy.quota_max_age_seconds > 86400
    ) {
      setPolicyError(t('Use a value within the displayed limits.'))
      return
    }
    setPolicyError(null)
    policyMutation.mutate(policy)
  }
  const handlePreview = () => {
    previewMutation.reset()
    previewMutation.mutate(previewRequest, {
      onSuccess: () => setPreviewIsStale(false),
    })
  }

  const policyHasUnsavedChanges =
    !!policyBaselineRef.current &&
    !!policy &&
    !policiesMatch(policy, policyBaselineRef.current)
  let policySaveStatus = t('Saved successfully')
  if (policyHasUnsavedChanges) policySaveStatus = t('Unsaved changes')
  if (policyMutation.isPending) policySaveStatus = t('Saving…')

  return (
    <div className='grid gap-6'>
      {routingQuery.isPending && (
        <p className='text-muted-foreground'>
          {t('Loading traffic allocation settings…')}
        </p>
      )}
      {routingQuery.isError && (
        <Alert variant='destructive'>
          <AlertTitle>
            {t('Unable to load traffic allocation settings')}
          </AlertTitle>
          <AlertDescription className='flex items-center justify-between gap-2'>
            <span>
              {getErrorMessage(routingQuery.error, t('Please try again.'))}
            </span>
            <Button
              size='sm'
              variant='outline'
              onClick={() => routingQuery.refetch()}
            >
              <RefreshCw data-icon='inline-start' />
              {t('Retry')}
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {routingQuery.data && policy && (
        <>
          <section
            className='grid gap-3 rounded-lg border p-4'
            aria-labelledby='routing-policy-title'
          >
            <div>
              <h3 id='routing-policy-title' className='font-medium'>
                {t('Policy')}
              </h3>
              {!isRoot && (
                <p className='text-muted-foreground text-sm'>
                  {t('Only the root administrator can change this policy.')}
                </p>
              )}
            </div>
            <div className='grid gap-3 sm:grid-cols-2'>
              <Label
                htmlFor='routing-enabled'
                className='justify-between rounded-md border p-3'
              >
                <span>{t('Enable intelligent allocation')}</span>
                <Switch
                  id='routing-enabled'
                  checked={policy.enabled}
                  disabled={!isRoot || policyMutation.isPending}
                  onCheckedChange={(checked) =>
                    updatePolicy('enabled', checked)
                  }
                />
              </Label>
              <Label
                htmlFor='routing-sticky'
                className='justify-between rounded-md border p-3'
              >
                <span>{t('Keep sessions on a channel')}</span>
                <Switch
                  id='routing-sticky'
                  checked={policy.sticky_enabled}
                  disabled={!isRoot || policyMutation.isPending}
                  onCheckedChange={(checked) =>
                    updatePolicy('sticky_enabled', checked)
                  }
                />
              </Label>
            </div>
            <p className='text-muted-foreground text-sm'>
              {t(
                'A session stays on its normal channel. Failures or exhausted quota can switch it and record the reason.'
              )}
            </p>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Chat uses /v1/chat/completions and /pg/chat/completions, favoring lower comparable remaining quota. Work uses /v1/responses, favoring higher comparable remaining quota.'
              )}
            </p>
            <details className='rounded-md border p-3'>
              <summary className='cursor-pointer font-medium'>
                {t('Advanced settings')}
              </summary>
              <div className='mt-3 grid gap-3 sm:grid-cols-2'>
                <div className='grid gap-1.5'>
                  <Label htmlFor='routing-session-ttl'>
                    {t('Session lifetime (seconds)')}
                  </Label>
                  <Input
                    id='routing-session-ttl'
                    type='number'
                    min={3600}
                    max={2592000}
                    value={policy.session_ttl_seconds}
                    disabled={!isRoot || policyMutation.isPending}
                    onChange={(event) =>
                      updatePolicy(
                        'session_ttl_seconds',
                        Number(event.target.value)
                      )
                    }
                  />
                </div>
                <div className='grid gap-1.5'>
                  <Label htmlFor='routing-quota-age'>
                    {t('Quota freshness (seconds)')}
                  </Label>
                  <Input
                    id='routing-quota-age'
                    type='number'
                    min={60}
                    max={86400}
                    value={policy.quota_max_age_seconds}
                    disabled={!isRoot || policyMutation.isPending}
                    onChange={(event) =>
                      updatePolicy(
                        'quota_max_age_seconds',
                        Number(event.target.value)
                      )
                    }
                  />
                </div>
              </div>
            </details>
            {policyError && (
              <p className='text-destructive text-sm'>{policyError}</p>
            )}
            {isRoot && !policyError ? (
              <p className='text-muted-foreground text-xs' role='status'>
                {policySaveStatus}
              </p>
            ) : null}
          </section>

          <section
            className='grid gap-3'
            aria-labelledby='routing-channels-title'
          >
            <div>
              <h3 id='routing-channels-title' className='font-medium'>
                {t('Channel allocation')}
              </h3>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Configured shares exclude disabled channels. Use preview for the actual model and group.'
                )}
              </p>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Choose a routing order and set each channel’s traffic share. Channel status is managed in Channel Management.'
                )}
              </p>
            </div>
            {channelError && (
              <p className='text-destructive text-sm'>{channelError}</p>
            )}
            {routingQuery.data.channels.length === 0 ? (
              <p className='text-muted-foreground rounded-md border p-4 text-sm'>
                {t('No channels are available for traffic allocation.')}
              </p>
            ) : (
              <div className='rounded-lg border'>
                <table className='block w-full text-sm sm:table'>
                  <thead className='bg-muted/50 sr-only text-left sm:not-sr-only sm:table-header-group'>
                    <tr>
                      <th className='w-[24%] p-2 font-medium sm:p-3'>
                        {t('Channel')}
                      </th>
                      <th className='w-[20%] p-2 font-medium sm:p-3'>
                        {t('Quota')}
                      </th>
                      <th className='w-[20%] p-2 font-medium sm:p-3'>
                        {t('Routing order')}
                      </th>
                      <th className='w-[20%] p-2 font-medium sm:p-3'>
                        {t('Traffic share')}
                      </th>
                      <th className='w-[16%] p-2 font-medium sm:p-3'>
                        <span className='sr-only'>{t('Save')}</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody className='block sm:table-row-group'>
                    {routingQuery.data.channels.map((channel) => {
                      const draft = channelDrafts[channel.id] ?? {
                        priority: channel.priority,
                        weight: channel.weight,
                        expected_priority: channel.priority,
                        expected_weight: channel.weight,
                        ...(channel.legacy_weight
                          ? {
                              expected_legacy_weight: channel.legacy_weight,
                            }
                          : {}),
                        ...(channel.legacy_priority
                          ? {
                              expected_legacy_priority: channel.legacy_priority,
                            }
                          : {}),
                      }
                      const changed =
                        draft.priority !== draft.expected_priority ||
                        draft.weight !== draft.expected_weight
                      const isSavingThisChannel =
                        channelMutation.isPending &&
                        channelMutation.variables?.channel.id === channel.id
                      let channelSaveStatus = t('Saved successfully')
                      if (changed) channelSaveStatus = t('Unsaved changes')
                      if (isSavingThisChannel) channelSaveStatus = t('Saving…')
                      const priorityValue = hasStandardPriority(draft.priority)
                        ? String(draft.priority)
                        : 'existing'
                      const currentPriority =
                        channel.legacy_priority ?? String(channel.priority)
                      const trafficShare = getTrafficShare(
                        routingQuery.data.channels,
                        channelDrafts,
                        channel
                      )
                      let shareLabel = t(
                        'No traffic share is configured for this order.'
                      )
                      if (channel.status !== 1) shareLabel = t('Disabled')
                      else if (trafficShare !== null) {
                        shareLabel = t('Traffic share: {{share}}', {
                          share: formatShare(trafficShare),
                        })
                      }
                      return (
                        <tr
                          key={channel.id}
                          className='grid grid-cols-2 gap-x-3 gap-y-2 border-t p-3 first:border-t-0 sm:table-row sm:p-0 sm:first:border-t'
                        >
                          <td className='col-span-2 min-w-0 sm:p-3'>
                            <div className='font-medium wrap-anywhere'>
                              {channel.name}
                            </div>
                            <div className='text-muted-foreground text-xs'>
                              #{channel.id}
                            </div>
                          </td>
                          <td className='col-span-2 min-w-0 sm:p-3'>
                            <div className='flex flex-wrap items-center gap-1.5'>
                              <Badge
                                variant={quotaVariant(channel.quota.state)}
                              >
                                {getQuotaStateLabel(t, channel.quota.state)}
                              </Badge>
                              {formatQuota(channel) && (
                                <span>{formatQuota(channel)}</span>
                              )}
                            </div>
                          </td>
                          <td className='min-w-0 sm:p-3'>
                            <Label
                              className='mb-1.5 sm:hidden'
                              htmlFor={`routing-priority-${channel.id}`}
                            >
                              {t('Routing order')}
                            </Label>
                            <select
                              id={`routing-priority-${channel.id}`}
                              aria-label={`${t('Routing order')} ${channel.name}`}
                              className='border-input h-8 w-full rounded-lg border bg-transparent px-2.5 text-sm'
                              value={priorityValue}
                              disabled={
                                !canEditChannels || channelMutation.isPending
                              }
                              onChange={(event) =>
                                event.target.value !== 'existing' &&
                                updateChannelDraft(
                                  channel,
                                  'priority',
                                  Number(event.target.value)
                                )
                              }
                            >
                              {routingPriorityOptions.map((option) => (
                                <option key={option.value} value={option.value}>
                                  {t(option.label)}
                                </option>
                              ))}
                              {!hasStandardPriority(draft.priority) && (
                                <option value='existing'>
                                  {t('Existing order')}
                                </option>
                              )}
                            </select>
                            {!hasStandardPriority(draft.priority) && (
                              <p className='text-muted-foreground mt-1 text-xs wrap-anywhere'>
                                {t(
                                  'Existing order: {{priority}}. Choose a routing order to replace it.',
                                  { priority: currentPriority }
                                )}
                              </p>
                            )}
                          </td>
                          <td className='min-w-0 sm:p-3'>
                            <Label
                              className='mb-1.5 sm:hidden'
                              htmlFor={`routing-weight-${channel.id}`}
                            >
                              {t('Traffic share')}
                            </Label>
                            <Input
                              id={`routing-weight-${channel.id}`}
                              aria-label={`${t('Traffic share')} ${channel.name}`}
                              type='number'
                              min={1}
                              max={1000000}
                              value={draft.weight > 0 ? draft.weight : ''}
                              disabled={
                                !canEditChannels || channelMutation.isPending
                              }
                              onChange={(event) =>
                                updateChannelDraft(
                                  channel,
                                  'weight',
                                  Number(event.target.value)
                                )
                              }
                            />
                            <p className='text-muted-foreground mt-1 text-xs'>
                              {shareLabel}
                            </p>
                          </td>
                          <td className='col-span-2 text-right sm:p-3'>
                            <p
                              className='text-muted-foreground mb-1 text-xs'
                              role='status'
                            >
                              {channelSaveStatus}
                            </p>
                            <Button
                              className='w-full sm:w-auto'
                              size='sm'
                              variant='outline'
                              disabled={
                                !canEditChannels ||
                                !changed ||
                                channelMutation.isPending
                              }
                              onClick={() => {
                                if (
                                  !Number.isSafeInteger(draft.priority) ||
                                  !Number.isSafeInteger(draft.weight) ||
                                  draft.priority < -1000000 ||
                                  draft.priority > 1000000 ||
                                  draft.weight < 1 ||
                                  (!!channel.legacy_priority &&
                                    !hasStandardPriority(draft.priority)) ||
                                  draft.weight > 1000000
                                ) {
                                  setChannelError(
                                    t(
                                      'Choose a routing order and a positive traffic share within the displayed limits.'
                                    )
                                  )
                                  return
                                }
                                setChannelError(null)
                                channelMutation.mutate({ channel, draft })
                              }}
                            >
                              {t('Save')}
                            </Button>
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </section>

          <section
            className='grid gap-3 rounded-lg border p-4'
            aria-labelledby='routing-preview-title'
          >
            <div>
              <h3 id='routing-preview-title' className='font-medium'>
                {t('Routing preview')}
              </h3>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'The preview uses the current server policy and does not send a request upstream.'
                )}
              </p>
            </div>
            <div className='grid gap-3 sm:grid-cols-3'>
              <div className='grid gap-1.5'>
                <Label htmlFor='routing-preview-kind'>
                  {t('Request type')}
                </Label>
                <select
                  id='routing-preview-kind'
                  aria-label={t('Request type')}
                  className='border-input h-8 rounded-lg border bg-transparent px-2.5 text-sm'
                  value={previewRequest.path}
                  onChange={(event) =>
                    setPreviewRequest((current) => ({
                      ...current,
                      path: event.target.value,
                    }))
                  }
                >
                  <option value={CHAT_PATH}>{t('Chat')}</option>
                  <option value={WORK_PATH}>{t('Work')}</option>
                </select>
              </div>
              <div className='grid gap-1.5'>
                <Label htmlFor='routing-preview-model'>{t('Model')}</Label>
                <Input
                  id='routing-preview-model'
                  value={previewRequest.model}
                  onChange={(event) =>
                    setPreviewRequest((current) => ({
                      ...current,
                      model: event.target.value,
                    }))
                  }
                />
              </div>
              <div className='grid gap-1.5'>
                <Label htmlFor='routing-preview-group'>{t('Group')}</Label>
                <Input
                  id='routing-preview-group'
                  value={previewRequest.group}
                  onChange={(event) =>
                    setPreviewRequest((current) => ({
                      ...current,
                      group: event.target.value,
                    }))
                  }
                />
              </div>
            </div>
            <p className='text-muted-foreground font-mono text-xs'>
              {previewRequest.path}
            </p>
            <div>
              <Button
                size='sm'
                onClick={handlePreview}
                disabled={previewMutation.isPending}
              >
                {previewMutation.isPending
                  ? t('Loading preview…')
                  : t('Preview routing')}
              </Button>
            </div>
            {previewMutation.isError && (
              <p className='text-destructive text-sm'>
                {getErrorMessage(
                  previewMutation.error,
                  t('Unable to preview traffic allocation')
                )}
              </p>
            )}
            {previewMutation.data && (
              <div className='bg-muted/50 grid gap-2 rounded-md p-3 text-sm'>
                {previewIsStale ? (
                  <p className='text-muted-foreground'>
                    {t(
                      'Configuration changed after this preview. Preview again to use the saved configuration.'
                    )}
                  </p>
                ) : null}
                <div className='flex flex-wrap items-center gap-2'>
                  <Badge variant='outline'>
                    {getWorkloadLabel(t, previewMutation.data.workload)}
                  </Badge>
                  <span>
                    <strong>{t('Reason')}:</strong>{' '}
                    {getRoutingReasonLabel(t, previewMutation.data.reason)}
                  </span>
                </div>
                {previewMutation.data.candidates.length === 0 ? (
                  <p className='text-muted-foreground'>
                    {t('No matching channels for this preview.')}
                  </p>
                ) : (
                  previewMutation.data.candidates.map((candidate) => (
                    <div
                      key={candidate.id}
                      className='flex flex-wrap items-center justify-between gap-2 border-t pt-2'
                    >
                      <span>
                        {candidate.name}{' '}
                        <span className='text-muted-foreground'>
                          #{candidate.id}
                        </span>
                      </span>
                      <span>
                        {formatShare(candidate.share)} ·{' '}
                        {getRoutingReasonLabel(t, candidate.reason)}
                        {formatAvailable(candidate.available, candidate.unit)
                          ? ` · ${formatAvailable(candidate.available, candidate.unit)}`
                          : ''}
                      </span>
                    </div>
                  ))
                )}
              </div>
            )}
          </section>
        </>
      )}
      {(props.onClose || isRoot) && (
        <div className='flex flex-wrap justify-end gap-2'>
          {props.onClose && (
            <Button variant='outline' onClick={props.onClose}>
              {t('Close')}
            </Button>
          )}
          {isRoot && (
            <Button
              onClick={handlePolicySave}
              disabled={!policy || policyMutation.isPending}
            >
              {policyMutation.isPending ? t('Saving…') : t('Save policy')}
            </Button>
          )}
        </div>
      )}
    </div>
  )
}

export function ChannelRoutingDialog() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  return (
    <>
      <Button variant='outline' size='sm' onClick={() => setOpen(true)}>
        <Network data-icon='inline-start' />
        {t('Traffic Allocation')}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogPortal keepMounted>
          <DialogOverlay />
          <DialogPrimitive.Popup className='bg-popover text-popover-foreground ring-foreground/10 data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95 fixed top-1/2 left-1/2 z-50 grid max-h-[calc(100dvh-2rem)] w-full max-w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 gap-4 overflow-y-auto rounded-xl p-4 text-sm ring-1 duration-100 outline-none sm:max-w-4xl'>
            <DialogHeader>
              <DialogTitle>{t('Traffic Allocation')}</DialogTitle>
              <DialogDescription>
                {t(
                  'Choose Preferred, Standard, or Backup. Channels in the same order split traffic by share.'
                )}
              </DialogDescription>
            </DialogHeader>
            <ChannelRoutingPanel active={open} onClose={() => setOpen(false)} />
          </DialogPrimitive.Popup>
        </DialogPortal>
      </Dialog>
    </>
  )
}
