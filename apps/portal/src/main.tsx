import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { MantineProvider } from '@mantine/core'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'

/* index.css first: it declares the cascade layer order that the Mantine base
   below is slotted into. */
import './theme/index.css'
import 'virtual:mantine-base.css'

import { theme } from './theme/theme'
import { router } from './router'
import { WorkspaceProvider } from './lib/WorkspaceContext'

/* Cache keys will carry tenant + definition version once the API exists — a
   cache that ignores who asked is a data leak. */
const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, refetchOnWindowFocus: false } },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <MantineProvider theme={theme} defaultColorScheme="auto">
      <QueryClientProvider client={queryClient}>
        <WorkspaceProvider>
          <RouterProvider router={router} />
        </WorkspaceProvider>
      </QueryClientProvider>
    </MantineProvider>
  </StrictMode>,
)
