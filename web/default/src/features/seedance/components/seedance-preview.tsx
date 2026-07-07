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

import { Loader2Icon, VideoIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { StatusBadge } from '@/components/status-badge'
import { Progress } from '@/components/ui/progress'
import { VIDEO_STATUS } from '../constants'
import type { OpenAIVideoTask } from '../types'

type SeedancePreviewProps = {
  taskId: string | null
  task?: OpenAIVideoTask
  playbackUrl: string | null
  isPolling: boolean
  isSubmitting: boolean
}

function statusMapper(status?: string) {
  switch (status) {
    case VIDEO_STATUS.COMPLETED:
      return { label: 'Completed', variant: 'success' as const }
    case VIDEO_STATUS.FAILED:
      return { label: 'Failed', variant: 'danger' as const }
    case VIDEO_STATUS.IN_PROGRESS:
      return { label: 'In progress', variant: 'info' as const }
    case VIDEO_STATUS.QUEUED:
      return { label: 'Queued', variant: 'warning' as const }
    default:
      return { label: status ?? 'Unknown', variant: 'neutral' as const }
  }
}

export function SeedancePreview(props: SeedancePreviewProps) {
  const { t } = useTranslation()

  const mapped = statusMapper(props.task?.status)
  const progress = props.task?.progress ?? (props.isSubmitting ? 0 : undefined)
  const isGenerating =
    props.isSubmitting ||
    props.isPolling ||
    (props.task?.status &&
      props.task.status !== VIDEO_STATUS.COMPLETED &&
      props.task.status !== VIDEO_STATUS.FAILED)
  const showVideo =
    props.playbackUrl && props.task?.status === VIDEO_STATUS.COMPLETED

  return (
    <div className='bg-muted/20 flex min-h-[320px] flex-1 flex-col overflow-hidden lg:min-h-0'>
      <div className='border-b px-4 py-3 sm:px-6'>
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div>
            <h2 className='text-sm font-semibold'>{t('Generated video')}</h2>
            {props.taskId ? (
              <p className='text-muted-foreground mt-0.5 font-mono text-xs'>
                {props.taskId}
              </p>
            ) : (
              <p className='text-muted-foreground mt-0.5 text-xs'>
                {t('Video preview will appear here after generation')}
              </p>
            )}
          </div>
          {props.task?.status ? (
            <StatusBadge
              label={t(mapped.label)}
              variant={mapped.variant}
              copyable={false}
            />
          ) : null}
        </div>
      </div>

      <div className='flex min-h-0 flex-1 flex-col items-center justify-center p-4 sm:p-6'>
        {showVideo ? (
          <video
            className={cn(
              'bg-background max-h-full w-full max-w-4xl rounded-xl border shadow-sm'
            )}
            controls
            playsInline
            src={props.playbackUrl ?? undefined}
          />
        ) : props.task?.status === VIDEO_STATUS.FAILED ? (
          <div className='text-center'>
            <VideoIcon className='text-muted-foreground/40 mx-auto size-12' />
            <p className='text-destructive mt-4 text-sm'>
              {props.task.error?.message ?? t('Video generation failed')}
            </p>
          </div>
        ) : isGenerating ? (
          <div className='flex w-full max-w-md flex-col items-center gap-4 text-center'>
            <Loader2Icon className='text-primary size-10 animate-spin' />
            <div className='space-y-1'>
              <p className='text-sm font-medium'>
                {props.isSubmitting && !props.task
                  ? t('Submitting video task...')
                  : t('Generating video...')}
              </p>
              {props.isPolling ? (
                <p className='text-muted-foreground text-xs'>
                  {t('Polling task status every 3 seconds...')}
                </p>
              ) : null}
            </div>
            {typeof progress === 'number' ? (
              <div className='w-full space-y-2'>
                <div className='text-muted-foreground flex items-center justify-between text-xs'>
                  <span>{t('Progress')}</span>
                  <span>{progress}%</span>
                </div>
                <Progress value={progress} />
              </div>
            ) : null}
          </div>
        ) : (
          <div className='text-muted-foreground flex max-w-sm flex-col items-center text-center'>
            <VideoIcon className='mb-4 size-14 opacity-30' />
            <p className='text-sm'>{t('Configure parameters on the left')}</p>
            <p className='mt-1 text-xs'>
              {t('Then click generate to preview the video here')}
            </p>
          </div>
        )}
      </div>
    </div>
  )
}
