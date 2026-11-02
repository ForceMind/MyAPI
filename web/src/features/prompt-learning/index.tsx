import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { LogPagination } from '@/components/log-pagination'
import { Alert, AlertTitle } from '@/components/ui/alert'
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
import { useAuthStore } from '@/stores/auth-store'

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
  const userId = useAuthStore((state) => state.auth.user?.id)
  const role = useAuthStore((state) => state.auth.user?.role)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  return (
    <PromptLearningSession
      key={JSON.stringify([userId, role, sessionId])}
      userId={userId}
      role={role}
      sessionId={sessionId}
    />
  )
}

function PromptLearningSession(props: {
  userId: number | undefined
  role: number | undefined
  sessionId: string | null
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [content, setContent] = useState('')
  const [error, setError] = useState('')
  const [versionPage, setVersionPage] = useState(1)
  const [versionPageSize, setVersionPageSize] = useState(20)
  const [runPage, setRunPage] = useState(1)
  const [runPageSize, setRunPageSize] = useState(20)
  const actorScope = [props.userId, props.role, props.sessionId]
  const policyKey = [...promptLearningPolicyQueryKey, ...actorScope]
  const versionKey = [...promptLearningVersionQueryKey, ...actorScope]
  const runKey = [...promptLearningRunQueryKey, ...actorScope]
  const isCurrentIdentity = () => {
    const auth = useAuthStore.getState().auth
    return (
      auth.user?.id === props.userId &&
      auth.user?.role === props.role &&
      (auth.session?.sid ?? null) === props.sessionId
    )
  }
  const policyQuery = useQuery({
    queryKey: policyKey,
    queryFn: () => {
      if (!isCurrentIdentity()) throw new Error('Learning identity changed')
      return getPromptLearningPolicy()
    },
  })
  const versionsQuery = useQuery({
    queryKey: [...versionKey, versionPage, versionPageSize],
    queryFn: () => {
      if (!isCurrentIdentity()) throw new Error('Learning identity changed')
      return getPromptLearningVersions(versionPage, versionPageSize)
    },
  })
  const runsQuery = useQuery({
    queryKey: [...runKey, runPage, runPageSize],
    queryFn: () => {
      if (!isCurrentIdentity()) throw new Error('Learning identity changed')
      return getPromptLearningRuns(runPage, runPageSize)
    },
  })
  const policyMutation = useMutation({
    mutationFn: (nextEnabled: boolean) => {
      if (!isCurrentIdentity()) throw new Error('Learning identity changed')
      return updatePromptLearningPolicy(nextEnabled)
    },
    onSuccess: async () => {
      if (!isCurrentIdentity()) return
      setError('')
      await queryClient.invalidateQueries({
        queryKey: policyKey,
      })
    },
    onError: () => {
      if (isCurrentIdentity()) setError(t('Failed to save learning policy'))
    },
  })
  const versionMutation = useMutation({
    mutationFn: (input: Parameters<typeof createPromptLearningVersion>[0]) => {
      if (!isCurrentIdentity()) throw new Error('Learning identity changed')
      return createPromptLearningVersion(input)
    },
    onSuccess: async () => {
      if (!isCurrentIdentity()) return
      setContent('')
      setError('')
      await queryClient.invalidateQueries({
        queryKey: versionKey,
      })
    },
    onError: () => {
      if (isCurrentIdentity()) setError(t('Failed to save instruction version'))
    },
  })
  const runCancellationMutation = useMutation({
    mutationFn: (runId: number) => {
      if (!isCurrentIdentity()) throw new Error('Learning identity changed')
      return cancelPromptLearningRun(runId)
    },
    onSuccess: async () => {
      if (!isCurrentIdentity()) return
      setError('')
      await queryClient.invalidateQueries({
        queryKey: runKey,
      })
    },
    onError: () => {
      if (isCurrentIdentity()) setError(t('Failed to cancel learning run.'))
    },
  })

  const policyReady =
    policyQuery.isSuccess && typeof policyQuery.data?.enabled === 'boolean'
  const versionsReady =
    versionsQuery.isSuccess && Array.isArray(versionsQuery.data?.items)
  const runsReady = runsQuery.isSuccess && Array.isArray(runsQuery.data?.items)
  const enabled = policyReady && policyQuery.data?.enabled === true
  const canChangePolicy = policyReady && !policyQuery.isFetching

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Prompt learning')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t(
          'Build versioned instructions from explicitly authorized, redacted user requests.'
        )}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <div className='flex w-full min-w-0 flex-col gap-4 sm:gap-5 [&_[data-myapi-button]]:min-h-11 [&_[data-myapi-button]]:min-w-11'>
          <Card data-card-hover='false'>
            <CardHeader>
              <CardTitle>
                <h2>{t('Learning policy')}</h2>
              </CardTitle>
              <CardDescription>
                {t('Only dashboard sessions can change this authorization.')}
              </CardDescription>
            </CardHeader>
            <CardContent className='flex items-center justify-between gap-4'>
              <div className='min-w-0 flex-1'>
                {policyQuery.isPending && (
                  <p role='status' className='text-muted-foreground'>
                    {t('Loading learning settings…')}
                  </p>
                )}
                {!policyQuery.isPending && !policyReady && (
                  <Alert variant='destructive'>
                    <AlertTitle>
                      {t('Failed to load learning policy')}
                    </AlertTitle>
                    <Button
                      variant='outline'
                      className='mt-2 w-fit'
                      disabled={policyQuery.isFetching}
                      onClick={() => void policyQuery.refetch()}
                    >
                      {t('Retry')}
                    </Button>
                  </Alert>
                )}
                {policyReady && (
                  <p className='font-medium'>
                    {enabled
                      ? t('Learning is enabled')
                      : t('Learning is disabled')}
                  </p>
                )}
                <p className='text-muted-foreground mt-1 text-sm'>
                  {t('No model call or file change is made from this page.')}
                </p>
              </div>
              <Switch
                aria-label={t('Enable learning')}
                checked={enabled}
                disabled={!canChangePolicy || policyMutation.isPending}
                onCheckedChange={(nextEnabled) =>
                  policyMutation.mutate(nextEnabled)
                }
              />
            </CardContent>
          </Card>

          <Card data-card-hover='false'>
            <CardHeader>
              <CardTitle>
                <h2>{t('Run history')}</h2>
              </CardTitle>
              <CardDescription>
                {t(
                  'Run history shows pending, completed, cancelled, and unknown analysis work.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent>
              {runsQuery.isPending && (
                <p role='status' className='text-muted-foreground'>
                  {t('Loading learning runs…')}
                </p>
              )}
              {!runsQuery.isPending && !runsReady && (
                <Alert variant='destructive'>
                  <AlertTitle>{t('Failed to load learning runs')}</AlertTitle>
                  <Button
                    variant='outline'
                    className='mt-2 w-fit'
                    disabled={runsQuery.isFetching}
                    onClick={() => void runsQuery.refetch()}
                  >
                    {t('Retry')}
                  </Button>
                </Alert>
              )}
              {runsReady && runsQuery.data?.items.length === 0 && (
                <p className='text-muted-foreground'>
                  {t('No learning runs yet.')}
                </p>
              )}
              {runsReady &&
                runsQuery.data &&
                runsQuery.data.items.length > 0 && (
                  <div className='space-y-3'>
                    {runsQuery.data.items.map((run) => (
                      <article
                        className='border-border rounded-lg border p-3'
                        key={run.id}
                      >
                        <div className='text-muted-foreground flex flex-wrap justify-between gap-3 text-xs [overflow-wrap:anywhere]'>
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
                              disabled={
                                !canChangePolicy ||
                                runsQuery.isFetching ||
                                runCancellationMutation.isPending
                              }
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
                        <p className='mt-2 text-sm [overflow-wrap:anywhere]'>
                          {run.model_ref}
                        </p>
                        <p className='text-muted-foreground mt-1 text-sm'>
                          {t('Samples: {{count}}', { count: run.sample_count })}
                        </p>
                      </article>
                    ))}
                  </div>
                )}
              <div className='mt-4 border-t pt-4'>
                <LogPagination
                  label={t('Learning run pagination')}
                  page={runPage}
                  pageSize={runPageSize}
                  total={
                    runsQuery.isSuccess ? runsQuery.data?.total : undefined
                  }
                  busy={runsQuery.isFetching}
                  onPageChange={setRunPage}
                  onPageSizeChange={(size) => {
                    setRunPage(1)
                    setRunPageSize(size)
                  }}
                />
              </div>
            </CardContent>
          </Card>

          <Card data-card-hover='false'>
            <CardHeader>
              <CardTitle>
                <h2>{t('Create manual version')}</h2>
              </CardTitle>
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
                disabled={
                  !canChangePolicy ||
                  !versionsReady ||
                  versionsQuery.isFetching ||
                  content.trim() === '' ||
                  versionMutation.isPending
                }
                onClick={() =>
                  versionMutation.mutate({ commandId: commandId(), content })
                }
              >
                {t('Save version')}
              </Button>
            </CardContent>
          </Card>

          <Card data-card-hover='false'>
            <CardHeader>
              <CardTitle>
                <h2>{t('Version history')}</h2>
              </CardTitle>
            </CardHeader>
            <CardContent>
              {versionsQuery.isPending && (
                <p role='status' className='text-muted-foreground'>
                  {t('Loading instruction versions…')}
                </p>
              )}
              {!versionsQuery.isPending && !versionsReady && (
                <Alert variant='destructive'>
                  <AlertTitle>
                    {t('Failed to load instruction versions')}
                  </AlertTitle>
                  <Button
                    variant='outline'
                    className='mt-2 w-fit'
                    disabled={versionsQuery.isFetching}
                    onClick={() => void versionsQuery.refetch()}
                  >
                    {t('Retry')}
                  </Button>
                </Alert>
              )}
              {versionsReady && versionsQuery.data?.items.length === 0 && (
                <p className='text-muted-foreground'>
                  {t('No instruction versions yet.')}
                </p>
              )}
              {versionsReady &&
                versionsQuery.data &&
                versionsQuery.data.items.length > 0 && (
                  <div className='space-y-3'>
                    {versionsQuery.data.items.map((version) => (
                      <article
                        className='border-border rounded-lg border p-3'
                        key={version.id}
                      >
                        <div className='text-muted-foreground mb-2 flex flex-wrap justify-between gap-3 text-xs'>
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
                        <pre className='font-sans text-sm [overflow-wrap:anywhere] whitespace-pre-wrap'>
                          {version.content}
                        </pre>
                      </article>
                    ))}
                  </div>
                )}
              {error && (
                <p role='alert' className='text-destructive mt-3 text-sm'>
                  {error}
                </p>
              )}
              <div className='mt-4 border-t pt-4'>
                <LogPagination
                  label={t('Instruction version pagination')}
                  page={versionPage}
                  pageSize={versionPageSize}
                  total={
                    versionsQuery.isSuccess ? versionsQuery.data?.total : undefined
                  }
                  busy={versionsQuery.isFetching}
                  onPageChange={setVersionPage}
                  onPageSizeChange={(size) => {
                    setVersionPage(1)
                    setVersionPageSize(size)
                  }}
                />
              </div>
            </CardContent>
          </Card>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
