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

import {
  DURATION_AUTO,
  SEEDANCE_AUDIO_ROLE,
  SEEDANCE_VIDEO_ROLE,
} from '../constants'
import type {
  MediaContentItem,
  SeedanceSubmitValues,
  VideoSubmitMetadata,
  VideoSubmitPayload,
} from '../types'

/**
 * Build payload aligned with Volcengine Seedance 2.0 API:
 * https://www.volcengine.com/docs/82379/1520757?lang=zh
 *
 * {
 *   "model": "...",
 *   "content": [
 *     { "type": "text", "text": "..." },
 *     { "type": "image_url", ... }
 *   ],
 *   "metadata": { "generate_audio": true, "ratio": "16:9", ... }
 * }
 */
export function buildVideoSubmitPayload(
  params: SeedanceSubmitValues & {
    model: string
    group?: string
  }
): VideoSubmitPayload {
  const promptText = params.prompt.trim()
  const content: MediaContentItem[] = []

  if (promptText) {
    content.push({
      type: 'text',
      text: promptText,
    })
  }

  content.push(
    ...params.referenceImages.map((item) => ({
      type: 'image_url' as const,
      image_url: { url: item.url },
      role: item.role,
    }))
  )

  if (params.referenceVideos) {
    for (const url of params.referenceVideos) {
      content.push({
        type: 'video_url',
        video_url: { url },
        role: SEEDANCE_VIDEO_ROLE,
      })
    }
  }

  if (params.referenceAudios) {
    for (const url of params.referenceAudios) {
      content.push({
        type: 'audio_url',
        audio_url: { url },
        role: SEEDANCE_AUDIO_ROLE,
      })
    }
  }

  const { settings } = params
  const metadata: VideoSubmitMetadata = {
    generate_audio: settings.generateAudio,
    ratio: settings.ratio,
    watermark: settings.watermark,
  }

  if (settings.duration !== DURATION_AUTO) {
    metadata.duration = settings.duration
  }

  if (settings.resolution) {
    metadata.resolution = settings.resolution
  }

  if (settings.cameraFixed) {
    metadata.camera_fixed = settings.cameraFixed
  }

  if (settings.returnLastFrame) {
    metadata.return_last_frame = settings.returnLastFrame
  }

  if (settings.seed !== undefined) {
    metadata.seed = settings.seed
  }

  if (settings.webSearch) {
    metadata.tools = [{ type: 'web_search' }]
  }

  const payload: VideoSubmitPayload = {
    model: params.model,
    metadata,
  }

  if (params.group) {
    payload.group = params.group
  }

  if (content.length > 0) {
    payload.content = content
  }

  return payload
}
