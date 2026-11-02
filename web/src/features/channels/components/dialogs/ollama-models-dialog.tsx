/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQueryClient } from '@tanstack/react-query'
import { ArrowDown01Icon, Delete02Icon, Loading03Icon, Refresh01Icon, Search01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import { Separator } from '@/components/ui/separator'
import { getFreshAuthHeaders } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import {
  deleteOllamaModel,
  fetchModels as fetchModelsFromEndpoint,
  fetchUpstreamModels,
  updateChannel,
} from '../../api'
import { channelsQueryKeys, parseModelsString } from '../../lib'
import {
  formatBytes,
  normalizeOllamaModels,
  resolveOllamaBaseUrl,
  type OllamaModel,
  type PullProgress,
} from '../../lib/ollama-utils'
import { useChannels } from '../channels-provider'
import { useChannelTask } from '../../hooks/use-channel-task'

const CHANNEL_TYPE_OLLAMA = 4

type OllamaModelsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  returnFocus?: () => HTMLElement | null
}

export function OllamaModelsDialog(props: OllamaModelsDialogProps) {
  const { currentRow } = useChannels()
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  return <OllamaModelsTask key={JSON.stringify([identity, currentRow?.id])} {...props} />
}

function OllamaModelsTask({
  open,
  onOpenChange,
  returnFocus,
}: OllamaModelsDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { currentRow } = useChannels()

  const isOllamaChannel = currentRow?.type === CHANNEL_TYPE_OLLAMA
  const channelId = currentRow?.id
  const task = useChannelTask(open, channelId)
  const readGeneration = useRef(0)
  const deletePending = useRef(false)
  const applyPending = useRef(false)
  const [readError, setReadError] = useState<string | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [applyError, setApplyError] = useState<string | null>(null)
  const [isApplying, setIsApplying] = useState(false)
  const refreshButton = useRef<HTMLButtonElement>(null)
  const deleteOpener = useRef<HTMLElement | null>(null)

  const [isFetching, setIsFetching] = useState(false)
  const [models, setModels] = useState<OllamaModel[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [search, setSearch] = useState('')

  const [pullName, setPullName] = useState('')
  const [isPulling, setIsPulling] = useState(false)
  const [pullProgress, setPullProgress] = useState<PullProgress | null>(null)
  const pullAbortRef = useRef<AbortController | null>(null)
  useEffect(() => () => { pullAbortRef.current?.abort() }, [])

  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)

  const filteredModels = useMemo(() => {
    if (!search.trim()) return models
    const keyword = search.trim().toLowerCase()
    return models.filter((m) => m.id.toLowerCase().includes(keyword))
  }, [models, search])

  const existingModels = useMemo(
    () => parseModelsString(currentRow?.models ?? ''),
    [currentRow?.models]
  )

  useEffect(() => {
    if (!open) {
      setModels([])
      setSelected([])
      setSearch('')
      setPullName('')
      setIsPulling(false)
      setPullProgress(null)
      pullAbortRef.current?.abort()
      pullAbortRef.current = null
      return
    }

    if (open && isOllamaChannel && channelId) {
      void fetchOllamaModels()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, isOllamaChannel, channelId])

  const fetchOllamaModels = useCallback(async () => {
    if (!channelId) return
    const snapshot = task.capture()
    const request = ++readGeneration.current
    setIsFetching(true)
    setReadError(null)
    try {
      let normalized: OllamaModel[] = []
      let lastErr = ''

      // 1) Prefer live fetch for Ollama if base_url is set (more accurate / supports unsaved changes)
      const baseUrl = resolveOllamaBaseUrl(currentRow ?? null)
      if (isOllamaChannel && baseUrl) {
        try {
          const payloadLive = await fetchModelsFromEndpoint({
            base_url: baseUrl,
            type: CHANNEL_TYPE_OLLAMA,
            key: typeof currentRow?.key === 'string' ? currentRow.key : '',
          })
          if (!snapshot.isCurrent() || request !== readGeneration.current) return
          if (payloadLive?.success) {
            normalized = normalizeOllamaModels(payloadLive.data)
          } else if (payloadLive?.message) {
            lastErr = String(payloadLive.message)
          }
        } catch (err: unknown) {
          if (!snapshot.isCurrent() || request !== readGeneration.current) return
          lastErr = err instanceof Error ? err.message : ''
        }
      }

      // 2) Fallback to server-side fetch by channelId
      if (!normalized.length) {
        const payload = await fetchUpstreamModels(Number(channelId))
        if (!snapshot.isCurrent() || request !== readGeneration.current) return
        if (payload?.success) {
          normalized = normalizeOllamaModels(payload.data)
          lastErr = ''
        } else {
          lastErr = String(payload?.message || '')
        }
      }

      if (!normalized.length && lastErr) {
        setReadError(lastErr)
        toast.error(lastErr || t('Failed to fetch models'))
      }

      setModels(normalized)
      setSelected((prev) => {
        if (!prev.length) return normalized.map((m) => m.id)
        const stillAvailable = prev.filter((id) =>
          normalized.some((m) => m.id === id)
        )
        return stillAvailable.length
          ? stillAvailable
          : normalized.map((m) => m.id)
      })
    } catch (err: unknown) {
      if (!snapshot.isCurrent() || request !== readGeneration.current) return
      const msg = err instanceof Error ? err.message : undefined
      setReadError(msg || t('Failed to fetch models'))
      toast.error(msg || t('Failed to fetch models'))
      setModels([])
    } finally {
      if (snapshot.isCurrent() && request === readGeneration.current) setIsFetching(false)
    }
  }, [channelId, currentRow, isOllamaChannel, t, task])

  const toggleSelected = (modelId: string, checked: boolean) => {
    setSelected((prev) => {
      if (checked) return prev.includes(modelId) ? prev : [...prev, modelId]
      return prev.filter((id) => id !== modelId)
    })
  }

  const selectAllFiltered = () => {
    setSelected((prev) => {
      const next = new Set(prev)
      filteredModels.forEach((m) => next.add(m.id))
      return [...next]
    })
  }

  const clearSelection = () => setSelected([])

  const applySelection = async (mode: 'append' | 'replace') => {
    if (!currentRow) return
    if (!selected.length) {
      toast.info(t('No models selected'))
      return
    }

    const next =
      mode === 'replace'
        ? [...new Set(selected)]
        : [...new Set([...existingModels, ...selected])]

    if (applyPending.current) return
    const snapshot = task.capture()
    if (!snapshot.isCurrent()) return
    applyPending.current = true
    setIsApplying(true)
    setApplyError(null)

    try {
      const res = await updateChannel(currentRow.id, { models: next.join(',') })
      if (res.success) queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
      if (!snapshot.isCurrent()) return
      if (res.success) {
        toast.success(
          mode === 'replace'
            ? t('Models updated successfully')
            : t('Models appended successfully')
        )
      } else {
        setApplyError(res.message || t('Failed to update models'))
        toast.error(res.message || t('Failed to update models'))
      }
    } catch (err: unknown) {
      if (!snapshot.isCurrent()) return
      setApplyError(err instanceof Error ? err.message : t('Failed to update models'))
      toast.error(
        err instanceof Error ? err.message : t('Failed to update models')
      )
    } finally {
      applyPending.current = false
      if (snapshot.isCurrent()) setIsApplying(false)
    }
  }

  const pullModel = async () => {
    if (pullAbortRef.current) return
    if (!channelId) return
    if (!pullName.trim()) {
      toast.error(t('Please enter model name'))
      return
    }

    if (!resolveOllamaBaseUrl(currentRow)) {
      toast.error(t('Please set Ollama API Base URL first'))
      return
    }

    const controller = new AbortController()
    pullAbortRef.current = controller
    const snapshot = task.capture()

    setIsPulling(true)
    setPullProgress({ status: 'starting', completed: 0, total: 0 })
    let reader: ReadableStreamDefaultReader<Uint8Array> | null = null
    let ended = false

    try {
      const authHeaders = await getFreshAuthHeaders()
      if (!snapshot.isCurrent() || controller.signal.aborted) return
      const response = await fetch('/api/channel/ollama/pull/stream', {
        method: 'POST',
        credentials: 'include',
        headers: {
          ...authHeaders,
          Accept: 'text/event-stream',
        },
        body: JSON.stringify({
          channel_id: channelId,
          model_name: pullName.trim(),
        }),
        signal: controller.signal,
      })
      if (!snapshot.isCurrent() || controller.signal.aborted) {
        if (response.body) void response.body.cancel().catch(() => undefined)
        return
      }

      if (!response.ok || !response.body) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`)
      }

      reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()
        if (!snapshot.isCurrent() || controller.signal.aborted) return
        if (done) { ended = true; break }

        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() || ''

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue
          const eventData = line.slice(6)
          if (!eventData) continue

          if (eventData === '[DONE]') {
            setIsPulling(false)
            setPullProgress(null)
            pullAbortRef.current = null
            return
          }

          try {
            const data = JSON.parse(eventData)
            if (data?.status) {
              setPullProgress(data)
            } else if (data?.error) {
              toast.error(String(data.error))
              setIsPulling(false)
              setPullProgress(null)
              pullAbortRef.current = null
              return
            } else if (data?.message) {
              toast.success(String(data.message))
              setPullName('')
              setIsPulling(false)
              setPullProgress(null)
              pullAbortRef.current = null
              await fetchOllamaModels()
              queryClient.invalidateQueries({
                queryKey: channelsQueryKeys.lists(),
              })
              return
            }
          } catch {
            // ignore malformed events
          }
        }
      }

      setIsPulling(false)
      setPullProgress(null)
      pullAbortRef.current = null
      await fetchOllamaModels()
      queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
    } catch (err: unknown) {
      if (!snapshot.isCurrent()) return
      const isAbort =
        typeof err === 'object' &&
        err !== null &&
        'name' in err &&
        (err as { name?: unknown }).name === 'AbortError'
      if (!isAbort) {
        const msg = err instanceof Error ? err.message : ''
        toast.error(t('Model pull failed: {{msg}}', { msg }))
      }
      setIsPulling(false)
      setPullProgress(null)
      pullAbortRef.current = null
    } finally {
      if (reader) {
        if (!ended) void reader.cancel().catch(() => undefined)
        reader.releaseLock()
      }
    }
  }

  const deleteModel = async (modelName: string) => {
    if (!channelId || deletePending.current) return
    const snapshot = task.capture()
    if (!snapshot.isCurrent()) return
    deletePending.current = true
    setDeleteError(null)
    try {
      setIsDeleting(true)
      const payload = await deleteOllamaModel({
        channel_id: Number(channelId),
        model_name: modelName,
      })
      if (payload?.success) queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
      if (!snapshot.isCurrent()) return
      if (payload?.success) {
        toast.success(t('Model deleted'))
        await fetchOllamaModels()
        if (!snapshot.isCurrent()) return
        setDeleteOpen(false)
        setDeleteTarget(null)
      } else {
        setDeleteError(payload?.message || t('Failed to delete model'))
        toast.error(payload?.message || t('Failed to delete model'))
      }
    } catch (err: unknown) {
      if (!snapshot.isCurrent()) return
      const msg = err instanceof Error ? err.message : undefined
      setDeleteError(msg || t('Failed to delete model'))
      toast.error(msg || t('Failed to delete model'))
    } finally {
      deletePending.current = false
      if (snapshot.isCurrent()) setIsDeleting(false)
    }
  }

  const close = () => {
    if (deletePending.current || applyPending.current) return
    readGeneration.current += 1
    pullAbortRef.current?.abort()
    pullAbortRef.current = null
    onOpenChange(false)
  }

  if (!open) return null

  return (
    <Dialog
      open={open}
      returnFocus={returnFocus}
      showCloseButton={!isDeleting && !isApplying}
      onOpenChange={close}
      title={t('Ollama Models')}
      description={
        <>
          {t('Manage local models for:')} <strong>{currentRow?.name}</strong>
        </>
      }
      contentClassName='h-dvh max-h-dvh w-screen max-w-none sm:h-auto sm:max-h-[calc(100dvh-2rem)] sm:max-w-3xl [&_button]:min-h-11 [&_button]:min-w-11 [&_input]:min-h-11'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <Button variant='outline' disabled={isDeleting || isApplying} onClick={close}>
          {t('Close')}
        </Button>
      }
    >
      {!isOllamaChannel ? (
        <div className='text-muted-foreground py-8 text-center'>
          {t('This channel is not an Ollama channel.')}
        </div>
      ) : (
        <div className='space-y-4 py-2 pr-1'>
          {readError ? <div role='alert' className='text-destructive text-sm break-words'>{readError}</div> : null}
          {applyError ? <div role='alert' className='text-destructive text-sm break-words'>{applyError}</div> : null}
          <div className='flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between'>
            <div className='flex-1 space-y-2'>
              <Label htmlFor='ollama-pull'>{t('Pull model')}</Label>
              <div className='flex gap-2'>
                <Input
                  id='ollama-pull'
                  placeholder={t('e.g. llama3.1:8b')}
                  value={pullName}
                  onChange={(e) => setPullName(e.target.value)}
                  disabled={!channelId || isPulling}
                />
                <Button
                  onClick={() => void pullModel()}
                  disabled={!channelId || isPulling}
                >
                  {isPulling ? (
                    <>
                      <HugeiconsIcon icon={Loading03Icon} aria-hidden='true' className='mr-2 h-4 w-4 animate-spin' />
                      {t('Pulling...')}
                    </>
                  ) : (
                    <>
                      <HugeiconsIcon icon={ArrowDown01Icon} aria-hidden='true' className='mr-2 h-4 w-4' />
                      {t('Pull')}
                    </>
                  )}
                </Button>
              </div>
              {pullProgress && (
                <div className='space-y-2'>
                  <div className='text-muted-foreground text-xs'>
                    {t('Status:')} {String(pullProgress.status || '-')}
                  </div>
                  <Progress
                    value={
                      typeof pullProgress.completed === 'number' &&
                      Number.isFinite(pullProgress.completed) &&
                      pullProgress.completed >= 0 &&
                      typeof pullProgress.total === 'number' &&
                      Number.isFinite(pullProgress.total) &&
                      pullProgress.total > 0
                        ? Math.min(
                            100,
                            Math.round(
                              (pullProgress.completed / pullProgress.total) *
                                100
                            )
                          )
                        : null
                    }
                  />
                </div>
              )}
            </div>

            <div className='flex gap-2'>
              <Button
                ref={refreshButton}
                variant='outline'
                onClick={() => void fetchOllamaModels()}
                disabled={!channelId || isFetching}
              >
                {isFetching ? (
                  <HugeiconsIcon icon={Loading03Icon} aria-hidden='true' className='mr-2 h-4 w-4 animate-spin' />
                ) : (
                  <HugeiconsIcon icon={Refresh01Icon} aria-hidden='true' className='mr-2 h-4 w-4' />
                )}
                {t('Refresh')}
              </Button>
            </div>
          </div>

          <Separator />

          <div className='space-y-3'>
            <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
              <div>
                <p className='text-sm font-medium'>{t('Local models')}</p>
                <p className='text-muted-foreground text-xs'>
                  {t('Select models and apply to channel models list.')}
                </p>
              </div>
              <div className='relative sm:w-72'>
                <HugeiconsIcon icon={Search01Icon} aria-hidden='true' className='text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2' />
                <Input
                  placeholder={t('Search models...')}
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  className='pl-9'
                />
              </div>
            </div>

            <div className='flex flex-wrap gap-2'>
              <Button variant='outline' size='sm' onClick={selectAllFiltered}>
                {t('Select all (filtered)')}
              </Button>
              <Button variant='outline' size='sm' onClick={clearSelection}>
                {t('Clear selection')}
              </Button>
              <Button
                size='sm'
                onClick={() => void applySelection('append')}
                disabled={!selected.length || isApplying}
              >
                {t('Append to channel')}
              </Button>
              <Button
                variant='secondary'
                size='sm'
                onClick={() => void applySelection('replace')}
                disabled={!selected.length || isApplying}
              >
                {t('Replace channel models')}
              </Button>
            </div>

            <div className='overflow-hidden rounded-md border'>
              <div>
                {filteredModels.length === 0 ? (
                  <div className='text-muted-foreground p-6 text-center text-sm'>
                    {t('No models found.')}
                  </div>
                ) : (
                  <div className='divide-y'>
                    {filteredModels.map((m) => {
                      const checked = selected.includes(m.id)
                      return (
                        <div
                          key={m.id}
                          className='flex items-center justify-between gap-3 p-3'
                        >
                          <div className='flex min-w-0 items-start gap-3'>
                            <Checkbox
                              checked={checked}
                              onCheckedChange={(v) => toggleSelected(m.id, !!v)}
                              aria-label={t('Select model {{name}}', { name: m.id })}
                            />
                            <div className='min-w-0'>
                              <div className='font-mono text-sm break-all'>
                                {m.id}
                              </div>
                              <div className='text-muted-foreground flex flex-wrap gap-x-3 gap-y-1 text-xs'>
                                <span>
                                  {t('Size:')} {formatBytes(m.size)}
                                </span>
                                {m.digest && (
                                  <span className='break-all'>
                                    {t('Digest:')} {String(m.digest)}
                                  </span>
                                )}
                              </div>
                            </div>
                          </div>

                          <Button
                            variant='ghost'
                            size='sm'
                            className='text-destructive hover:text-destructive'
                            aria-label={t('Delete model {{name}}', { name: m.id })}
                            onClick={() => {
                              deleteOpener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
                              setDeleteError(null)
                              setDeleteTarget(m.id)
                              setDeleteOpen(true)
                            }}
                            disabled={!channelId}
                          >
                            <HugeiconsIcon icon={Delete02Icon} aria-hidden='true' className='h-4 w-4' />
                          </Button>
                        </div>
                      )
                    })}
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      )}

      <AlertDialog
        open={deleteOpen}
        onOpenChange={(v) => {
          if (deletePending.current) return
          setDeleteOpen(v)
          if (!v) setDeleteTarget(null)
        }}
      >
        <AlertDialogContent finalFocus={() => deleteOpener.current?.isConnected ? deleteOpener.current : refreshButton.current} className='[&_button]:min-h-11 [&_button]:min-w-11'>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Confirm delete')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('Delete model "{{name}}"? This cannot be undone.', {
                name: deleteTarget || '',
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteError ? <div role='alert' className='text-destructive text-sm break-words'>{deleteError}</div> : null}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeleting}>
              {t('Cancel')}
            </AlertDialogCancel>
            <Button
              variant='destructive'
              disabled={isDeleting || !deleteTarget}
              onClick={() => {
                if (!deleteTarget) return
                void deleteModel(deleteTarget)
              }}
            >
              {isDeleting ? (
                <HugeiconsIcon icon={Loading03Icon} aria-hidden='true' className='mr-2 h-4 w-4 animate-spin' />
              ) : null}
              {t('Delete')}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Dialog>
  )
}
