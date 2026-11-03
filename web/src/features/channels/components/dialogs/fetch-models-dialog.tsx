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
import { ArrowDown01Icon, InformationCircleIcon, Loading03Icon, Search01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState, useEffect, useMemo, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { hasPermission } from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { fetchUpstreamModels, updateChannel } from '../../api'
import {
  categorizeModels,
  categorizeModelsWithRedirect,
  channelsQueryKeys,
  normalizeModelName,
  parseModelsString,
} from '../../lib'
import { useChannels } from '../channels-provider'
import { useChannelTask } from '../../hooks/use-channel-task'

function normalizeModelNameList(models: readonly string[]): string[] {
  return [...new Set(models.map((m) => normalizeModelName(m)).filter(Boolean))]
}

type FetchModelsDialogBaseProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  redirectModels?: string[]
  redirectSourceModels?: string[]
  customFetcher?: (signal?: AbortSignal) => Promise<string[]>
  sourceScope?: object
  channelName?: string | null
  returnFocus?: () => HTMLElement | null
}

type FetchModelsDialogProps = FetchModelsDialogBaseProps &
  (
    | {
        onModelsSelected: (models: string[]) => void
        existingModelsOverride: string[]
      }
    | {
        onModelsSelected?: undefined
        existingModelsOverride?: undefined
      }
  )

export function FetchModelsDialog({
  open,
  onOpenChange,
  onModelsSelected,
  redirectModels = [],
  redirectSourceModels = [],
  customFetcher,
  sourceScope,
  existingModelsOverride,
  channelName,
  returnFocus,
}: FetchModelsDialogProps) {
  const { t } = useTranslation()
  const { currentRow } = useChannels()
  const activeChannel = customFetcher ? null : currentRow
  const queryClient = useQueryClient()
  const task = useChannelTask(open, activeChannel?.id)
  const readGeneration = useRef(0)
  const readController = useRef<AbortController | null>(null)
  const wasOpen = useRef(false)
  const sourceToken = sourceScope ?? customFetcher ?? activeChannel?.id
  const currentSource = useRef(sourceToken)
  currentSource.current = sourceToken
  const [hasCurrentResult, setHasCurrentResult] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)
  const [isFetching, setIsFetching] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [fetchedModels, setFetchedModels] = useState<string[]>([])
  const [selectedModels, setSelectedModels] = useState<string[]>([])
  const [searchKeyword, setSearchKeyword] = useState('')

  // Parse existing models
  const existingModels = useMemo(
    () =>
      existingModelsOverride ?? parseModelsString(activeChannel?.models || ''),
    [existingModelsOverride, activeChannel?.models]
  )

  // Categorize models with redirect models
  const modelCategories = useMemo(
    () => categorizeModelsWithRedirect(existingModels, redirectModels),
    [existingModels, redirectModels]
  )

  const { classificationSet, redirectOnlySet } = modelCategories

  const fetchedModelSet = useMemo(
    () => new Set(normalizeModelNameList(fetchedModels)),
    [fetchedModels]
  )

  // Source keys in model_mapping are aliases, not real upstream IDs, so we
  // must skip them when computing "removed upstream" entries to avoid false
  // positives.
  const redirectSourceKeysSet = useMemo(
    () => new Set(normalizeModelNameList(redirectSourceModels)),
    [redirectSourceModels]
  )

  const removedModels = useMemo(() => {
    const kw = searchKeyword.toLowerCase().trim()
    return normalizeModelNameList(selectedModels).filter((model) => {
      if (fetchedModelSet.has(model)) return false
      if (redirectSourceKeysSet.has(model)) return false
      if (!kw) return true
      return model.toLowerCase().includes(kw)
    })
  }, [fetchedModelSet, redirectSourceKeysSet, searchKeyword, selectedModels])

  useEffect(() => {
    const opening = open && !wasOpen.current
    wasOpen.current = open
    readGeneration.current += 1
    readController.current?.abort()
    setHasCurrentResult(false)
    setFetchedModels([])
    setSelectedModels([])
    setSearchKeyword('')
    setActionError(open && !opening ? t('The model source changed. Fetch models again.') : null)
    setIsSaving(false)
    setIsFetching(false)
    if (opening && (activeChannel || customFetcher)) {
      void handleFetchModels()
    }
    return () => {
      readGeneration.current += 1
      readController.current?.abort()
    }
    // A changed source invalidates results; only an explicit opening starts discovery.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, activeChannel?.id, sourceToken, task.identity])

  const handleFetchModels = async () => {
    if (!activeChannel && !customFetcher) return
    const snapshot = task.capture()
    if (!snapshot.isCurrent()) return
    const request = ++readGeneration.current
    readController.current?.abort()
    const controller = new AbortController()
    readController.current = controller
    const source = sourceToken
    const isCurrent = () => snapshot.isCurrent() && !controller.signal.aborted && request === readGeneration.current && source === currentSource.current
    setHasCurrentResult(false)
    setFetchedModels([])
    setSelectedModels([])
    setIsFetching(true)
    setActionError(null)
    try {
      let list: unknown
      if (customFetcher) {
        list = await customFetcher(controller.signal)
      } else {
        if (!activeChannel || !hasPermission(useAuthStore.getState().auth.user, 'channel', 'operate')) {
          throw new Error(t("You don't have necessary permission"))
        }
        const response = await fetchUpstreamModels(activeChannel.id)
        if (!isCurrent()) return
        if (!hasPermission(useAuthStore.getState().auth.user, 'channel', 'operate')) {
          throw new Error(t("You don't have necessary permission"))
        }
        if (!response.success) throw new Error(response.message || t('Failed to fetch models'))
        list = response.data
      }
      if (!isCurrent()) return
      if (!Array.isArray(list) || !list.every((model) => typeof model === 'string')) {
        throw new Error(t('The channel did not return a model list. Its model support is unconfirmed.'))
      }
      setFetchedModels(list)
      setSelectedModels(existingModels)
      setHasCurrentResult(true)
      toast.success(t('Fetched {{count}} models', { count: list.length }))
    } catch (error: unknown) {
      if (!isCurrent()) return
      setActionError(error instanceof Error ? error.message : t('Failed to fetch models'))
      toast.error(error instanceof Error ? error.message : t('Failed to fetch models'))
    } finally {
      if (isCurrent()) setIsFetching(false)
    }
  }

  const handleSave = async () => {
    if (!hasCurrentResult || sourceToken !== currentSource.current) return
    const operation = task.begin()
    if (!operation) return
    setActionError(null)
    try {
      // If onModelsSelected callback is provided, use it (form filling mode)
      if (onModelsSelected) {
        onModelsSelected(selectedModels)
        toast.success(t('Models filled to form'))
        onOpenChange(false)
        return
      }

      // Otherwise, directly save to API (standalone mode)
      if (!activeChannel) return
      if (!hasPermission(useAuthStore.getState().auth.user, 'channel', 'write') || !hasPermission(useAuthStore.getState().auth.user, 'channel', 'operate')) {
        throw new Error(t("You don't have necessary permission"))
      }
      setIsSaving(true)
      const modelsString = selectedModels.join(',')
      const response = await updateChannel(activeChannel.id, {
        models: modelsString,
      })
      if (!operation.isCurrent() || !hasPermission(useAuthStore.getState().auth.user, 'channel', 'write') || !hasPermission(useAuthStore.getState().auth.user, 'channel', 'operate')) return
      if (response.success) queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
      if (response.success) {
        toast.success(t('Models updated successfully'))
        onOpenChange(false)
      } else {
        setActionError(response.message || t('Failed to update models'))
        toast.error(response.message || t('Failed to update models'))
      }
    } catch (error: unknown) {
      if (!operation.isCurrent()) return
      setActionError(error instanceof Error ? error.message : t('Failed to update models'))
      toast.error(
        error instanceof Error ? error.message : t('Failed to update models')
      )
    } finally {
      if (operation.isCurrent()) setIsSaving(false)
      operation.finish()
    }
  }

  const handleClose = () => {
    if (task.isPending()) return
    readGeneration.current += 1
    readController.current?.abort()
    setHasCurrentResult(false)
    setFetchedModels([])
    setSelectedModels([])
    setSearchKeyword('')
    onOpenChange(false)
  }

  // Filter models by search
  const filteredModels = useMemo(() => {
    if (!searchKeyword) return fetchedModels
    return fetchedModels.filter((model) =>
      model.toLowerCase().includes(searchKeyword.toLowerCase())
    )
  }, [fetchedModels, searchKeyword])

  const {
    newModels,
    existingFilteredModels,
    newModelsByCategory,
    existingModelsByCategory,
  } = useMemo(() => {
    const newModels: string[] = []
    const existingFilteredModels: string[] = []

    for (const model of filteredModels) {
      if (classificationSet.has(normalizeModelName(model))) {
        existingFilteredModels.push(model)
      } else {
        newModels.push(model)
      }
    }

    return {
      newModels,
      existingFilteredModels,
      newModelsByCategory: categorizeModels(newModels),
      existingModelsByCategory: categorizeModels(existingFilteredModels),
    }
  }, [classificationSet, filteredModels])

  // 厂商分类按 a-z 排序，Other 放最后，便于查找
  const getSortedCategoryEntries = (
    categories: Record<string, string[]>
  ): [string, string[]][] =>
    Object.entries(categories).sort(([a], [b]) => {
      if (a === 'Other') return 1
      if (b === 'Other') return -1
      return a.localeCompare(b, undefined, { sensitivity: 'base' })
    })

  const toggleModel = (model: string) => {
    setSelectedModels((prev) =>
      prev.includes(model) ? prev.filter((m) => m !== model) : [...prev, model]
    )
  }

  const toggleCategory = (categoryModels: string[], isChecked: boolean) => {
    setSelectedModels((prev) => {
      if (isChecked) {
        const newSelected = [...prev]
        categoryModels.forEach((model) => {
          if (!newSelected.includes(model)) {
            newSelected.push(model)
          }
        })
        return newSelected
      } else {
        return prev.filter((m) => !categoryModels.includes(m))
      }
    })
  }

  const isCategorySelected = (categoryModels: string[]) => {
    return categoryModels.every((m) => selectedModels.includes(m))
  }

  const renderModelCategory = (
    categoryName: string,
    categoryModels: string[]
  ) => {
    const allSelected = isCategorySelected(categoryModels)

    return (
      <Collapsible key={categoryName} defaultOpen>
        <div className='flex min-w-0 items-center justify-between gap-2 rounded-lg border p-3'>
        <CollapsibleTrigger className='hover:bg-muted/50 min-h-11 min-w-0 flex-1 text-start'>
          <div className='flex items-center gap-2'>
            <HugeiconsIcon icon={ArrowDown01Icon} aria-hidden='true' className='h-4 w-4' />
            <span className='font-medium'>
              {categoryName} ({categoryModels.length})
            </span>
          </div>
        </CollapsibleTrigger>
          <div className='flex items-center gap-2'>
            <span className='text-muted-foreground text-sm'>
              {categoryModels.filter((m) => selectedModels.includes(m)).length}{' '}
              / {categoryModels.length} {t('Selected')}
            </span>
            <Checkbox
              checked={allSelected}
              aria-label={t('Select all models in "{{category}}"', { category: categoryName })}
              onCheckedChange={(checked) =>
                toggleCategory(categoryModels, !!checked)
              }
              onClick={(e) => e.stopPropagation()}
            />
          </div>
        </div>
        <CollapsibleContent className='px-4 py-2'>
          <div className='grid grid-cols-1 gap-2 sm:grid-cols-2'>
            {categoryModels.map((model) => (
              <div key={model} className='flex min-w-0 items-start gap-2 py-1'>
                <Checkbox
                  id={model}
                  checked={selectedModels.includes(model)}
                  onCheckedChange={() => toggleModel(model)}
                />
                <Label
                  htmlFor={model}
                  className='flex min-h-11 min-w-0 cursor-pointer items-center gap-1.5 text-sm font-normal break-all'
                >
                  <span>{model}</span>
                  {redirectOnlySet.has(normalizeModelName(model)) && (
                    <Tooltip>
                      <TooltipTrigger
                        render={<HugeiconsIcon icon={InformationCircleIcon} aria-hidden='true' className='h-3.5 w-3.5 text-amber-500' />}
                      />
                      <TooltipContent>
                        {t('From model redirect, not yet added to models list')}
                      </TooltipContent>
                    </Tooltip>
                  )}
                </Label>
              </div>
            ))}
          </div>
        </CollapsibleContent>
      </Collapsible>
    )
  }

  const showFooterActions =
    !!(activeChannel || customFetcher) &&
    !isFetching &&
    hasCurrentResult &&
    (fetchedModels.length > 0 || removedModels.length > 0)

  let dialogDescription: ReactNode = t('Fetch available models from upstream')
  if (activeChannel) {
    dialogDescription = (
      <>
        {t('Channel:')} <strong>{activeChannel.name}</strong>
      </>
    )
  } else if (channelName) {
    dialogDescription = (
      <>
        {t('Channel:')} <strong>{channelName}</strong>
      </>
    )
  }

  let defaultTab = 'existing'
  if (newModels.length > 0) {
    defaultTab = 'new'
  } else if (removedModels.length > 0) {
    defaultTab = 'removed'
  }

  let dialogBody: ReactNode
  if (!activeChannel && !customFetcher) {
    dialogBody = (
      <div className='text-muted-foreground py-8 text-center'>
        {t('No channel selected')}
      </div>
    )
  } else if (isFetching) {
    dialogBody = (
      <div className='flex items-center justify-center py-12'>
        <HugeiconsIcon icon={Loading03Icon} aria-hidden='true' className='text-muted-foreground h-8 w-8 animate-spin' />
      </div>
    )
  } else if (hasCurrentResult && actionError && fetchedModels.length === 0 && selectedModels.length > 0) {
    dialogBody = (
      <div className='space-y-4'>
        <p className='text-muted-foreground text-sm'>{t('{{n}} model(s) selected', { n: selectedModels.length })}</p>
        {renderModelCategory(t('Models'), selectedModels)}
        <Button variant='outline' onClick={handleFetchModels}>{t('Fetch Models')}</Button>
      </div>
    )
  } else if (fetchedModels.length === 0 && removedModels.length === 0) {
    dialogBody = (
      <div className='text-muted-foreground py-8 text-center'>
        <p>{t('No models fetched yet.')}</p>
        <Button
          type='button'
          className='mt-4'
          onClick={handleFetchModels}
          disabled={isFetching}
        >
          {t('Fetch Models')}
        </Button>
      </div>
    )
  } else {
    dialogBody = (
      <div className='space-y-4'>
        {/* Search Bar */}
        <div className='relative'>
          <HugeiconsIcon icon={Search01Icon} aria-hidden='true' className='text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2' />
          <Input
            placeholder={t('Search models...')}
            value={searchKeyword}
            onChange={(e) => setSearchKeyword(e.target.value)}
            className='pl-9'
          />
        </div>

        {/* Tabs for New vs Existing vs Removed */}
        <Tabs
          key={`${activeChannel?.id ?? 'custom'}-${fetchedModels.length}-${removedModels.length}`}
          defaultValue={defaultTab}
        >
          <TabsList
            className={`grid w-full ${removedModels.length > 0 ? 'grid-cols-3' : 'grid-cols-2'}`}
          >
            <TabsTrigger value='new' disabled={newModels.length === 0}>
              {t('New Models ({{count}})', { count: newModels.length })}
            </TabsTrigger>
            <TabsTrigger
              value='existing'
              disabled={existingFilteredModels.length === 0}
            >
              {t('Existing Models ({{count}})', {
                count: existingFilteredModels.length,
              })}
            </TabsTrigger>
            {removedModels.length > 0 && (
              <TabsTrigger value='removed'>
                {t('Removed Models ({{count}})', {
                  count: removedModels.length,
                })}
              </TabsTrigger>
            )}
          </TabsList>

          <TabsContent
            value='new'
            className='space-y-2'
          >
            {getSortedCategoryEntries(newModelsByCategory).map(
              ([category, models]) => renderModelCategory(category, models)
            )}
          </TabsContent>

          <TabsContent
            value='existing'
            className='space-y-2'
          >
            {getSortedCategoryEntries(existingModelsByCategory).map(
              ([category, models]) => renderModelCategory(category, models)
            )}
          </TabsContent>

          {removedModels.length > 0 && (
            <TabsContent
              value='removed'
              className='space-y-2'
            >
              <p className='text-muted-foreground text-xs'>
                {t(
                  'These models are still in your selection but were not returned by the upstream listing. Entries that are only model_mapping source aliases are omitted. Toggle to adjust before saving.'
                )}
              </p>
              {renderModelCategory(t('Removed'), removedModels)}
            </TabsContent>
          )}
        </Tabs>

        {/* Selection Summary */}
        <div className='bg-muted/50 rounded-lg border p-3 text-sm'>
          {t('{{n}} model(s) selected', { n: selectedModels.length })}
        </div>
      </div>
    )
  }

  return (
    <Dialog
      open={open}
      returnFocus={returnFocus}
      onOpenChange={handleClose}
      showCloseButton={!isSaving}
      title={t('Fetch Models')}
      description={dialogDescription}
      contentClassName='h-dvh max-h-dvh w-screen max-w-none sm:h-auto sm:max-h-[calc(100dvh-2rem)] sm:max-w-3xl [&_button]:min-h-11 [&_button]:min-w-11 [&_input]:min-h-11'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        showFooterActions ? (
          <>
            <Button variant='outline' onClick={handleClose} disabled={isSaving}>
              {t('Cancel')}
            </Button>
            <Button onClick={handleSave} disabled={isSaving}>
              {isSaving && <HugeiconsIcon icon={Loading03Icon} aria-hidden='true' className='mr-2 h-4 w-4 animate-spin' />}
              {isSaving ? t('Saving...') : t('Save Models')}
            </Button>
          </>
        ) : null
      }
    >
      <p className='text-muted-foreground text-sm'>{t('Model-list results come from the current connection. They do not prove request support or available quota.')}</p>
      {actionError ? <div role='alert' className='text-destructive text-sm break-words'>{actionError}</div> : null}
      <fieldset disabled={isSaving} className='contents'>{dialogBody}</fieldset>
    </Dialog>
  )
}
