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
  LanguageSkillIcon,
  Logout01Icon,
  PaintBoardIcon,
  Settings01Icon,
  UserIcon,
  Wallet01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useNavigate } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { useInterfaceLanguage } from '@/components/language-switcher'
import { SignOutDialog } from '@/components/sign-out-dialog'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useTheme } from '@/context/theme-provider'
import useDialogState from '@/hooks/use-dialog'
import { useFundingPresentation } from '@/hooks/use-funding-presentation'
import { useProductEdition } from '@/hooks/use-product-edition'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { useUserDisplay } from '@/hooks/use-user-display'
import { getUserAvatarFallback, getUserAvatarStyle } from '@/lib/avatar'
import { getBuildRevision } from '@/lib/build-metadata'
import { ROLE } from '@/lib/roles'
import { SELF_USE_MINIMAL } from '@/lib/self-use-build'
import { useAuthStore } from '@/stores/auth-store'

const avatarFallbackClassName = 'font-semibold text-white'

export function ProfileDropdown() {
  const { commercialEnabled } = useFundingPresentation()
  const { capabilities } = useProductEdition()
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { theme, setTheme } = useTheme()
  const language = useInterfaceLanguage()
  const [open, setOpen] = useDialogState()
  const user = useAuthStore((state) => state.auth.user)
  const { displayName, roleLabel } = useUserDisplay(user)
  const isSuperAdmin = user?.role === ROLE.SUPER_ADMIN
  const isWalletVisible = useIsSidebarModuleVisible('/wallet')
  const avatarName = user?.username || displayName
  const avatarFallback = getUserAvatarFallback(avatarName)
  const avatarFallbackStyle = useMemo(
    () => getUserAvatarStyle(avatarName),
    [avatarName]
  )
  const version = import.meta.env.VITE_REACT_APP_VERSION || getBuildRevision()

  return (
    <>
      <DropdownMenu modal={false}>
        <DropdownMenuTrigger
          aria-label={t('Account menu')}
          render={
            <Button
              variant='ghost'
              className='relative size-9 rounded-full p-1'
            />
          }
        >
          <Avatar className='size-7'>
            <AvatarFallback
              className={`${avatarFallbackClassName} text-[11px]`}
              style={avatarFallbackStyle}
            >
              {avatarFallback}
            </AvatarFallback>
          </Avatar>
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end' sideOffset={8} className='w-60'>
          <div className='flex items-center gap-2 px-1.5 py-1.5'>
            <Avatar className='size-8'>
              <AvatarFallback
                className={`${avatarFallbackClassName} text-xs`}
                style={avatarFallbackStyle}
              >
                {avatarFallback}
              </AvatarFallback>
            </Avatar>
            <div className='flex flex-1 flex-col gap-0.5 overflow-hidden'>
              <p className='text-foreground truncate text-sm font-medium'>
                {displayName}
              </p>
              <div className='flex items-center gap-1.5'>
                <span className='text-muted-foreground text-xs'>
                  {roleLabel}
                </span>
                {user?.group && (
                  <>
                    <span className='text-muted-foreground text-xs'>·</span>
                    <span className='text-muted-foreground truncate text-xs'>
                      {String(user.group)}
                    </span>
                  </>
                )}
              </div>
            </div>
          </div>

          <DropdownMenuSeparator />

          <DropdownMenuGroup>
            <DropdownMenuItem onClick={() => navigate({ to: '/profile' })}>
              <HugeiconsIcon icon={UserIcon} aria-hidden='true' />
              {t('Profile')}
            </DropdownMenuItem>

            {!SELF_USE_MINIMAL &&
              capabilities.payments &&
              commercialEnabled &&
              isWalletVisible && (
                <DropdownMenuItem onClick={() => navigate({ to: '/wallet' })}>
                  <HugeiconsIcon icon={Wallet01Icon} aria-hidden='true' />
                  {t('Wallet')}
                </DropdownMenuItem>
              )}

            {isSuperAdmin && (
              <DropdownMenuItem
                onClick={() =>
                  navigate({
                    to: '/system-settings/site/$section',
                    params: { section: 'system-info' },
                  })
                }
              >
                <HugeiconsIcon icon={Settings01Icon} aria-hidden='true' />
                {t('System Settings')}
              </DropdownMenuItem>
            )}
          </DropdownMenuGroup>

          <DropdownMenuSeparator />

          <DropdownMenuGroup>
            <DropdownMenuSub>
              <DropdownMenuSubTrigger>
                <HugeiconsIcon icon={LanguageSkillIcon} aria-hidden='true' />
                {t('Language')}
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent>
                <DropdownMenuGroup>
                  <DropdownMenuRadioGroup
                    value={language.current}
                    onValueChange={(code) => void language.changeLanguage(code)}
                  >
                    {language.options.map((option) => (
                      <DropdownMenuRadioItem
                        key={option.code}
                        value={option.code}
                      >
                        {option.label}
                      </DropdownMenuRadioItem>
                    ))}
                  </DropdownMenuRadioGroup>
                </DropdownMenuGroup>
              </DropdownMenuSubContent>
            </DropdownMenuSub>
            <DropdownMenuSub>
              <DropdownMenuSubTrigger>
                <HugeiconsIcon icon={PaintBoardIcon} aria-hidden='true' />
                {t('Theme')}
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent>
                <DropdownMenuGroup>
                  <DropdownMenuRadioGroup
                    value={theme}
                    onValueChange={(value) =>
                      setTheme(value as 'light' | 'dark' | 'system')
                    }
                  >
                    <DropdownMenuRadioItem value='light'>
                      {t('Light')}
                    </DropdownMenuRadioItem>
                    <DropdownMenuRadioItem value='dark'>
                      {t('Dark')}
                    </DropdownMenuRadioItem>
                    <DropdownMenuRadioItem value='system'>
                      {t('System')}
                    </DropdownMenuRadioItem>
                  </DropdownMenuRadioGroup>
                </DropdownMenuGroup>
              </DropdownMenuSubContent>
            </DropdownMenuSub>
          </DropdownMenuGroup>

          <DropdownMenuSeparator />

          <DropdownMenuGroup>
            <DropdownMenuItem
              variant='destructive'
              onClick={() => setOpen(true)}
            >
              <HugeiconsIcon icon={Logout01Icon} aria-hidden='true' />
              {t('Sign out')}
            </DropdownMenuItem>
          </DropdownMenuGroup>

          <p
            className='text-muted-foreground px-2 pt-1 pb-1.5 text-[11px] break-all'
            data-testid='console-build-revision'
            title={getBuildRevision()}
          >
            {version}
          </p>
        </DropdownMenuContent>
      </DropdownMenu>

      <SignOutDialog open={!!open} onOpenChange={setOpen} />
    </>
  )
}
