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
import { api } from '@/lib/api'

import type { SetupFormValues, SetupResponse } from './types'

export class SetupStatusError extends Error {
  constructor(cause?: unknown) {
    super('Failed to load setup status', { cause })
    this.name = 'SetupStatusError'
  }
}

export async function getSetupStatus(
  signal?: AbortSignal
): Promise<SetupResponse> {
  if (signal?.aborted) throw new SetupStatusError()
  const controller = new AbortController()
  const cancel = () => controller.abort()
  const cancelled = new Promise<never>((_, reject) => {
    controller.signal.addEventListener(
      'abort',
      () => reject(new SetupStatusError()),
      { once: true }
    )
  })
  signal?.addEventListener('abort', cancel, { once: true })
  const deadline = globalThis.setTimeout(cancel, 15000)
  try {
    const res = await Promise.race([
      api.get<SetupResponse>('/api/setup', {
        params: { t: Date.now() },
        signal: controller.signal,
        timeout: 15000,
        disableDuplicate: true,
        skipErrorHandler: true,
        skipBusinessError: true,
        // Setup is public. A rejected status read must not wait for a session.
        skipAuthRefresh: true,
      }),
      cancelled,
    ])
    const response = res.data
    if (
      response?.success !== true ||
      typeof response.data?.status !== 'boolean' ||
      typeof response.data.root_init !== 'boolean' ||
      typeof response.data.database_type !== 'string'
    ) {
      throw new SetupStatusError()
    }
    return response
  } catch (error) {
    throw error instanceof SetupStatusError ? error : new SetupStatusError(error)
  } finally {
    globalThis.clearTimeout(deadline)
    signal?.removeEventListener('abort', cancel)
  }
}

export async function submitSetup(
  payload: Record<string, unknown>
): Promise<SetupResponse> {
  const res = await api.post('/api/setup', payload)
  return res.data
}

export function buildSetupPayload(
  values: SetupFormValues,
  rootInitialized: boolean
) {
  const { usageMode, ...rest } = values

  const basePayload = {
    SelfUseModeEnabled: usageMode === 'self',
    DemoSiteEnabled: usageMode === 'demo',
  }

  if (rootInitialized) {
    return basePayload
  }

  return {
    ...rest,
    ...basePayload,
  }
}
