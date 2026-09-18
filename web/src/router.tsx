import {
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
  type RouterHistory,
} from '@tanstack/react-router'
import { Layout } from '@/components/Layout'
import { NotFound } from '@/pages/NotFound'

const root = createRootRoute({ component: Layout, notFoundComponent: NotFound })
const routes = root.addChildren([
  createRoute({
    getParentRoute: () => root,
    path: '/',
    component: lazyRouteComponent(() => import('@/pages/Overview'), 'Overview'),
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/accounts',
    component: lazyRouteComponent(() => import('@/pages/Accounts'), 'Accounts'),
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
