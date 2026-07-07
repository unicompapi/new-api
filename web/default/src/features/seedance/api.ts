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

import { api, getCommonHeaders } from '@/lib/api'
import { VIDEO_API } from './constants'
import type {
  GroupOption,
  ModelOption,
  OpenAIVideoTask,
  VideoSubmitPayload,
} from './types'

async function parseRelayError(res: Response): Promise<string> {
  try {
    const body = await res.json()
    if (body?.error?.message) return String(body.error.message)
    if (body?.message) return String(body.message)
  } catch {
    /* empty */
  }
  return `HTTP ${res.status}`
}

function relayHeaders(apiKey: string): Record<string, string> {
  return {
    ...getCommonHeaders(),
    Authorization: `Bearer ${apiKey}`,
  }
}

export async function submitVideoTask(
  apiKey: string,
  payload: VideoSubmitPayload
): Promise<OpenAIVideoTask> {
  const res = await fetch(VIDEO_API.SUBMIT, {
    method: 'POST',
    headers: relayHeaders(apiKey),
    credentials: 'include',
    body: JSON.stringify(payload),
  })

  if (!res.ok) {
    throw new Error(await parseRelayError(res))
  }

  return res.json() as Promise<OpenAIVideoTask>
}

export async function fetchVideoTask(
  apiKey: string,
  taskId: string
): Promise<OpenAIVideoTask> {
  const res = await fetch(VIDEO_API.TASK(taskId), {
    headers: relayHeaders(apiKey),
    credentials: 'include',
  })

  if (!res.ok) {
    throw new Error(await parseRelayError(res))
  }

  return res.json() as Promise<OpenAIVideoTask>
}

export async function getSeedanceModels(): Promise<ModelOption[]> {
  const res = await api.get('/api/user/models')
  const { data } = res

  if (!data.success || !Array.isArray(data.data)) {
    return []
  }

  return (data.data as string[])
    .filter((model) => model.includes('seedance-2-0'))
    .map((model) => ({
      label: model,
      value: model,
    }))
}

export async function getUserGroups(): Promise<GroupOption[]> {
  const res = await api.get('/api/user/self/groups')
  const { data } = res

  if (!data.success || !data.data) {
    return []
  }

  const groupData = data.data as Record<
    string,
    { desc: string; ratio: number | string }
  >

  return Object.entries(groupData).map(([group, info]) => ({
    label: group,
    value: group,
    ratio: Number(info.ratio),
    desc: info.desc,
  }))
}
