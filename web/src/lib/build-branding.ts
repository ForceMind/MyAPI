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

/**
 * Optional build-time branding for a self-hosted distribution.
 *
 * An administrator-provided system name/logo wins. These values provide a
 * distribution default during first paint, setup, and degraded /api/status
 * responses. Leaving the variables unset uses the MyAPI defaults.
 */

function nonEmptyString(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const trimmed = value.trim()
  return trimmed.length > 0 ? trimmed : undefined
}

const buildBrandName = nonEmptyString(
  import.meta.env.VITE_BRAND_NAME ??
    (import.meta.env as ImportMetaEnv & { MYAPI_BRAND_NAME?: unknown })
      .MYAPI_BRAND_NAME
)
const buildBrandLogo = nonEmptyString(
  import.meta.env.VITE_BRAND_LOGO ??
    (import.meta.env as ImportMetaEnv & { MYAPI_BRAND_LOGO?: unknown })
      .MYAPI_BRAND_LOGO
)

// Existing installations can still return these values from `/api/status`.
// They are treated as unset so a branded build does not regress to the legacy
// product name/logo. A deliberately configured non-legacy runtime value always
// remains authoritative.
const LEGACY_SYSTEM_NAME_FALLBACKS = ['New API', 'NewAPI'] as const
const LEGACY_LOGO_FALLBACKS = ['/logo.png', '/favicon.ico'] as const

export const MYAPI_REPOSITORY_URL = 'https://github.com/ForceMind/MyAPI'
export const MYAPI_ISSUES_URL = `${MYAPI_REPOSITORY_URL}/issues`
export const MYAPI_LICENSE_URL = `${MYAPI_REPOSITORY_URL}/blob/main/LICENSE`
export const MYAPI_NOTICES_URL = `${MYAPI_REPOSITORY_URL}/blob/main/NOTICE`
export const MYAPI_DOCS_URL = `${MYAPI_REPOSITORY_URL}#readme`

function equalsBrandValue(value: string, candidate: string): boolean {
  return (
    value.localeCompare(candidate, undefined, {
      sensitivity: 'accent',
    }) === 0
  )
}

function isKnownNameFallback(value: string, upstreamFallback: string): boolean {
  return [upstreamFallback, ...LEGACY_SYSTEM_NAME_FALLBACKS].some((candidate) =>
    equalsBrandValue(value, candidate)
  )
}

function isKnownLogoFallback(value: string, upstreamFallback: string): boolean {
  return [upstreamFallback, ...LEGACY_LOGO_FALLBACKS].some((candidate) =>
    equalsBrandValue(value, candidate)
  )
}

/** Resolve an administrator name, then build override, then the distribution fallback. */
export function resolveBrandName(
  runtimeName: unknown,
  upstreamFallback: string
): string {
  const runtime = nonEmptyString(runtimeName)
  if (runtime && !isKnownNameFallback(runtime, upstreamFallback)) return runtime
  return buildBrandName ?? upstreamFallback
}

/** Resolve an administrator logo, then build override, then the distribution fallback. */
export function resolveBrandLogo(
  runtimeLogo: unknown,
  upstreamFallback: string
): string {
  const runtime = nonEmptyString(runtimeLogo)
  if (runtime && !isKnownLogoFallback(runtime, upstreamFallback)) return runtime
  return buildBrandLogo ?? upstreamFallback
}

/** Build-time name, useful for placeholders and initial forms. */
export function getBuildBrandName(upstreamFallback: string): string {
  return buildBrandName ?? upstreamFallback
}

/** Return the explicit build override, if one was provided. */
export function getBuildBrandNameOverride(): string | undefined {
  return buildBrandName
}

/** Build-time logo, useful for initial HTML-independent rendering. */
export function getBuildBrandLogo(upstreamFallback: string): string {
  return buildBrandLogo ?? upstreamFallback
}
