import { api } from '@/lib/api'

type DownloadCountResponse = {
  success: boolean
  message: string
  data: { count: number }
}

export async function getWindowsDownloadCount() {
  const res = await api.get<DownloadCountResponse>(
    '/api/download/windows/count'
  )
  return res.data.data.count
}
