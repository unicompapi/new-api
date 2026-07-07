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

export const SEEDANCE_MODELS = [
  'doubao-seedance-2-0-260128',
  'doubao-seedance-2-0-fast-260128',
] as const

export const VIDEO_API = {
  SUBMIT: '/v1/videos',
  TASK: (taskId: string) => `/v1/videos/${taskId}`,
  CONTENT: (taskId: string) => `/v1/videos/${taskId}/content`,
} as const

export const VIDEO_STATUS = {
  QUEUED: 'queued',
  IN_PROGRESS: 'in_progress',
  COMPLETED: 'completed',
  FAILED: 'failed',
} as const

export const POLL_INTERVAL_MS = 3000

export const SEEDANCE_HISTORY_STORAGE_KEY = 'seedance:history:v1'
export const SEEDANCE_HISTORY_MAX_ITEMS = 50

export const SEEDANCE_MODEL_PREFIX = 'doubao-seedance-2-0'

/** Volcengine Seedance 2.0 supported resolutions */
export const SEEDANCE_RESOLUTIONS = ['480p', '720p', '1080p'] as const

/** Volcengine Seedance 2.0 supported aspect ratios */
export const SEEDANCE_RATIOS = [
  '16:9',
  '4:3',
  '1:1',
  '3:4',
  '9:16',
  '21:9',
  'adaptive',
] as const

export const SEEDANCE_IMAGE_ROLES = [
  'reference_image',
  'first_frame',
  'last_frame',
] as const

export const SEEDANCE_VIDEO_ROLE = 'reference_video'
export const SEEDANCE_AUDIO_ROLE = 'reference_audio'

/** Max reference media counts per Volcengine Seedance 2.0 API */
export const SEEDANCE_MAX_REFERENCE_IMAGES = 9
export const SEEDANCE_MAX_REFERENCE_VIDEOS = 3
export const SEEDANCE_MAX_REFERENCE_AUDIOS = 3

export const DURATION_AUTO = -1

export type SeedanceResolution = (typeof SEEDANCE_RESOLUTIONS)[number]
export type SeedanceRatio = (typeof SEEDANCE_RATIOS)[number]
export type SeedanceImageRole = (typeof SEEDANCE_IMAGE_ROLES)[number]

export type SeedanceGenerationSettings = {
  resolution: SeedanceResolution
  ratio: SeedanceRatio
  duration: number
  generateAudio: boolean
  webSearch: boolean
  cameraFixed: boolean
  watermark: boolean
  returnLastFrame: boolean
  seed?: number
}

export const DEFAULT_GENERATION_SETTINGS: SeedanceGenerationSettings = {
  resolution: '720p',
  ratio: '16:9',
  duration: 5,
  generateAudio: false,
  webSearch: false,
  cameraFixed: false,
  watermark: false,
  returnLastFrame: false,
}

export function getDurationRange(model: string) {
  const isFast = model.includes('fast')
  return {
    min: 4,
    max: isFast ? 12 : 15,
    supportsAuto: true,
  }
}

export function clampDuration(model: string, duration: number) {
  if (duration === DURATION_AUTO) return DURATION_AUTO
  const { min, max } = getDurationRange(model)
  return Math.min(max, Math.max(min, duration))
}
