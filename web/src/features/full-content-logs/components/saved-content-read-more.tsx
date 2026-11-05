/* Copyright (C) 2026 ForceMind. AGPL-3.0-or-later. */
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

import type { useFullContentLogDetail } from '../hooks/use-full-content-log-detail'

/** The larger read is explicit, bounded and limited to the same saved request. */
export function SavedContentReadMore(props: { content: ReturnType<typeof useFullContentLogDetail> }) {
  const { t } = useTranslation()
  const content = props.content
  if (!content.canRead || !content.detail || (!content.detail.response_body_truncated && !content.expanded)) return null
  return <div className='min-w-0 space-y-2'>
    {!content.expanded && <Button variant='outline' className='min-h-11' disabled={content.expanding} onClick={content.readExpanded}>{t('Read more saved content (up to 16 MiB)')}</Button>}
    {content.expanding && <p role='status'>{t('Loading...')}</p>}
    {content.expansionError && <Alert variant='destructive'>
      <AlertTitle>{t('Could not load the saved body')}</AlertTitle>
      <AlertDescription>{content.expansionError.message}</AlertDescription>
    </Alert>}
    {content.expanded && <p role='status' className='text-caption text-muted-foreground'>{content.detail.response_body_truncated
      ? t('Saved content is still truncated at the 16 MiB read limit. The remainder is unavailable in this view.')
      : t('Loaded saved content with the 16 MiB read limit.')}</p>}
  </div>
}
