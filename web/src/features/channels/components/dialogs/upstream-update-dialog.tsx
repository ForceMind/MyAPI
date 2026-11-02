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
import { Search01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState, useMemo, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useChannelTask } from '../../hooks/use-channel-task'

interface UpstreamUpdateDialogProps {
  open: boolean
  addModels: string[]
  removeModels: string[]
  preferredTab: 'add' | 'remove'
  confirmLoading: boolean
  onConfirm: (data: { addModels: string[]; removeModels: string[] }) => void | Promise<void>
  onCancel: () => void
  channelId?: number | null
  returnFocus?: () => HTMLElement | null
}

export function UpstreamUpdateDialog(props: UpstreamUpdateDialogProps) {
  const { t } = useTranslation()
  const task = useChannelTask(props.open, props.channelId)
  const initialized = useRef<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)
  const busy = props.confirmLoading || submitting
  const [activeTab, setActiveTab] = useState(props.preferredTab)
  const [searchAdd, setSearchAdd] = useState('')
  const [searchRemove, setSearchRemove] = useState('')
  const [selectedAdd, setSelectedAdd] = useState<Set<string>>(
    () => new Set(props.addModels)
  )
  const [selectedRemove, setSelectedRemove] = useState<Set<string>>(
    () => new Set(props.removeModels)
  )
  const [partialConfirmOpen, setPartialConfirmOpen] = useState(false)
  useEffect(() => {
    if (!props.open) { initialized.current = null; return }
    const key = JSON.stringify([task.identity, props.channelId])
    if (initialized.current === key) return
    initialized.current = key
    setActiveTab(props.preferredTab)
    setSelectedAdd(new Set(props.addModels))
    setSelectedRemove(new Set(props.removeModels))
    setSearchAdd('')
    setSearchRemove('')
    setSubmitting(false)
    setActionError(null)
    setPartialConfirmOpen(false)
  }, [props.open, props.channelId, task.identity, props.preferredTab, props.addModels, props.removeModels])

  const filteredAdd = useMemo(
    () =>
      props.addModels.filter((m) =>
        m.toLowerCase().includes(searchAdd.toLowerCase())
      ),
    [props.addModels, searchAdd]
  )

  const filteredRemove = useMemo(
    () =>
      props.removeModels.filter((m) =>
        m.toLowerCase().includes(searchRemove.toLowerCase())
      ),
    [props.removeModels, searchRemove]
  )

  const toggleModel = (
    model: string,
    set: Set<string>,
    setter: (s: Set<string>) => void
  ) => {
    const next = new Set(set)
    if (next.has(model)) next.delete(model)
    else next.add(model)
    setter(next)
  }

  const toggleAllVisible = (
    models: string[],
    set: Set<string>,
    setter: (s: Set<string>) => void
  ) => {
    const allSelected = models.every((m) => set.has(m))
    const next = new Set(set)
    if (allSelected) {
      models.forEach((m) => next.delete(m))
    } else {
      models.forEach((m) => next.add(m))
    }
    setter(next)
  }

  const submitSelection = async () => {
    if (props.confirmLoading) return
    const operation = task.begin()
    if (!operation) return
    setSubmitting(true)
    setActionError(null)
    try {
      await props.onConfirm({ addModels: [...selectedAdd], removeModels: [...selectedRemove] })
    } catch (error) {
      if (operation.isCurrent()) setActionError(error instanceof Error ? error.message : t('Operation failed'))
    } finally {
      if (operation.isCurrent()) setSubmitting(false)
      operation.finish()
    }
  }

  const handleConfirm = () => {
    if (busy || task.isPending()) return
    const hasAdd = props.addModels.length > 0
    const hasRemove = props.removeModels.length > 0
    const selectedAddArr = [...selectedAdd]
    const selectedRemoveArr = [...selectedRemove]
    const anyAdd = selectedAddArr.length > 0
    const anyRemove = selectedRemoveArr.length > 0

    if (hasAdd && hasRemove && anyAdd !== anyRemove) {
      setPartialConfirmOpen(true)
      return
    }

    void submitSelection()
  }

  return (
    <>
      <Dialog
        open={props.open}
        onOpenChange={(v) => { if (!v && !busy && !task.isPending()) props.onCancel() }}
        returnFocus={props.returnFocus}
        showCloseButton={!busy}
        title={t('Upstream Model Updates')}
        contentClassName='h-dvh max-h-dvh w-screen max-w-none sm:h-auto sm:max-h-[calc(100dvh-2rem)] sm:max-w-2xl [&_button]:min-h-11 [&_button]:min-w-11 [&_input]:min-h-11'
        contentHeight='auto'
        bodyClassName='space-y-4'
        footer={
          <>
            <Button variant='outline' disabled={busy} onClick={props.onCancel}>
              {t('Cancel')}
            </Button>
            <Button
              onClick={handleConfirm}
              disabled={
                busy ||
                (props.addModels.length === 0 &&
                  props.removeModels.length === 0)
              }
            >
              {t('Confirm')}
            </Button>
          </>
        }
      >
        {actionError ? <div role='alert' className='text-destructive text-sm break-words'>{actionError}</div> : null}
        <fieldset disabled={busy} className='contents'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Select models to process. Unselected "add" models will be ignored.'
          )}
        </p>
        <Tabs
          value={activeTab}
          onValueChange={(v) => setActiveTab(v as 'add' | 'remove')}
        >
          <TabsList className='grid w-full grid-cols-2'>
            <TabsTrigger value='add' className='gap-1'>
              {t('Add Models')}
              <StatusBadge variant='neutral' className='ml-1' copyable={false}>
                {selectedAdd.size}/{props.addModels.length}
              </StatusBadge>
            </TabsTrigger>
            <TabsTrigger value='remove' className='gap-1'>
              {t('Remove Models')}
              <StatusBadge variant='neutral' className='ml-1' copyable={false}>
                {selectedRemove.size}/{props.removeModels.length}
              </StatusBadge>
            </TabsTrigger>
          </TabsList>

          <TabsContent value='add' className='space-y-3'>
            <div className='relative'>
              <HugeiconsIcon icon={Search01Icon} aria-hidden='true' className='text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4' />
              <Input
                placeholder={t('Search models...')}
                className='pl-8'
                value={searchAdd}
                onChange={(e) => setSearchAdd(e.target.value)}
              />
            </div>
            {filteredAdd.length > 0 && (
              <div className='flex items-center gap-2'>
                <Checkbox
                  checked={filteredAdd.every((m) => selectedAdd.has(m))}
                  onCheckedChange={() =>
                    toggleAllVisible(filteredAdd, selectedAdd, setSelectedAdd)
                  }
                />
                <span className='text-muted-foreground text-xs'>
                  {t('Select All Visible')}
                </span>
              </div>
            )}
            <div className='rounded-md border p-2'>
              {filteredAdd.length > 0 ? (
                <div className='space-y-1'>
                  {filteredAdd.map((model) => (
                    <label
                      key={model}
                      className='hover:bg-accent flex min-h-11 cursor-pointer items-start gap-2 rounded px-2 py-2'
                    >
                      <Checkbox
                        checked={selectedAdd.has(model)}
                        onCheckedChange={() =>
                          toggleModel(model, selectedAdd, setSelectedAdd)
                        }
                      />
                      <span className='min-w-0 text-sm break-all'>{model}</span>
                    </label>
                  ))}
                </div>
              ) : (
                <p className='text-muted-foreground py-8 text-center text-sm'>
                  {props.addModels.length === 0
                    ? t('No models to add')
                    : t('No matching results')}
                </p>
              )}
            </div>
          </TabsContent>

          <TabsContent value='remove' className='space-y-3'>
            <div className='relative'>
              <HugeiconsIcon icon={Search01Icon} aria-hidden='true' className='text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4' />
              <Input
                placeholder={t('Search models...')}
                className='pl-8'
                value={searchRemove}
                onChange={(e) => setSearchRemove(e.target.value)}
              />
            </div>
            {filteredRemove.length > 0 && (
              <div className='flex items-center gap-2'>
                <Checkbox
                  checked={filteredRemove.every((m) => selectedRemove.has(m))}
                  onCheckedChange={() =>
                    toggleAllVisible(
                      filteredRemove,
                      selectedRemove,
                      setSelectedRemove
                    )
                  }
                />
                <span className='text-muted-foreground text-xs'>
                  {t('Select All Visible')}
                </span>
              </div>
            )}
            <div className='rounded-md border p-2'>
              {filteredRemove.length > 0 ? (
                <div className='space-y-1'>
                  {filteredRemove.map((model) => (
                    <label
                      key={model}
                      className='hover:bg-accent flex min-h-11 cursor-pointer items-start gap-2 rounded px-2 py-2'
                    >
                      <Checkbox
                        checked={selectedRemove.has(model)}
                        onCheckedChange={() =>
                          toggleModel(model, selectedRemove, setSelectedRemove)
                        }
                      />
                      <span className='min-w-0 text-sm break-all'>{model}</span>
                    </label>
                  ))}
                </div>
              ) : (
                <p className='text-muted-foreground py-8 text-center text-sm'>
                  {props.removeModels.length === 0
                    ? t('No models to remove')
                    : t('No matching results')}
                </p>
              )}
            </div>
          </TabsContent>
        </Tabs>
        </fieldset>
      </Dialog>

      <ConfirmDialog
        open={partialConfirmOpen}
        onOpenChange={(next) => { if (!busy && !task.isPending()) setPartialConfirmOpen(next) }}
        isLoading={busy}
        title={t('Partial Submission')}
        desc={t(
          'There are both add and remove models pending, but you only selected one type. Confirm submitting only the selected items?'
        )}
        handleConfirm={() => {
          setPartialConfirmOpen(false)
          void submitSelection()
        }}
      />
    </>
  )
}
