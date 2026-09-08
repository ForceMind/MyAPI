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

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from '../channel-form'

const routingBaseline = { priority: 10, weight: 5 }

describe('channel routing update payload', () => {
  test('creates channels with the standard routing order and one configured part', () => {
    const payload = transformFormDataToCreatePayload(
      CHANNEL_FORM_DEFAULT_VALUES
    )

    expect(payload.channel.priority).toBe(0)
    expect(payload.channel.weight).toBe(1)
  })

  test('omits unchanged routing fields when editing only a remark', () => {
    const payload = transformFormDataToUpdatePayload(
      {
        ...CHANNEL_FORM_DEFAULT_VALUES,
        priority: routingBaseline.priority,
        weight: routingBaseline.weight,
        remark: 'operator note',
      },
      101,
      routingBaseline
    )

    expect(payload.remark).toBe('operator note')
    expect(payload).not.toHaveProperty('priority')
    expect(payload).not.toHaveProperty('weight')
  })

  test('includes an explicit priority change to zero', () => {
    const payload = transformFormDataToUpdatePayload(
      {
        ...CHANNEL_FORM_DEFAULT_VALUES,
        priority: 0,
        weight: routingBaseline.weight,
      },
      101,
      routingBaseline
    )

    expect(payload.priority).toBe(0)
    expect(payload).not.toHaveProperty('weight')
  })

  test('includes an explicit weight change to zero', () => {
    const payload = transformFormDataToUpdatePayload(
      {
        ...CHANNEL_FORM_DEFAULT_VALUES,
        priority: routingBaseline.priority,
        weight: 0,
      },
      101,
      routingBaseline
    )

    expect(payload).not.toHaveProperty('priority')
    expect(payload.weight).toBe(0)
  })
})
