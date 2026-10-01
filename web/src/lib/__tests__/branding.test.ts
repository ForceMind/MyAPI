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
import { describe, expect, it } from 'vitest'

import {
  MYAPI_DOCS_URL,
  getBuildBrandLogo,
  getBuildBrandNameOverride,
  resolveBrandLogo,
  resolveBrandName,
  resolveDocsLink,
} from '@/lib/build-branding'

describe('build-time branding', () => {
  it('prefers a non-empty runtime system name', () => {
    expect(resolveBrandName('  Runtime API  ', 'Upstream API')).toBe(
      'Runtime API'
    )
  })

  it('uses the build override before the upstream fallback', () => {
    const buildName = getBuildBrandNameOverride()
    expect(resolveBrandName('   ', 'Upstream API')).toBe(
      buildName ?? 'Upstream API'
    )
  })

  it('treats the upstream default as unset for a branded build', () => {
    const buildName = getBuildBrandNameOverride()
    expect(resolveBrandName('New API', 'New API')).toBe(buildName ?? 'New API')
  })

  it('migrates a legacy runtime name to the MyAPI distribution fallback', () => {
    const buildName = getBuildBrandNameOverride()
    expect(resolveBrandName('New API', 'MyAPI')).toBe(buildName ?? 'MyAPI')
    expect(resolveBrandName('NewAPI', 'MyAPI')).toBe(buildName ?? 'MyAPI')
  })

  it('applies the same precedence rules to logos', () => {
    expect(resolveBrandLogo(' /runtime-logo.svg ', '/upstream-logo.svg')).toBe(
      '/runtime-logo.svg'
    )
    expect(resolveBrandLogo('/logo.png', '/logo.png')).toBe(
      getBuildBrandLogo('/logo.png')
    )
    expect(resolveBrandLogo('', '/upstream-logo.svg')).toBe(
      getBuildBrandLogo('/upstream-logo.svg')
    )
  })

  it('keeps an explicit administrator logo when a build logo exists', () => {
    expect(
      resolveBrandLogo('https://example.com/custom.svg', '/logo.png')
    ).toBe('https://example.com/custom.svg')
  })

  it('migrates a legacy runtime logo to the distribution fallback', () => {
    expect(resolveBrandLogo('/logo.png', '/myapi-logo-v1.png')).toBe(
      getBuildBrandLogo('/myapi-logo-v1.png')
    )
  })
})

describe('documentation link fallback', () => {
  it.each([undefined, '  ', 'javascript:alert(1)', '//docs.example.test'])(
    'uses the project README when the runtime link is missing or unsafe: %s',
    (value) => {
      expect(resolveDocsLink(value)).toEqual({
        href: MYAPI_DOCS_URL,
        external: true,
      })
    }
  )

  it('keeps an explicit site-relative documentation path in the app', () => {
    expect(resolveDocsLink('/custom-docs')).toEqual({
      href: '/custom-docs',
      external: false,
    })
  })

  it('keeps an explicit HTTPS documentation URL external', () => {
    expect(resolveDocsLink('https://docs.example.test')).toEqual({
      href: 'https://docs.example.test',
      external: true,
    })
  })
})
