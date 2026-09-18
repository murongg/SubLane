import type { ReactNode } from 'react'
import { LanguageSelect, ThemeSelect } from './Preferences'
import { Logo } from './Logo'

export function AuthShell({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-svh flex-col">
      <header className="flex min-h-16 items-center justify-between gap-3 px-5 sm:px-6">
        <div className="flex items-center gap-2.5">
          <Logo />
          <span className="text-base font-semibold tracking-tight">
            SubLane
          </span>
        </div>
        <div className="flex items-center gap-1">
          <LanguageSelect compact />
          <ThemeSelect compact />
        </div>
      </header>
      <main className="flex flex-1 items-center justify-center px-5 py-10">
        <section className="w-full max-w-md rounded-xl border border-border bg-card p-6 sm:p-8">
          {children}
        </section>
      </main>
    </div>
  )
}
