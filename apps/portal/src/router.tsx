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

const accountRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/account',
  component: lazyRouteComponent(() => import('./routes/AccountPage'), 'AccountPage'),
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

/* Outside the shell, and outside sign-in. A share is read by somebody with no
   account, which is the whole point of handing one out. */
const sharedRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/s/$id',
  component: lazyRouteComponent(() => import('./routes/SharedPage'), 'SharedPage'),
})

const activityRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/activity',
  component: lazyRouteComponent(() => import('./routes/ActivityPage'), 'ActivityPage'),
})

/* Editing is its own route rather than a panel, so a definition somebody is
   part-way through changing has a URL — which is what makes it linkable, and
   what makes the browser's back button mean "leave the editor". */
const editReportRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/reports/$name/edit',
  component: lazyRouteComponent(() => import('./routes/EditPages'), 'EditReportPage'),
})

const editDatasetRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/data/datasets/$name/edit',
  component: lazyRouteComponent(() => import('./routes/EditPages'), 'EditDatasetPage'),
})

const editSourceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/data/sources/$name/edit',
  component: lazyRouteComponent(() => import('./routes/EditPages'), 'EditDataSourcePage'),
})

const editScheduleRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/schedules/$name/edit',
  component: lazyRouteComponent(() => import('./routes/EditPages'), 'EditSchedulePage'),
})

const schedulesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/schedules',
  component: lazyRouteComponent(() => import('./routes/SchedulesPage'), 'SchedulesPage'),
})

const routeTree = rootRoute.addChildren([
  indexRoute, newReportRoute, reportRoute, settingsRoute, accountRoute,
  dataRoute, schedulesRoute, activityRoute, sharedRoute,
  editReportRoute, editDatasetRoute, editSourceRoute, editScheduleRoute,
])

/* defaultPreload 'intent' fetches the route chunk on hover, so the split is
   invisible in use — the chunk is usually there before the click lands. */
export const router = createRouter({ routeTree, defaultPreload: 'intent' })

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
