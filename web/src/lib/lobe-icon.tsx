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
import type React from 'react'

import { IconSub2api } from '@/assets/custom/icon-sub2api'

const CUSTOM_ICONS: Record<string, React.ComponentType<{ size?: number }>> = {
  Sub2API: IconSub2api,
}

function getFallbackLabel(iconName: string): string {
  const baseName = iconName.split('.')[0]?.trim()
  return baseName?.charAt(0).toUpperCase() || '?'
}

/**
 * Render a compact provider marker without loading a full third-party icon UI
 * system. Existing icon descriptor strings remain accepted so stored model and
 * vendor metadata stays compatible; custom project icons are preserved and all
 * other providers use a deterministic initial fallback.
 *
 * @param iconName - Existing icon descriptor (e.g. "OpenAI.Color")
 * @param size - Icon size (default: 20)
 * @returns Icon component or fallback
 *
 * @example
 * getLobeIcon("OpenAI", 24)
 * getLobeIcon("OpenAI.Color", 20)
 * getLobeIcon("Claude.Avatar.type={'platform'}", 32)
 */
export function getLobeIcon(
  iconName: string | undefined | null,
  size: number = 20
): React.ReactNode {
  if (!iconName || typeof iconName !== 'string') {
    return (
      <div
        className='bg-muted text-muted-foreground flex items-center justify-center rounded-full text-xs font-medium'
        style={{ width: size, height: size }}
      >
        ?
      </div>
    )
  }

  const trimmedName = iconName.trim()
  if (!trimmedName) {
    return (
      <div
        className='bg-muted text-muted-foreground flex items-center justify-center rounded-full text-xs font-medium'
        style={{ width: size, height: size }}
      >
        ?
      </div>
    )
  }

  const baseKey = trimmedName.split('.')[0]
  const CustomIcon = CUSTOM_ICONS[baseKey]
  if (CustomIcon) {
    return <CustomIcon size={size} />
  }

  return (
    <div
      aria-label={baseKey}
      title={baseKey}
      className='bg-muted text-muted-foreground flex items-center justify-center rounded-full text-xs font-medium'
      style={{ width: size, height: size }}
    >
      {getFallbackLabel(trimmedName)}
    </div>
  )
}
