/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, test } from 'vitest'

import { parseUserModelOptions } from './api'

describe('parseUserModelOptions', () => {
  test('attaches validated Codex parameter restrictions to the model', () => {
    expect(
      parseUserModelOptions({
        success: true,
        data: ['gpt-5.6', 'other-model'],
        capabilities: {
          'gpt-5.6': {
            provider: 'Codex',
            unsupported_parameters: [
              'temperature',
              'top_p',
              'unknown_parameter',
            ],
          },
        },
      })
    ).toEqual([
      {
        label: 'gpt-5.6',
        value: 'gpt-5.6',
        provider: 'Codex',
        unsupportedParameters: ['temperature', 'top_p'],
      },
      { label: 'other-model', value: 'other-model' },
    ])
  })

  test('returns an empty list for an invalid response', () => {
    expect(
      parseUserModelOptions({ success: false, data: ['gpt-5.6'] })
    ).toEqual([])
  })
})
