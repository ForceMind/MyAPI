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
import type { i18n as I18nInstance } from 'i18next'

import { SELF_USE_MINIMAL } from '@/lib/self-use-build'

export const INTERFACE_LANGUAGE_OPTIONS = [
  { code: 'zhCN', label: '简体中文' },
  { code: 'en', label: 'English' },
  { code: 'fr', label: 'Français' },
  { code: 'ru', label: 'Русский' },
  { code: 'ja', label: '日本語' },
  { code: 'vi', label: 'Tiếng Việt' },
  { code: 'zhTW', label: '繁體中文' },
] as const

export type InterfaceLanguageCode =
  (typeof INTERFACE_LANGUAGE_OPTIONS)[number]['code']

export const AVAILABLE_INTERFACE_LANGUAGE_OPTIONS = SELF_USE_MINIMAL
  ? INTERFACE_LANGUAGE_OPTIONS.filter(
      (language) => language.code === 'zhCN' || language.code === 'en'
    )
  : INTERFACE_LANGUAGE_OPTIONS

export function normalizeInterfaceLanguage(value?: string | null): string {
  if (!value) return 'en'

  let normalized = value.trim().replaceAll('_', '-').toLowerCase()
  if (
    value === 'zh-TW' ||
    value === 'zh-HK' ||
    value === 'zh-MO' ||
    value === 'zhTW'
  ) {
    normalized = 'zhTW'
  }
  if (value === 'zh-CN' || value === 'zh-Hans' || value === 'zhCN') {
    normalized = 'zhCN'
  }

  return INTERFACE_LANGUAGE_OPTIONS.some((lang) => lang.code === normalized)
    ? normalized
    : 'en'
}

/**
 * Map a browser-detected locale onto the interface language codes this project
 * uses with i18next (`zhCN` / `zhTW`).
 *
 * Browsers report standard BCP-47 tags (`zh-CN`, `zh-TW`, `zh-Hant`, `zh`, ...),
 * but `supportedLngs`/resources use the non-standard camelCase codes, so without
 * this mapping a Chinese browser would never match and fall back to English.
 * Non-Chinese codes are returned unchanged so i18next's own `supportedLngs`
 * matching still applies (e.g. `fr-FR` -> `fr`, `ja` -> `ja`).
 */
export function convertDetectedLanguage(value: string): string {
  const lower = value.trim().replaceAll('_', '-').toLowerCase()
  if (!lower.startsWith('zh')) return value
  if (
    lower === 'zhtw' ||
    lower === 'zh-tw' ||
    lower === 'zh-hk' ||
    lower === 'zh-mo' ||
    lower.startsWith('zh-hant')
  ) {
    return 'zhTW'
  }
  return 'zhCN'
}

/**
 * Convert an interface language code (the values i18next uses, such as `zhCN` /
 * `zhTW`) into a valid BCP-47 locale tag that the `Intl.*` APIs accept.
 *
 * `new Intl.NumberFormat('zhCN')` throws `RangeError: Invalid language tag`, so
 * any locale derived from `i18n.language` / `i18n.resolvedLanguage` MUST be run
 * through this before it reaches an `Intl` constructor. Unknown values fall back
 * to `undefined`, which makes `Intl` use the runtime default locale.
 */
export function toIntlLocale(value?: string | null): string | undefined {
  if (!value) return undefined
  switch (value) {
    case 'zhCN':
      return 'zh-CN'
    case 'zhTW':
      return 'zh-TW'
    default:
      break
  }
  try {
    return Intl.getCanonicalLocales(value)[0]
  } catch {
    return undefined
  }
}

/**
 * Convert an interface language code into the BCP-47 tag written to
 * `<html lang>`. Chinese uses script subtags (`zh-Hans` / `zh-Hant`) because
 * the script, not the region, decides which CJK glyphs and font stack apply
 * (see the `:lang()` blocks in styles/theme.css).
 */
export function toDocumentLanguage(value?: string | null): string {
  if (value === 'zhCN') return 'zh-Hans'
  if (value === 'zhTW') return 'zh-Hant'
  return toIntlLocale(value) ?? 'en'
}

/**
 * Keep `<html lang>` equal to the interface language: screen readers pick
 * pronunciation from it, and the CSS font stacks are selected with `:lang()`.
 * Applies the current language immediately (i18next may already have
 * initialised) and then follows every language change.
 */
export function syncDocumentLanguage(instance: I18nInstance): void {
  if (typeof document === 'undefined') return
  const apply = (language?: string) => {
    document.documentElement.lang = toDocumentLanguage(
      instance.resolvedLanguage ?? language
    )
  }
  apply(instance.language)
  instance.on('languageChanged', apply)
}
