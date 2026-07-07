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

import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getSeedanceModels, getUserGroups } from './api'
import { SeedanceForm } from './components/seedance-form'
import { SeedanceHistory } from './components/seedance-history'
import { SeedancePreview } from './components/seedance-preview'
import { SEEDANCE_MODELS } from './constants'
import { useSeedanceHistory } from './hooks/use-seedance-history'
import { useSeedanceGeneration } from './hooks/use-seedance-generation'
import { buildVideoSubmitPayload } from './lib/payload-builder'
import type { SeedanceSubmitValues } from './types'

export function Seedance() {
  const { t } = useTranslation()
  const [model, setModel] = useState<string>(SEEDANCE_MODELS[0])
  const [group, setGroup] = useState('default')
  const [viewTaskId, setViewTaskId] = useState<string | null>(null)

  const history = useSeedanceHistory()

  const {
    apiKeyQuery,
    submitMutation,
    taskId,
    task,
    playbackUrl,
    reset,
    isSubmitting,
    isPolling,
  } = useSeedanceGeneration({ viewTaskId })

  const { data: models = [], isLoading: isLoadingModels } = useQuery({
    queryKey: ['seedance-models'],
    queryFn: getSeedanceModels,
  })

  const { data: groups = [] } = useQuery({
    queryKey: ['seedance-groups'],
    queryFn: getUserGroups,
  })

  useEffect(() => {
    if (models.length === 0) return
    if (!models.some((item) => item.value === model)) {
      setModel(models[0].value)
    }
  }, [models, model])

  useEffect(() => {
    if (groups.length === 0) return
    if (!groups.some((item) => item.value === group)) {
      const fallback =
        groups.find((item) => item.value === 'default')?.value ??
        groups[0].value
      setGroup(fallback)
    }
  }, [groups, group])

  useEffect(() => {
    if (apiKeyQuery.isError) {
      toast.error(
        apiKeyQuery.error instanceof Error
          ? apiKeyQuery.error.message
          : t('Failed to load API key')
      )
    }
  }, [apiKeyQuery.isError, apiKeyQuery.error, t])

  useEffect(() => {
    if (!taskId || !task?.status) return
    history.updateStatus(taskId, task.status)
  }, [taskId, task?.status, history.updateStatus])

  const handleSubmit = async (values: {
    prompt: string
    settings: SeedanceSubmitValues['settings']
    referenceImages: Array<{ url: string; role: string }>
    referenceVideos: string[]
    referenceAudios: string[]
  }) => {
    if (apiKeyQuery.isLoading) {
      toast.error(t('Loading API key, please wait'))
      return
    }

    if (apiKeyQuery.isError || !apiKeyQuery.data) {
      toast.error(t('No enabled API key found'))
      return
    }

    try {
      reset()
      setViewTaskId(null)

      const payload = buildVideoSubmitPayload({
        model,
        prompt: values.prompt,
        group,
        settings: values.settings,
        referenceImages: values.referenceImages,
        referenceVideos:
          values.referenceVideos.length > 0 ? values.referenceVideos : undefined,
        referenceAudios:
          values.referenceAudios.length > 0 ? values.referenceAudios : undefined,
      })

      const result = await submitMutation.mutateAsync(payload)
      history.addEntry({
        taskId: result.id,
        prompt: values.prompt.trim(),
        model,
        createdAt: Date.now(),
        status: result.status,
      })
      toast.success(t('Video task submitted'))
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to submit video task')
      )
    }
  }

  const handleSelectHistory = (selectedId: string) => {
    reset()
    setViewTaskId(selectedId)
  }

  const handleRemoveHistory = (selectedId: string) => {
    history.removeEntry(selectedId)
    if (viewTaskId === selectedId) {
      setViewTaskId(null)
      reset()
    }
  }

  const handleClearHistory = () => {
    history.clearAll()
    setViewTaskId(null)
    reset()
  }

  const fallbackModels = SEEDANCE_MODELS.map((value) => ({
    label: value,
    value,
  }))

  const modelOptions = models.length > 0 ? models : fallbackModels
  const formDisabled =
    isSubmitting ||
    isPolling ||
    apiKeyQuery.isLoading ||
    isLoadingModels

  return (
    <div className='flex size-full min-h-0 flex-col overflow-hidden lg:flex-row'>
      <aside className='bg-background flex w-full shrink-0 flex-col border-b lg:w-[min(100%,480px)] lg:border-r lg:border-b-0 xl:w-[500px]'>
        <SeedanceForm
          models={modelOptions}
          groups={groups}
          model={model}
          group={group}
          onModelChange={setModel}
          onGroupChange={setGroup}
          disabled={formDisabled}
          onSubmit={handleSubmit}
        />
      </aside>

      <div className='flex min-h-[320px] flex-1 flex-col overflow-hidden lg:min-h-0'>
        <SeedancePreview
          taskId={taskId}
          task={task}
          playbackUrl={playbackUrl}
          isPolling={isPolling}
          isSubmitting={isSubmitting}
        />
        <SeedanceHistory
          entries={history.entries}
          activeTaskId={taskId}
          onSelect={handleSelectHistory}
          onRemove={handleRemoveHistory}
          onClear={handleClearHistory}
        />
      </div>
    </div>
  )
}
