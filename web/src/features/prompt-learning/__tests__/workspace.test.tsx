import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import {
  cancelPromptLearningRun,
  createPromptLearningVersion,
  getPromptLearningPolicy,
  getPromptLearningRuns,
  getPromptLearningVersions,
  updatePromptLearningPolicy,
} from '../api'
import { PromptLearning } from '../index'

vi.mock('../api', () => ({
  cancelPromptLearningRun: vi.fn(),
  createPromptLearningVersion: vi.fn(),
  getPromptLearningPolicy: vi.fn(),
  getPromptLearningRuns: vi.fn(),
  getPromptLearningVersions: vi.fn(),
  updatePromptLearningPolicy: vi.fn(),
}))

function renderWorkspace() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <PromptLearning />
    </QueryClientProvider>
  )
}

describe('prompt learning workspace', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getPromptLearningPolicy).mockResolvedValue({
      scope: 'self',
      enabled: false,
      generation: 0,
    })
    vi.mocked(getPromptLearningVersions).mockResolvedValue({
      page: 1,
      page_size: 20,
      total: 0,
      items: [],
    })
    vi.mocked(getPromptLearningRuns).mockResolvedValue({
      page: 1,
      page_size: 20,
      total: 0,
      items: [],
    })
  })

  test('shows a disabled policy and enables it only after the user changes the switch', async () => {
    vi.mocked(updatePromptLearningPolicy).mockResolvedValue({
      scope: 'self',
      enabled: true,
      generation: 1,
    })
    const user = userEvent.setup()
    renderWorkspace()

    expect(await screen.findByText('Learning is disabled')).toBeInTheDocument()
    expect(
      await screen.findByText('No instruction versions yet.')
    ).toBeInTheDocument()
    expect(await screen.findByText('No learning runs yet.')).toBeInTheDocument()
    await user.click(screen.getByRole('switch', { name: 'Enable learning' }))

    await waitFor(() => {
      expect(updatePromptLearningPolicy).toHaveBeenCalled()
    })
    expect(vi.mocked(updatePromptLearningPolicy).mock.calls[0]?.[0]).toBe(true)
  })

  test('creates a manual immutable version only after content is entered', async () => {
    vi.mocked(createPromptLearningVersion).mockResolvedValue({
      created: true,
      version: {
        id: 1,
        content: '# Instructions',
        content_hash: 'a'.repeat(64),
        source: 'manual',
        created_at: 1,
      },
    })
    const user = userEvent.setup()
    renderWorkspace()

    const saveButton = await screen.findByRole('button', {
      name: 'Save version',
    })
    expect(saveButton).toBeDisabled()
    await user.type(
      screen.getByLabelText('Instruction content'),
      '# Instructions'
    )
    await user.click(saveButton)

    await waitFor(() => {
      expect(createPromptLearningVersion).toHaveBeenCalled()
    })
    expect(vi.mocked(createPromptLearningVersion).mock.calls[0]?.[0]).toEqual(
      expect.objectContaining({ content: '# Instructions' })
    )
  })

  test('downloads an existing immutable version without applying it', async () => {
    vi.mocked(getPromptLearningVersions).mockResolvedValue({
      page: 1,
      page_size: 20,
      total: 1,
      items: [
        {
          id: 12,
          content: '# Reviewed instructions',
          content_hash: 'a'.repeat(64),
          source: 'manual',
          created_at: 1,
        },
      ],
    })
    const createObjectURL = vi.fn(() => 'blob:prompt-instruction')
    const revokeObjectURL = vi.fn()
    const click = vi.fn()
    Object.assign(URL, { createObjectURL, revokeObjectURL })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(click)
    const user = userEvent.setup()
    renderWorkspace()

    await user.click(await screen.findByRole('button', { name: 'Download' }))

    expect(createObjectURL).toHaveBeenCalledWith(expect.any(Blob))
    expect(click).toHaveBeenCalledOnce()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:prompt-instruction')
  })

  test('shows only safe run lifecycle metadata', async () => {
    vi.mocked(getPromptLearningRuns).mockResolvedValue({
      page: 1,
      page_size: 20,
      total: 1,
      items: [
        {
          id: 8,
          state: 'submission_unknown',
          sample_count: 4,
          model_ref: 'gpt-6',
          template_version: 'prompt-learning-v1',
          created_at: 1,
          updated_at: 2,
        },
      ],
    })
    renderWorkspace()

    expect(await screen.findByText('submission_unknown')).toBeInTheDocument()
    expect(screen.getByText('gpt-6')).toBeInTheDocument()
    expect(screen.getByText('Samples: 4')).toBeInTheDocument()
  })

  test('cancels a pre-submission run and refreshes its history', async () => {
    vi.mocked(getPromptLearningRuns).mockResolvedValue({
      page: 1,
      page_size: 20,
      total: 1,
      items: [
        {
          id: 8,
          state: 'pending',
          sample_count: 4,
          model_ref: 'gpt-6',
          template_version: 'prompt-learning-v1',
          created_at: 1,
          updated_at: 2,
        },
      ],
    })
    vi.mocked(cancelPromptLearningRun).mockResolvedValue()
    const user = userEvent.setup()
    renderWorkspace()

    await user.click(await screen.findByRole('button', { name: 'Cancel' }))
    await waitFor(() => {
      expect(vi.mocked(cancelPromptLearningRun).mock.calls[0]?.[0]).toBe(8)
    })
  })
})
