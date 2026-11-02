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
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { useModels } from './models-provider'

type DescriptionCellProps = {
  modelId: number
  modelName: string
  description: string
}

export function DescriptionCell({
  modelId,
  modelName,
  description,
}: DescriptionCellProps) {
  const { t } = useTranslation()
  const { setOpen, setDescriptionData, rememberOpener } = useModels()

  if (!description) {
    return <span className='text-muted-foreground text-xs'>-</span>
  }

  const handleClick = () => {
    rememberOpener({ kind: 'description', id: modelId })
    setDescriptionData({ modelName, description })
    setOpen('description')
  }

  return (
    <div className='min-w-0'>
      <Button
        variant='link'
        onClick={handleClick}
        data-model-description={modelId}
        aria-label={t('View description for {{model}}', { model: modelName })}
        className='text-muted-foreground hover:text-foreground block h-auto min-h-11 w-full cursor-pointer py-2 text-left text-sm wrap-anywhere whitespace-normal no-underline'
      >
        {description}
      </Button>
    </div>
  )
}
