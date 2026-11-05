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
import { useState, useEffect, useId, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { ComboboxInput } from '@/components/ui/combobox-input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { useKeyClientModels } from '../../hooks/use-key-client-models'
import { useKeyMaintenanceSession } from '../../hooks/use-key-maintenance-session'
import { keyClientModelAllowed, keyClientModels, savedKeyClientAddresses, type KeyClientAddresses, type KeyClientScope } from '../../lib/client-configuration'

const APP_CONFIGS = {
  claude: {
    label: 'Claude',
    defaultName: 'My Claude',
    modelFields: [
      { key: 'model', labelKey: 'Primary Model', required: true },
      { key: 'haikuModel', labelKey: 'Haiku Model', required: false },
      { key: 'sonnetModel', labelKey: 'Sonnet Model', required: false },
      { key: 'opusModel', labelKey: 'Opus Model', required: false },
    ],
  },
  codex: {
    label: 'Codex',
    defaultName: 'My Codex',
    modelFields: [{ key: 'model', labelKey: 'Primary Model', required: true }],
  },
  gemini: {
    label: 'Gemini',
    defaultName: 'My Gemini',
    modelFields: [{ key: 'model', labelKey: 'Primary Model', required: true }],
  },
} as const

type AppType = keyof typeof APP_CONFIGS

function buildCCSwitchURL(
  app: string,
  name: string,
  models: Record<string, string>,
  apiKey: string,
  address: KeyClientAddresses
): string {
  const endpoint = app === 'codex' ? address.openAI : address.root
  const params = new URLSearchParams()
  params.set('resource', 'provider')
  params.set('app', app)
  params.set('name', name)
  params.set('endpoint', endpoint)
  params.set('apiKey', apiKey)
  for (const [k, v] of Object.entries(models)) {
    if (v) params.set(k, v)
  }
  params.set('homepage', address.root)
  params.set('enabled', 'true')
  return `ccswitch://v1/import?${params.toString()}`
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  tokenKey: string
  apiKey?: KeyClientScope
  selectedModel?: string
  address?: KeyClientAddresses
}

export function CCSwitchDialog(props: Props) {
  const { t } = useTranslation()
  const isCurrentSession = useKeyMaintenanceSession()
  const [app, setApp] = useState<AppType>('claude')
  const [name, setName] = useState<string>(APP_CONFIGS.claude.defaultName)
  const [models, setModels] = useState<Record<string, string>>({})
  const wasOpen = useRef(false)
  const id = useId()

  const modelsQuery = useKeyClientModels(props.open && isCurrentSession())

  const modelsData = modelsQuery.data
  const modelOptions = useMemo(() => {
    const items = keyClientModels(modelsData ?? [], props.apiKey)
    return items.map((m) => ({ value: m, label: m }))
  }, [modelsData, props.apiKey])

  useEffect(() => {
    const opening = props.open && !wasOpen.current
    wasOpen.current = props.open
    if (opening) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setModels(props.selectedModel ? { model: props.selectedModel } : {})

      setApp('claude')

      setName(APP_CONFIGS.claude.defaultName)
    }
  }, [props.open, props.selectedModel])

  const currentConfig = APP_CONFIGS[app]
  const address = props.address ?? savedKeyClientAddresses()
  const endpoint = app === 'codex' ? address.openAI : address.root
  const modelsWithinScope = Object.values(models).every((model) => !model || keyClientModelAllowed(model, props.apiKey))

  const handleAppChange = (val: string) => {
    const appVal = val as AppType
    setApp(appVal)
    setName(APP_CONFIGS[appVal].defaultName)
    setModels({})
  }

  const handleSubmit = () => {
    if (!isCurrentSession() || !props.tokenKey) return
    if (!models.model) {
      toast.warning(t('Please select a primary model'))
      return
    }
    if (!modelsWithinScope) {
      toast.warning(t('Please select a candidate model for this Key'))
      return
    }
    const key = props.tokenKey.startsWith('sk-')
      ? props.tokenKey
      : `sk-${props.tokenKey}`
    const url = buildCCSwitchURL(app, name, models, key, address)
    window.open(url, '_blank')
    props.onOpenChange(false)
  }

  if (!isCurrentSession()) return null

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='flex h-dvh max-h-dvh w-full max-w-none flex-col gap-0 overflow-hidden rounded-none p-0 sm:h-auto sm:max-h-[90dvh] sm:max-w-xl sm:rounded-xl [&_[data-slot=dialog-close]]:min-h-11 [&_[data-slot=dialog-close]]:min-w-11'>
        <DialogHeader className='shrink-0 border-b px-4 py-4 pr-16 sm:px-6 sm:pr-16'>
          <DialogTitle>{t('Import to CC Switch')}</DialogTitle>
        </DialogHeader>
        <section role='region' aria-label={t('CC Switch import task')} className='flex min-h-0 min-w-0 flex-1 flex-col gap-4 overflow-y-auto overscroll-contain px-4 py-5 [overflow-wrap:anywhere] sm:px-6 [&_[role=listbox]]:max-h-none [&_[role=listbox]]:overflow-visible [&_div:has(>[role=listbox])]:static [&_[role=option]]:min-h-11 [&_[role=option]]:items-start [&_[role=option]_span]:overflow-visible [&_[role=option]_span]:text-clip [&_[role=option]_span]:whitespace-normal [&_[role=option]_span]:[overflow-wrap:anywhere]'>
          {modelsQuery.isFetching && <p role='status' className='text-muted-foreground text-sm'>{t('Loading...')}</p>}
          {modelsQuery.isError && <Alert variant='destructive'><AlertTitle>{t('Unable to load available models')}</AlertTitle><AlertDescription>{modelsQuery.error instanceof Error ? modelsQuery.error.message : t('Please try again.')}</AlertDescription><Button className='mt-3 min-h-11 w-fit' variant='outline' disabled={modelsQuery.isFetching} onClick={() => { void modelsQuery.refetch() }}>{t('Retry')}</Button></Alert>}
          <p className='text-caption text-muted-foreground'>{t('Candidates follow this Key’s restrictions. Protocol support and upstream availability have not been tested.')}</p>
          <p className='text-caption text-muted-foreground'>{t('You can enter a model ID manually. Its protocol and availability remain unverified.')}</p>
          <fieldset className='min-w-0'>
            <legend className='mb-2 text-sm font-medium'>{t('Application')}</legend>
            <RadioGroup aria-label={t('Application')} value={app} onValueChange={handleAppChange} className='grid gap-2 sm:grid-cols-3'>
              {(Object.entries(APP_CONFIGS) as [AppType, (typeof APP_CONFIGS)[AppType]][]).map(([key, config]) => <div key={key} className='flex min-h-11 min-w-0 items-center gap-2 rounded-lg border px-3'>
                <RadioGroupItem value={key} id={`${id}-app-${key}`} aria-labelledby={`${id}-app-${key}-label`} />
                <Label id={`${id}-app-${key}-label`} htmlFor={`${id}-app-${key}`} className='min-h-11 flex-1 cursor-pointer'>{config.label}</Label>
              </div>)}
            </RadioGroup>
          </fieldset>
          <dl className='flex min-w-0 flex-col gap-1'>
            <dt className='text-caption text-muted-foreground'>{t('Client endpoint')}</dt>
            <dd className='text-data font-mono wrap-anywhere'>{endpoint}</dd>
            <dd className='text-caption text-muted-foreground'>{address.source === 'setting' ? t('From the server address setting') : t('Current access address')}</dd>
          </dl>
          <p className='text-caption text-muted-foreground'>{t('Claude and Gemini use the installation root; Codex uses /v1. Choose models that support the selected client’s API.')}</p>
          <div className='flex min-w-0 flex-col gap-2'>
            <Label htmlFor={`${id}-name`}>{t('Name')}</Label>
            <ComboboxInput id={`${id}-name`} className='min-h-11' options={[]} value={name} onValueChange={setName} placeholder={currentConfig.defaultName} emptyText='' allowCustomValue />
          </div>
          {currentConfig.modelFields.map((field) => <div key={field.key} className='flex min-w-0 flex-col gap-2'>
            <Label htmlFor={`${id}-${field.key}`}>{t(field.labelKey)}{field.required && ' '}{field.required && <span className='text-destructive'>*</span>}</Label>
            <ComboboxInput id={`${id}-${field.key}`} className='min-h-11' options={modelOptions} value={models[field.key] || ''} onValueChange={(value) => setModels((current) => ({ ...current, [field.key]: value }))} placeholder={t('Select or enter model name')} emptyText={t('No models found')} allowCustomValue />
          </div>)}
          {!modelsWithinScope && <p role='alert' className='text-destructive text-sm'>{t('One or more model IDs are outside this Key’s restrictions.')}</p>}
        </section>
        <DialogFooter role='group' aria-label={t('Actions')} className='m-0 shrink-0 flex-row flex-wrap justify-end rounded-none px-4 py-4 sm:px-6'>
          <Button className='min-h-11' variant='outline' onClick={() => props.onOpenChange(false)}>{t('Cancel')}</Button>
          <Button className='min-h-11' disabled={!models.model || !modelsWithinScope} onClick={handleSubmit}>{t('Open CC Switch')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
