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
import { FileTextIcon, XIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import type { ChatAttachment } from '../../types'

interface PlaygroundAttachmentPreviewsProps {
  attachments: ChatAttachment[]
  disabled?: boolean
  onRemove?: (id: string) => void
}

export function PlaygroundAttachmentPreviews(
  props: PlaygroundAttachmentPreviewsProps
) {
  const { t } = useTranslation()
  return (
    <ul aria-label={t('Attachments')} className='flex flex-wrap gap-3 p-3'>
      {props.attachments.map((image) => (
        <li key={image.id} className='relative w-28'>
          {image.mimeType === 'application/pdf' ? (
            <div
              role='img'
              aria-label={image.name}
              className='flex h-24 w-28 items-center justify-center gap-1 rounded-md border'
            >
              <FileTextIcon aria-hidden='true' size={24} />
              <span>PDF</span>
            </div>
          ) : (
            <img
              src={image.dataUrl}
              alt={image.name}
              className='h-24 w-28 rounded-md border object-contain'
            />
          )}
          <p
            className='text-muted-foreground truncate text-xs'
            title={image.name}
          >
            {image.name}
          </p>
          {props.onRemove && (
            <Button
              type='button'
              aria-label={t('Remove attachment {{name}}', { name: image.name })}
              size='icon'
              variant='secondary'
              className='absolute top-0 right-0 size-6'
              disabled={props.disabled}
              onClick={() => props.onRemove?.(image.id)}
            >
              <XIcon aria-hidden='true' size={14} />
            </Button>
          )}
        </li>
      ))}
    </ul>
  )
}
