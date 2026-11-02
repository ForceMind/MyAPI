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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { toast } from 'sonner'

import {
  createCustomOAuthProvider,
  updateCustomOAuthProvider,
  deleteCustomOAuthProvider,
  discoverOIDCEndpoints,
} from '../api'
import type { CustomOAuthProvider, DiscoveryResponse } from '../types'

type MutationTaskOptions = { captureResultGuard?: () => (() => boolean) }

function useInvalidateOnSuccess() {
  const queryClient = useQueryClient()
  return {
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['custom-oauth-providers'] })
      queryClient.invalidateQueries({ queryKey: ['status'] })
    },
  }
}

export function useCreateProvider(options?: MutationTaskOptions) {
  const invalidate = useInvalidateOnSuccess()

  return useMutation({
    retry: false,
    onMutate: () => ({ isCurrent: options?.captureResultGuard?.() }),
    mutationFn: (data: Omit<CustomOAuthProvider, 'id'>) =>
      createCustomOAuthProvider(data),
    onSuccess: (res, _variables, context) => {
      if (res.success) {
        invalidate.onSuccess()
        if (context?.isCurrent?.() !== false) toast.success(i18next.t('Provider created successfully'))
      } else if (context?.isCurrent?.() !== false) {
        toast.error(res.message || i18next.t('Failed to update setting'))
      }
    },
    onError: (error: Error, _variables, context) => {
      if (context?.isCurrent?.() === false) return
      toast.error(error.message || i18next.t('Failed to create provider'))
    },
  })
}

export function useUpdateProvider(options?: MutationTaskOptions) {
  const invalidate = useInvalidateOnSuccess()

  return useMutation({
    retry: false,
    onMutate: () => ({ isCurrent: options?.captureResultGuard?.() }),
    mutationFn: ({
      id,
      data,
    }: {
      id: number
      data: Partial<CustomOAuthProvider>
    }) => updateCustomOAuthProvider(id, data),
    onSuccess: (res, _variables, context) => {
      if (res.success) {
        invalidate.onSuccess()
        if (context?.isCurrent?.() !== false) toast.success(i18next.t('Provider updated successfully'))
      } else if (context?.isCurrent?.() !== false) {
        toast.error(res.message || i18next.t('Failed to update setting'))
      }
    },
    onError: (error: Error, _variables, context) => {
      if (context?.isCurrent?.() === false) return
      toast.error(error.message || i18next.t('Failed to update provider'))
    },
  })
}

export function useDeleteProvider(options?: MutationTaskOptions) {
  const invalidate = useInvalidateOnSuccess()

  return useMutation({
    retry: false,
    onMutate: () => ({ isCurrent: options?.captureResultGuard?.() }),
    mutationFn: (id: number) => deleteCustomOAuthProvider(id),
    onSuccess: (res, _variables, context) => {
      if (res.success) {
        invalidate.onSuccess()
        if (context?.isCurrent?.() !== false) toast.success(i18next.t('Provider deleted successfully'))
      } else if (context?.isCurrent?.() !== false) {
        toast.error(res.message || i18next.t('Failed to update setting'))
      }
    },
    onError: (error: Error, _variables, context) => {
      if (context?.isCurrent?.() === false) return
      toast.error(error.message || i18next.t('Failed to delete provider'))
    },
  })
}

export function useDiscoverEndpoints(options?: MutationTaskOptions) {
  return useMutation({
    retry: false,
    onMutate: () => ({ isCurrent: options?.captureResultGuard?.() }),
    mutationFn: (wellKnownUrl: string) => discoverOIDCEndpoints(wellKnownUrl),
    onSuccess: (res: DiscoveryResponse, _variables, context) => {
      if (res.success) {
        if (context?.isCurrent?.() !== false) toast.success(i18next.t('OIDC endpoints discovered successfully'))
      } else if (context?.isCurrent?.() !== false) {
        toast.error(res.message || i18next.t('Failed to update setting'))
      }
    },
    onError: (error: Error, _variables, context) => {
      if (context?.isCurrent?.() === false) return
      toast.error(
        error.message || i18next.t('Failed to discover OIDC endpoints')
      )
    },
  })
}
