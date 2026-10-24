import type { TFunction } from 'i18next'
import { expect, test } from 'vitest'

import { PRODUCT_CAPABILITIES } from '@/lib/product-edition'

import { SYSTEM_SETTINGS_VIEW } from '../system-settings.config'

const t = ((key: string) => key) as TFunction
const editionOnlyUrls = [
  '/system-settings/auth/oauth',
  '/system-settings/auth/custom-oauth',
  '/system-settings/billing/payment',
  '/system-settings/billing/checkin',
  '/system-settings/models/model-deployment',
  '/system-settings/content/dashboard',
]

function settingsUrls(enabled: boolean) {
  const capabilities = Object.fromEntries(
    PRODUCT_CAPABILITIES.map((capability) => [capability, enabled])
  ) as Parameters<typeof SYSTEM_SETTINGS_VIEW.getNavGroups>[1]
  return SYSTEM_SETTINGS_VIEW.getNavGroups(t, capabilities)
    .flatMap((group) => group.items)
    .flatMap((item) => ('items' in item && item.items ? item.items : [item]))
    .map((item) => item.url)
}

test('the Full edition lists every capability-gated settings section', () => {
  expect(settingsUrls(true)).toEqual(expect.arrayContaining(editionOnlyUrls))
})

test('the Lite edition hides settings sections whose capability is off and keeps the rest', () => {
  const urls = settingsUrls(false)
  for (const url of editionOnlyUrls) expect(urls).not.toContain(url)
  expect(urls).toContain('/system-settings/auth/basic-auth')
  expect(urls).toContain('/system-settings/operations/logs')
})
