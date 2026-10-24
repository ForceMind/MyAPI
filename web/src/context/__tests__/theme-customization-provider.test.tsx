import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import {
  ThemeCustomizationProvider,
  useThemeCustomization,
} from '@/context/theme-customization-provider'
import { THEME_PRESETS } from '@/lib/theme-customization'

function setCookie(name: string, value: string) {
  document.cookie = `${name}=${value}; path=/`
}

function clearCookie(name: string) {
  document.cookie = `${name}=; path=/; max-age=0`
}

function ActivePreset() {
  const { customization } = useThemeCustomization()
  return (
    <output aria-label='preferences'>
      {`${customization.preset}|${customization.radius}|${customization.scale}|${customization.contentLayout}`}
    </output>
  )
}

const COOKIES = [
  'theme_preset',
  'theme_radius',
  'theme_scale',
  'theme_content_layout',
]

afterEach(() => {
  for (const name of COOKIES) clearCookie(name)
  for (const attribute of [
    'data-theme-preset',
    'data-theme-radius',
    'data-theme-scale',
    'data-theme-content-layout',
    'data-theme-font',
  ]) {
    document.body.removeAttribute(attribute)
  }
})

describe('colour presets', () => {
  it('offers only the MyAPI default and high-contrast presets', () => {
    expect(THEME_PRESETS.map((preset) => preset.value)).toEqual([
      'default',
      'high-contrast',
    ])
  })

  it.each(['anthropic', 'rose-garden', 'simple-large'])(
    'a saved retired preset %s falls back to the default without touching other preferences',
    (retired) => {
      setCookie('theme_preset', retired)
      setCookie('theme_radius', 'lg')
      setCookie('theme_scale', 'xl')
      setCookie('theme_content_layout', 'centered')

      render(
        <ThemeCustomizationProvider>
          <ActivePreset />
        </ThemeCustomizationProvider>
      )

      expect(screen.getByLabelText('preferences')).toHaveTextContent(
        'default|lg|xl|centered'
      )
      expect(document.body).not.toHaveAttribute('data-theme-preset')
      expect(document.body).toHaveAttribute('data-theme-radius', 'lg')
      expect(document.body).toHaveAttribute('data-theme-scale', 'xl')
    }
  )

  it('a saved high-contrast preset is applied to the body', () => {
    setCookie('theme_preset', 'high-contrast')

    render(
      <ThemeCustomizationProvider>
        <ActivePreset />
      </ThemeCustomizationProvider>
    )

    expect(screen.getByLabelText('preferences')).toHaveTextContent(
      'high-contrast|default|default|full'
    )
    expect(document.body).toHaveAttribute('data-theme-preset', 'high-contrast')
  })
})
