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
import { useQuery } from '@tanstack/react-query'
import {
  Bot,
  Download,
  HardDrive,
  MessageSquare,
  MessagesSquare,
  Monitor,
  Package,
  Video,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { PublicLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { getWindowsDownloadCount } from './api'
import {
  UNICOMP_AI_EXECUTABLE,
  UNICOMP_AI_INSTALLER_NAME,
  UNICOMP_AI_VERSION,
} from './constants'

const FEATURE_ICONS = [Package, Bot, Video, MessageSquare, MessagesSquare] as const

export function DownloadPage() {
  const { t } = useTranslation()
  const { data: downloadCount = 0 } = useQuery({
    queryKey: ['windows-download-count'],
    queryFn: getWindowsDownloadCount,
  })

  const features = [
    {
      icon: FEATURE_ICONS[0],
      title: t('Application Management'),
      description: t(
        'Create applications, manage API addresses, and organize access tokens securely in local SQLite storage.'
      ),
    },
    {
      icon: FEATURE_ICONS[1],
      title: t('Model Management'),
      description: t(
        'Browse and manage available AI models connected to your API gateway.'
      ),
    },
    {
      icon: FEATURE_ICONS[2],
      title: t('Seedance 2.0'),
      description: t(
        'Generate AI videos with prompts, reference assets, task polling, and local generation history.'
      ),
    },
    {
      icon: FEATURE_ICONS[3],
      title: t('AI Chat'),
      description: t(
        'Multi-turn conversations with models via the Playground interface.'
      ),
    },
    {
      icon: FEATURE_ICONS[4],
      title: t('Chatroom'),
      description: t(
        'Real-time messaging with WebSocket support for team collaboration.'
      ),
    },
  ]

  const requirements = [
    t('Windows 10 or later (64-bit)'),
    t('No Node.js or Python required — standalone installer'),
    t('Internet connection for API access'),
  ]

  const installSteps = [
    t('Download the installer using the button above'),
    t('Run UniComp AI Setup and follow the wizard'),
    t('Launch {{app}} from the desktop or Start menu shortcut', {
      app: UNICOMP_AI_EXECUTABLE,
    }),
    t('Add your API address and token in Application Management'),
  ]

  return (
    <PublicLayout>
      <div className='mx-auto flex max-w-5xl flex-col gap-10 py-4 md:py-8'>
        <section className='flex flex-col items-center gap-6 text-center'>
          <Badge variant='secondary' className='gap-1.5 px-3 py-1'>
            <Monitor className='size-3.5' />
            Windows
          </Badge>

          <div className='space-y-3'>
            <h1 className='text-3xl font-bold tracking-tight md:text-4xl'>
              {t('UniComp AI Desktop Client')}
            </h1>
            <p className='text-muted-foreground mx-auto max-w-2xl text-base leading-relaxed md:text-lg'>
              {t(
                'A Windows desktop companion for the UniComp API gateway. Manage API tokens, run AI chat, generate videos with Seedance 2.0, and more — all in one app.'
              )}
            </p>
          </div>

          <div className='flex flex-col items-center gap-3'>
            <form action='/api/download/windows' method='post'>
              <Button
                type='submit'
                size='lg'
                className='h-11 min-w-52 gap-2 px-6 text-base'
              >
                <Download className='size-5' />
                {t('Download for Windows')}
              </Button>
            </form>
            <p className='text-muted-foreground text-sm'>
              {t('Version {{version}}', { version: UNICOMP_AI_VERSION })}
              {' · '}
              {UNICOMP_AI_INSTALLER_NAME}
            </p>
            <p className='text-muted-foreground text-xs'>
              累计下载 {downloadCount.toLocaleString()} 次
            </p>
          </div>

          <div className='bg-muted/50 text-muted-foreground flex items-start gap-2 rounded-lg border px-4 py-3 text-sm'>
            <HardDrive className='mt-0.5 size-4 shrink-0' />
            <p>
              {t(
                'Local-first design with embedded SQLite — your data stays on your device'
              )}
            </p>
          </div>
        </section>

        <section className='space-y-4'>
          <h2 className='text-xl font-semibold'>{t('Key Features')}</h2>
          <div className='grid gap-4 sm:grid-cols-2'>
            {features.map((feature) => (
              <Card key={feature.title} size='sm'>
                <CardHeader>
                  <div className='flex items-center gap-3'>
                    <div className='bg-primary/10 text-primary flex size-9 items-center justify-center rounded-lg'>
                      <feature.icon className='size-4.5' />
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
          <section className='space-y-4'>
            <h2 className='text-xl font-semibold'>{t('System Requirements')}</h2>
            <Card size='sm'>
              <CardContent className='pt-4'>
                <ul className='text-muted-foreground list-disc space-y-2 pl-5 text-sm leading-relaxed'>
                  {requirements.map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ul>
              </CardContent>
            </Card>
          </section>

          <section className='space-y-4'>
            <h2 className='text-xl font-semibold'>{t('Installation')}</h2>
            <Card size='sm'>
              <CardContent className='pt-4'>
                <ol className='text-muted-foreground list-decimal space-y-2 pl-5 text-sm leading-relaxed'>
                  {installSteps.map((step) => (
                    <li key={step}>{step}</li>
                  ))}
                </ol>
              </CardContent>
            </Card>
          </section>
        </div>
      </div>
    </PublicLayout>
  )
}
