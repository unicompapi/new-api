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

import type { SeedanceGenerationSettings } from './constants'

export type MediaContentItem =
  | {
      type: 'text'
      text: string
    }
  | {
      type: 'image_url'
      image_url: { url: string }
      role?: string
    }
  | {
      type: 'video_url'
      video_url: { url: string }
      role?: string
    }
  | {
      type: 'audio_url'
      audio_url: { url: string }
      role?: string
    }

export type VideoSubmitTool = {
  type: 'web_search'
}

export type VideoSubmitMetadata = {
  resolution?: string
  ratio?: string
  duration?: number
  generate_audio?: boolean
  tools?: VideoSubmitTool[]
  seed?: number
  camera_fixed?: boolean
  watermark?: boolean
  return_last_frame?: boolean
}

export type VideoSubmitPayload = {
  model: string
  group?: string
  content?: MediaContentItem[]
  metadata?: VideoSubmitMetadata
}

export type OpenAIVideoTask = {
  id: string
  object?: string
  model?: string
  status: string
  progress?: number
  created_at?: number
  completed_at?: number
  error?: {
    message?: string
    code?: string
  }
  metadata?: Record<string, unknown>
}

export type ModelOption = {
  label: string
  value: string
}

export type GroupOption = {
  label: string
  value: string
  ratio?: number
  desc?: string
}

export type ReferenceMediaUploadStatus = 'uploading' | 'ready' | 'error'

export type ReferenceMediaFile = {
  id: string
  file: File
  previewUrl: string
  kind: 'image' | 'video' | 'audio'
  role?: string
  uploadStatus: ReferenceMediaUploadStatus
  remoteUrl?: string
  uploadError?: string
}

export type ReferenceImageInput = {
  url: string
  role: string
}

export type SeedanceSubmitValues = {
  prompt: string
  settings: SeedanceGenerationSettings
  referenceImages: ReferenceImageInput[]
  referenceVideos?: string[]
  referenceAudios?: string[]
}

export type SeedanceHistoryEntry = {
  taskId: string
  prompt: string
  model: string
  createdAt: number
  status?: string
}
