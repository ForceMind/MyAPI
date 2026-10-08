import { describe, expect, it } from 'vitest'

import { convertDetectedLanguage } from '../languages'

describe('persisted interface language', () => {
  it('keeps saved Traditional Chinese after the page reloads', () => {
    expect(convertDetectedLanguage('zhTW')).toBe('zhTW')
  })

  it('keeps Simplified Chinese and standard browser variants distinct', () => {
    expect(convertDetectedLanguage('zhCN')).toBe('zhCN')
    expect(convertDetectedLanguage('zh-TW')).toBe('zhTW')
    expect(convertDetectedLanguage('zh-Hant-HK')).toBe('zhTW')
    expect(convertDetectedLanguage('zh-CN')).toBe('zhCN')
  })
})
