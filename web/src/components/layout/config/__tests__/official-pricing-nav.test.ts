import type { TFunction } from 'i18next'
import { expect, test } from 'vitest'

import { SYSTEM_SETTINGS_VIEW } from '../system-settings.config'

test('administrator model navigation links to the official price source page', () => {
  const t = ((key: string) => key) as TFunction
  const links = SYSTEM_SETTINGS_VIEW.getNavGroups(t)
    .flatMap((group) => group.items)
    .flatMap((item) => ('items' in item ? item.items : [item]))
  expect(links).toContainEqual({
    title: 'OpenAI official pricing source',
    url: '/system-settings/models/openai-pricing-source',
  })
})
