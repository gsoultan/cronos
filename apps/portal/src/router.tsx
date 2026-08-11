import { createRootRoute, createRoute, createRouter, lazyRouteComponent } from '@tanstack/react-router'
import { Shell } from './components/Shell'
import { ReportsPage } from './routes/ReportsPage'

const rootRoute = createRootRoute({ component: Shell })

/* The list is the landing route, so it ships eagerly. Everything else loads on
   navigation — charts, the virtualiser and the filter builder are the bulk of
   the app and no one needs them to read a list of report names. */
const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: ReportsPage,
})

const reportRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/reports/$name',
  component: lazyRouteComponent(() => import('./routes/ReportPage'), 'ReportPage'),
})

const newReportRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/reports/new',
  component: lazyRouteComponent(() => import('./routes/NewReportPage'), 'NewReportPage'),
})

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings',
  component: lazyRouteComponent(() => import('./routes/SettingsPage'), 'SettingsPage'),
})

const dataRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/data',
  component: lazyRouteComponent(() => import('./routes/DataPage'), 'DataPage'),
})

const schedulesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/schedules',
  component: lazyRouteComponent(() => import('./routes/SchedulesPage'), 'SchedulesPage'),
})

const routeTree = rootRoute.addChildren([
  indexRoute, newReportRoute, reportRoute, settingsRoute,
  dataRoute, schedulesRoute,
])

/* defaultPreload 'intent' fetches the route chunk on hover, so the split is
   invisible in use — the chunk is usually there before the click lands. */
export const router = createRouter({ routeTree, defaultPreload: 'intent' })

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
