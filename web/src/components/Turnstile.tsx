import { useEffect, useEffectEvent, useRef, useState } from 'react'
import { useTheme } from 'next-themes'
import { useTranslation } from 'react-i18next'
import { useIsMobile } from '@/hooks/use-mobile'
import { Button } from './ui/Button'

type TurnstileAPI = {
  render: (element: HTMLElement, options: Record<string, unknown>) => string
  remove: (id: string) => void
}
declare global {
  interface Window {
    turnstile?: TurnstileAPI
  }
}

let loading: Promise<TurnstileAPI> | undefined
function loadTurnstile() {
  if (window.turnstile) return Promise.resolve(window.turnstile)
  if (loading) return loading
  // Keep one in-flight script across React remounts; removing a script does not cancel its execution.
  loading = new Promise<TurnstileAPI>((resolve, reject) => {
    const script = document.createElement('script')
    const fail = () => {
      clearTimeout(timer)
      loading = undefined
      script.remove()
      reject(new Error('turnstile_unavailable'))
    }
    const timer = setTimeout(fail, 15000)
    script.src =
      'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'
    script.async = true
    script.onerror = fail
    script.onload = () => {
      clearTimeout(timer)
      if (window.turnstile) resolve(window.turnstile)
      else fail()
    }
    document.head.append(script)
  })
  return loading
}

export function Turnstile({
  siteKey,
  onToken,
}: {
  siteKey: string
  onToken: (token: string) => void
}) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const mobile = useIsMobile()
  const container = useRef<HTMLDivElement>(null)
  const [failed, setFailed] = useState(false)
  const [attempt, setAttempt] = useState(0)
  const publish = useEffectEvent(onToken)
  useEffect(() => {
    let id: string | undefined
    let active = true
    let api: TurnstileAPI | undefined
    const fail = () => {
      if (!active) return
      publish('')
      setFailed(true)
    }
    const render = (loaded: TurnstileAPI) => {
      if (!active || !container.current) return
      api = loaded
      try {
        id = api.render(container.current, {
          sitekey: siteKey,
          action: 'login',
          // Flexible widgets have a 300px minimum, which exceeds the narrow login form.
          size: mobile ? 'compact' : 'flexible',
          theme: resolvedTheme ?? 'auto',
          callback: (token: string) => {
            if (active) {
              setFailed(false)
              publish(token)
            }
          },
          'expired-callback': () => {
            if (active) publish('')
          },
          'error-callback': fail,
          'timeout-callback': fail,
        })
      } catch {
        fail()
      }
    }
    publish('')
    loadTurnstile().then(render, fail)
    return () => {
      active = false
      if (id !== undefined) api?.remove(id)
    }
  }, [siteKey, resolvedTheme, mobile, attempt])
  return (
    <div className="space-y-2">
      <div
        ref={container}
        className={mobile ? 'min-h-[140px]' : 'min-h-[65px]'}
        aria-label={t('turnstileVerification')}
      />
      {failed && (
        <div className="space-y-2">
          <p role="alert" className="text-sm text-error">
            {t('turnstileUnavailable')}
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => {
              setFailed(false)
              setAttempt((value) => value + 1)
            }}
          >
            {t('turnstileRetry')}
          </Button>
        </div>
      )}
    </div>
  )
}
