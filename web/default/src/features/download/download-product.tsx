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
  Alert02Icon,
  Download04Icon,
  HardDriveIcon,
  WindowsNewIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type { DownloadProductContent } from './product-content'

type DownloadProductProps = {
  content: DownloadProductContent
  downloadCount?: number
  isCountPending: boolean
}

function DownloadCount(props: { count?: number; isPending: boolean }) {
  const { i18n, t } = useTranslation()

  if (props.isPending) {
    return (
      <div className='flex h-4 items-center' role='status'>
        <Skeleton className='h-3 w-32' />
        <span className='sr-only'>{t('Loading download count')}</span>
      </div>
    )
  }

  if (props.count === undefined) {
    return null
  }

  return (
    <p className='text-muted-foreground text-xs'>
      {t('Cumulative downloads: {{count}}', {
        count: props.count.toLocaleString(i18n.resolvedLanguage),
      })}
    </p>
  )
}

export function DownloadProduct(props: DownloadProductProps) {
  const { t } = useTranslation()

  return (
    <div className='flex flex-col gap-10 pt-4 md:pt-6'>
      <section className='flex flex-col items-center gap-6 text-center'>
        <Badge variant='secondary' className='gap-1.5 px-3 py-1'>
          <HugeiconsIcon
            icon={WindowsNewIcon}
            className='size-3.5'
            aria-hidden='true'
          />
          {t('Windows')}
        </Badge>

        <div className='flex flex-col gap-3'>
          <h1 className='text-3xl font-bold md:text-4xl'>
            {props.content.title}
          </h1>
          <p className='text-muted-foreground mx-auto max-w-2xl text-base leading-relaxed md:text-lg'>
            {props.content.description}
          </p>
        </div>

        <div className='flex w-full flex-col items-center gap-3 sm:w-auto'>
          <form
            action={props.content.downloadAction}
            method='post'
            className='w-full sm:w-auto'
          >
            <Button
              type='submit'
              size='lg'
              className='h-11 w-full gap-2 px-6 text-base sm:min-w-52'
            >
              <HugeiconsIcon
                icon={Download04Icon}
                data-icon='inline-start'
                aria-hidden='true'
              />
              {t('Download for Windows')}
            </Button>
          </form>
          <p className='text-muted-foreground max-w-full text-sm leading-relaxed'>
            {t('Version {{version}}', { version: props.content.version })}
            {' · '}
            <span className='break-all'>{props.content.installerName}</span>
          </p>
          <DownloadCount
            count={props.downloadCount}
            isPending={props.isCountPending}
          />
        </div>

        <div className='bg-muted/50 text-muted-foreground flex max-w-3xl items-start gap-2 rounded-lg border px-4 py-3 text-left text-sm'>
          <HugeiconsIcon
            icon={HardDriveIcon}
            className='mt-0.5 size-4 shrink-0'
            aria-hidden='true'
          />
          <p>{props.content.localNote}</p>
        </div>
      </section>

      {props.content.warning && (
        <Alert className='mx-auto max-w-3xl'>
          <HugeiconsIcon icon={Alert02Icon} aria-hidden='true' />
          <AlertTitle>{props.content.warning.title}</AlertTitle>
          <AlertDescription>
            {props.content.warning.description}
          </AlertDescription>
        </Alert>
      )}

      <section className='flex flex-col gap-4'>
        <h2 className='text-xl font-semibold'>
          {props.content.featureSectionTitle}
        </h2>
        <div className='grid items-stretch gap-4 sm:grid-cols-2'>
          {props.content.features.map((feature) => (
            <Card key={feature.title} size='sm' className='h-full'>
              <CardHeader>
                <div className='flex items-center gap-3'>
                  <div className='bg-primary/10 text-primary flex size-9 shrink-0 items-center justify-center rounded-lg'>
                    <HugeiconsIcon
                      icon={feature.icon}
                      className='size-4.5'
                      aria-hidden='true'
                    />
                  </div>
                  <CardTitle>{feature.title}</CardTitle>
                </div>
              </CardHeader>
              <CardContent className='pt-0'>
                <CardDescription>{feature.description}</CardDescription>
              </CardContent>
            </Card>
          ))}
        </div>
      </section>

      <div className='grid gap-6 md:grid-cols-2'>
        <section className='flex flex-col gap-4'>
          <h2 className='text-xl font-semibold'>{t('System Requirements')}</h2>
          <Card size='sm'>
            <CardContent className='pt-4'>
              <ul className='text-muted-foreground flex list-disc flex-col gap-2 pl-5 text-sm leading-relaxed'>
                {props.content.requirements.map((item) => (
                  <li key={item}>{item}</li>
                ))}
              </ul>
            </CardContent>
          </Card>
        </section>

        <section className='flex flex-col gap-4'>
          <h2 className='text-xl font-semibold'>{t('Installation')}</h2>
          <Card size='sm'>
            <CardContent className='pt-4'>
              <ol className='text-muted-foreground flex list-decimal flex-col gap-2 pl-5 text-sm leading-relaxed'>
                {props.content.installSteps.map((step) => (
                  <li key={step}>{step}</li>
                ))}
              </ol>
            </CardContent>
          </Card>
        </section>
      </div>
    </div>
  )
}
