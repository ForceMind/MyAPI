import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

export function UsageAccuracyBadge(props: { accuracy: unknown }) {
  const { t } = useTranslation()
  const descriptionId = useId()
  let label = t('Usage provenance unavailable')
  let description = t(
    'This record has no usage provenance; accuracy cannot be confirmed.'
  )
  let variant: 'outline' | 'secondary' | 'warning' | 'destructive' = 'secondary'
  switch (props.accuracy) {
    case 'reported':
      label = t('Reported usage')
      description = t(
        'Token usage was reported by upstream; the provider bill has not been reconciled.'
      )
      variant = 'outline'
      break
    case 'estimated':
      label = t('Estimated usage')
      description = t('Token usage is estimated; cost is not exact.')
      variant = 'warning'
      break
    case 'unknown':
      label = t('Usage unknown')
      description = t(
        'Token usage is unavailable; the displayed cost is not confirmed.'
      )
      variant = 'destructive'
      break
  }
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger
          render={
            <Badge
              variant={variant}
              tabIndex={0}
              aria-describedby={descriptionId}
              className='h-auto min-h-5 max-w-full cursor-help justify-start text-left whitespace-normal'
            >
              {label}
            </Badge>
          }
        />
        <TooltipContent id={descriptionId} role='tooltip'>
          {description}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
