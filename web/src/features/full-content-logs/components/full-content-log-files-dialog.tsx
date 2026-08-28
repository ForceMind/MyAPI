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

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, FileJson, RefreshCw, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

import {
  deleteAllFullContentLogFiles,
  deleteFullContentLogFile,
  downloadFullContentLogFile,
  getFullContentLogFiles,
} from '../api'
import { formatLogBytes } from '../lib/format'

interface FullContentLogFilesDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function FullContentLogFilesDialog(
  props: FullContentLogFilesDialogProps
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const [clearAllOpen, setClearAllOpen] = useState(false)
  const [downloadingFile, setDownloadingFile] = useState<string | null>(null)

  const filesQuery = useQuery({
    queryKey: ['full-content-logs', 'files'],
    queryFn: async () => {
      const response = await getFullContentLogFiles()
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load log files'))
      }
      return response.data
    },
    enabled: props.open,
    retry: false,
  })

  const refreshAllLogQueries = async () => {
    await queryClient.invalidateQueries({ queryKey: ['full-content-logs'] })
  }

  const deleteMutation = useMutation({
    mutationFn: async (filename: string) => {
      const response = await deleteFullContentLogFile(filename)
      if (!response.success) {
        throw new Error(response.message || t('Failed to delete log file'))
      }
      return response
    },
    onSuccess: async () => {
      toast.success(t('Log file deleted'))
      setDeleteTarget(null)
      await refreshAllLogQueries()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Failed to delete log file')
      )
    },
  })

  const clearMutation = useMutation({
    mutationFn: async () => {
      const response = await deleteAllFullContentLogFiles()
      if (!response.success) {
        throw new Error(response.message || t('Failed to clear log files'))
      }
      return response
    },
    onSuccess: async (response) => {
      toast.success(
        t('Deleted {{count}} log files and freed {{size}}.', {
          count: response.data?.deleted_count ?? 0,
          size: formatLogBytes(response.data?.freed_bytes ?? 0),
        })
      )
      setClearAllOpen(false)
      await refreshAllLogQueries()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Failed to clear log files')
      )
    },
  })

  const handleDownload = async (filename: string) => {
    setDownloadingFile(filename)
    try {
      await downloadFullContentLogFile(filename)
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to download log file')
      )
    } finally {
      setDownloadingFile(null)
    }
  }

  const files = filesQuery.data?.files ?? []

  return (
    <>
      <Dialog
        open={props.open}
        onOpenChange={props.onOpenChange}
        title={t('Full Content Log Files')}
        description={t(
          'Download raw JSONL files or remove records that are no longer needed.'
        )}
        contentClassName='sm:max-w-3xl'
        contentHeight='min(65vh, 620px)'
        footer={
          <div className='flex w-full items-center justify-between gap-2'>
            <span className='text-muted-foreground text-xs'>
              {t('{{count}} files, {{size}} total', {
                count: filesQuery.data?.count ?? 0,
                size: formatLogBytes(filesQuery.data?.total_size ?? 0),
              })}
            </span>
            <Button
              variant='destructive'
              disabled={files.length === 0}
              onClick={() => setClearAllOpen(true)}
            >
              <Trash2 />
              {t('Delete All')}
            </Button>
          </div>
        }
      >
        <div className='mb-3 flex justify-end'>
          <Button
            size='sm'
            variant='outline'
            disabled={filesQuery.isFetching}
            onClick={() => void filesQuery.refetch()}
          >
            <RefreshCw
              className={filesQuery.isFetching ? 'animate-spin' : ''}
            />
            {t('Refresh')}
          </Button>
        </div>

        {filesQuery.isLoading && (
          <div className='space-y-2' aria-label={t('Loading')}>
            <Skeleton className='h-14 w-full' />
            <Skeleton className='h-14 w-full' />
          </div>
        )}

        {filesQuery.isError && (
          <div className='border-destructive/30 bg-destructive/5 text-destructive rounded-lg border p-4 text-sm'>
            {filesQuery.error instanceof Error
              ? filesQuery.error.message
              : t('Failed to load log files')}
          </div>
        )}

        {!filesQuery.isLoading && !filesQuery.isError && files.length === 0 && (
          <div className='text-muted-foreground rounded-lg border border-dashed p-10 text-center text-sm'>
            {t('No full content log files')}
          </div>
        )}

        <div className='space-y-2'>
          {files.map((file) => (
            <div
              key={file.name}
              className='flex flex-col gap-3 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between'
            >
              <div className='flex min-w-0 items-center gap-3'>
                <span className='bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-lg'>
                  <FileJson className='size-4' aria-hidden='true' />
                </span>
                <div className='min-w-0'>
                  <div className='truncate font-mono text-xs'>{file.name}</div>
                  <div className='text-muted-foreground mt-1 text-xs'>
                    {formatLogBytes(file.size)} ·{' '}
                    {new Date(file.modified_at).toLocaleString()}
                  </div>
                </div>
              </div>
              <div className='flex shrink-0 gap-2 self-end sm:self-auto'>
                <Button
                  size='sm'
                  variant='outline'
                  disabled={downloadingFile === file.name}
                  onClick={() => void handleDownload(file.name)}
                >
                  <Download />
                  {t('Download')}
                </Button>
                <Button
                  size='sm'
                  variant='destructive'
                  onClick={() => setDeleteTarget(file.name)}
                >
                  <Trash2 />
                  {t('Delete')}
                </Button>
              </div>
            </div>
          ))}
        </div>
      </Dialog>

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t('Delete log file')}
        desc={t(
          'This permanently deletes the selected full request and response records. This action cannot be undone.'
        )}
        destructive
        confirmText={t('Delete')}
        isLoading={deleteMutation.isPending}
        handleConfirm={() => {
          if (deleteTarget) deleteMutation.mutate(deleteTarget)
        }}
      />

      <ConfirmDialog
        open={clearAllOpen}
        onOpenChange={setClearAllOpen}
        title={t('Delete all full content logs')}
        desc={t(
          'This permanently deletes every full request and response log file. New requests will start a new file.'
        )}
        destructive
        confirmText={t('Delete All')}
        isLoading={clearMutation.isPending}
        handleConfirm={() => clearMutation.mutate()}
      />
    </>
  )
}
