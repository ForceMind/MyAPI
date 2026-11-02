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
import { Cancel01Icon, Loading03Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { FieldGroup } from '@/components/ui/field'
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
import { Textarea } from '@/components/ui/textarea'

import { createVendor, updateVendor } from '../../api'
import { vendorsQueryKeys, modelsQueryKeys } from '../../lib'
import { vendorFormSchema, type Vendor } from '../../types'

type VendorMutateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentVendor?: Vendor | null
  returnFocus?: () => HTMLElement | null
}

const VENDOR_MUTATE_FORM_ID = 'vendor-mutate-form'

export function VendorMutateDialog({
  open,
  onOpenChange,
  currentVendor,
  returnFocus,
}: VendorMutateDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isEdit = Boolean(currentVendor?.id)
  const [isSaving, setIsSaving] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)
  const savingRef = useRef(false)

  const form = useForm({
    resolver: zodResolver(vendorFormSchema),
    defaultValues: {
      name: '',
      description: '',
      icon: '',
      status: 1,
    },
  })

  // Load vendor data for editing
  useEffect(() => {
    if (open) setSubmitError(null)
    if (open && isEdit && currentVendor) {
      form.reset({
        id: currentVendor.id,
        name: currentVendor.name,
        description: currentVendor.description || '',
        icon: currentVendor.icon || '',
        status: currentVendor.status || 1,
      })
    } else if (open && !isEdit) {
      form.reset({
        name: '',
        description: '',
        icon: '',
        status: 1,
      })
    }
  }, [open, isEdit, currentVendor, form])

  const onSubmit = async (values: Record<string, unknown>) => {
    if (savingRef.current) return
    savingRef.current = true
    setIsSaving(true)
    setSubmitError(null)
    try {
      let response
      if (isEdit) {
        if (!currentVendor) return
        response = await updateVendor({ ...values, id: currentVendor.id })
      } else {
        response = await createVendor(values)
      }

      if (response.success) {
        toast.success(
          isEdit
            ? t('Vendor updated successfully')
            : t('Vendor created successfully')
        )
        queryClient.invalidateQueries({ queryKey: vendorsQueryKeys.lists() })
        queryClient.invalidateQueries({ queryKey: modelsQueryKeys.lists() })
        onOpenChange(false)
      } else {
        const message = response.message || t('Operation failed')
        setSubmitError(message)
        toast.error(message)
      }
    } catch (error: unknown) {
      const message = (error as Error)?.message || t('Operation failed')
      setSubmitError(message)
      toast.error(message)
    } finally {
      savingRef.current = false
      setIsSaving(false)
    }
  }

  let submitLabel = t('Create')
  if (isSaving) {
    submitLabel = t('Saving...')
  } else if (isEdit) {
    submitLabel = t('Update')
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!isSaving) onOpenChange(nextOpen)
      }}
    >
      <DialogContent
        finalFocus={returnFocus}
        showCloseButton={false}
        className='flex h-dvh max-h-dvh w-full max-w-none flex-col gap-0 overflow-hidden rounded-none p-0 sm:h-auto sm:max-h-[calc(100dvh-3rem)] sm:max-w-lg sm:rounded-xl'
      >
        <DialogHeader className='shrink-0 border-b px-4 py-4 pr-16 text-left sm:px-6 sm:pr-16'>
          <DialogTitle className='wrap-anywhere'>
            {isEdit ? t('Edit Vendor') : t('Create Vendor')}
          </DialogTitle>
          <DialogDescription className='wrap-anywhere'>
            {isEdit
              ? t('Update vendor information for {{name}}', {
                  name: currentVendor?.name,
                })
              : t('Add a new vendor to the system')}
          </DialogDescription>
        </DialogHeader>
        <DialogClose
          disabled={isSaving}
          render={
            <Button
              type='button'
              variant='ghost'
              size='icon'
              className='absolute top-2 right-2 min-h-11 min-w-11'
            />
          }
        >
          <HugeiconsIcon icon={Cancel01Icon} aria-hidden='true' />
          <span className='sr-only'>{t('Close')}</span>
        </DialogClose>
        <div className='min-h-0 flex-1 overflow-x-hidden overflow-y-auto overscroll-contain px-4 py-5 sm:px-6'>
          <Form {...form}>
            <form
              id={VENDOR_MUTATE_FORM_ID}
              onSubmit={form.handleSubmit(onSubmit)}
              aria-busy={isSaving}
              className='flex min-w-0 flex-col gap-5'
            >
              {submitError ? (
                <Alert variant='destructive'>
                  <AlertDescription className='wrap-anywhere'>
                    {submitError}
                  </AlertDescription>
                </Alert>
              ) : null}
              <FieldGroup>
                <FormField
                  control={form.control}
                  name='name'
                  render={({ field }) => (
                    <FormItem className='min-w-0'>
                      <FormLabel>{t('Vendor Name *')}</FormLabel>
                      <FormControl>
                        <Input
                          placeholder={t('OpenAI, Anthropic, etc.')}
                          className='min-h-11'
                          disabled={isSaving}
                          aria-required='true'
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        {t('The unique name for this vendor')}
                      </FormDescription>
                      <FormMessage className='wrap-anywhere' />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='description'
                  render={({ field }) => (
                    <FormItem className='min-w-0'>
                      <FormLabel>{t('Description')}</FormLabel>
                      <FormControl>
                        <Textarea
                          placeholder={t('Describe this vendor...')}
                          rows={3}
                          className='min-h-28 max-h-none resize-none overflow-y-hidden'
                          disabled={isSaving}
                          {...field}
                        />
                      </FormControl>
                      <FormMessage className='wrap-anywhere' />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='icon'
                  render={({ field }) => (
                    <FormItem className='min-w-0'>
                      <FormLabel>{t('Icon')}</FormLabel>
                      <FormControl>
                        <Input
                          placeholder={t('OpenAI, Anthropic, Google, etc.')}
                          className='min-h-11'
                          disabled={isSaving}
                          {...field}
                        />
                      </FormControl>
                      <FormMessage className='wrap-anywhere' />
                    </FormItem>
                  )}
                />
              </FieldGroup>
            </form>
          </Form>
        </div>
        <DialogFooter className='m-0 grid shrink-0 grid-cols-2 gap-2 rounded-none border-t px-4 py-3 sm:flex sm:px-6'>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={isSaving}
            className='min-h-11 min-w-0 whitespace-normal'
          >
            {t('Cancel')}
          </Button>
          <Button
            type='submit'
            form={VENDOR_MUTATE_FORM_ID}
            disabled={isSaving}
            className='min-h-11 min-w-0 whitespace-normal'
          >
            {isSaving ? (
              <HugeiconsIcon
                icon={Loading03Icon}
                className='size-4 animate-spin'
                aria-hidden='true'
              />
            ) : null}
            {submitLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
