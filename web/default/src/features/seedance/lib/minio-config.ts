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

export type MinioConfig = {
  endpoint: string
  bucket: string
  accessKey: string
  secretKey: string
  region: string
  publicBaseUrl: string
  /** Object retention in days; MinIO bucket ILM should match (see .env.example). */
  objectExpireDays: number
}

function parsePositiveInt(value: string | undefined, fallback: number) {
  if (!value?.trim()) return fallback
  const parsed = Number.parseInt(value, 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback
}

function trimTrailingSlash(value: string) {
  return value.replace(/\/+$/, '')
}

export function getMinioConfig(): MinioConfig | null {
  const endpoint = import.meta.env.VITE_MINIO_ENDPOINT?.trim()
  const bucket = import.meta.env.VITE_MINIO_BUCKET?.trim()
  const accessKey = import.meta.env.VITE_MINIO_ACCESS_KEY?.trim()
  const secretKey = import.meta.env.VITE_MINIO_SECRET_KEY?.trim()
  const region = import.meta.env.VITE_MINIO_REGION?.trim() || 'us-east-1'
  const publicBaseUrl =
    import.meta.env.VITE_MINIO_PUBLIC_BASE_URL?.trim() ||
    (endpoint && bucket ? `${trimTrailingSlash(endpoint)}/${bucket}` : '')
  const objectExpireDays = parsePositiveInt(
    import.meta.env.VITE_MINIO_OBJECT_EXPIRE_DAYS,
    7
  )

  if (!endpoint || !bucket || !accessKey || !secretKey || !publicBaseUrl) {
    return null
  }

  return {
    endpoint: trimTrailingSlash(endpoint),
    bucket,
    accessKey,
    secretKey,
    region,
    publicBaseUrl: trimTrailingSlash(publicBaseUrl),
    objectExpireDays,
  }
}

export function isMinioUploadEnabled() {
  return getMinioConfig() !== null
}
