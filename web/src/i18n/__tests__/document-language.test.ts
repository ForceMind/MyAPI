import i18next from 'i18next'
import { afterEach, describe, expect, it } from 'vitest'

import { syncDocumentLanguage, toDocumentLanguage } from '../languages'

const supportedLngs = ['en', 'zhCN', 'fr', 'ru', 'ja', 'vi', 'zhTW']

async function createInstance(lng: string) {
  const instance = i18next.createInstance()
  await instance.init({
    lng,
    supportedLngs,
    fallbackLng: 'en',
    resources: Object.fromEntries(
      supportedLngs.map((code) => [code, { translation: {} }])
    ),
  })
  return instance
}

afterEach(() => {
  document.documentElement.lang = 'en'
})

describe('document language tag', () => {
  it.each([
    ['zhCN', 'zh-Hans'],
    ['zhTW', 'zh-Hant'],
    ['en', 'en'],
    ['fr', 'fr'],
    ['ru', 'ru'],
    ['ja', 'ja'],
    ['vi', 'vi'],
  ])('maps interface language %s to <html lang="%s">', (code, tag) => {
    expect(toDocumentLanguage(code)).toBe(tag)
  })

  it('falls back to English when the language is missing', () => {
    expect(toDocumentLanguage(undefined)).toBe('en')
  })
})

describe('syncDocumentLanguage', () => {
  it('applies the already-initialised language immediately', async () => {
    const instance = await createInstance('ru')

    syncDocumentLanguage(instance)

    expect(document.documentElement.lang).toBe('ru')
  })

  it('follows later language switches, including Traditional Chinese', async () => {
    const instance = await createInstance('en')
    syncDocumentLanguage(instance)

    await instance.changeLanguage('zhTW')
    expect(document.documentElement.lang).toBe('zh-Hant')

    await instance.changeLanguage('ja')
    expect(document.documentElement.lang).toBe('ja')
  })
})
