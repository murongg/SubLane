import {
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
  type RouterHistory,
} from '@tanstack/react-router'
import { AuthGate } from '@/components/AuthGate'
import { Auth } from '@/pages/Auth'
import { Setup } from '@/pages/Setup'
import { NotFound } from '@/pages/NotFound'

const root = createRootRoute({
  component: AuthGate,
  notFoundComponent: NotFound,
})
const routes = root.addChildren([
  createRoute({
    getParentRoute: () => root,
    path: '/setup',
    component: Setup,
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/setup/admin',
    component: () => <Auth mode="setup" />,
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/login',
    component: () => <Auth mode="login" />,
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/',
    component: lazyRouteComponent(() => import('@/pages/Home'), 'Home'),
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/accounts',
    component: lazyRouteComponent(() => import('@/pages/Accounts'), 'Accounts'),
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/keys',
    component: lazyRouteComponent(() => import('@/pages/Keys'), 'Keys'),
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/members',
    component: lazyRouteComponent(() => import('@/pages/Members'), 'Members'),
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/preferences',
    component: lazyRouteComponent(() => import('@/pages/Settings'), 'Settings'),
  }),
])

export function createAppRouter(history?: RouterHistory) {
  return createRouter({ routeTree: routes, history, defaultPreload: 'intent' })
}

declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof createAppRouter>
  }
}
