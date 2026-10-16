/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import type { TFunction } from 'i18next'
import { AlertTriangle, Check, Plus, RotateCcw, X } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuthStore } from '@/stores/auth-store'

import { useKeyMaintenanceSession } from '../hooks/use-key-maintenance-session'
import {
  modelScopeCatalog,
  modelScopeCommand,
  modelScopeDraft,
  modelScopeListItems,
  modelScopeMatchesCommand,
  sameModelScopeDraft,
  validateModelScopeDraft,
  type ModelScopeDraft,
  type ModelScopeMode,
  type ModelScopeValidationError,
} from '../lib/model-scope'
import {
  getModelScope,
  getModelScopeCatalog,
  updateModelScope,
  type ModelScope,
} from '../model-scope-api'
import type { ApiKey } from '../types'
import { useApiKeys } from './api-keys-provider'

type ModelScopeDialogProps = {
  apiKey: ApiKey
  onClose: () => void
}

type ModelScopeSessionProps = ModelScopeDialogProps & {
  userId: number
  role: number
  sid: string | null
}

type SaveError =
  | 'conflict'
  | 'invalid'
  | 'unknown'
  | 'changed'
  | 'unauthorized'
  | null

export function ModelScopeDialog(props: ModelScopeDialogProps) {
  const actor = useAuthStore((state) => state.auth.user)
  const sid = useAuthStore((state) => state.auth.session?.sid ?? null)
  if (!actor) return null
  return (
    <ModelScopeSession
      key={`${actor.id}:${actor.role}:${sid}:${props.apiKey.id}`}
      {...props}
      userId={actor.id}
      role={actor.role}
      sid={sid}
    />
  )
}

function validationMessage(
  error: ModelScopeValidationError,
  t: TFunction
): string | null {
  if (error === 'empty-restricted-scope') {
    return t('Select at least one model, or choose a different scope.')
  }
  if (error === 'invalid-new-model') {
    return t('Model IDs cannot contain commas, spaces, or control characters.')
  }
  if (error === 'duplicate-model') {
    return t('Remove duplicate model IDs before saving.')
  }
  if (error === 'scope-too-large') {
    return t('The model scope is too large to save.')
  }
  return null
}

function saveErrorTitle(error: Exclude<SaveError, null>, t: TFunction) {
  if (error === 'conflict') {
    return t('The model scope changed in another operation')
  }
  if (error === 'invalid') return t('The model scope is invalid')
  if (error === 'unknown') return t('The save result could not be confirmed')
  if (error === 'changed') {
    return t('The current server scope differs from your selection')
  }
  return t('Your session could not authorize this change')
}

function saveErrorDescription(error: Exclude<SaveError, null>, t: TFunction) {
  if (error === 'conflict' || error === 'changed') {
    return t(
      'The current server scope is shown. Review it before saving again.'
    )
  }
  if (error === 'unknown') {
    return t('Reload the current scope before deciding whether to try again.')
  }
  return t('No automatic retry was attempted.')
}

function ModelScopeSession(props: ModelScopeSessionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isCurrentSession = useKeyMaintenanceSession()
  const { triggerRefresh } = useApiKeys()
  const [draft, setDraft] = useState<ModelScopeDraft | null>(null)
  const [loadedRevision, setLoadedRevision] = useState('')
  const [search, setSearch] = useState('')
  const [blockConfirmed, setBlockConfirmed] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<SaveError>(null)
  const [needsReadback, setNeedsReadback] = useState(false)
  const writeController = useRef<AbortController | null>(null)
  const scopeQueryKey = [
    'api-key-model-scope',
    props.userId,
    props.role,
    props.sid,
    props.apiKey.id,
  ] as const

  useEffect(() => {
    return () => writeController.current?.abort()
  }, [])

  const scopeQuery = useQuery({
    queryKey: scopeQueryKey,
    queryFn: async ({ signal }) => {
      const scope = await getModelScope(props.apiKey.id, signal)
      if (!isCurrentSession()) throw new Error('Session changed')
      return scope
    },
    retry: false,
  })
  const catalogQuery = useQuery({
    queryKey: [
      'api-key-model-scope-catalog',
      props.userId,
      props.role,
      props.sid,
      props.apiKey.group || '',
    ],
    queryFn: async ({ signal }) => {
      const catalog = await getModelScopeCatalog(
        props.apiKey.group || '',
        signal
      )
      if (!isCurrentSession()) throw new Error('Session changed')
      return catalog
    },
    retry: false,
  })

  useEffect(() => {
    if (!scopeQuery.data || scopeQuery.data.revision === loadedRevision) return
    setDraft(modelScopeDraft(scopeQuery.data))
    setLoadedRevision(scopeQuery.data.revision)
    setBlockConfirmed(false)
    setNeedsReadback(false)
  }, [loadedRevision, scopeQuery.data])

  const originalDraft = scopeQuery.data
    ? modelScopeDraft(scopeQuery.data)
    : null
  const validationError =
    draft && scopeQuery.data
      ? validateModelScopeDraft(draft, scopeQuery.data.models)
      : null
  const dirty = Boolean(
    draft && originalDraft && !sameModelScopeDraft(draft, originalDraft)
  )
  const canSave = Boolean(
    draft &&
    dirty &&
    !validationError &&
    (draft.mode !== 'blocked' || blockConfirmed) &&
    !needsReadback &&
    !saving
  )
  const candidates = useMemo(
    () =>
      modelScopeCatalog(catalogQuery.data ?? [], draft?.models ?? [], search),
    [catalogQuery.data, draft?.models, search]
  )
  const catalogSet = useMemo(
    () => new Set(catalogQuery.data ?? []),
    [catalogQuery.data]
  )
  const selectedModels = useMemo(
    () => modelScopeListItems(draft?.models ?? []),
    [draft?.models]
  )

  const applyServerScope = (scope: ModelScope) => {
    queryClient.setQueryData(scopeQueryKey, scope)
    setDraft(modelScopeDraft(scope))
    setLoadedRevision(scope.revision)
    setBlockConfirmed(false)
    setNeedsReadback(false)
  }

  const readCurrentScope = async (
    signal: AbortSignal
  ): Promise<ModelScope | null> => {
    try {
      const scope = await getModelScope(props.apiKey.id, signal)
      if (!isCurrentSession()) return null
      applyServerScope(scope)
      return scope
    } catch {
      return null
    }
  }

  const save = async () => {
    if (!scopeQuery.data || !draft || !canSave || !isCurrentSession()) return
    const command = modelScopeCommand(scopeQuery.data, draft)
    setSaving(true)
    setSaveError(null)
    const controller = new AbortController()
    writeController.current = controller
    try {
      await updateModelScope(props.apiKey.id, command, controller.signal)
      if (!isCurrentSession()) return
      const current = await readCurrentScope(controller.signal)
      if (!isCurrentSession()) return
      if (current && modelScopeMatchesCommand(current, command)) {
        triggerRefresh()
        toast.success(t('Model scope saved'))
        props.onClose()
        return
      }
      if (!current) setNeedsReadback(true)
      setSaveError(current ? 'changed' : 'unknown')
    } catch (error) {
      if (!isCurrentSession() || controller.signal.aborted) return
      const status = axios.isAxiosError(error)
        ? error.response?.status
        : undefined
      if (status === 400) {
        setSaveError('invalid')
      } else if (status === 401) {
        setNeedsReadback(true)
        setSaveError('unauthorized')
      } else if (status === 409) {
        const current = await readCurrentScope(controller.signal)
        if (isCurrentSession()) {
          if (!current) setNeedsReadback(true)
          setSaveError(current ? 'conflict' : 'unknown')
        }
      } else {
        const current = await readCurrentScope(controller.signal)
        if (!isCurrentSession()) return
        if (current && modelScopeMatchesCommand(current, command)) {
          triggerRefresh()
          toast.success(t('Model scope saved'))
          props.onClose()
          return
        }
        if (!current) setNeedsReadback(true)
        setSaveError(current ? 'changed' : 'unknown')
      }
    } finally {
      if (writeController.current === controller) {
        writeController.current = null
      }
      if (isCurrentSession()) setSaving(false)
    }
  }

  const setMode = (mode: ModelScopeMode) => {
    setDraft((current) => (current ? { ...current, mode } : current))
    setBlockConfirmed(false)
    if (!needsReadback) setSaveError(null)
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !saving) props.onClose()
      }}
    >
      <DialogContent
        className='max-h-[90dvh] min-w-0 overflow-y-auto sm:max-w-2xl'
        showCloseButton={false}
      >
        <DialogHeader>
          <DialogTitle>{t('Model scope')}</DialogTitle>
          <DialogDescription>
            {t(
              'Set an additional model limit for this Key. Account, assigned access, and route restrictions still apply.'
            )}
          </DialogDescription>
        </DialogHeader>

        {scopeQuery.isPending && (
          <div role='status' className='flex flex-col gap-3'>
            <span className='sr-only'>{t('Loading model scope...')}</span>
            <Skeleton className='h-20 rounded-xl' />
            <Skeleton className='h-48 rounded-xl' />
          </div>
        )}
        {scopeQuery.isError && (
          <Alert variant='destructive'>
            <AlertTitle>{t('Could not load model scope')}</AlertTitle>
            <AlertDescription className='mt-2'>
              <Button
                variant='outline'
                disabled={scopeQuery.isFetching}
                onClick={() => void scopeQuery.refetch()}
              >
                <RotateCcw data-icon='inline-start' />
                {t('Retry')}
              </Button>
            </AlertDescription>
          </Alert>
        )}

        {scopeQuery.data && draft && (
          <FieldGroup>
            <section className='border-primary/20 bg-primary/5 dark:bg-primary/10 min-w-0 rounded-xl border p-4'>
              <p className='font-medium break-words'>
                {scopeQuery.data.token_name || props.apiKey.name}
              </p>
              <p className='text-muted-foreground mt-1 font-mono text-xs'>
                {t('Key ID')}: {scopeQuery.data.token_id}
              </p>
            </section>

            <Alert>
              <AlertTitle>{t('This scope only narrows access')}</AlertTitle>
              <AlertDescription>
                {t(
                  'The model catalog is a current suggestion list. A selected model may still be unavailable because of account, access profile, provider, or route restrictions.'
                )}
              </AlertDescription>
            </Alert>

            <FieldSet disabled={saving}>
              <FieldLegend>{t('Allowed models')}</FieldLegend>
              <RadioGroup
                value={draft.mode}
                onValueChange={(value) => setMode(value as ModelScopeMode)}
              >
                <FieldLabel htmlFor='model-scope-unlimited'>
                  <Field orientation='horizontal'>
                    <RadioGroupItem
                      id='model-scope-unlimited'
                      value='unlimited'
                    />
                    <span>
                      <span className='block'>
                        {t('No additional model limit')}
                      </span>
                      <span className='text-muted-foreground block text-xs font-normal'>
                        {t('Other account and route restrictions still apply.')}
                      </span>
                    </span>
                  </Field>
                </FieldLabel>
                <FieldLabel htmlFor='model-scope-restricted'>
                  <Field orientation='horizontal'>
                    <RadioGroupItem
                      id='model-scope-restricted'
                      value='restricted'
                    />
                    <span>
                      <span className='block'>{t('Only selected models')}</span>
                      <span className='text-muted-foreground block text-xs font-normal'>
                        {t('At least one model must be selected.')}
                      </span>
                    </span>
                  </Field>
                </FieldLabel>
                <FieldLabel htmlFor='model-scope-blocked'>
                  <Field orientation='horizontal'>
                    <RadioGroupItem id='model-scope-blocked' value='blocked' />
                    <span>
                      <span className='block'>{t('Block all models')}</span>
                      <span className='text-muted-foreground block text-xs font-normal'>
                        {t('This Key cannot start model requests.')}
                      </span>
                    </span>
                  </Field>
                </FieldLabel>
              </RadioGroup>
            </FieldSet>

            {draft.mode !== 'blocked' && (
              <Field data-disabled={draft.mode === 'unlimited'}>
                <FieldLabel htmlFor='model-scope-search'>
                  {t('Selected model IDs')}
                </FieldLabel>
                <FieldDescription>
                  {draft.mode === 'unlimited'
                    ? t(
                        'The saved list is retained while the limit is off. Turn on selected models to edit or apply it.'
                      )
                    : t(
                        'Saved model IDs remain visible even when they are no longer in the current catalog.'
                      )}
                </FieldDescription>
                <div className='flex min-w-0 flex-col gap-2'>
                  {draft.models.length === 0 && (
                    <p className='text-muted-foreground rounded-lg border p-3 text-sm'>
                      {t('No models selected')}
                    </p>
                  )}
                  {selectedModels.map((item, index) => {
                    const modelLabel = item.model || t('Empty model ID')
                    return (
                      <div
                        key={item.key}
                        className='bg-card grid min-w-0 grid-cols-1 gap-2 rounded-lg border p-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center'
                      >
                        <span className='w-full min-w-0 font-mono text-sm break-all'>
                          {modelLabel}
                        </span>
                        <div className='grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-2 sm:flex sm:shrink-0 sm:justify-start'>
                          <Badge
                            variant='outline'
                            className='h-auto w-fit max-w-full text-left whitespace-normal'
                          >
                            {catalogSet.has(item.model)
                              ? t('Current catalog')
                              : t('Preserved model ID')}
                          </Badge>
                          {draft.mode === 'restricted' && (
                            <Button
                              type='button'
                              variant='ghost'
                              size='icon-sm'
                              aria-label={t('Remove {{model}}', {
                                model: modelLabel,
                              })}
                              onClick={() => {
                                setDraft({
                                  ...draft,
                                  models: draft.models.filter(
                                    (_item, itemIndex) => itemIndex !== index
                                  ),
                                })
                                if (!needsReadback) setSaveError(null)
                              }}
                            >
                              <X aria-hidden='true' />
                            </Button>
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
                {draft.mode === 'restricted' && (
                  <div className='mt-2 flex min-w-0 flex-col gap-2'>
                    <Input
                      id='model-scope-search'
                      value={search}
                      placeholder={t('Search current model catalog')}
                      onChange={(event) => setSearch(event.target.value)}
                    />
                    {catalogQuery.isPending && (
                      <p
                        role='status'
                        className='text-muted-foreground text-sm'
                      >
                        {t('Loading current model catalog...')}
                      </p>
                    )}
                    {catalogQuery.isError && (
                      <Alert variant='destructive'>
                        <AlertTitle>
                          {t('Could not load the current model catalog')}
                        </AlertTitle>
                        <AlertDescription className='mt-2'>
                          <p>
                            {t(
                              'Saved model IDs are preserved. Retry only reloads suggestions.'
                            )}
                          </p>
                          <Button
                            className='mt-2'
                            type='button'
                            variant='outline'
                            disabled={catalogQuery.isFetching}
                            onClick={() => void catalogQuery.refetch()}
                          >
                            {t('Retry catalog')}
                          </Button>
                        </AlertDescription>
                      </Alert>
                    )}
                    {catalogQuery.data && candidates.length === 0 && (
                      <p className='text-muted-foreground rounded-lg border p-3 text-sm'>
                        {search
                          ? t('No matching catalog models')
                          : t('No additional catalog models are available')}
                      </p>
                    )}
                    {candidates.length > 0 && (
                      <div
                        className='flex max-h-48 min-w-0 flex-col gap-1 overflow-y-auto rounded-lg border p-1'
                        aria-label={t('Current model catalog suggestions')}
                      >
                        {candidates.map((model) => (
                          <Button
                            key={model}
                            type='button'
                            variant='ghost'
                            className='h-auto min-h-10 justify-start whitespace-normal'
                            onClick={() => {
                              setDraft({
                                ...draft,
                                models: [...draft.models, model],
                              })
                              if (!needsReadback) setSaveError(null)
                            }}
                          >
                            <Plus data-icon='inline-start' />
                            <span className='text-left font-mono break-all'>
                              {model}
                            </span>
                          </Button>
                        ))}
                      </div>
                    )}
                  </div>
                )}
                {draft.mode === 'unlimited' && draft.models.length > 0 && (
                  <Button
                    className='mt-2 w-fit'
                    type='button'
                    variant='outline'
                    onClick={() => {
                      setDraft({ ...draft, models: [] })
                      if (!needsReadback) setSaveError(null)
                    }}
                  >
                    {t('Clear retained model list')}
                  </Button>
                )}
                {validationError && (
                  <FieldError>
                    {validationMessage(validationError, t)}
                  </FieldError>
                )}
              </Field>
            )}

            {draft.mode === 'blocked' && dirty && (
              <Alert variant='destructive'>
                <AlertTriangle aria-hidden='true' />
                <AlertTitle>{t('Confirm blocking all models')}</AlertTitle>
                <AlertDescription className='mt-2'>
                  <Label className='flex cursor-pointer items-start gap-2'>
                    <Checkbox
                      checked={blockConfirmed}
                      onCheckedChange={(checked) =>
                        setBlockConfirmed(checked === true)
                      }
                      disabled={saving}
                    />
                    <span>
                      {t(
                        'I understand that this Key will be unable to start model requests.'
                      )}
                    </span>
                  </Label>
                </AlertDescription>
              </Alert>
            )}

            {saveError && (
              <Alert variant='destructive'>
                <AlertTitle>{saveErrorTitle(saveError, t)}</AlertTitle>
                <AlertDescription className='mt-2'>
                  <p>{saveErrorDescription(saveError, t)}</p>
                  {(saveError === 'unknown' ||
                    saveError === 'unauthorized') && (
                    <Button
                      className='mt-2'
                      type='button'
                      variant='outline'
                      disabled={saving}
                      onClick={async () => {
                        const controller = new AbortController()
                        writeController.current = controller
                        setSaving(true)
                        const current = await readCurrentScope(
                          controller.signal
                        )
                        if (writeController.current === controller) {
                          writeController.current = null
                        }
                        if (isCurrentSession()) {
                          setSaveError(current ? null : 'unknown')
                          setSaving(false)
                        }
                      }}
                    >
                      {t('Reload current scope')}
                    </Button>
                  )}
                </AlertDescription>
              </Alert>
            )}
          </FieldGroup>
        )}

        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            disabled={saving}
            onClick={props.onClose}
          >
            {t('Cancel')}
          </Button>
          <Button type='button' disabled={!canSave} onClick={() => void save()}>
            {saving ? t('Saving...') : t('Save model scope')}
            {!saving && <Check data-icon='inline-end' />}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
