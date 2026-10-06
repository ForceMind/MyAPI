import { describe, expect, test } from 'vitest'

import {
  normalizeModelList,
  parseUpstreamUpdateMeta,
} from '../upstream-update-utils'

describe('upstream update metadata', () => {
  test('keeps the first normalized model occurrence while parsing pending changes', () => {
    expect(
      parseUpstreamUpdateMeta(
        JSON.stringify({
          upstream_model_update_check_enabled: true,
          upstream_model_update_last_detected_models: [
            ' gpt-5 ',
            'gpt-4.1',
            'gpt-5',
          ],
          upstream_model_update_last_removed_models: [' o3 ', 'o3'],
        })
      )
    ).toEqual({
      enabled: true,
      pendingAddModels: ['gpt-5', 'gpt-4.1'],
      pendingRemoveModels: ['o3'],
    })
    expect(normalizeModelList([null, ' ', 'gpt-5'])).toEqual(['gpt-5'])
  })
})
