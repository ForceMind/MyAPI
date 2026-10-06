import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Main } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import {
  cancelPromptLearningRun,
  createPromptLearningVersion,
  getPromptLearningPolicy,
  getPromptLearningRuns,
  getPromptLearningVersions,
  updatePromptLearningPolicy,
} from './api'

const promptLearningPolicyQueryKey = ['prompt-learning', 'policy'] as const
const promptLearningVersionQueryKey = ['prompt-learning', 'versions'] as const
const promptLearningRunQueryKey = ['prompt-learning', 'runs'] as const

function commandId() {
  return crypto.randomUUID()
}

function downloadInstructionVersion(version: { content: string; id: number }) {
  const blob = new Blob([version.content], {
    type: 'text/markdown;charset=utf-8',
  })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `myapi-prompt-instructions-v${version.id}.md`
  link.click()
  URL.revokeObjectURL(url)
}

export function PromptLearning() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [content, setContent] = useState('')
  const [error, setError] = useState('')
  const policyQuery = useQuery({
    queryKey: promptLearningPolicyQueryKey,
    queryFn: getPromptLearningPolicy,
  })
  const versionsQuery = useQuery({
    queryKey: promptLearningVersionQueryKey,
    queryFn: () => getPromptLearningVersions(),
  })
  const runsQuery = useQuery({
    queryKey: promptLearningRunQueryKey,
    queryFn: () => getPromptLearningRuns(),
  })
  const policyMutation = useMutation({
    mutationFn: updatePromptLearningPolicy,
    onSuccess: async () => {
      setError('')
      await queryClient.invalidateQueries({
        queryKey: promptLearningPolicyQueryKey,
      })
    },
    onError: () => setError(t('Failed to save learning policy')),
  })
  const versionMutation = useMutation({
    mutationFn: createPromptLearningVersion,
    onSuccess: async () => {
      setContent('')
      setError('')
      await queryClient.invalidateQueries({
        queryKey: promptLearningVersionQueryKey,
      })
    },
    onError: () => setError(t('Failed to save instruction version')),
  })
  const runCancellationMutation = useMutation({
    mutationFn: cancelPromptLearningRun,
    onSuccess: async () => {
      setError('')
      await queryClient.invalidateQueries({
        queryKey: promptLearningRunQueryKey,
      })
    },
    onError: () => setError(t('Failed to cancel learning run.')),
  })

  const enabled = policyQuery.data?.enabled === true
  const isLoading =
    policyQuery.isLoading || versionsQuery.isLoading || runsQuery.isLoading

  return (
    <Main>
      <div className='min-h-0 flex-1 overflow-auto px-3 py-3 sm:px-4 sm:py-6'>
        <div className='mx-auto flex w-full max-w-5xl flex-col gap-4 sm:gap-6'>
          <div>
            <h1 className='text-xl font-semibold'>{t('Prompt learning')}</h1>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t(
                'Build versioned instructions from explicitly authorized, redacted user requests.'
              )}
            </p>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>{t('Learning policy')}</CardTitle>
              <CardDescription>
                {t('Only dashboard sessions can change this authorization.')}
              </CardDescription>
            </CardHeader>
            <CardContent className='flex items-center justify-between gap-4'>
              <div>
                <p className='font-medium'>
                  {enabled
                    ? t('Learning is enabled')
                    : t('Learning is disabled')}
                </p>
                <p className='text-muted-foreground mt-1 text-sm'>
                  {t('No model call or file change is made from this page.')}
                </p>
              </div>
              <Switch
                aria-label={t('Enable learning')}
                checked={enabled}
                disabled={policyQuery.isLoading || policyMutation.isPending}
                onCheckedChange={(nextEnabled) =>
                  policyMutation.mutate(nextEnabled)
                }
              />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('Run history')}</CardTitle>
              <CardDescription>
                {t(
                  'Run history shows pending, completed, cancelled, and unknown analysis work.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent>
              {!runsQuery.isLoading && runsQuery.isError && (
                <Button
                  variant='outline'
                  onClick={() => void runsQuery.refetch()}
                >
                  {t('Retry loading')}
                </Button>
              )}
              {!runsQuery.isLoading &&
                !runsQuery.isError &&
                runsQuery.data?.items.length === 0 && (
                  <p className='text-muted-foreground'>
                    {t('No learning runs yet.')}
                  </p>
                )}
              {!runsQuery.isLoading &&
                !runsQuery.isError &&
                runsQuery.data &&
                runsQuery.data.items.length > 0 && (
                  <div className='space-y-3'>
                    {runsQuery.data.items.map((run) => (
                      <article
                        className='border-border rounded-lg border p-3'
                        key={run.id}
                      >
                        <div className='text-muted-foreground flex justify-between gap-3 text-xs'>
                          <span>{run.state}</span>
                          <span>
                            {new Date(run.updated_at * 1000).toLocaleString()}
                          </span>
                        </div>
                        {['pending', 'leased', 'preparing'].includes(
                          run.state
                        ) && (
                          <div className='mt-2 flex justify-end'>
                            <Button
                              disabled={runCancellationMutation.isPending}
                              onClick={() =>
                                runCancellationMutation.mutate(run.id)
                              }
                              size='sm'
                              variant='outline'
                            >
                              {t('Cancel')}
                            </Button>
                          </div>
                        )}
                        <p className='mt-2 text-sm'>{run.model_ref}</p>
                        <p className='text-muted-foreground mt-1 text-sm'>
                          {t('Samples: {{count}}', { count: run.sample_count })}
                        </p>
                      </article>
                    ))}
                  </div>
                )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('Create manual version')}</CardTitle>
              <CardDescription>
                {t(
                  'Manual edits create an immutable version and never apply it to a file automatically.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent className='space-y-3'>
              <Textarea
                aria-label={t('Instruction content')}
                disabled={versionMutation.isPending}
                placeholder={t(
                  'Write the instruction text to preserve as a version.'
                )}
                value={content}
                onChange={(event) => setContent(event.target.value)}
              />
              <Button
                disabled={content.trim() === '' || versionMutation.isPending}
                onClick={() =>
                  versionMutation.mutate({ commandId: commandId(), content })
                }
              >
                {t('Save version')}
              </Button>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('Version history')}</CardTitle>
            </CardHeader>
            <CardContent>
              {isLoading && (
                <p className='text-muted-foreground'>
                  {t('Loading learning settings…')}
                </p>
              )}
              {!isLoading && versionsQuery.isError && (
                <Button
                  variant='outline'
                  onClick={() => void versionsQuery.refetch()}
                >
                  {t('Retry loading')}
                </Button>
              )}
              {!isLoading &&
                !versionsQuery.isError &&
                versionsQuery.data?.items.length === 0 && (
                  <p className='text-muted-foreground'>
                    {t('No instruction versions yet.')}
                  </p>
                )}
              {!isLoading &&
                !versionsQuery.isError &&
                versionsQuery.data &&
                versionsQuery.data.items.length > 0 && (
                  <div className='space-y-3'>
                    {versionsQuery.data.items.map((version) => (
                      <article
                        className='border-border rounded-lg border p-3'
                        key={version.id}
                      >
                        <div className='text-muted-foreground mb-2 flex justify-between gap-3 text-xs'>
                          <span>{version.source}</span>
                          <span>
                            {new Date(
                              version.created_at * 1000
                            ).toLocaleString()}
                          </span>
                        </div>
                        <div className='mb-2 flex justify-end'>
                          <Button
                            aria-label={t('Download')}
                            onClick={() => downloadInstructionVersion(version)}
                            size='sm'
                            variant='outline'
                          >
                            {t('Download')}
                          </Button>
                        </div>
                        <pre className='font-sans text-sm break-words whitespace-pre-wrap'>
                          {version.content}
                        </pre>
                      </article>
                    ))}
                  </div>
                )}
              {error && (
                <p className='text-destructive mt-3 text-sm'>{error}</p>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </Main>
  )
}
