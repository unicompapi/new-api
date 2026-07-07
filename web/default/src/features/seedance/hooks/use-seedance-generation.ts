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

import { useMutation, useQuery } from '@tanstack/react-query'
import { fetchActiveChatKey } from '@/features/chat/hooks/use-active-chat-key'
import { fetchVideoTask, submitVideoTask } from '../api'
import { POLL_INTERVAL_MS, VIDEO_STATUS } from '../constants'
import type { OpenAIVideoTask, VideoSubmitPayload } from '../types'

export function isTerminalVideoStatus(status?: string): boolean {
  return (
    status === VIDEO_STATUS.COMPLETED ||
    status === VIDEO_STATUS.FAILED
  )
}

export function getVideoPlaybackUrl(
  task: OpenAIVideoTask | undefined,
  taskId: string | null
): string | null {
  if (!task || !taskId) return null

  const metadataUrl = task.metadata?.url
  if (typeof metadataUrl === 'string' && metadataUrl.trim() !== '') {
    return metadataUrl
  }

  if (task.status === VIDEO_STATUS.COMPLETED) {
    return `/v1/videos/${taskId}/content`
  }

  return null
}

type UseSeedanceGenerationOptions = {
  /** Task selected from local history (when no active submission). */
  viewTaskId?: string | null
}

export function useSeedanceGeneration(options: UseSeedanceGenerationOptions = {}) {
  const viewTaskId = options.viewTaskId ?? null

  const apiKeyQuery = useQuery({
    queryKey: ['seedance-api-key'],
    queryFn: fetchActiveChatKey,
    staleTime: 5 * 60 * 1000,
  })

  const submitMutation = useMutation({
    mutationFn: async (payload: VideoSubmitPayload) => {
      const apiKey = apiKeyQuery.data
      if (!apiKey) {
        throw new Error('API key is not ready')
      }
      return submitVideoTask(apiKey, payload)
    },
  })

  const submittedTaskId = submitMutation.data?.id ?? null
  const activeTaskId = submittedTaskId ?? viewTaskId

  const taskQuery = useQuery({
    queryKey: ['seedance-video-task', activeTaskId, apiKeyQuery.data],
    queryFn: async () => {
      const apiKey = apiKeyQuery.data
      if (!apiKey || !activeTaskId) {
        throw new Error('Missing task context')
      }
      return fetchVideoTask(apiKey, activeTaskId)
    },
    enabled: Boolean(activeTaskId && apiKeyQuery.data),
    refetchInterval: (query) => {
      const status = query.state.data?.status
      if (isTerminalVideoStatus(status)) {
        return false
      }
      return POLL_INTERVAL_MS
    },
  })

  const reset = () => {
    submitMutation.reset()
  }

  return {
    apiKeyQuery,
    submitMutation,
    taskQuery,
    taskId: activeTaskId,
    task: taskQuery.data,
    playbackUrl: getVideoPlaybackUrl(taskQuery.data, activeTaskId),
    reset,
    isSubmitting: submitMutation.isPending,
    isPolling:
      Boolean(activeTaskId) &&
      !isTerminalVideoStatus(taskQuery.data?.status) &&
      !taskQuery.isError,
  }
}
