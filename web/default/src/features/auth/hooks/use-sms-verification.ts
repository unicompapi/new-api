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
import { useState } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import { useCountdown } from '@/hooks/use-countdown'
import { sendSMSCode } from '../api'
import { MAINLAND_PHONE_REGEX, SMS_VERIFICATION_COUNTDOWN } from '../constants'
import { useAliyunGraphCaptcha } from './use-aliyun-graph-captcha'

interface UseSMSVerificationOptions {
  purpose: 'login' | 'register'
}

export function useSMSVerification(options: UseSMSVerificationOptions) {
  const [isSending, setIsSending] = useState(false)
  const { verify } = useAliyunGraphCaptcha()
  const {
    secondsLeft,
    isActive,
    start: startCountdown,
  } = useCountdown({ initialSeconds: SMS_VERIFICATION_COUNTDOWN })

  const sendCode = async (phone: string) => {
    const normalizedPhone = phone.trim()
    if (!MAINLAND_PHONE_REGEX.test(normalizedPhone)) {
      toast.error(i18next.t('Please enter a valid mobile phone number'))
      return false
    }
    const graphCaptcha = await verify()
    if (!graphCaptcha) return false

    setIsSending(true)
    try {
      const res = await sendSMSCode({
        phone: normalizedPhone,
        purpose: options.purpose,
        graph_captcha: graphCaptcha,
      })
      if (res?.success) {
        startCountdown()
        toast.success(i18next.t('Verification code sent'))
        return true
      }
      toast.error(res?.message || i18next.t('Failed to send verification code'))
      return false
    } catch (_error) {
      return false
    } finally {
      setIsSending(false)
    }
  }

  return { isSending, secondsLeft, isActive, sendCode }
}
