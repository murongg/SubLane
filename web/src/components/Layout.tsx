import { Link, Outlet, useRouterState } from '@tanstack/react-router'
import {
  CircleHelp,
  LayoutDashboard,
  Network,
  Settings2,
  Workflow,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { LanguageSelect, ThemeSelect } from './Preferences'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
  useSidebar,
} from './ui/Sidebar'

const navigation = [
  { to: '/', label: 'overview', icon: LayoutDashboard },
  { to: '/accounts', label: 'accounts', icon: Workflow },
  { to: '/preferences', label: 'preferences', icon: Settings2 },
] as const

function Navigation() {
  const { t } = useTranslation()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const { setOpenMobile } = useSidebar()
  return (
    <Sidebar className="border-r border-border">
      <SidebarHeader className="px-5 py-6">
        <Link
          to="/"
          className="flex items-center gap-2.5 rounded-md focus-visible:outline-2 focus-visible:outline-ring"
          aria-label="SubLane"
          onClick={() => setOpenMobile(false)}
        >
          <span className="grid size-8 place-items-center rounded-lg bg-primary text-primary-foreground">
            <Network className="size-4" aria-hidden="true" />
          </span>
          <span className="text-lg font-semibold tracking-tight">SubLane</span>
        </Link>
        <p className="mt-2 text-xs text-muted-foreground">
          {t('internalGateway')}
        </p>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup className="px-3">
          <SidebarGroupLabel>{t('workspace')}</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {navigation.map(({ to, label, icon: Icon }) => (
                <SidebarMenuItem key={to}>
                  <SidebarMenuButton
                    asChild
                    isActive={pathname === to}
                    className="h-10"
                  >
                    <Link
                      to={to}
                      onClick={() => setOpenMobile(false)}
                      aria-current={pathname === to ? 'page' : undefined}
                    >
                      <Icon aria-hidden="true" />
                      <span>{t(label)}</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter className="gap-2 border-t border-border px-5 py-4">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <CircleHelp className="size-3.5" aria-hidden="true" />
          {t('foundation')}
        </div>
        <span className="text-xs text-muted-foreground">AGPL-3.0</span>
      </SidebarFooter>
    </Sidebar>
  )
}

export function Layout() {
  const { t } = useTranslation()
  const isPreferences = useRouterState({
    select: (s) => s.location.pathname === '/preferences',
  })
  return (
    <SidebarProvider>
      <a
        href="#main-content"
        className="sr-only z-50 rounded-md bg-primary p-3 text-primary-foreground focus:not-sr-only focus:fixed focus:left-4 focus:top-4"
      >
        {t('skipContent')}
      </a>
      <Navigation />
      <SidebarInset className="min-w-0 bg-background">
        <header className="flex min-h-16 flex-wrap items-center justify-between gap-3 border-b border-border px-4 md:px-8">
          <div className="flex items-center gap-3">
            <SidebarTrigger className="size-9" />
            <span className="hidden text-sm text-muted-foreground sm:inline">
              {t('workspace')}
            </span>
          </div>
          {!isPreferences && (
            <div className="flex items-center gap-2">
              <LanguageSelect />
              <ThemeSelect />
            </div>
          )}
        </header>
        <main
          id="main-content"
          className="mx-auto w-full max-w-6xl px-5 py-8 md:px-10 md:py-10"
        >
          <Outlet />
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}
