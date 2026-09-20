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
import { useMutation } from '@tanstack/react-query'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import { createChannel, updateChannel } from '../api'
import { ERROR_MESSAGES, SUCCESS_MESSAGES } from '../constants'
import {
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
  resolveChannelOperationAttempt,
  notifyChannelMutationSuccess,
  type ChannelFormValues,
} from '../lib'
import type { Channel, ChannelMutationResponse } from '../types'

type UseChannelMutateFormParams = {
  currentRow?: Channel | null
  isEditing: boolean
  isMultiKeyChannel: boolean
  onSuccess: () => void
}

const SENSITIVE_UPDATE_FIELDS = [
  'type',
  'key',
  'base_url',
  'openai_organization',
  'param_override',
  'header_override',
  'setting',
  'settings',
  'other',
] satisfies (keyof Channel)[]

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function getErrorMessage(error: unknown): string | undefined {
  if (error instanceof Error && typeof error.message === 'string') {
    return error.message
  }

  if (!isRecord(error)) return undefined

  const response = error.response
  if (isRecord(response)) {
    const data = response.data
    if (isRecord(data)) {
      const message = data.message
      if (typeof message === 'string') return message
    }
  }

  const message = error.message
  if (typeof message === 'string') return message
  return undefined
}

export function useChannelMutateForm(props: UseChannelMutateFormParams) {
  const { t } = useTranslation()
  const createOperationRef = useRef<{
    fingerprint: string
    operationKey: string
  } | null>(null)
  const currentUser = useAuthStore((s) => s.auth.user)
  const canEditSensitive = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )

  return useMutation({
    mutationFn: async (
      data: ChannelFormValues
    ): Promise<{
      messageKey: string
      response: ChannelMutationResponse<unknown>
    }> => {
      if (props.isEditing && props.currentRow) {
        const payload = transformFormDataToUpdatePayload(
          data,
          props.currentRow.id
        )
        if (!data.key?.trim()) {
          delete payload.key
        }
        if (!canEditSensitive) {
          for (const field of SENSITIVE_UPDATE_FIELDS) {
            delete payload[field]
          }
        }
        const payloadWithKeyMode =
          canEditSensitive &&
          props.isMultiKeyChannel &&
          data.key?.trim() &&
          data.key_mode
            ? {
                ...payload,
                key_mode: data.key_mode,
              }
            : payload

        const response = await updateChannel(
          props.currentRow.id,
          payloadWithKeyMode
        )
        if (!response.success) {
          throw new Error(response.message || t(ERROR_MESSAGES.UPDATE_FAILED))
        }
        return { messageKey: SUCCESS_MESSAGES.UPDATED, response }
      }

      const payload = transformFormDataToCreatePayload(data)
      const fingerprint = JSON.stringify(payload)
      const operation = resolveChannelOperationAttempt(
        createOperationRef.current,
        fingerprint
      )
      createOperationRef.current = operation
      const response = await createChannel(payload, operation.operationKey)
      if (!response.success) {
        throw new Error(response.message || t(ERROR_MESSAGES.CREATE_FAILED))
      }
      createOperationRef.current = null
      return { messageKey: SUCCESS_MESSAGES.CREATED, response }
    },
    onSuccess: ({ messageKey, response }) => {
      notifyChannelMutationSuccess(response, t(messageKey))
      props.onSuccess()
    },
    onError: (error: unknown) => {
      toast.error(getErrorMessage(error) || t(ERROR_MESSAGES.CREATE_FAILED))
    },
  })
}
