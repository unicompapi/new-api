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
import { useEffect, useState } from 'react'
import type { z } from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Loader2, LogIn } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { useStatus } from '@/hooks/use-status'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Turnstile } from '@/components/turnstile'
import { smsLogin } from '@/features/auth/api'
import { LegalConsent } from '@/features/auth/components/legal-consent'
import { smsLoginFormSchema } from '@/features/auth/constants'
import { useAuthRedirect } from '@/features/auth/hooks/use-auth-redirect'
import { useSMSVerification } from '@/features/auth/hooks/use-sms-verification'
import { useTurnstile } from '@/features/auth/hooks/use-turnstile'
import type { AuthFormProps } from '@/features/auth/types'

export function SMSAuthForm({
  className,
  redirectTo,
  ...props
}: AuthFormProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const [isLoading, setIsLoading] = useState(false)
  const [agreedToLegal, setAgreedToLegal] = useState(false)
  const {
    isTurnstileEnabled,
    turnstileSiteKey,
    turnstileToken,
    turnstileVersion,
    setTurnstileToken,
    validateTurnstile,
  } = useTurnstile()
  const { handleLoginSuccess, redirectTo2FA } = useAuthRedirect()

  const form = useForm<z.infer<typeof smsLoginFormSchema>>({
    resolver: zodResolver(smsLoginFormSchema),
    defaultValues: { phone: '', code: '' },
  })
  const phone = form.watch('phone')
  const hasUserAgreement = Boolean(status?.user_agreement_enabled)
  const hasPrivacyPolicy = Boolean(status?.privacy_policy_enabled)
  const requiresLegalConsent = hasUserAgreement || hasPrivacyPolicy
  const turnstileReady = !isTurnstileEnabled || Boolean(turnstileToken)
  const { isSending, secondsLeft, isActive, sendCode } = useSMSVerification({
    purpose: 'login',
  })

  useEffect(() => {
    setAgreedToLegal(!requiresLegalConsent)
  }, [requiresLegalConsent])

  async function handleSendCode() {
    if (!(await form.trigger('phone'))) return
    await sendCode(phone)
  }

  async function onSubmit(data: z.infer<typeof smsLoginFormSchema>) {
    if (requiresLegalConsent && !agreedToLegal) {
      toast.error(t('Please agree to the legal terms first'))
      return
    }
    if (!validateTurnstile()) return

    setIsLoading(true)
    try {
      const res = await smsLogin({
        phone: data.phone,
        code: data.code,
        turnstile: turnstileToken,
      })
      if (res.success) {
        if (res.data?.require_2fa) {
          redirectTo2FA()
          return
        }
        await handleLoginSuccess(res.data as { id?: number } | null, redirectTo)
        toast.success(t('Welcome back!'))
      } else {
        toast.error(res.message || t('Login failed'))
      }
    } catch (_error) {
      // Errors are handled by the global interceptor.
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className={cn('grid gap-4', className)}
        {...props}
      >
        <FormField
          control={form.control}
          name='phone'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Mobile phone number')}</FormLabel>
              <FormControl>
                <Input
                  placeholder={t('Enter your 11-digit mobile phone number')}
                  inputMode='tel'
                  autoComplete='tel'
                  maxLength={11}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name='code'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('SMS verification code')}</FormLabel>
              <div className='flex gap-2'>
                <FormControl>
                  <Input
                    placeholder={t('Enter 6-digit code')}
                    inputMode='numeric'
                    autoComplete='one-time-code'
                    maxLength={6}
                    {...field}
                  />
                </FormControl>
                <Button
                  type='button'
                  variant='outline'
                  className='shrink-0'
                  disabled={
                    isLoading ||
                    isSending ||
                    isActive ||
                    !phone ||
                    !turnstileReady
                  }
                  onClick={handleSendCode}
                >
                  {isActive ? (
                    t('Resend ({{seconds}}s)', { seconds: secondsLeft })
                  ) : isSending ? (
                    <Loader2 className='h-4 w-4 animate-spin' />
                  ) : (
                    t('Send code')
                  )}
                </Button>
              </div>
              <FormMessage />
            </FormItem>
          )}
        />

        {isTurnstileEnabled && (
          <div className='mt-2'>
            <Turnstile
              key={turnstileVersion}
              siteKey={turnstileSiteKey}
              onVerify={setTurnstileToken}
            />
          </div>
        )}

        <LegalConsent
          status={status}
          checked={agreedToLegal}
          onCheckedChange={setAgreedToLegal}
          className='mt-1'
        />

        <Button
          type='submit'
          className='mt-2 w-full justify-center gap-2'
          disabled={
            isLoading ||
            !turnstileReady ||
            (requiresLegalConsent && !agreedToLegal)
          }
        >
          {isLoading ? <Loader2 className='animate-spin' /> : <LogIn />}
          {t('Sign in')}
        </Button>
      </form>
    </Form>
  )
}
