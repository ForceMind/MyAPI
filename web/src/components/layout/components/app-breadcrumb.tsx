import { Link } from '@tanstack/react-router'
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

/**
 * Current location as group / page / current view (design 5.1). The group is
 * plain text; the parent page is a link that keeps the user's place. An open
 * inspector adds its object, and the page item then closes the inspector.
 */
export function AppBreadcrumb() {
  const { t } = useTranslation()
  const page = useConsolePage()
  const inspector = useInspectorStore((state) => state.current)

  return (
    <Breadcrumb aria-label={t('Current page')} className='min-w-0'>
      <BreadcrumbList className='flex-nowrap gap-1.5 overflow-hidden'>
        {page.group && (
          <>
            <BreadcrumbItem className='max-w-[30%] min-w-0 shrink'>
              <span className='text-caption truncate' title={page.group}>
                {page.group}
              </span>
            </BreadcrumbItem>
            <BreadcrumbSeparator className='shrink-0' />
          </>
        )}
        {page.parent && (
          <>
            <BreadcrumbItem className='max-w-[35%] min-w-0 shrink'>
              <BreadcrumbLink
                className='truncate text-sm'
                title={page.parent.title}
                render={<Link to={page.parent.url} preload={false} />}
              >
                {page.parent.title}
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator className='shrink-0' />
          </>
        )}
        {inspector ? (
          <>
            <BreadcrumbItem className='max-w-[35%] min-w-0 shrink'>
              <BreadcrumbLink
                className='truncate text-sm'
                title={page.title}
                render={<button type='button' onClick={inspector.close} />}
              >
                {page.title}
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator className='shrink-0' />
            <BreadcrumbItem className='min-w-0 flex-1'>
              <BreadcrumbPage
                className='truncate text-sm font-medium'
                title={inspector.title}
              >
                {inspector.title}
              </BreadcrumbPage>
            </BreadcrumbItem>
          </>
        ) : (
          <BreadcrumbItem className='min-w-0 flex-1'>
            <BreadcrumbPage
              className='truncate text-sm font-medium'
              title={page.title}
            >
              {page.title}
            </BreadcrumbPage>
          </BreadcrumbItem>
        )}
      </BreadcrumbList>
    </Breadcrumb>
  )
}
