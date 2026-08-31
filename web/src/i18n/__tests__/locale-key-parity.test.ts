import { describe, expect, it } from 'vitest'

import en from '../locales/en.json'
import fr from '../locales/fr.json'
import ja from '../locales/ja.json'
import ru from '../locales/ru.json'
import vi from '../locales/vi.json'
import zhTW from '../locales/zh-TW.json'

const baseKeys = Object.keys(en.translation).sort()

describe('locale key parity', () => {
  it.each([
    ['fr', fr],
    ['ja', ja],
    ['ru', ru],
    ['vi', vi],
    ['zh-TW', zhTW],
  ])('%s contains every English translation key', (_locale, resource) => {
    const missing = baseKeys.filter((key) => !(key in resource.translation))
    expect(missing).toEqual([])
  })
})
