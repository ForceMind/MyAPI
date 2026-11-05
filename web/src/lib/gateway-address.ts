/* Copyright (C) 2026 ForceMind. AGPL-3.0-or-later. */

/** One installation root and one OpenAI-compatible /v1 URL for all client entry points. */
export function gatewayClientAddresses(serverAddress: unknown, origin: string) {
  const configured = typeof serverAddress === 'string' ? serverAddress.trim() : ''
  const root = (configured || origin).replace(/\/+$/, '').replace(/\/v1$/, '')
  return {
    root,
    openAI: `${root}/v1`,
    source: configured ? ('setting' as const) : ('current' as const),
  }
}
