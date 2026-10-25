import { Link } from '@tanstack/react-router'
import { Fragment, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { useConsolePage } from '@/hooks/use-console-page'
import { useInspectorStore } from '@/stores/inspector-store'

type Crumb = { key: string; node: ReactNode }

/**
 * Where the page sits: area / parent page / page, plus the object of an open
 * inspector, at most three levels (design direction 3). The area is plain
 * text, the parent page is a link that keeps the user's place, and with an
 * inspector open the page item closes it. A page without a level above it
 * shows no breadcrumb; its own heading names it. Under page tabs, which
 * already show the area and the page, it appears only with a third level,
 * except on an object page, where it is the way up. Objects an object page
 * sits in (a request's Key) come between the page and the object.
 */
export function AppBreadcrumb(props: { underTabs?: boolean }) {
  const { t } = useTranslation()
  const page = useConsolePage()
  const inspector = useInspectorStore((state) => state.current)

  const crumbs: Crumb[] = []
  if (page.group) {
    crumbs.push({
      key: 'group',
      node: <span>{page.group}</span>,
    })
  }
  if (page.parent) {
    crumbs.push({
      key: 'parent',
      node: (
        <BreadcrumbLink
          render={<Link to={page.parent.url} preload={false} />}
          title={page.parent.title}
        >
          {page.parent.title}
        </BreadcrumbLink>
      ),
    })
  }
  crumbs.push({
    key: 'page',
    node: inspector ? (
      <BreadcrumbLink
        title={page.title}
        render={
          <button
            type='button'
            onClick={inspector.closeAll ?? inspector.close}
          />
        }
      >
        {page.title}
      </BreadcrumbLink>
    ) : (
      <BreadcrumbPage title={page.title}>{page.title}</BreadcrumbPage>
    ),
  })
  inspector?.parents?.forEach((parent, index) => {
    crumbs.push({
      key: `parent-object-${index}`,
      node: (
        <BreadcrumbLink
          title={parent.title}
          render={<button type='button' onClick={parent.open} />}
        >
          {parent.title}
        </BreadcrumbLink>
      ),
    })
  })
  if (inspector) {
    crumbs.push({
      key: 'inspector',
      node: (
        <BreadcrumbPage title={inspector.title}>
          {inspector.title}
        </BreadcrumbPage>
      ),
    })
  }
  if (crumbs.length < (props.underTabs && !inspector?.page ? 3 : 2)) {
    return null
  }
  const visible = crumbs.slice(-3)

  return (
    <Breadcrumb
      aria-label={t('Current page')}
      className='pc-breadcrumb min-w-0'
    >
      <BreadcrumbList>
        {visible.map((crumb, index) => (
          <Fragment key={crumb.key}>
            {index > 0 && <BreadcrumbSeparator />}
            <BreadcrumbItem className='min-w-0'>{crumb.node}</BreadcrumbItem>
          </Fragment>
        ))}
      </BreadcrumbList>
    </Breadcrumb>
  )
}
