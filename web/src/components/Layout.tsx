import { Link, Outlet, useRouterState } from '@tanstack/react-router'
import { LayoutDashboard, Settings, Workflow } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { LanguageSelect, ThemeSelect } from './Preferences'
import { Logo } from './Logo'
import { Session } from './Session'
import { Button } from './ui/Button'
import { Separator } from './ui/Separator'
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
] as const

function Navigation() {
  const { t } = useTranslation()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const { setOpenMobile } = useSidebar()
  return (
    <Sidebar className="border-r border-border">
      <SidebarHeader className="px-4 py-5">
        <Link
          to="/"
          className="flex items-center gap-2.5 rounded-md focus-visible:outline-2 focus-visible:outline-ring"
          aria-label="SubLane"
          onClick={() => setOpenMobile(false)}
        >
          <Logo />
          <div>
            <span className="text-base font-semibold tracking-tight">
              SubLane
            </span>
            <p className="mt-0.5 text-xs text-muted-foreground">
              {t('internalGateway')}
            </p>
          </div>
        </Link>
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
      <SidebarFooter className="p-3">
        <Session />
      </SidebarFooter>
    </Sidebar>
  )
}

export function Layout() {
  const { t } = useTranslation()
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
        <header className="flex min-h-16 items-center justify-between gap-3 px-4 md:px-6">
          <div className="flex items-center gap-3">
            <SidebarTrigger className="size-7 text-muted-foreground [@media(pointer:coarse)]:size-11" />
            <Separator orientation="vertical" className="h-6" />
            <span className="hidden text-sm text-muted-foreground sm:inline">
              {t('workspace')}
            </span>
          </div>
          <div className="flex items-center gap-1">
            <LanguageSelect compact />
            <ThemeSelect compact />
            <Button
              asChild
              variant="ghost"
              size="icon"
              className="text-muted-foreground [@media(pointer:coarse)]:size-11"
            >
              <Link
                to="/preferences"
                aria-label={t('preferences')}
                title={t('preferences')}
              >
                <Settings aria-hidden="true" />
              </Link>
            </Button>
          </div>
        </header>
        <main
          id="main-content"
          className="mx-auto w-full max-w-6xl px-5 py-6 md:px-6"
        >
          <Outlet />
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}
