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
import { DEFAULT_CONFIG } from '../../constants'
import type {
  PlaygroundConfig,
  PlaygroundKey,
  PlaygroundParameterKey,
} from '../../types'

// Mirror only the already qualified native Chat client contract. The server
// still checks the current key policy, provider, final payload, and usage.
export const STRICT_CHAT_MAX_TOKENS = 128000
export const STRICT_CHAT_UNSUPPORTED_PARAMETERS: PlaygroundParameterKey[] = [
  'temperature',
  'top_p',
  'frequency_penalty',
  'presence_penalty',
  'seed',
]

export function usesStrictChatParameters(
  config: PlaygroundConfig,
  selectedKey?: PlaygroundKey
): boolean {
  return (
    selectedKey?.strict_token_budget === true &&
    selectedKey.id === config.keyId &&
    config.model === 'gpt-6.1-sol'
  )
}

export function normalizeStrictChatMaxTokens(value: number): number {
  if (!Number.isFinite(value)) return DEFAULT_CONFIG.max_tokens
  return Math.min(STRICT_CHAT_MAX_TOKENS, Math.max(1, Math.trunc(value)))
}
