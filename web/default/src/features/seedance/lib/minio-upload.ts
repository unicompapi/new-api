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

import { PutObjectCommand, S3Client } from '@aws-sdk/client-s3'
import { nanoid } from 'nanoid'
import { getMinioConfig } from './minio-config'

function sanitizeFilename(name: string) {
  const base = name.split(/[/\\]/).pop() ?? 'file'
  return base.replace(/[^\w.\-()+]/g, '_') || 'file'
}

function buildObjectKey(file: File) {
  const date = new Date()
  const prefix = [
    date.getFullYear(),
    String(date.getMonth() + 1).padStart(2, '0'),
    String(date.getDate()).padStart(2, '0'),
  ].join('/')
  return `seedance/${prefix}/${nanoid()}-${sanitizeFilename(file.name)}`
}

function buildPublicUrl(publicBaseUrl: string, objectKey: string) {
  const encodedKey = objectKey
    .split('/')
    .map((part) => encodeURIComponent(part))
    .join('/')
  return `${publicBaseUrl}/${encodedKey}`
}

let cachedClient: S3Client | null = null
let cachedClientKey = ''

function getS3Client(config: NonNullable<ReturnType<typeof getMinioConfig>>) {
  const cacheKey = `${config.endpoint}:${config.region}:${config.accessKey}`
  if (cachedClient && cachedClientKey === cacheKey) {
    return cachedClient
  }

  cachedClient = new S3Client({
    endpoint: config.endpoint,
    region: config.region,
    credentials: {
      accessKeyId: config.accessKey,
      secretAccessKey: config.secretKey,
    },
    forcePathStyle: true,
  })
  cachedClientKey = cacheKey
  return cachedClient
}

function buildObjectExpiration(config: NonNullable<ReturnType<typeof getMinioConfig>>) {
  const days = config.objectExpireDays
  const expiresAt = new Date(Date.now() + days * 86_400_000)
  const maxAgeSeconds = days * 86_400

  return {
    expiresAt,
    maxAgeSeconds,
    metadata: {
      'expire-after-days': String(days),
      'expire-at': expiresAt.toISOString(),
    },
    // Tag for MinIO/S3 lifecycle rules that filter by tag (optional).
    tagging: `expire-days=${days}`,
  }
}

export async function uploadFileToMinio(file: File): Promise<string> {
  const config = getMinioConfig()
  if (!config) {
    throw new Error('MinIO is not configured')
  }

  const objectKey = buildObjectKey(file)
  const client = getS3Client(config)
  const expiration = buildObjectExpiration(config)
  // AWS SDK expects ReadableStream in Node; in browser, File/Blob lacks getReader().
  const body = new Uint8Array(await file.arrayBuffer())

  try {
    await client.send(
      new PutObjectCommand({
        Bucket: config.bucket,
        Key: objectKey,
        Body: body,
        ContentType: file.type || 'application/octet-stream',
        Expires: expiration.expiresAt,
        CacheControl: `max-age=${expiration.maxAgeSeconds}`,
        Metadata: expiration.metadata,
        Tagging: expiration.tagging,
      })
    )
  } catch (error) {
    const message =
      error instanceof Error ? error.message : 'MinIO upload failed'
    throw new Error(
      `${message}. Check MinIO endpoint, bucket policy, and CORS settings.`
    )
  }

  return buildPublicUrl(config.publicBaseUrl, objectKey)
}

export async function uploadFilesToMinio(files: File[]): Promise<string[]> {
  return Promise.all(files.map((file) => uploadFileToMinio(file)))
}
