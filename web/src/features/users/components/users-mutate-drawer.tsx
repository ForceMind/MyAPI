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
import { useQuery } from '@tanstack/react-query'
import { Pencil } from 'lucide-react'
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
import { SummaryDisclosure } from '@/components/summary-disclosure'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { Label } from '@/components/ui/label'
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
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  EMPTY_PERMISSION_CATALOG,
  hasPermission,
  normalizeAdminPermissions,
} from '@/lib/admin-permissions'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { formatQuota, parseQuotaFromDollars } from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  createUser,
  updateUser,
  getUser,
  getGroups,
  getPermissionCatalog,
} from '../api'
import { BINDING_FIELDS, ERROR_MESSAGES, SUCCESS_MESSAGES } from '../constants'
import {
  userFormSchema,
  type UserFormValues,
  USER_FORM_DEFAULT_VALUES,
  transformFormDataToPayload,
  transformUserToFormDefaults,
  getAccountTierDescription,
  getAccountTierLabel,
  getAccountTierId,
} from '../lib'
import type { User } from '../types'
import { UserQuotaDialog } from './user-quota-dialog'
import { useUsers } from './users-provider'
import { useUserTask } from '../hooks/use-user-task'

type UsersMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: User
  returnFocus?: () => HTMLElement | null
}

export function UsersMutateDrawer({
  open,
  onOpenChange,
  currentRow,
  returnFocus,
}: UsersMutateDrawerProps) {
  const { t } = useTranslation()
  const isUpdate = !!currentRow
  const { triggerRefresh } = useUsers()
  const currentUser = useAuthStore((s) => s.auth.user)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [quotaDialogOpen, setQuotaDialogOpen] = useState(false)
  const quotaOpener = useRef<HTMLButtonElement>(null)
  const task = useUserTask(open, currentRow?.id)
  const initialized = useRef<string | null>(null)
  const [readError, setReadError] = useState<string | null>(null)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [detailLoaded, setDetailLoaded] = useState(!isUpdate)
  const [readAttempt, setReadAttempt] = useState(0)
  const [quotaReadError, setQuotaReadError] = useState<string | null>(null)
  const [quotaReading, setQuotaReading] = useState(false)

  // Fetch groups
  const { data: groupsData } = useQuery({
    queryKey: ['groups', task.identity],
    queryFn: getGroups,
    staleTime: 5 * 60 * 1000,
  })

  const groups = groupsData?.data || []

  // Permission catalog is owned by the backend; fetched once and reused.
  const permissionCatalogQuery = useQuery({
    queryKey: ['admin-permission-catalog', task.identity],
    queryFn: getPermissionCatalog,
    staleTime: 5 * 60 * 1000,
  })
  const permissionCatalog = permissionCatalogQuery.data ?? EMPTY_PERMISSION_CATALOG
  const permissionCatalogReading = permissionCatalogQuery.isPending || permissionCatalogQuery.isFetching
  const permissionCatalogConfirmed = !permissionCatalogReading && !permissionCatalogQuery.isError

  const form = useForm<UserFormValues>({
    resolver: zodResolver(userFormSchema),
    defaultValues: USER_FORM_DEFAULT_VALUES,
  })

  // Load existing data when updating
  useEffect(() => {
    if (!open) { initialized.current = null; return }
    const snapshot = task.capture()
    const key = JSON.stringify([task.identity, currentRow?.id ?? 'create', readAttempt])
    if (initialized.current === key) return
    initialized.current = key
    setReadError(null)
    setSaveError(null)
    setQuotaReadError(null)
    setQuotaReading(false)
    setIsSubmitting(false)
    setDetailLoaded(!isUpdate)
    if (open && isUpdate && currentRow) {
      // For update, fetch fresh data
      void getUser(currentRow.id)
        .then((result) => {
          if (!snapshot.isCurrent()) return
          if (result.success && result.data && result.data.id === currentRow.id) {
            form.reset(transformUserToFormDefaults(result.data))
            setDetailLoaded(true)
          } else setReadError(result.message || t('Failed to load user'))
        })
        .catch((error: unknown) => {
          if (!snapshot.isCurrent()) return
          setReadError(t('Failed to load user'))
          // eslint-disable-next-line no-console
          console.error('Failed to load user:', error)
        })
    } else if (open && !isUpdate) {
      // For create, reset to defaults
      form.reset(USER_FORM_DEFAULT_VALUES)
    }
  }, [open, isUpdate, currentRow, form, task, t, readAttempt])

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'

  const currentQuotaRaw = form.watch('quota_dollars') || 0
  let quotaDisplay = tokensOnly ? String(currentQuotaRaw || 0) : (currentQuotaRaw || 0).toFixed(6)
  if (quotaReadError || quotaReading) quotaDisplay = t('Unknown')
  const selectedRole = form.watch('role')
  const selectedGroup = form.watch('group')
  const explicitAccountTierId = form.watch('account_tier_id')
  const derivedAccountTierId = getAccountTierId(selectedGroup)
  const hasExplicitTierMismatch =
    isUpdate &&
    Boolean(explicitAccountTierId) &&
    explicitAccountTierId !== derivedAccountTierId
  const canEditAdminPermissions = currentUser?.role === ROLE.SUPER_ADMIN
  const targetIsAdmin = (selectedRole ?? currentRow?.role ?? 0) >= ROLE.ADMIN

  const onSubmit = async (data: UserFormValues) => {
    if (isUpdate && !detailLoaded) return
    if (!isUpdate) {
      const passwordLength = data.password?.length || 0
      if (passwordLength < 8 || passwordLength > 20) {
        form.setError('password', {
          type: 'manual',
          message: t('Password must be between 8 and 20 characters'),
        })
        return
      }
    }

    const operation = task.begin()
    if (!operation) return

    setIsSubmitting(true)
    setSaveError(null)
    try {
      const payload = transformFormDataToPayload(
        data,
        currentRow?.id,
        permissionCatalog
      )
      const result = isUpdate
        ? await updateUser(payload as typeof payload & { id: number })
        : await createUser(payload)
      if (!operation.isCurrent()) return

      if (result.success) {
        toast.success(
          isUpdate
            ? t(SUCCESS_MESSAGES.USER_UPDATED)
            : t(SUCCESS_MESSAGES.USER_CREATED)
        )
        onOpenChange(false)
        triggerRefresh()
      } else {
        setSaveError(result.message || (isUpdate ? t(ERROR_MESSAGES.UPDATE_FAILED) : t(ERROR_MESSAGES.CREATE_FAILED)))
        toast.error(
          result.message ||
            (isUpdate
              ? t(ERROR_MESSAGES.UPDATE_FAILED)
              : t(ERROR_MESSAGES.CREATE_FAILED))
        )
      }
    } catch {
      if (!operation.isCurrent()) return
      setSaveError(t(ERROR_MESSAGES.UNEXPECTED))
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      if (operation.isCurrent()) setIsSubmitting(false)
      operation.finish()
    }
  }

  const refreshUserData = async () => {
    if (!currentRow) return
    const snapshot = task.capture()
    triggerRefresh()
    setQuotaReading(true)
    setQuotaReadError(null)
    try {
      const result = await getUser(currentRow.id)
      if (!snapshot.isCurrent()) return
      if (result.success && result.data && result.data.id === currentRow.id) {
        form.setValue('quota_dollars', transformUserToFormDefaults(result.data).quota_dollars)
        setQuotaReadError(null)
      } else setQuotaReadError(t('Failed to load user'))
    } catch {
      if (snapshot.isCurrent()) setQuotaReadError(t('Failed to load user'))
    } finally {
      if (snapshot.isCurrent()) setQuotaReading(false)
    }
  }

  return (
    <>
      <Sheet
        open={open}
        onOpenChange={(v) => {
          if (task.isPending()) return
          onOpenChange(v)
          if (!v) {
            form.reset()
          }
        }}
      >
        <SheetContent
          showCloseButton={!isSubmitting}
          finalFocus={returnFocus}
          className={sideDrawerContentClassName('sm:max-w-[680px] [&_[data-slot=button]]:min-h-11 [&_[data-slot=button]]:min-w-11 [&_[data-slot=select-trigger]]:min-h-11 [&_input]:min-h-11 [&_textarea]:min-h-11')}
        >
          <SheetHeader className={sideDrawerHeaderClassName()}>
            <SheetTitle>
              {isUpdate ? t('Update') : t('Create')} {t('User')}
            </SheetTitle>
            <SheetDescription>
              {isUpdate
                ? t('Update the user by providing necessary info.')
                : t('Add a new user by providing necessary info.')}
            </SheetDescription>
          </SheetHeader>
          <Form {...form}>
            <form
              id='user-form'
              onSubmit={form.handleSubmit(onSubmit)}
              className={sideDrawerFormClassName()}
            >
              {currentRow ? <p className='text-muted-foreground text-sm break-words'>{t('User ID')}: <span className='font-mono'>{currentRow.id}</span> · {currentRow.username}</p> : null}
              {readError ? <div role='alert' className='text-destructive space-y-3 text-sm break-words'><p>{readError}</p><Button type='button' variant='outline' onClick={() => setReadAttempt((value) => value + 1)}>{t('Retry')}</Button></div> : null}
              {saveError ? <div role='alert' className='text-destructive text-sm break-words'>{saveError}</div> : null}
              {quotaReading ? <p role='status'>{t('Loading...')}</p> : null}
              {quotaReadError ? <div role='alert' className='text-destructive space-y-3 text-sm'><p>{quotaReadError}</p><Button type='button' variant='outline' disabled={isSubmitting || quotaReading} onClick={() => void refreshUserData()}>{t('Refresh')}</Button></div> : null}
              <fieldset disabled={isSubmitting || (isUpdate && !detailLoaded)} className='contents'>
              {/* Basic Information */}
              <SideDrawerSection>
                <h3 className='text-sm font-medium'>
                  {t('Basic Information')}
                </h3>

                <FormField
                  control={form.control}
                  name='username'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Username')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder={t('Enter username')}
                          disabled={isUpdate}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {!isUpdate && (
                  <FormField
                    control={form.control}
                    name='role'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Role')}</FormLabel>
                        <Select
                          items={[
                            { value: '1', label: t('Common User') },
                            { value: '10', label: t('Admin') },
                          ]}
                          onValueChange={(value) =>
                            value !== null &&
                            field.onChange(Number.parseInt(value))
                          }
                          value={String(field.value)}
                        >
                          <FormControl>
                            <SelectTrigger>
                              <SelectValue placeholder={t('Select a role')} />
                            </SelectTrigger>
                          </FormControl>
                          <SelectContent alignItemWithTrigger={false}>
                            <SelectGroup>
                              <SelectItem value='1'>
                                {t('Common User')}
                              </SelectItem>
                              <SelectItem value='10'>{t('Admin')}</SelectItem>
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                        <FormDescription>
                          {t("Set the user's role (cannot be Root)")}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                )}

                <FormField
                  control={form.control}
                  name='display_name'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Display Name')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder={t('Enter display name')}
                        />
                      </FormControl>
                      <FormDescription>
                        {t('Leave empty to use username')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='password'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Password')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='password'
                          placeholder={
                            isUpdate
                              ? t('Leave empty to keep unchanged')
                              : t('Enter password (8-20 characters)')
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </SideDrawerSection>

              {/* Account tier & quota settings (Update only) */}
              {isUpdate && (
                <SideDrawerSection>
                  <h3 className='text-sm font-medium'>
                    {t('Account tier & quota')}
                  </h3>

                  <FormField
                    control={form.control}
                    name='group'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Account tier')}</FormLabel>
                        <FormDescription>
                          {getAccountTierDescription(field.value, t)}{' '}
                          {t(
                            'This is separate from the access profile selected for each API key.'
                          )}
                        </FormDescription>
                        <Select
                          items={groups.map((group) => ({
                            value: group,
                            label: getAccountTierLabel(group, t),
                          }))}
                          onValueChange={(value) => {
                            field.onChange(value)
                            form.setValue(
                              'account_tier_id',
                              getAccountTierId(value ?? undefined),
                              { shouldDirty: true }
                            )
                          }}
                          value={field.value}
                        >
                          <FormControl>
                            <SelectTrigger>
                              <SelectValue placeholder={t('Select a group')} />
                            </SelectTrigger>
                          </FormControl>
                          <SelectContent alignItemWithTrigger={false}>
                            <SelectGroup>
                              {groups.map((group) => (
                                <SelectItem key={group} value={group}>
                                  {getAccountTierLabel(group, t)}
                                </SelectItem>
                              ))}
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                        <div className='text-muted-foreground text-xs'>
                          {t('Stable account tier ID')}:{' '}
                          {explicitAccountTierId || derivedAccountTierId}
                        </div>
                        {hasExplicitTierMismatch && (
                          <div
                            className='border-warning/40 bg-warning/10 text-warning rounded-md border p-2 text-xs'
                            role='status'
                          >
                            ⚠ {t('Account tier')} {explicitAccountTierId} ≠{' '}
                            {derivedAccountTierId} ·{' '}
                            {t(
                              'This is separate from the access profile selected for each API key.'
                            )}
                          </div>
                        )}
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='quota_dollars'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {t('Remaining Quota ({{currency}})', {
                            currency: currencyLabel,
                          })}
                        </FormLabel>
                        <div className='flex gap-2'>
                          <FormControl>
                            <Input
                              value={quotaDisplay}
                              readOnly
                              className='flex-1'
                            />
                          </FormControl>
                          <Button
                            type='button'
                            variant='outline'
                            ref={quotaOpener}
                            disabled={Boolean(quotaReadError) || quotaReading}
                            onClick={() => setQuotaDialogOpen(true)}
                          >
                            <Pencil className='mr-1 h-4 w-4' />
                            {t('Adjust Quota')}
                          </Button>
                        </div>
                        <FormDescription>
                          {quotaReadError || quotaReading ? t('Unknown') : formatQuota(parseQuotaFromDollars(field.value || 0))}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='remark'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Remark')}</FormLabel>
                        <FormControl>
                          <Textarea
                            {...field}
                            placeholder={t(
                              'Admin notes (only visible to admins)'
                            )}
                            rows={3}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </SideDrawerSection>
              )}

              {(isUpdate || targetIsAdmin) && (
                <p className='text-muted-foreground text-xs'>
                  {t('Role')}: {detailLoaded ? selectedRole ?? t('Unknown') : t('Unknown')}
                  {targetIsAdmin && <> · {t('Only Root can edit administrator permissions.')}</>}
                </p>
              )}
              {canEditAdminPermissions &&
                targetIsAdmin && (
                  <SideDrawerSection>
                    <h3 className='text-sm font-medium'>
                      {t('Admin Permissions')}
                    </h3>
                    {permissionCatalogReading && (
                      <p role='status' className='text-muted-foreground text-sm'>
                        {t('Permission catalog')}: {t('Unknown')} · {t('Loading...')}
                      </p>
                    )}
                    {!permissionCatalogReading && permissionCatalogQuery.isError && (
                      <div role='alert' className='text-destructive space-y-3 text-sm'>
                        <p>{t('Unable to load the permission catalog.')} {t('Permission catalog')}: {t('Unknown')}</p>
                        <Button type='button' variant='outline' onClick={() => { void permissionCatalogQuery.refetch() }}>{t('Retry')}</Button>
                      </div>
                    )}
                    {permissionCatalogConfirmed && permissionCatalog.resources.length === 0 && (
                      <p className='text-muted-foreground text-sm'>{t('No administrator permissions are listed in this catalog.')}</p>
                    )}
                    {permissionCatalogConfirmed && permissionCatalog.resources.length > 0 && (
                    <>
                    <p className='text-muted-foreground text-xs'>
                      {t(
                        'Default administrator permissions can be overridden for this user.'
                      )}
                    </p>
                    <p className='text-muted-foreground text-xs'>
                      {t('True grants permission, false denies it, and omitted values inherit the administrator default.')}
                    </p>
                    <FormField
                      control={form.control}
                      name='admin_permissions'
                      render={({ field }) => {
                        const selected = normalizeAdminPermissions(
                          field.value,
                          permissionCatalog
                        )
                        return (
                          <FormItem>
                            <div className='space-y-3'>
                              {permissionCatalog.resources.map((resource) => {
                                const values = field.value?.[resource.resource]
                                const granted = resource.actions.filter((option) => values?.[option.action] === true).length
                                const denied = resource.actions.filter((option) => values?.[option.action] === false).length
                                const inherited = resource.actions.length - granted - denied
                                return (
                                  <div key={resource.resource} className='min-w-0 rounded-md border p-3'>
                                    <SummaryDisclosure summary={t('{{resource}} · True: {{granted}} · False: {{denied}} · Inherited: {{inherited}}', {
                                      resource: t(resource.label_key), granted, denied, inherited, interpolation: { escapeValue: false },
                                    })}>
                                      <div className='space-y-2'>
                                        {resource.actions.map((option) => {
                                          const value = values?.[option.action]
                                          let valueLabel = t('Inherited default: {{value}}', { value: selected[resource.resource]?.[option.action] === true ? t('Granted') : t('Denied') })
                                          if (value === true) valueLabel = t('Granted (true)')
                                          if (value === false) valueLabel = t('Denied (false)')
                                          return (
                                            <div key={option.action} className='min-w-0 space-y-1'>
                                              <label className='flex min-h-11 min-w-0 items-start gap-3 py-1'>
                                                <Checkbox
                                                  checked={selected[resource.resource]?.[option.action] === true}
                                                  onCheckedChange={(checked) => {
                                                    field.onChange({
                                                      ...selected,
                                                      [resource.resource]: {
                                                        ...selected[resource.resource],
                                                        [option.action]: checked === true,
                                                      },
                                                    })
                                                  }}
                                                />
                                                <span className='flex min-w-0 flex-col gap-1 break-words'>
                                                  <span className='text-sm font-medium'>{t(option.label_key)}</span>
                                                  <span className='text-muted-foreground text-xs'>{t(option.description_key)}</span>
                                                </span>
                                              </label>
                                              <p className='text-muted-foreground pl-7 text-xs'>{valueLabel}</p>
                                            </div>
                                          )
                                        })}
                                      </div>
                                    </SummaryDisclosure>
                                  </div>
                                )
                              })}
                            </div>
                            <FormMessage />
                          </FormItem>
                        )
                      }}
                    />
                    </>
                    )}
                    {currentUser && (
                      <p className='text-muted-foreground text-xs'>
                        {hasPermission(
                          currentUser,
                          ADMIN_PERMISSION_RESOURCES.CHANNEL,
                          ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
                        )
                          ? t(
                              'Your account can edit sensitive channel settings.'
                            )
                          : t(
                              'Your account cannot edit sensitive channel settings.'
                            )}
                      </p>
                    )}
                  </SideDrawerSection>
                )}

              {/* Binding Information (Read-only) */}
              {isUpdate && (
                <SideDrawerSection>
                  <h3 className='text-sm font-medium'>
                    {t('Binding Information')}
                  </h3>
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      'Third-party account bindings (read-only, managed by user in profile settings)'
                    )}
                  </p>

                  <SummaryDisclosure summary={t('Binding values ({{count}})', { count: BINDING_FIELDS.length })}>
                  <div className='flex flex-col gap-3'>
                    {BINDING_FIELDS.map(({ key, label }) => (
                      <div key={key}>
                        <Label className='text-muted-foreground text-xs'>
                          {t(label)}
                        </Label>
                        <Input
                          value={
                            (currentRow?.[key as keyof User] as string) || '-'
                          }
                          disabled
                          className='mt-1'
                        />
                      </div>
                    ))}
                  </div>
                  </SummaryDisclosure>
                </SideDrawerSection>
              )}
              </fieldset>
            </form>
          </Form>
          <SheetFooter className={sideDrawerFooterClassName()}>
            <SheetClose render={<Button variant='outline' />}>
              {t('Close')}
            </SheetClose>
            <Button form='user-form' type='submit' disabled={isSubmitting || (isUpdate && !detailLoaded)}>
              {isSubmitting ? t('Saving...') : t('Save changes')}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      {/* Adjust Quota Dialog */}
      {currentRow && (
        <UserQuotaDialog
          open={quotaDialogOpen}
          onOpenChange={setQuotaDialogOpen}
          returnFocus={() => quotaOpener.current}
          userId={currentRow.id}
          currentQuota={parseQuotaFromDollars(currentQuotaRaw || 0)}
          onSuccess={refreshUserData}
        />
      )}
    </>
  )
}
