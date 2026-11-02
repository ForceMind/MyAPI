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
import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  SideDrawerSection,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { JsonEditor } from '@/components/json-editor'
import { StatusBadge } from '@/components/status-badge'
import { TagInput } from '@/components/tag-input'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'
import { Spinner } from '@/components/ui/spinner'

import { createPrefillGroup, updatePrefillGroup } from '../../api'
import { ENDPOINT_TEMPLATES } from '../../constants'
import { prefillGroupsQueryKeys } from '../../lib'
import {
  prefillGroupFormSchema,
  type PrefillGroup,
  type PrefillGroupFormValues,
} from '../../types'
import {
  DEFAULT_FORM_VALUES,
  PREFILL_GROUP_TYPE_META,
  PREFILL_GROUP_TYPES,
  type PrefillGroupType,
  parseStringItems,
  serializeEndpointItems,
} from '../prefill-group-shared'

type PrefillGroupFormDrawerProps = {
  open: boolean
  onClose: () => void
  currentGroup: PrefillGroup | null
  returnFocus?: () => HTMLElement | null
}

export function PrefillGroupFormDrawer({
  open,
  onClose,
  currentGroup,
  returnFocus,
}: PrefillGroupFormDrawerProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isEdit = Boolean(currentGroup?.id)
  const [isSaving, setIsSaving] = useState(false)
  const saving = useRef(false)
  const itemDrafts = useRef<Record<PrefillGroupType, PrefillGroupFormValues['items']>>({ model: [], tag: [], endpoint: '' })

  const form = useForm<PrefillGroupFormValues>({
    resolver: zodResolver(prefillGroupFormSchema),
    defaultValues: DEFAULT_FORM_VALUES,
  })

  const selectedType = form.watch('type')
  const saveError = form.formState.errors.root?.server?.message

  useEffect(() => {
    if (open) {
      itemDrafts.current = { model: [], tag: [], endpoint: '' }
      if (isEdit && currentGroup) {
        const items = currentGroup.type === 'endpoint' ? serializeEndpointItems(currentGroup.items) : parseStringItems(currentGroup.items)
        itemDrafts.current[currentGroup.type] = items
        form.reset({
          id: currentGroup.id,
          name: currentGroup.name,
          description: currentGroup.description || '',
          type: currentGroup.type,
          items,
        })
      } else {
        form.reset(DEFAULT_FORM_VALUES)
      }
    }
  }, [open, isEdit, currentGroup, form])

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen && !saving.current) {
      onClose()
    }
  }

  const handleSubmit = async (values: PrefillGroupFormValues) => {
    if (saving.current) return
    saving.current = true
    setIsSaving(true)
    form.clearErrors('root.server')
    let items: PrefillGroupFormValues['items']
    if (values.type === 'endpoint') {
      items = typeof values.items === 'string' ? values.items : ''
    } else {
      items = Array.isArray(values.items) ? values.items : []
    }
    const payload = {
      name: values.name.trim(),
      type: values.type,
      description: values.description?.trim() || '',
      items,
    }

    try {
      let response
      if (isEdit) {
        if (!currentGroup) {
          toast.error(t('Operation failed'))
          return
        }
        response = await updatePrefillGroup({
          id: currentGroup.id,
          ...payload,
        })
      } else {
        response = await createPrefillGroup(payload)
      }

      if (response.success) {
        toast.success(
          isEdit ? t('Prefill group updated') : t('Prefill group created')
        )
        queryClient.invalidateQueries({
          queryKey: prefillGroupsQueryKeys.lists(),
        })
        onClose()
      } else {
        form.setError('root.server', { type: 'server', message: response.message || t('Operation failed') })
      }
    } catch (err: unknown) {
      form.setError('root.server', { type: 'server', message: err instanceof Error ? err.message : t('Operation failed') })
    } finally {
      saving.current = false
      setIsSaving(false)
    }
  }

  const meta =
    PREFILL_GROUP_TYPE_META[selectedType] || PREFILL_GROUP_TYPE_META.model
  let submitLabel = t('Create')
  if (isSaving) {
    submitLabel = t('Saving...')
  } else if (isEdit) {
    submitLabel = t('Save changes')
  }

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetContent finalFocus={returnFocus} showCloseButton={!isSaving} className={sideDrawerContentClassName('sm:max-w-2xl [&_[data-slot=sheet-close]]:min-h-11 [&_[data-slot=sheet-close]]:min-w-11 [&_input]:min-h-11 [&_button]:min-h-11 [&_button]:min-w-11')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {isEdit ? t('Edit Prefill Group') : t('Create Prefill Group')}
          </SheetTitle>
          <SheetDescription>
            {isEdit
              ? t('Update the reusable bundle below.')
              : t('Capture a reusable bundle of models, tags, or endpoints.')}
          </SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form
            id='prefill-group-form'
            onSubmit={form.handleSubmit(handleSubmit)}
            className={sideDrawerFormClassName()}
          >
            <fieldset disabled={isSaving} className='min-w-0 space-y-6'>
            <SideDrawerSection>
              <div className='flex flex-col gap-1'>
                <h3 className='text-sm font-semibold'>{t('Group details')}</h3>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Give the group a recognizable name and optional description.'
                  )}
                </p>
              </div>

              <FormField
                control={form.control}
                name='name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Group Name')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('Premium chat models')}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Give this group a recognizable name.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='description'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Description')}</FormLabel>
                    <FormControl>
                      <Textarea
                        placeholder={t(
                          'Optional notes about when to use this group'
                        )}
                        rows={3}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Make it easier for teammates to pick the right group.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </SideDrawerSection>

            <SideDrawerSection>
              <div className='flex flex-col gap-1'>
                <h3 className='text-sm font-semibold'>{t('Configuration')}</h3>
                <p className='text-muted-foreground text-sm'>
                  {t('Choose the bundle type and define the items inside it.')}
                </p>
              </div>

              <FormField
                control={form.control}
                name='type'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Group Type')}</FormLabel>
                    <Select
                      items={PREFILL_GROUP_TYPES.map((type) => ({
                        value: type.value,
                        label: (
                          <div className='flex flex-col text-left'>
                            <span className='font-medium'>{t(type.label)}</span>
                            <span
                              data-prefill-description
                              className='text-muted-foreground text-xs whitespace-normal break-words'
                            >
                              {t(type.description)}
                            </span>
                          </div>
                        ),
                      }))}
                      value={field.value}
                      onValueChange={(value) => {
                        if (value === null || value === field.value) return
                        const nextType = value as PrefillGroupType
                        itemDrafts.current[field.value] = form.getValues('items')
                        field.onChange(nextType)
                        form.setValue('items', itemDrafts.current[nextType], { shouldDirty: true, shouldValidate: false })
                        form.clearErrors('items')
                      }}
                    >
                      <FormControl>
                        <SelectTrigger className='min-h-11 w-full [&_[data-slot=select-value]_[data-prefill-description]]:hidden'>
                          <SelectValue placeholder={t('Select a group type')} />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false} className='max-w-[calc(100vw-2rem)] [&_[data-slot=select-item-text]]:whitespace-normal'>
                        <SelectGroup>
                          {PREFILL_GROUP_TYPES.map((type) => (
                            <SelectItem key={type.value} value={type.value}>
                              <div className='flex flex-col text-left'>
                                <span className='font-medium'>
                                  {t(type.label)}
                                </span>
                                <span
                                  data-prefill-description
                                  className='text-muted-foreground text-xs whitespace-normal break-words'
                                >
                                  {t(type.description)}
                                </span>
                              </div>
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormDescription>{t('Each type keeps its own draft while this editor is open.')}</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className='border-border/60 flex flex-col gap-3 border-y py-4'>
                <div className='flex items-center gap-2'>
                  <h4 className='text-sm font-medium'>{t('Items')}</h4>
                  <StatusBadge
                    label={t(meta.label)}
                    variant={meta.badge}
                    size='sm'
                    copyable={false}
                  />
                </div>
                <FormField
                  control={form.control}
                  name='items'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className='sr-only'>{t('Items')}</FormLabel>
                      <FormControl>
                        {selectedType === 'endpoint' ? (
                          <JsonEditor
                            value={(field.value as string) || ''}
                            onChange={field.onChange}
                            keyPlaceholder={t('Provider')}
                            valuePlaceholder='{"path": "/v1/...","method": "POST"}'
                            keyLabel={t('Provider')}
                            valueLabel={t('Endpoint config')}
                            valueType='any'
                            template={ENDPOINT_TEMPLATES}
                            emptyMessage={t(
                              'Define endpoint mappings for each provider.'
                            )}
                          />
                        ) : (
                          <TagInput
                            className='[&_[data-slot=badge]]:max-w-full [&_[data-slot=badge]]:whitespace-normal [&_[data-slot=badge]]:break-words [&_[data-slot=badge]]:[overflow-wrap:anywhere]'
                            value={
                              Array.isArray(field.value) ? field.value : []
                            }
                            onChange={field.onChange}
                            placeholder={t('Enter a value and press Enter')}
                          />
                        )}
                      </FormControl>
                      <FormDescription>
                        {selectedType === 'endpoint'
                          ? t(
                              'Provide a JSON object where each key maps to an endpoint definition.'
                            )
                          : t('Add each model or tag you want to include.')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </SideDrawerSection>
            </fieldset>
          </form>
        </Form>

        <SheetFooter className={sideDrawerFooterClassName('[&_button]:min-h-11 [&_button]:min-w-11')}>
          {saveError && <p role='alert' className='text-destructive col-span-2 text-sm break-words sm:mr-auto sm:max-w-sm'>{saveError}</p>}
          <SheetClose
            render={
              <Button type='button' variant='outline' disabled={isSaving} />
            }
          >
            {t('Cancel')}
          </SheetClose>
          <Button type='submit' form='prefill-group-form' disabled={isSaving}>
            {isSaving && <Spinner aria-hidden='true' className='mr-2 h-4 w-4 animate-spin' />}
            {submitLabel}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
