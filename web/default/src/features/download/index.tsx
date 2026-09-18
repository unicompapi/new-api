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
import { useNavigate, useSearch } from '@tanstack/react-router'
import {
  AiComputerIcon,
  ComputerProgramming01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PublicLayout } from '@/components/layout'
import { getUniCompWindowsDownloadCount, getWindowsDownloadCount } from './api'
import {
  DOWNLOAD_DEFAULT_TAB_ID,
  UNICOMP_AI_TAB_ID,
  UNICOMP_DESKTOP_TAB_ID,
  type DownloadTabId,
} from './constants'
import { DownloadProduct } from './download-product'
import {
  getUniCompAiContent,
  getUniCompDesktopContent,
} from './product-content'

export function DownloadPage() {
  const { t } = useTranslation()
  const { tab } = useSearch({ from: '/download/' })
  const navigate = useNavigate()
  const activeTab: DownloadTabId = tab ?? DOWNLOAD_DEFAULT_TAB_ID
  const windowsDownloadCountQuery = useQuery({
    queryKey: ['windows-download-count'],
    queryFn: getWindowsDownloadCount,
    enabled: activeTab === UNICOMP_AI_TAB_ID,
  })
  const uniCompDownloadCountQuery = useQuery({
    queryKey: ['unicomp-windows-download-count'],
    queryFn: getUniCompWindowsDownloadCount,
    enabled: activeTab === UNICOMP_DESKTOP_TAB_ID,
  })

  const uniCompAiContent = getUniCompAiContent(t)
  const uniCompDesktopContent = getUniCompDesktopContent(t)

  const handleTabChange = (value: string) => {
    void navigate({
      to: '/download',
      search: (prev) => ({
        ...prev,
        tab:
          value === DOWNLOAD_DEFAULT_TAB_ID
            ? undefined
            : (value as DownloadTabId),
      }),
    })
  }

  return (
    <PublicLayout>
      <Tabs
        value={activeTab}
        onValueChange={handleTabChange}
        className='mx-auto w-full max-w-5xl gap-2 py-4 md:py-8'
      >
        <div className='flex flex-col items-center gap-3'>
          <p className='text-muted-foreground text-sm font-medium'>
            {t('Desktop applications')}
          </p>
          <TabsList
            aria-label={t('Desktop applications')}
            className='grid h-auto w-full max-w-xl grid-cols-1 gap-1 p-1 sm:grid-cols-2'
          >
            <TabsTrigger
              value={UNICOMP_AI_TAB_ID}
              className='h-auto min-h-11 min-w-0 px-3 py-2 whitespace-normal'
            >
              <HugeiconsIcon
                icon={AiComputerIcon}
                className='size-4'
                aria-hidden='true'
              />
              <span className='min-w-0'>{t('UniComp AI Desktop Client')}</span>
            </TabsTrigger>
            <TabsTrigger
              value={UNICOMP_DESKTOP_TAB_ID}
              className='h-auto min-h-11 min-w-0 px-3 py-2 whitespace-normal'
            >
              <HugeiconsIcon
                icon={ComputerProgramming01Icon}
                className='size-4'
                aria-hidden='true'
              />
              <span className='min-w-0'>{t('UniComp Desktop')}</span>
            </TabsTrigger>
          </TabsList>
        </div>

        <TabsContent value={UNICOMP_AI_TAB_ID}>
          <DownloadProduct
            content={uniCompAiContent}
            downloadCount={windowsDownloadCountQuery.data}
            isCountPending={windowsDownloadCountQuery.isPending}
          />
        </TabsContent>

        <TabsContent value={UNICOMP_DESKTOP_TAB_ID}>
          <DownloadProduct
            content={uniCompDesktopContent}
            downloadCount={uniCompDownloadCountQuery.data}
            isCountPending={uniCompDownloadCountQuery.isPending}
          />
        </TabsContent>
      </Tabs>
    </PublicLayout>
  )
}
