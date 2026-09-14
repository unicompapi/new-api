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
import { useCallback, useEffect, useRef, useState } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import { useStatus } from '@/hooks/use-status'
import type { AliyunGraphCaptchaValidation } from '../types'

interface AliyunGraphCaptchaInstance {
  showCaptcha: () => void
  getValidate: () => AliyunGraphCaptchaValidation | false
  reset: () => void
  destroy: () => void
  onNextReady: (callback: () => void) => AliyunGraphCaptchaInstance
  onSuccess: (callback: () => void) => AliyunGraphCaptchaInstance
  onError: (callback: () => void) => AliyunGraphCaptchaInstance
  onClose: (callback: () => void) => AliyunGraphCaptchaInstance
}

declare global {
  interface Window {
    initAlicom4?: (
      config: { captchaId: string; product: 'bind'; protocol: 'https://' },
      callback: (captcha: AliyunGraphCaptchaInstance) => void
    ) => void
  }
}

let sdkPromise: Promise<void> | null = null

function loadSDK() {
  if (window.initAlicom4) return Promise.resolve()
  if (sdkPromise) return sdkPromise

  sdkPromise = new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.src = '/ct4.js'
    script.async = true
    script.onload = () => resolve()
    script.onerror = () =>
      reject(new Error('Aliyun Graph CAPTCHA SDK failed to load'))
    document.head.appendChild(script)
  })
  return sdkPromise
}

export function useAliyunGraphCaptcha() {
  const { status } = useStatus()
  const appId =
    status?.aliyun_graph_captcha_app_id ??
    status?.data?.aliyun_graph_captcha_app_id ??
    ''
  const captchaRef = useRef<AliyunGraphCaptchaInstance | null>(null)
  const pendingRef = useRef<
    ((result: AliyunGraphCaptchaValidation | undefined) => void) | null
  >(null)
  const [ready, setReady] = useState(false)

  useEffect(() => {
    let active = true
    if (!appId) return

    void loadSDK()
      .then(() => {
        if (!active || !window.initAlicom4) return
        window.initAlicom4(
          { captchaId: appId, product: 'bind', protocol: 'https://' },
          (captcha) => {
            if (!active) {
              captcha.destroy()
              return
            }
            captchaRef.current = captcha
            captcha
              .onNextReady(() => setReady(true))
              .onSuccess(() => {
                const result = captcha.getValidate()
                pendingRef.current?.(result || undefined)
                pendingRef.current = null
                captcha.reset()
              })
              .onError(() => {
                if (pendingRef.current) {
                  pendingRef.current(undefined)
                  pendingRef.current = null
                  toast.error(i18next.t('Verification failed'))
                }
              })
              .onClose(() => {
                pendingRef.current?.(undefined)
                pendingRef.current = null
              })
          }
        )
      })
      .catch(() => {
        if (active) toast.error(i18next.t('Verification failed'))
      })

    return () => {
      active = false
      pendingRef.current?.(undefined)
      pendingRef.current = null
      captchaRef.current?.destroy()
      captchaRef.current = null
      setReady(false)
    }
  }, [appId])

  const verify = useCallback(() => {
    if (!ready || !captchaRef.current || pendingRef.current) {
      toast.info(
        i18next.t('Please wait a moment, human check is initializing...')
      )
      return Promise.resolve(undefined)
    }
    return new Promise<AliyunGraphCaptchaValidation | undefined>((resolve) => {
      pendingRef.current = resolve
      captchaRef.current?.showCaptcha()
    })
  }, [ready])

  return { verify }
}
