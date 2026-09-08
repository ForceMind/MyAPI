/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { api, type ApiRequestConfig } from '@/lib/api'

import type {
  ChannelRoutingPolicy,
  ChannelRoutingResponse,
  RoutingChannelUpdate,
  RoutingMutationResponse,
  RoutingPreviewRequest,
  RoutingPreviewResponse,
} from './types'

const routingRequestConfig: ApiRequestConfig = {
  skipBusinessError: true,
  skipErrorHandler: true,
}

export async function getChannelRouting(): Promise<ChannelRoutingResponse> {
  const response = await api.get('/api/channel/routing', routingRequestConfig)
  return response.data
}

export async function updateChannelRoutingPolicy(
  policy: ChannelRoutingPolicy
): Promise<RoutingMutationResponse> {
  const response = await api.put(
    '/api/channel/routing',
    policy,
    routingRequestConfig
  )
  return response.data
}

export async function previewChannelRouting(
  request: RoutingPreviewRequest
): Promise<RoutingPreviewResponse> {
  const response = await api.post(
    '/api/channel/routing/preview',
    request,
    routingRequestConfig
  )
  return response.data
}

export async function updateRoutingChannel(
  id: number,
  update: RoutingChannelUpdate
): Promise<RoutingMutationResponse> {
  const response = await api.put(
    `/api/channel/routing/channels/${id}`,
    update,
    routingRequestConfig
  )
  return response.data
}
