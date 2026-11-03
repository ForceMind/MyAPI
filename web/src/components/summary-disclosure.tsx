import { ArrowDown01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { ReactNode } from 'react'

import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'

/** An explicit local disclosure; keeping its contents mounted preserves drafts. */
export function SummaryDisclosure(props: {
  summary: ReactNode
  children: ReactNode
  className?: string
  contentClassName?: string
}) {
  return (
    <Collapsible className={cn('min-w-0', props.className)}>
      <CollapsibleTrigger
        render={<Button type='button' variant='ghost' />}
        className='group min-h-11 max-w-full justify-start gap-2 px-2 text-left whitespace-normal pointer-fine:min-h-9'
      >
        <span className='min-w-0 [overflow-wrap:anywhere]'>{props.summary}</span>
        <HugeiconsIcon icon={ArrowDown01Icon} aria-hidden='true' className='size-4 shrink-0 transition-transform group-aria-expanded:rotate-180' />
      </CollapsibleTrigger>
      <CollapsibleContent
        keepMounted
        className={cn('min-w-0 pt-2 [overflow-wrap:anywhere]', props.contentClassName)}
      >
        {props.children}
      </CollapsibleContent>
    </Collapsible>
  )
}
