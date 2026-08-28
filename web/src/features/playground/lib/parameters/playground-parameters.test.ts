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

import type { ParameterEnabled } from '../../types'
import { applyUnsupportedParameterRestrictions } from './playground-parameters'

const enabledParameters: ParameterEnabled = {
  temperature: true,
  top_p: true,
  max_tokens: false,
  frequency_penalty: true,
  presence_penalty: true,
  seed: true,
}

describe('applyUnsupportedParameterRestrictions', () => {
  test('turns off unsupported parameters without mutating saved preferences', () => {
    const restricted = applyUnsupportedParameterRestrictions(
      enabledParameters,
      ['temperature', 'top_p', 'frequency_penalty', 'presence_penalty']
    )

    expect(restricted).toEqual({
      temperature: false,
      top_p: false,
      max_tokens: false,
      frequency_penalty: false,
      presence_penalty: false,
      seed: true,
    })
    expect(enabledParameters.temperature).toBe(true)
    expect(enabledParameters.top_p).toBe(true)
  })

  test('reuses the original object when no restrictions apply', () => {
    expect(applyUnsupportedParameterRestrictions(enabledParameters, [])).toBe(
      enabledParameters
    )
  })
})
