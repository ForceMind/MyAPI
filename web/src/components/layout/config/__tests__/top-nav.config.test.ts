import { describe, expect, test } from 'vitest'

import { withTopNavLinkKeys } from '../top-nav.config'

describe('top navigation keys', () => {
  test('uses link identity and a duplicate ordinal instead of array indexes', () => {
    expect(
      withTopNavLinkKeys([
        { title: 'Docs', href: '/docs' },
        { title: 'Docs', href: '/docs' },
        { title: 'Status', href: '/status' },
      ]).map((link) => link.key)
    ).toEqual(['/docs-Docs-0', '/docs-Docs-1', '/status-Status-0'])
  })
})
