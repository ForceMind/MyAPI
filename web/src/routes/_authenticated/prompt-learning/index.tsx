import { createFileRoute } from '@tanstack/react-router'

import { PromptLearning } from '@/features/prompt-learning'

export const Route = createFileRoute('/_authenticated/prompt-learning/')({
  component: PromptLearning,
})
