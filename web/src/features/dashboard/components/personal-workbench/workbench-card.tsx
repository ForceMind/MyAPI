import type { ReactNode } from 'react'

import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { cn } from '@/lib/utils'

export function WorkbenchCard(props: {
  id: string
  title: ReactNode
  description: ReactNode
  action?: ReactNode
  children: ReactNode
  className?: string
  testId?: string
}) {
  return (
    <Card
      role='region'
      aria-labelledby={props.id}
      data-testid={props.testId}
      className={cn('min-w-0 border-0 shadow-sm', props.className)}
    >
      <CardHeader className='gap-1.5 border-b border-border/60 pb-4'>
        <CardTitle>
          <h2 id={props.id}>{props.title}</h2>
        </CardTitle>
        <CardDescription className='max-w-2xl leading-relaxed'>
          {props.description}
        </CardDescription>
        {props.action ? <CardAction>{props.action}</CardAction> : null}
      </CardHeader>
      <CardContent className='min-w-0'>{props.children}</CardContent>
    </Card>
  )
}
