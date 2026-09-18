import { useState } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { ThemeProvider } from 'next-themes'
import { I18nextProvider } from 'react-i18next'
import { i18n } from '@/lib/i18n'
import type { createAppRouter } from './router'

export function App({
  router,
}: {
  router: ReturnType<typeof createAppRouter>
}) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 15_000,
            retry: false,
            refetchOnWindowFocus: false,
          },
        },
      }),
  )
  return (
    <I18nextProvider i18n={i18n}>
      <ThemeProvider
        attribute="class"
        defaultTheme="system"
        storageKey="sublane-theme"
        enableSystem
      >
        <QueryClientProvider client={queryClient}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </ThemeProvider>
    </I18nextProvider>
  )
}
