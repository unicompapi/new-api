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
/* eslint-disable react-refresh/only-export-components */
import { useState, useMemo } from 'react'
import type { ColumnDef } from '@tanstack/react-table'
import { ImageIcon, Music, Video } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getUserAvatarFallback, getUserAvatarStyle } from '@/lib/avatar'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { DataTableColumnHeader } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { TASK_ACTIONS, TASK_STATUS } from '../../constants'
import { taskActionMapper, taskStatusMapper } from '../../lib/mappers'
import type { TaskLog } from '../../types'
import {
  AudioPreviewDialog,
  type AudioClip,
} from '../dialogs/audio-preview-dialog'
import { FailReasonDialog } from '../dialogs/fail-reason-dialog'
import { ImageDialog } from '../dialogs/image-dialog'
import { useUsageLogsContext } from '../usage-logs-provider'
import {
  createDurationColumn,
  createChannelColumn,
  createProgressColumn,
} from './column-helpers'

type TaskMediaPreview = {
  type: 'image' | 'video'
  url: string
  sourceUrl?: string
}

const imageUrlPattern = /\.(avif|gif|jpe?g|png|webp)(\?.*)?$/i
const videoUrlPattern = /\.(m3u8|m4v|mov|mp4|mpeg|ogg|webm)(\?.*)?$/i
const imageKeys = ['image_url', 'image_large_url', 'cover_url', 'thumbnail_url']
const videoKeys = ['result_url', 'video_url']

function parseTaskJson(data: unknown): unknown {
  if (typeof data === 'string') {
    try {
      return JSON.parse(data)
    } catch {
      return data
    }
  }
  return data
}

function parseTaskData(data: unknown): unknown[] {
  const parsed = parseTaskJson(data)
  return Array.isArray(parsed) ? parsed : []
}

function isUrl(value: unknown): value is string {
  return typeof value === 'string' && /^(https?:\/\/|\/)/.test(value)
}

function getUrlValue(value: unknown): string | undefined {
  if (isUrl(value)) return value
  if (value && typeof value === 'object') {
    const url = (value as Record<string, unknown>).url
    if (isUrl(url)) return url
  }
  return undefined
}

function detectUrlType(url: string): TaskMediaPreview['type'] | undefined {
  if (videoUrlPattern.test(url)) return 'video'
  if (imageUrlPattern.test(url)) return 'image'
  return undefined
}

function isVideoTask(log: TaskLog) {
  return (
    log.action === TASK_ACTIONS.GENERATE ||
    log.action === TASK_ACTIONS.TEXT_GENERATE ||
    log.action === TASK_ACTIONS.FIRST_TAIL_GENERATE ||
    log.action === TASK_ACTIONS.REFERENCE_GENERATE ||
    log.action === TASK_ACTIONS.REMIX_GENERATE
  )
}

function findDataMediaUrl(
  data: unknown,
  type: TaskMediaPreview['type']
): string | undefined {
  const parsed = parseTaskJson(data)
  const items = Array.isArray(parsed) ? parsed : [parsed]
  const keys = type === 'video' ? videoKeys : imageKeys

  for (const item of items) {
    if (!item || typeof item !== 'object') continue

    const record = item as Record<string, unknown>
    for (const key of keys) {
      const url = getUrlValue(record[key])
      if (url) return url
    }
  }

  return undefined
}

function getTaskMediaPreview(log: TaskLog): TaskMediaPreview | undefined {
  if (log.status !== TASK_STATUS.SUCCESS) return undefined

  const resultUrl = getUrlValue(log.result_url) || getUrlValue(log.fail_reason)
  if (resultUrl) {
    const type = detectUrlType(resultUrl) || (isVideoTask(log) ? 'video' : 'image')
    return {
      type,
      url: type === 'video' ? `/v1/videos/${log.task_id}/content` : resultUrl,
      sourceUrl: resultUrl,
    }
  }

  const videoUrl = findDataMediaUrl(log.data, 'video')
  if (videoUrl) {
    return {
      type: 'video',
      url: isVideoTask(log) ? `/v1/videos/${log.task_id}/content` : videoUrl,
      sourceUrl: videoUrl,
    }
  }

  const imageUrl = findDataMediaUrl(log.data, 'image')
  if (imageUrl) return { type: 'image', url: imageUrl }

  return undefined
}

function VideoPreviewDialog(props: {
  url: string
  sourceUrl?: string
  taskId: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [hasError, setHasError] = useState(false)
  const displayUrl = props.sourceUrl || props.url

  const handleOpenChange = (open: boolean) => {
    if (open) setHasError(false)
    props.onOpenChange(open)
  }

  return (
    <Dialog open={props.open} onOpenChange={handleOpenChange}>
      <DialogContent className='sm:max-w-3xl'>
        <DialogHeader>
          <DialogTitle>{t('Preview')}</DialogTitle>
          <DialogDescription>{`${t('Task ID:')} ${props.taskId}`}</DialogDescription>
        </DialogHeader>
        <div className='py-4'>
          <video
            src={props.url}
            controls
            preload='metadata'
            onError={() => setHasError(true)}
            className='bg-muted max-h-[60vh] w-full rounded-lg border'
          />
          {hasError && (
            <p className='text-destructive mt-3 text-sm'>
              视频加载失败，源链接可能已过期或不支持浏览器播放。
            </p>
          )}
          <div className='bg-muted mt-4 rounded-md p-3'>
            <p className='text-muted-foreground font-mono text-xs break-all'>
              {displayUrl}
            </p>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function TaskMediaPreviewCell({ log }: { log: TaskLog }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const preview = getTaskMediaPreview(log)

  if (!preview) {
    return <span className='text-muted-foreground/60 text-xs'>-</span>
  }

  const Icon = preview.type === 'video' ? Video : ImageIcon

  return (
    <>
      <button
        type='button'
        className='group flex items-center gap-1 text-left text-xs'
        onClick={() => setOpen(true)}
        title={t('Preview')}
      >
        <Icon className='text-muted-foreground size-3' />
        <span className='text-foreground leading-snug group-hover:underline'>
          {t('Preview')}
        </span>
      </button>
      {preview.type === 'video' ? (
        <VideoPreviewDialog
          url={preview.url}
          sourceUrl={preview.sourceUrl}
          taskId={log.task_id}
          open={open}
          onOpenChange={setOpen}
        />
      ) : (
        <ImageDialog
          imageUrl={preview.url}
          taskId={log.task_id}
          open={open}
          onOpenChange={setOpen}
        />
      )}
    </>
  )
}

function AudioPreviewCell({ log }: { log: TaskLog }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const clips = useMemo(() => {
    const data = parseTaskData(log.data)
    return data.filter(
      (c) =>
        c && typeof c === 'object' && (c as Record<string, unknown>).audio_url
    )
  }, [log.data])

  if (clips.length === 0) return null

  return (
    <>
      <button
        type='button'
        className='group flex items-center gap-1 text-left text-xs'
        onClick={() => setOpen(true)}
      >
        <Music className='text-muted-foreground size-3' />
        <span className='text-foreground leading-snug group-hover:underline'>
          {t('Click to preview audio')}
        </span>
      </button>
      <AudioPreviewDialog
        open={open}
        onOpenChange={setOpen}
        clips={clips as AudioClip[]}
      />
    </>
  )
}

export function useTaskLogsColumns(isAdmin: boolean): ColumnDef<TaskLog>[] {
  const { t } = useTranslation()
  const columns: ColumnDef<TaskLog>[] = [
    {
      accessorKey: 'submit_time',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Submit Time')} />
      ),
      cell: ({ row }) => {
        const log = row.original
        const submitTime = row.getValue('submit_time') as number

        return (
          <div className='flex flex-col gap-0.5'>
            <span className='font-mono text-xs tabular-nums'>
              {formatTimestampToDate(submitTime, 'seconds')}
            </span>
            {log.finish_time ? (
              <span className='text-muted-foreground/60 font-mono text-[11px] tabular-nums'>
                {formatTimestampToDate(log.finish_time, 'seconds')}
              </span>
            ) : (
              <span className='text-muted-foreground/50 text-[11px]'>-</span>
            )}
          </div>
        )
      },
      meta: { label: t('Submit Time') },
    },
  ]

  if (isAdmin) {
    columns.push(createChannelColumn<TaskLog>({ headerLabel: t('Channel') }), {
      id: 'user',
      accessorFn: (row) => row.username || row.user_id,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('User')} />
      ),
      cell: function UserCell({ row }) {
        const { sensitiveVisible, setSelectedUserId, setUserInfoDialogOpen } =
          useUsageLogsContext()
        const log = row.original
        const displayName = log.username || String(log.user_id || '?')

        return (
          <button
            type='button'
            className='flex items-center gap-1.5 text-left'
            onClick={(e) => {
              e.stopPropagation()
              setSelectedUserId(log.user_id)
              setUserInfoDialogOpen(true)
            }}
          >
            <Avatar className='ring-border/60 size-6 ring-1 max-sm:hidden'>
              <AvatarFallback
                className={cn(
                  'text-[11px] font-semibold',
                  !sensitiveVisible && 'bg-muted text-muted-foreground'
                )}
                style={
                  sensitiveVisible ? getUserAvatarStyle(displayName) : undefined
                }
              >
                {sensitiveVisible ? getUserAvatarFallback(displayName) : '•'}
              </AvatarFallback>
            </Avatar>
            <span className='text-muted-foreground truncate text-sm hover:underline'>
              {sensitiveVisible ? displayName : '••••'}
            </span>
          </button>
        )
      },
      meta: { label: t('User') },
    })
  }

  columns.push(
    {
      accessorKey: 'task_id',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Task ID')} />
      ),
      cell: ({ row }) => {
        const log = row.original
        const taskId = row.getValue('task_id') as string
        if (!taskId) {
          return <span className='text-muted-foreground/60 text-xs'>-</span>
        }
        return (
          <div className='flex max-w-[170px] flex-col gap-0.5'>
            <StatusBadge
              label={taskId}
              autoColor={taskId}
              size='sm'
              className='border-border/60 bg-muted/30 max-w-full truncate rounded-md border px-1.5 py-0.5 font-mono'
            />
            <span className='text-muted-foreground/60 truncate text-[11px]'>
              {t(log.platform)} · {t(taskActionMapper.getLabel(log.action))}
            </span>
          </div>
        )
      },
      meta: { label: t('Task ID'), mobileTitle: true },
    },
    createDurationColumn<TaskLog>({
      submitTimeKey: 'submit_time',
      finishTimeKey: 'finish_time',
      unit: 'seconds',
      headerLabel: t('Duration'),
      warningThresholdSec: 300,
    }),
    {
      accessorKey: 'status',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Status')} />
      ),
      cell: ({ row }) => {
        const status = row.getValue('status') as string
        return (
          <StatusBadge
            label={t(taskStatusMapper.getLabel(status, status || 'Submitting'))}
            variant={taskStatusMapper.getVariant(status)}
            size='sm'
            copyable={false}
          />
        )
      },
      meta: { label: t('Status') },
    },
    createProgressColumn<TaskLog>({ headerLabel: t('Progress') }),
    {
      id: 'preview',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Preview')} />
      ),
      cell: ({ row }) => <TaskMediaPreviewCell log={row.original} />,
      meta: { label: t('Preview') },
      size: 90,
      maxSize: 110,
    },
    {
      accessorKey: 'fail_reason',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Details')} />
      ),
      cell: function DetailsCell({ row }) {
        const log = row.original
        const failReason = row.getValue('fail_reason') as string
        const status = log.status
        const [dialogOpen, setDialogOpen] = useState(false)

        const isSunoSuccess =
          log.platform === 'suno' && status === TASK_STATUS.SUCCESS
        if (isSunoSuccess) {
          const data = parseTaskData(log.data)
          if (
            data.some(
              (c) =>
                c &&
                typeof c === 'object' &&
                (c as Record<string, unknown>).audio_url
            )
          ) {
            return <AudioPreviewCell log={log} />
          }
        }

        const isUrl = failReason?.startsWith('http')

        if (!failReason || (status === TASK_STATUS.SUCCESS && isUrl)) {
          return <span className='text-muted-foreground/60 text-xs'>-</span>
        }

        return (
          <>
            <button
              type='button'
              className='group flex max-w-[200px] items-center gap-1 text-left text-xs'
              onClick={() => setDialogOpen(true)}
              title={t('Click to view full error message')}
            >
              <span className='truncate leading-snug text-red-600 group-hover:underline dark:text-red-400'>
                {failReason}
              </span>
            </button>
            <FailReasonDialog
              failReason={failReason}
              open={dialogOpen}
              onOpenChange={setDialogOpen}
            />
          </>
        )
      },
      meta: { label: t('Details') },
      size: 200,
      maxSize: 220,
    }
  )

  return columns
}
