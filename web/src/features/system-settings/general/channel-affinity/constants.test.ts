import { describe, expect, it } from 'vitest'

import { cloneTemplate } from './constants'

describe('cloneTemplate', () => {
  it('creates an independent copy of nested affinity rule data', () => {
    const template = {
      name: 'cache key',
      key_sources: [{ type: 'gjson', path: 'metadata.user_id' }],
    }

    const copy = cloneTemplate(template)
    copy.key_sources[0].path = 'metadata.session_id'

    expect(copy).not.toBe(template)
    expect(template.key_sources[0].path).toBe('metadata.user_id')
  })
})
