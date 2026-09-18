import { useEffect, useRef } from 'react'
import { Link } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AuthShell } from '@/components/AuthShell'
import { Logo } from '@/components/Logo'
import { Button } from '@/components/ui/Button'

const steps = [
  ['setupAccountStep', 'setupAccountDetail'],
  ['setupWorkspaceStep', 'setupWorkspaceDetail'],
] as const

export function Setup() {
  const { t } = useTranslation()
  const heading = useRef<HTMLHeadingElement>(null)

  useEffect(() => {
    heading.current?.focus()
  }, [])

  return (
    <AuthShell>
      <Logo className="mb-7 size-12" />
      <h1
        ref={heading}
        tabIndex={-1}
        className="text-2xl font-semibold tracking-tight text-balance outline-none"
      >
        {t('setupWelcome')}
      </h1>
      <p className="mt-3 text-sm leading-6 text-muted-foreground">
        {t('setupWelcomeDescription')}
      </p>
      <ol className="my-8 space-y-5 border-y border-border py-6">
        {steps.map(([title, description], index) => (
          <li key={title} className="flex gap-4">
            <span
              className="pt-0.5 text-xs leading-5 text-muted-foreground tabular-nums"
              aria-hidden="true"
            >
              {index + 1}
            </span>
            <div>
              <h2 className="text-sm font-medium">{t(title)}</h2>
              <p className="mt-1 text-sm leading-6 text-muted-foreground">
                {t(description)}
              </p>
            </div>
          </li>
        ))}
      </ol>
      <Button asChild className="h-11 w-full">
        <Link to="/setup/admin">
          {t('startSetup')}
          <ArrowRight aria-hidden="true" />
        </Link>
      </Button>
    </AuthShell>
  )
}
