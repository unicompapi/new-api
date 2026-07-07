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

import { useEffect, useRef, useState } from 'react'
import { ImageIcon, Loader2Icon, MusicIcon, Trash2Icon, VideoIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { nanoid } from 'nanoid'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { ModelGroupSelector } from '@/components/model-group-selector'
import {
  DEFAULT_GENERATION_SETTINGS,
  DURATION_AUTO,
  SEEDANCE_AUDIO_ROLE,
  SEEDANCE_IMAGE_ROLES,
  SEEDANCE_MAX_REFERENCE_AUDIOS,
  SEEDANCE_MAX_REFERENCE_IMAGES,
  SEEDANCE_MAX_REFERENCE_VIDEOS,
  SEEDANCE_RATIOS,
  SEEDANCE_RESOLUTIONS,
  SEEDANCE_VIDEO_ROLE,
  clampDuration,
  getDurationRange,
  type SeedanceGenerationSettings,
  type SeedanceImageRole,
} from '../constants'
import type { GroupOption, ModelOption, ReferenceMediaFile } from '../types'
import { isMinioUploadEnabled } from '../lib/minio-config'
import { uploadFileToMinio } from '../lib/minio-upload'

function revokeBlobPreviewUrl(url: string) {
  if (url.startsWith('blob:')) {
    URL.revokeObjectURL(url)
  }
}

function getReferenceImagePreviewSrc(item: ReferenceMediaFile) {
  return item.remoteUrl ?? item.previewUrl
}

type SeedanceFormProps = {
  models: ModelOption[]
  groups: GroupOption[]
  model: string
  group: string
  onModelChange: (value: string) => void
  onGroupChange: (value: string) => void
  disabled?: boolean
  onSubmit: (values: {
    prompt: string
    settings: SeedanceGenerationSettings
    referenceImages: Array<{ url: string; role: SeedanceImageRole }>
    referenceVideos: string[]
    referenceAudios: string[]
  }) => void
}

function MediaUploadField(props: {
  id: string
  label: string
  accept: string
  multiple?: boolean
  appendOneAtATime?: boolean
  uploadLabel?: string
  maxFiles?: number
  kind: ReferenceMediaFile['kind']
  icon: React.ReactNode
  hint: string
  disabled?: boolean
  files: ReferenceMediaFile[]
  onChange: (files: ReferenceMediaFile[]) => void
  onPatch: (id: string, patch: Partial<ReferenceMediaFile>) => void
  showImageRole?: boolean
}) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement>(null)
  const maxFiles = props.maxFiles
  const atMax = maxFiles !== undefined && props.files.length >= maxFiles
  const isUploading = props.files.some((item) => item.uploadStatus === 'uploading')

  const handleSelect = async (fileList: FileList | null) => {
    if (!fileList || fileList.length === 0) return
    if (atMax) return

    const file = fileList[0]
    if (!file) return

    if (!isMinioUploadEnabled()) {
      toast.error(t('MinIO upload is not configured'))
      return
    }

    const defaultRole =
      props.kind === 'image'
        ? 'reference_image'
        : props.kind === 'video'
          ? SEEDANCE_VIDEO_ROLE
          : SEEDANCE_AUDIO_ROLE

    const itemId = nanoid()
    const nextItem: ReferenceMediaFile = {
      id: itemId,
      file,
      previewUrl: URL.createObjectURL(file),
      kind: props.kind,
      role: defaultRole,
      uploadStatus: 'uploading',
    }

    if (props.appendOneAtATime || props.maxFiles !== undefined) {
      props.onChange([...props.files, nextItem].slice(0, maxFiles))
    } else if (props.multiple) {
      const next = Array.from(fileList).map((item) => ({
        id: nanoid(),
        file: item,
        previewUrl: URL.createObjectURL(item),
        kind: props.kind,
        role: defaultRole,
        uploadStatus: 'uploading' as const,
      }))
      props.onChange([...props.files, ...next])
    } else {
      props.onChange([nextItem])
    }

    try {
      const remoteUrl = await uploadFileToMinio(file)
      revokeBlobPreviewUrl(nextItem.previewUrl)
      props.onPatch(itemId, {
        uploadStatus: 'ready',
        remoteUrl,
        previewUrl: remoteUrl,
        uploadError: undefined,
      })
    } catch (error) {
      const message =
        error instanceof Error ? error.message : t('MinIO upload failed')
      props.onPatch(itemId, {
        uploadStatus: 'error',
        uploadError: message,
      })
      toast.error(message)
    }
  }

  const removeFile = (id: string) => {
    props.onChange(
      props.files.filter((item) => {
        if (item.id === id) {
          revokeBlobPreviewUrl(item.previewUrl)
        }
        return item.id !== id
      })
    )
  }

  const updateRole = (id: string, role: string) => {
    props.onChange(
      props.files.map((item) => (item.id === id ? { ...item, role } : item))
    )
  }

  return (
    <div className='space-y-2'>
      <Label htmlFor={props.id}>{props.label}</Label>
      <p className='text-muted-foreground text-xs'>
        {props.hint}
        {maxFiles !== undefined
          ? ` (${props.files.length}/${maxFiles})`
          : null}
      </p>
      <div className='flex flex-wrap items-center gap-2'>
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={props.disabled || atMax || isUploading}
          onClick={() => inputRef.current?.click()}
        >
          {props.icon}
          {props.uploadLabel ?? t('Upload')}
        </Button>
        <Input
          ref={inputRef}
          id={props.id}
          type='file'
          accept={props.accept}
          multiple={props.multiple && !props.appendOneAtATime && maxFiles === undefined}
          disabled={props.disabled || atMax || isUploading}
          className='hidden'
          onChange={(event) => {
            void handleSelect(event.target.files)
            event.target.value = ''
          }}
        />
      </div>
      {props.files.length > 0 ? (
        <ul className='space-y-2'>
          {props.files.map((item) => (
            <li
              key={item.id}
              className='bg-muted/40 space-y-2 rounded-lg border px-3 py-2'
            >
              <div className='flex items-center justify-between gap-3'>
                <div className='flex min-w-0 items-center gap-3'>
                  {item.kind === 'image' ? (
                    <img
                      src={getReferenceImagePreviewSrc(item)}
                      alt={item.file.name}
                      className='size-10 rounded object-cover'
                    />
                  ) : (
                    <div className='bg-background flex size-10 items-center justify-center rounded border'>
                      {item.kind === 'video' ? (
                        <VideoIcon className='text-muted-foreground size-4' />
                      ) : (
                        <MusicIcon className='text-muted-foreground size-4' />
                      )}
                    </div>
                  )}
                  <div className='min-w-0'>
                    <p className='truncate text-sm font-medium'>
                      {item.file.name}
                    </p>
                    <p className='text-muted-foreground text-xs'>
                      {(item.file.size / 1024 / 1024).toFixed(2)} MB
                      {item.uploadStatus === 'uploading'
                        ? ` · ${t('Uploading to MinIO...')}`
                        : null}
                      {item.uploadStatus === 'ready' && item.remoteUrl
                        ? ` · ${t('Uploaded')}`
                        : null}
                    </p>
                    {item.uploadStatus === 'error' && item.uploadError ? (
                      <p className='text-destructive text-xs'>{item.uploadError}</p>
                    ) : null}
                  </div>
                </div>
                <div className='flex shrink-0 items-center gap-1'>
                  {item.uploadStatus === 'uploading' ? (
                    <Loader2Icon className='text-muted-foreground size-4 animate-spin' />
                  ) : null}
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-sm'
                  disabled={props.disabled}
                  onClick={() => removeFile(item.id)}
                >
                  <Trash2Icon className='size-4' />
                </Button>
                </div>
              </div>
              {props.showImageRole && item.kind === 'image' ? (
                <Select
                  value={item.role ?? 'reference_image'}
                  onValueChange={(value) => {
                    if (value) updateRole(item.id, value)
                  }}
                  disabled={props.disabled}
                >
                  <SelectTrigger className='h-8 w-full'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    {SEEDANCE_IMAGE_ROLES.map((role) => (
                      <SelectItem key={role} value={role}>
                        {t(`Image role: ${role}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}

function SettingSwitch(props: {
  id: string
  label: string
  hint: string
  checked: boolean
  disabled?: boolean
  onCheckedChange: (checked: boolean) => void
}) {
  return (
    <div className='flex items-start justify-between gap-3 rounded-lg border px-3 py-2.5'>
      <div className='space-y-0.5'>
        <Label htmlFor={props.id} className='text-sm'>
          {props.label}
        </Label>
        <p className='text-muted-foreground text-xs'>{props.hint}</p>
      </div>
      <Switch
        id={props.id}
        checked={props.checked}
        disabled={props.disabled}
        onCheckedChange={props.onCheckedChange}
      />
    </div>
  )
}

export function SeedanceForm(props: SeedanceFormProps) {
  const { t } = useTranslation()
  const [prompt, setPrompt] = useState('')
  const [settings, setSettings] = useState<SeedanceGenerationSettings>({
    ...DEFAULT_GENERATION_SETTINGS,
  })
  const [autoDuration, setAutoDuration] = useState(false)
  const [referenceImages, setReferenceImages] = useState<ReferenceMediaFile[]>(
    []
  )
  const [referenceVideo, setReferenceVideo] = useState<ReferenceMediaFile[]>([])
  const [referenceAudio, setReferenceAudio] = useState<ReferenceMediaFile[]>([])

  const durationRange = getDurationRange(props.model)

  useEffect(() => {
    if (autoDuration) return
    setSettings((current) => ({
      ...current,
      duration: clampDuration(props.model, current.duration),
    }))
  }, [props.model, autoDuration])

  useEffect(() => {
    return () => {
      for (const item of [
        ...referenceImages,
        ...referenceVideo,
        ...referenceAudio,
      ]) {
        revokeBlobPreviewUrl(item.previewUrl)
      }
    }
  }, [referenceImages, referenceVideo, referenceAudio])

  const updateSettings = <K extends keyof SeedanceGenerationSettings>(
    key: K,
    value: SeedanceGenerationSettings[K]
  ) => {
    setSettings((current) => ({ ...current, [key]: value }))
  }

  const patchReferenceImages = (
    id: string,
    patch: Partial<ReferenceMediaFile>
  ) => {
    setReferenceImages((current) =>
      current.map((item) => (item.id === id ? { ...item, ...patch } : item))
    )
  }

  const patchReferenceVideo = (
    id: string,
    patch: Partial<ReferenceMediaFile>
  ) => {
    setReferenceVideo((current) =>
      current.map((item) => (item.id === id ? { ...item, ...patch } : item))
    )
  }

  const patchReferenceAudio = (
    id: string,
    patch: Partial<ReferenceMediaFile>
  ) => {
    setReferenceAudio((current) =>
      current.map((item) => (item.id === id ? { ...item, ...patch } : item))
    )
  }

  const allReferenceMedia = [
    ...referenceImages,
    ...referenceVideo,
    ...referenceAudio,
  ]
  const hasMediaUploading = allReferenceMedia.some(
    (item) => item.uploadStatus === 'uploading'
  )
  const hasMediaUploadError = allReferenceMedia.some(
    (item) => item.uploadStatus === 'error'
  )

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault()
    if (hasMediaUploading) {
      toast.error(t('Please wait for MinIO uploads to finish'))
      return
    }
    if (hasMediaUploadError) {
      toast.error(t('Remove failed uploads or retry before generating'))
      return
    }
    props.onSubmit({
      prompt,
      settings: {
        ...settings,
        duration: autoDuration ? DURATION_AUTO : settings.duration,
        seed:
          settings.seed === undefined || Number.isNaN(settings.seed)
            ? undefined
            : settings.seed,
      },
      referenceImages: referenceImages
        .filter((item) => item.remoteUrl)
        .map((item) => ({
          url: item.remoteUrl!,
          role: (item.role ?? 'reference_image') as SeedanceImageRole,
        })),
      referenceVideos: referenceVideo
        .map((item) => item.remoteUrl)
        .filter((url): url is string => Boolean(url)),
      referenceAudios: referenceAudio
        .map((item) => item.remoteUrl)
        .filter((url): url is string => Boolean(url)),
    })
  }

  const isDisabled = props.disabled || props.models.length === 0
  const hasReferenceMedia =
    referenceImages.length > 0 ||
    referenceVideo.length > 0 ||
    referenceAudio.length > 0

  useEffect(() => {
    if (!hasReferenceMedia) return
    setSettings((current) =>
      current.webSearch ? { ...current, webSearch: false } : current
    )
  }, [hasReferenceMedia])

  return (
    <form className='flex h-full min-h-0 flex-col' onSubmit={handleSubmit}>
      <div className='min-h-0 flex-1 space-y-5 overflow-y-auto px-4 py-4 sm:px-5 sm:py-5'>
        <div>
          <h2 className='text-sm font-semibold'>{t('Seedance 2.0')}</h2>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t(
              'Parameters aligned with Volcengine Ark Seedance 2.0 video generation API.'
            )}
          </p>
        </div>

        <div className='flex flex-wrap items-center gap-2'>
          <ModelGroupSelector
            selectedModel={props.model}
            models={props.models}
            onModelChange={props.onModelChange}
            selectedGroup={props.group}
            groups={props.groups}
            onGroupChange={props.onGroupChange}
            disabled={isDisabled}
          />
        </div>

        <div className='space-y-2'>
          <Label htmlFor='seedance-prompt'>{t('Prompt')}</Label>
          <Textarea
            id='seedance-prompt'
            value={prompt}
            onChange={(event) => setPrompt(event.target.value)}
            placeholder={t('Describe the video you want to generate...')}
            rows={5}
            disabled={isDisabled}
            required
            className='min-h-[120px] resize-none'
          />
        </div>

        <div className='space-y-3'>
          <Label>{t('Generation settings')}</Label>

          <div className='grid gap-3 sm:grid-cols-2'>
            <div className='space-y-2'>
              <Label htmlFor='seedance-resolution'>{t('Resolution')}</Label>
              <Select
                value={settings.resolution}
                onValueChange={(value) =>
                  updateSettings(
                    'resolution',
                    value as SeedanceGenerationSettings['resolution']
                  )
                }
                disabled={isDisabled}
              >
                <SelectTrigger id='seedance-resolution' className='w-full'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  {SEEDANCE_RESOLUTIONS.map((item) => (
                    <SelectItem key={item} value={item}>
                      {item}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className='space-y-2'>
              <Label htmlFor='seedance-ratio'>{t('Aspect ratio')}</Label>
              <Select
                value={settings.ratio}
                onValueChange={(value) =>
                  updateSettings(
                    'ratio',
                    value as SeedanceGenerationSettings['ratio']
                  )
                }
                disabled={isDisabled}
              >
                <SelectTrigger id='seedance-ratio' className='w-full'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  {SEEDANCE_RATIOS.map((item) => (
                    <SelectItem key={item} value={item}>
                      {item}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className='space-y-2'>
            <div className='flex items-center justify-between gap-3'>
              <Label htmlFor='seedance-duration'>{t('Duration (seconds)')}</Label>
              <div className='flex items-center gap-2'>
                <Label htmlFor='seedance-auto-duration' className='text-xs'>
                  {t('Auto duration')}
                </Label>
                <Switch
                  id='seedance-auto-duration'
                  checked={autoDuration}
                  disabled={isDisabled}
                  onCheckedChange={setAutoDuration}
                />
              </div>
            </div>
            <Input
              id='seedance-duration'
              type='number'
              min={durationRange.min}
              max={durationRange.max}
              value={settings.duration}
              disabled={isDisabled || autoDuration}
              onChange={(event) =>
                updateSettings(
                  'duration',
                  clampDuration(props.model, Number(event.target.value))
                )
              }
            />
            <p className='text-muted-foreground text-xs'>
              {t('Duration range: {{min}}-{{max}} seconds', {
                min: durationRange.min,
                max: durationRange.max,
              })}
            </p>
          </div>

          <div className='space-y-2'>
            <Label htmlFor='seedance-seed'>{t('Seed')}</Label>
            <Input
              id='seedance-seed'
              type='number'
              placeholder={t('Optional random seed')}
              value={settings.seed ?? ''}
              disabled={isDisabled}
              onChange={(event) => {
                const raw = event.target.value.trim()
                updateSettings(
                  'seed',
                  raw === '' ? undefined : Number(raw)
                )
              }}
            />
          </div>

          <div className='space-y-2'>
            <SettingSwitch
              id='seedance-generate-audio'
              label={t('Generate audio')}
              hint={t('Generate synchronized audio with the video')}
              checked={settings.generateAudio}
              disabled={isDisabled}
              onCheckedChange={(checked) =>
                updateSettings('generateAudio', checked)
              }
            />
            <SettingSwitch
              id='seedance-web-search'
              label={t('Web search')}
              hint={t(
                'Allow the model to search the web for timely content. May increase latency. Only for text-only prompts without reference media.'
              )}
              checked={settings.webSearch}
              disabled={isDisabled || hasReferenceMedia}
              onCheckedChange={(checked) =>
                updateSettings('webSearch', checked)
              }
            />
            <SettingSwitch
              id='seedance-camera-fixed'
              label={t('Camera fixed')}
              hint={t('Keep the camera position fixed during generation')}
              checked={settings.cameraFixed}
              disabled={isDisabled}
              onCheckedChange={(checked) =>
                updateSettings('cameraFixed', checked)
              }
            />
            <SettingSwitch
              id='seedance-watermark'
              label={t('Watermark')}
              hint={t('Add watermark to the generated video')}
              checked={settings.watermark}
              disabled={isDisabled}
              onCheckedChange={(checked) =>
                updateSettings('watermark', checked)
              }
            />
            <SettingSwitch
              id='seedance-return-last-frame'
              label={t('Return last frame')}
              hint={t('Return the last frame image with the task result')}
              checked={settings.returnLastFrame}
              disabled={isDisabled}
              onCheckedChange={(checked) =>
                updateSettings('returnLastFrame', checked)
              }
            />
          </div>
        </div>

        <div className='space-y-5'>
          <MediaUploadField
            id='seedance-reference-images'
            label={t('Reference images')}
            hint={t(
              'Add one image at a time, up to {{max}} reference images.',
              { max: SEEDANCE_MAX_REFERENCE_IMAGES }
            )}
            accept='image/*'
            appendOneAtATime
            maxFiles={SEEDANCE_MAX_REFERENCE_IMAGES}
            uploadLabel={t('Add image')}
            kind='image'
            icon={<ImageIcon className='mr-2 size-4' />}
            disabled={isDisabled}
            files={referenceImages}
            onChange={setReferenceImages}
            onPatch={patchReferenceImages}
            showImageRole
          />
          <MediaUploadField
            id='seedance-reference-video'
            label={t('Reference video')}
            hint={t(
              'Add one video at a time, up to {{max}} reference videos.',
              { max: SEEDANCE_MAX_REFERENCE_VIDEOS }
            )}
            accept='video/*'
            appendOneAtATime
            maxFiles={SEEDANCE_MAX_REFERENCE_VIDEOS}
            uploadLabel={t('Add video')}
            kind='video'
            icon={<VideoIcon className='mr-2 size-4' />}
            disabled={isDisabled}
            files={referenceVideo}
            onChange={setReferenceVideo}
            onPatch={patchReferenceVideo}
          />
          <MediaUploadField
            id='seedance-reference-audio'
            label={t('Reference audio')}
            hint={t(
              'Add one audio file at a time, up to {{max}} reference audios.',
              { max: SEEDANCE_MAX_REFERENCE_AUDIOS }
            )}
            accept='audio/*'
            appendOneAtATime
            maxFiles={SEEDANCE_MAX_REFERENCE_AUDIOS}
            uploadLabel={t('Add audio')}
            kind='audio'
            icon={<MusicIcon className='mr-2 size-4' />}
            disabled={isDisabled}
            files={referenceAudio}
            onChange={setReferenceAudio}
            onPatch={patchReferenceAudio}
          />
        </div>
      </div>

      <div className='bg-background shrink-0 border-t px-4 py-4 sm:px-5'>
        <div className='flex flex-col gap-2'>
          <Button
            type='submit'
            className='w-full'
            disabled={isDisabled || prompt.trim() === '' || hasMediaUploading || hasMediaUploadError}
          >
            {t('Generate video')}
          </Button>
          {props.models.length === 0 ? (
            <p className='text-muted-foreground text-center text-xs'>
              {t('No Seedance 2.0 models available for your account')}
            </p>
          ) : null}
        </div>
      </div>
    </form>
  )
}
