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

import { ClockIcon, Trash2Icon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { VIDEO_STATUS } from '../constants'
import type { SeedanceHistoryEntry } from '../types'

type SeedanceHistoryProps = {
  entries: SeedanceHistoryEntry[]
  activeTaskId: string | null
  onSelect: (taskId: string) => void
  onRemove: (taskId: string) => void
  onClear: () => void
}

function historyStatusVariant(status?: string) {
  switch (status) {
    case VIDEO_STATUS.COMPLETED:
      return 'success' as const
    case VIDEO_STATUS.FAILED:
      return 'danger' as const
    case VIDEO_STATUS.IN_PROGRESS:
      return 'info' as const
    case VIDEO_STATUS.QUEUED:
      return 'warning' as const
    default:
      return 'neutral' as const
  }
}

function historyStatusLabel(status?: string) {
  switch (status) {
    case VIDEO_STATUS.COMPLETED:
      return 'Completed'
    case VIDEO_STATUS.FAILED:
      return 'Failed'
    case VIDEO_STATUS.IN_PROGRESS:
      return 'In progress'
    case VIDEO_STATUS.QUEUED:
      return 'Queued'
    default:
      return 'Submitted'
  }
}

function formatTime(timestamp: number) {
  return new Date(timestamp).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function SeedanceHistory(props: SeedanceHistoryProps) {
  const { t } = useTranslation()

  return (
    <div className='bg-background shrink-0 border-t'>
      <div className='flex items-center justify-between gap-2 px-4 py-2.5 sm:px-6'>
        <div className='flex items-center gap-2'>
          <ClockIcon className='text-muted-foreground size-4' />
          <h3 className='text-sm font-medium'>{t('Local history')}</h3>
          <span className='text-muted-foreground text-xs'>
            ({props.entries.length})
          </span>
        </div>
        {props.entries.length > 0 ? (
          <Button
            type='button'
            variant='ghost'
            size='sm'
            className='text-muted-foreground h-8 text-xs'
            onClick={props.onClear}
          >
            {t('Clear all')}
          </Button>
        ) : null}
      </div>

      {props.entries.length === 0 ? (
        <p className='text-muted-foreground px-4 pb-4 text-xs sm:px-6'>
          {t('Generated videos on this device will appear here.')}
        </p>
      ) : (
        <ul className='max-h-44 overflow-y-auto px-2 pb-2 sm:px-4 sm:pb-3'>
          {props.entries.map((entry) => {
            const isActive = entry.taskId === props.activeTaskId
            return (
              <li key={entry.taskId}>
                <div
                  className={`hover:bg-muted/60 flex items-start gap-2 rounded-lg border px-2.5 py-2 transition-colors ${
                    isActive
                      ? 'border-primary/40 bg-primary/5'
                      : 'border-transparent'
                  }`}
                >
                  <button
                    type='button'
                    className='min-w-0 flex-1 text-left'
                    onClick={() => props.onSelect(entry.taskId)}
                  >
                    <p className='line-clamp-2 text-sm leading-snug'>
                      {entry.prompt || t('Untitled video task')}
                    </p>
                    <div className='mt-1.5 flex flex-wrap items-center gap-2'>
                      <span className='text-muted-foreground font-mono text-[11px]'>
                        {entry.taskId}
                      </span>
                      <StatusBadge
                        label={t(historyStatusLabel(entry.status))}
                        variant={historyStatusVariant(entry.status)}
                        size='sm'
                        copyable={false}
                      />
                    </div>
                    <p className='text-muted-foreground mt-1 text-[11px]'>
                      {entry.model} · {formatTime(entry.createdAt)}
                    </p>
                  </button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-sm'
                    className='text-muted-foreground shrink-0'
                    onClick={() => props.onRemove(entry.taskId)}
                    aria-label={t('Remove from history')}
                  >
                    <Trash2Icon className='size-3.5' />
                  </Button>
                </div>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
