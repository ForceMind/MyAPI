import { api } from '@/lib/api'

export interface PromptLearningPolicy {
  scope: 'self'
  enabled: boolean
  generation: number
}

export interface PromptLearningVersion {
  id: number
  parent_id?: number
  content: string
  content_hash: string
  source: 'manual' | 'generated'
  created_at: number
}

export interface PromptLearningVersionPage {
  page: number
  page_size: number
  total: number
  items: PromptLearningVersion[]
}

export interface PromptLearningRun {
  id: number
  state: string
  sample_count: number
  model_ref: string
  template_version: string
  created_at: number
  updated_at: number
}

export interface PromptLearningRunPage {
  page: number
  page_size: number
  total: number
  items: PromptLearningRun[]
}

interface ApiResponse<T> {
  success: boolean
  message: string
  data: T
}

function requireSuccess<T>(response: ApiResponse<T>): T {
  if (!response.success) {
    throw new Error(response.message || 'Request failed')
  }
  return response.data
}

export async function getPromptLearningPolicy(): Promise<PromptLearningPolicy> {
  const response = await api.get<ApiResponse<PromptLearningPolicy>>(
    '/api/user/prompt-learning'
  )
  return requireSuccess(response.data)
}

export async function updatePromptLearningPolicy(
  enabled: boolean
): Promise<PromptLearningPolicy> {
  const response = await api.put<ApiResponse<PromptLearningPolicy>>(
    '/api/user/prompt-learning',
    { enabled }
  )
  return requireSuccess(response.data)
}

export async function getPromptLearningVersions(
  page = 1,
  pageSize = 20
): Promise<PromptLearningVersionPage> {
  const response = await api.get<ApiResponse<PromptLearningVersionPage>>(
    '/api/user/prompt-learning/versions',
    { params: { p: page, page_size: pageSize } }
  )
  return requireSuccess(response.data)
}

export async function getPromptLearningRuns(
  page = 1,
  pageSize = 20
): Promise<PromptLearningRunPage> {
  const response = await api.get<ApiResponse<PromptLearningRunPage>>(
    '/api/user/prompt-learning/runs',
    { params: { p: page, page_size: pageSize } }
  )
  return requireSuccess(response.data)
}

export async function cancelPromptLearningRun(runId: number): Promise<void> {
  const response = await api.post<ApiResponse<{ cancelled: boolean }>>(
    `/api/user/prompt-learning/runs/${runId}/cancel`
  )
  const result = requireSuccess(response.data)
  if (!result.cancelled) {
    throw new Error('Run was not cancelled')
  }
}

export async function createPromptLearningVersion(input: {
  commandId: string
  content: string
  parentId?: number
}): Promise<{ version: PromptLearningVersion; created: boolean }> {
  const response = await api.post<
    ApiResponse<{ version: PromptLearningVersion; created: boolean }>
  >('/api/user/prompt-learning/versions', {
    command_id: input.commandId,
    content: input.content,
    parent_id: input.parentId,
  })
  return requireSuccess(response.data)
}
