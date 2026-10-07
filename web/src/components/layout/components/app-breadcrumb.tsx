import { useLocation } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { getDashboardSectionNavItems } from '@/features/dashboard/section-registry'
import { getModelsSectionNavItems } from '@/features/models/section-registry'
import { getUsageLogsSectionNavItems } from '@/features/usage-logs/section-registry'
import { useSidebarView } from '@/hooks/use-sidebar-view'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { checkIsActive } from '../lib/url-utils'

/** Current location context; labels never introduce another permission path. */
export function AppBreadcrumb() {
  const { t } = useTranslation()
  const href = useLocation({ select: (location) => location.href })
  const pathname = useLocation({ select: (location) => location.pathname })
  const role = useAuthStore((state) => state.auth.user?.role ?? ROLE.GUEST)
  const { navGroups, view } = useSidebarView()
  const group = navGroups.find((candidate) =>
    candidate.items.some((item) => checkIsActive(href, item))
  )
  const item = group?.items.find((candidate) => checkIsActive(href, candidate))
  const child = item?.items?.find((candidate) => checkIsActive(href, candidate))
  const section = [
    ...getDashboardSectionNavItems(t, { isAdmin: role >= ROLE.ADMIN }),
    ...getModelsSectionNavItems(t),
    ...getUsageLogsSectionNavItems(t),
  ].find((candidate) => candidate.url === pathname)

  let title = child?.title ?? section?.title ?? item?.title ?? t('Console')
  if (pathname.startsWith('/chat/')) title = t('Chat')
  if (pathname === '/prompt-learning') title = t('Prompt learning')
  const context = view ? t('System Settings') : (group?.title ?? t('Console'))
  const showContext = title !== context

  return (
    <Breadcrumb aria-label={t('Current page')} className='min-w-0'>
      <BreadcrumbList className='flex-nowrap gap-1.5 overflow-hidden'>
        {showContext && (
          <>
            <BreadcrumbItem className='max-w-[40%] min-w-0 shrink'>
              <span className='truncate text-xs' title={context}>
                {context}
              </span>
            </BreadcrumbItem>
            <BreadcrumbSeparator className='shrink-0' />
          </>
        )}
        <BreadcrumbItem className='min-w-0 flex-1'>
          <BreadcrumbPage
            className='truncate text-sm font-medium'
            title={title}
          >
            {title}
          </BreadcrumbPage>
        </BreadcrumbItem>
      </BreadcrumbList>
    </Breadcrumb>
  )
}
