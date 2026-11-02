import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { JsonCodeEditor } from '@/components/json-code-editor'
import { Button } from '@/components/ui/button'
import { FormNavigationGuard } from './form-navigation-guard'

export function SettingsJSONRepair(props: { source: string; canApply: (source: string) => boolean; onApply: (source: string) => boolean }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState(props.source)
  const [baseline, setBaseline] = useState(props.source)
  const opener = useRef<HTMLButtonElement>(null)
  const initialized = useRef(false)
  useEffect(() => {
    if (!open) { initialized.current = false; return }
    if (initialized.current) return
    initialized.current = true
    setDraft(props.source)
    setBaseline(props.source)
  }, [open, props.source])
  return <>
    <FormNavigationGuard when={open && draft !== baseline} />
    <div role='alert' className='text-destructive space-y-3 rounded-lg border p-4 text-sm'>
      <p>{t('This configuration cannot be edited safely as a list. The original JSON has been preserved.')}</p>
      <Button ref={opener} type='button' variant='outline' onClick={() => setOpen(true)}>{t('Repair JSON')}</Button>
    </div>
    <Dialog open={open} onOpenChange={setOpen} returnFocus={() => opener.current} title={t('Repair JSON')}
      description={t('Repairing this JSON updates the local draft. Use the page Save action to persist it.')}
      contentClassName='h-dvh max-h-dvh w-screen max-w-none sm:h-auto sm:max-h-[calc(100dvh-2rem)] sm:max-w-3xl'
      footer={<><Button type='button' variant='outline' onClick={() => setOpen(false)}>{t('Cancel')}</Button><Button type='button' disabled={!props.canApply(draft)} onClick={() => { if (props.onApply(draft)) setOpen(false) }}>{t('Apply to draft')}</Button></>}
    >
      <JsonCodeEditor value={draft} onChange={setDraft} ariaLabel={t('Original JSON')} heightClassName='h-72 min-h-72 max-h-72' />
    </Dialog>
  </>
}
