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
import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import {
  getPlaygroundKeys,
  getUserModels,
  isPlaygroundKeyAvailable,
} from '../api'
import type { PlaygroundConfig } from '../types'

interface UsePlaygroundOptionsParams {
  userId?: number
  keyId?: number | null
  currentModel: string
  updateConfig: <K extends keyof PlaygroundConfig>(
    key: K,
    value: PlaygroundConfig[K]
  ) => void
}

export function usePlaygroundOptions(props: UsePlaygroundOptionsParams) {
  const { t } = useTranslation()
  const { currentModel, updateConfig } = props
  const keysQuery = useQuery({
    queryKey: ['playground-keys', props.userId],
    queryFn: async () => {
      try {
        return await getPlaygroundKeys()
      } catch {
        throw new Error('Failed to load API keys')
      }
    },
    retry: false,
    refetchInterval: 60_000,
  })
  const keys = keysQuery.data ?? []
  const selectedKey = keys.find((key) => key.id === props.keyId)
  const isKeyAvailable = Boolean(
    selectedKey && isPlaygroundKeyAvailable(selectedKey) && !keysQuery.isError
  )
  const modelsQuery = useQuery({
    queryKey: ['playground-models', props.userId, props.keyId],
    queryFn: async ({ signal }) => {
      try {
        return await getUserModels(props.keyId ?? 0, signal)
      } catch {
        throw new Error('Failed to load playground models')
      }
    },
    enabled: isKeyAvailable,
    retry: false,
  })
  const models =
    isKeyAvailable && !modelsQuery.isError ? (modelsQuery.data ?? []) : []

  useEffect(() => {
    if (!modelsQuery.data || !isKeyAvailable) return
    if (!modelsQuery.data.some((model) => model.value === currentModel)) {
      updateConfig('model', modelsQuery.data[0]?.value ?? '')
    }
  }, [modelsQuery.data, isKeyAvailable, currentModel, updateConfig])

  let keyNotice: string | null = null
  if (keysQuery.isError) keyNotice = t('Failed to load API keys')
  else if (props.keyId && !keysQuery.isPending && !isKeyAvailable) {
    keyNotice = t('The selected API key is unavailable. Choose another key.')
  } else if (!props.keyId) {
    keyNotice = t('Select an available API key before sending')
  } else if (modelsQuery.isError) {
    keyNotice = t('Failed to load playground models')
  } else if (!modelsQuery.isPending && models.length === 0) {
    keyNotice = t('No models are available for this API key')
  }

  return {
    keys,
    selectedKey,
    models,
    keyNotice,
    isLoadingKeys: keysQuery.isPending,
    isLoadingModels: modelsQuery.isPending && isKeyAvailable,
    canSend:
      isKeyAvailable && models.some((model) => model.value === currentModel),
    refreshKeys: () => {
      void keysQuery.refetch()
      if (isKeyAvailable) void modelsQuery.refetch()
    },
  }
}
