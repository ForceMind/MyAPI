import { useId, type ReactNode } from 'react'

import { Main } from '@/components/layout/components/main'
import { cn } from '@/lib/utils'

export type ErrorPageProps = {
  embedded?: boolean
}

type ErrorPageLayoutProps = ErrorPageProps & {
  code: number
  title: string
  children: ReactNode
  actions?: ReactNode
  className?: string
  minimal?: boolean
}

export function ErrorPageLayout(props: ErrorPageLayoutProps) {
  const titleId = useId()
  const descriptionId = useId()
  const Container = props.minimal ? 'div' : Main
  const Heading = props.minimal ? 'h2' : 'h1'

  return (
    <Container
      aria-labelledby={titleId}
      aria-describedby={descriptionId}
      className={cn(
        'myapi-auth-surface flex w-full min-w-0 flex-col overflow-auto px-4 py-8 sm:px-6',
        !props.embedded && !props.minimal && 'min-h-svh',
        props.className
      )}
    >
      <div className='myapi-auth-panel mx-auto my-auto w-full max-w-xl p-6 sm:p-10'>
        {!props.minimal && (
          <p className='text-primary mb-3 font-mono text-sm font-semibold tracking-widest'>
            {props.code}
          </p>
        )}
        <Heading
          id={titleId}
          className='text-xl leading-tight font-semibold tracking-tight wrap-break-word sm:text-2xl'
        >
          {props.title}
        </Heading>
        <div
          id={descriptionId}
          className='text-muted-foreground mt-3 space-y-3 text-sm leading-relaxed'
        >
          {props.children}
        </div>
        {props.actions && (
          <div className='mt-6 flex flex-col gap-2 sm:flex-row sm:flex-wrap [&_[data-slot=button]]:h-auto [&_[data-slot=button]]:min-h-10 [&_[data-slot=button]]:whitespace-normal'>
            {props.actions}
          </div>
        )}
      </div>
    </Container>
  )
}
