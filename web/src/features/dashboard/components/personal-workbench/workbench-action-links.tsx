import {
  Activity02Icon,
  LinkSquare02Icon,
  TerminalIcon,
  Wrench01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import type { ReactElement, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'

function ActionLink(props: {
  icon: ReactNode
  title: string
  description: string
  link: ReactElement
}) {
  return (
    <Button
      variant='outline'
      className='h-auto min-w-0 items-start justify-start gap-3 p-3 text-left whitespace-normal'
      render={props.link}
    >
      <span className='bg-muted mt-0.5 rounded-md p-1.5'>{props.icon}</span>
      <span className='min-w-0'>
        <span className='block font-medium'>{props.title}</span>
        <span className='text-muted-foreground mt-0.5 block text-xs font-normal'>
          {props.description}
        </span>
      </span>
    </Button>
  )
}

export function WorkbenchActionLinks(props: { canReadChannels: boolean }) {
  const { t } = useTranslation()
  const showKeys = useIsSidebarModuleVisible('/keys')
  const showLogs = useIsSidebarModuleVisible('/usage-logs/common')
  const showProfile = useIsSidebarModuleVisible('/profile')
  const showChannels = useIsSidebarModuleVisible('/channels')

  return (
    <nav
      aria-label={t('Personal workbench actions')}
      className='mt-4 grid gap-2 border-t pt-4'
    >
      {showKeys ? (
        <ActionLink
          icon={
            <HugeiconsIcon
              icon={TerminalIcon}
              strokeWidth={2}
              className='size-4'
              aria-hidden='true'
            />
          }
          title={t('Open Keys')}
          description={t('Create, disable, or inspect your API Keys.')}
          link={<Link to='/keys' />}
        />
      ) : null}
      {showLogs ? (
        <ActionLink
          icon={
            <HugeiconsIcon
              icon={Activity02Icon}
              strokeWidth={2}
              className='size-4'
              aria-hidden='true'
            />
          }
          title={t('Open usage logs')}
          description={t('Inspect your request and usage records.')}
          link={
            <Link to='/usage-logs/$section' params={{ section: 'common' }} />
          }
        />
      ) : null}
      {showProfile ? (
        <ActionLink
          icon={
            <HugeiconsIcon
              icon={Wrench01Icon}
              strokeWidth={2}
              className='size-4'
              aria-hidden='true'
            />
          }
          title={t('Open profile')}
          description={t('Review your account and security settings.')}
          link={<Link to='/profile' />}
        />
      ) : null}
      {showChannels && props.canReadChannels ? (
        <ActionLink
          icon={
            <HugeiconsIcon
              icon={LinkSquare02Icon}
              strokeWidth={2}
              className='size-4'
              aria-hidden='true'
            />
          }
          title={t('Open Channels')}
          description={t('Inspect upstream routing and account windows.')}
          link={<Link to='/channels' />}
        />
      ) : null}
    </nav>
  )
}
