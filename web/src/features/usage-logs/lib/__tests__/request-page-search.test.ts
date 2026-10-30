import { expect, test } from 'vitest'

import { legacyRequestLinkTarget, requestPageSearchSchema } from '../request-page-search'

test.each(['detail', 'body', 'sse', 'raw'])('request tab %s preserves its direct-link state', (tab) => {
  expect(requestPageSearchSchema.parse({ tab })).toEqual({ tab })
})

test('missing or invalid tabs preserve the established default body behavior', () => {
  expect(requestPageSearchSchema.parse({})).toEqual({})
  expect(requestPageSearchSchema.parse({ tab: 'external', redirect: 'https://synthetic.invalid' })).toEqual({ tab: undefined })
})

test.each([
  ['diagnosis', 'detail'], ['body', 'body'], ['sse', 'sse'], [undefined, undefined],
] as const)('legacy inspector tab %s maps to the corresponding saved evidence tab', (requestTab, tab) => {
  expect(legacyRequestLinkTarget({ inspect: 'request', requestId: 'REQ-SAFE', requestTab })).toEqual({ requestId: 'REQ-SAFE', search: { tab } })
})
