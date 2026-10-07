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
import {
  Children,
  isValidElement,
  useId,
  useState,
  type ReactElement,
  type ReactNode,
} from 'react'

import { Main } from './main'
import { PageFooterProvider } from './page-footer'

type SlotProps = { children?: ReactNode }

function SectionPageLayoutTitle(_props: SlotProps) {
  return null
}
SectionPageLayoutTitle.displayName = 'SectionPageLayout.Title'

function SectionPageLayoutDescription(_props: SlotProps) {
  return null
}
SectionPageLayoutDescription.displayName = 'SectionPageLayout.Description'

function SectionPageLayoutActions(_props: SlotProps) {
  return null
}
SectionPageLayoutActions.displayName = 'SectionPageLayout.Actions'

function SectionPageLayoutContent(_props: SlotProps) {
  return null
}
SectionPageLayoutContent.displayName = 'SectionPageLayout.Content'

function SectionPageLayoutBreadcrumb(_props: SlotProps) {
  return null
}
SectionPageLayoutBreadcrumb.displayName = 'SectionPageLayout.Breadcrumb'

export type SectionPageLayoutProps = {
  children: ReactNode
  fixedContent?: boolean
}

export function SectionPageLayout(props: SectionPageLayoutProps) {
  const titleId = useId()
  const descriptionId = useId()
  const [footerContainer, setFooterContainer] = useState<HTMLDivElement | null>(
    null
  )

  let title: ReactNode = null
  let description: ReactNode = null
  let actions: ReactNode = null
  let content: ReactNode = null
  let breadcrumb: ReactNode = null

  Children.forEach(props.children, (node) => {
    if (!isValidElement(node)) return
    const child = node as ReactElement<SlotProps>
    if (child.type === SectionPageLayoutTitle) {
      title = child.props.children
    } else if (child.type === SectionPageLayoutDescription) {
      description = child.props.children
    } else if (child.type === SectionPageLayoutActions) {
      actions = child.props.children
    } else if (child.type === SectionPageLayoutContent) {
      content = child.props.children
    } else if (child.type === SectionPageLayoutBreadcrumb) {
      breadcrumb = child.props.children
    }
  })

  return (
    <PageFooterProvider container={footerContainer}>
      <Main
        className='myapi-page'
        aria-labelledby={titleId}
        aria-describedby={description != null ? descriptionId : undefined}
      >
        <div className='myapi-page-header shrink-0 px-4 py-4 sm:px-6 sm:py-5'>
          {breadcrumb != null && (
            <div className='mb-2 sm:mb-3'>{breadcrumb}</div>
          )}
          <div className='flex flex-wrap items-start justify-between gap-x-5 gap-y-3'>
            <div className='min-w-0 flex-1 basis-52'>
              <h1
                id={titleId}
                className='text-xl leading-tight font-semibold tracking-tight wrap-break-word sm:text-2xl'
              >
                {title}
              </h1>
              {description != null && (
                <p
                  id={descriptionId}
                  className='text-muted-foreground mt-1.5 max-w-3xl text-sm leading-relaxed wrap-break-word'
                >
                  {description}
                </p>
              )}
            </div>
            {actions != null && (
              <div className='myapi-page-actions flex max-w-full flex-wrap items-center gap-2 sm:ms-auto sm:justify-end'>
                {actions}
              </div>
            )}
          </div>
        </div>

        <div
          data-page-scroll={props.fixedContent ? 'contained' : 'page'}
          className={
            props.fixedContent
              ? 'myapi-page-content min-h-0 min-w-0 flex-1 overflow-hidden px-4 py-4 sm:px-6 sm:py-5'
              : 'myapi-page-content min-h-0 min-w-0 flex-1 overflow-auto px-4 py-4 sm:px-6 sm:py-5'
          }
        >
          {content}
        </div>

        <div
          ref={setFooterContainer}
          className='myapi-page-footer bg-card shrink-0 border-t px-4 py-3 empty:hidden sm:px-6'
        />
      </Main>
    </PageFooterProvider>
  )
}

SectionPageLayout.Title = SectionPageLayoutTitle
SectionPageLayout.Description = SectionPageLayoutDescription
SectionPageLayout.Actions = SectionPageLayoutActions
SectionPageLayout.Content = SectionPageLayoutContent
SectionPageLayout.Breadcrumb = SectionPageLayoutBreadcrumb
