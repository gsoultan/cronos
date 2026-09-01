import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { MantineProvider } from '@mantine/core'
import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'

/* index.css first: it declares the cascade layer order that the Mantine base
   below is slotted into. */
import './theme/index.css'
import 'virtual:mantine-base.css'

import { theme } from './theme/theme'
import { router } from './router'
import { WorkspaceProvider } from './lib/WorkspaceContext'
import { UpdatePrompt } from './components/UpdatePrompt'

/* A cache that ignores who asked is a data leak, and no key here names who
   asked. The cache is emptied when the session changes instead — see
   lib/queryClient.ts for why that is the fix and not the keys. */
import { queryClient } from './lib/queryClient'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <MantineProvider theme={theme} defaultColorScheme="auto">
      <QueryClientProvider client={queryClient}>
        <WorkspaceProvider>
          <RouterProvider router={router} />
          <UpdatePrompt />
        </WorkspaceProvider>
      </QueryClientProvider>
    </MantineProvider>
  </StrictMode>,
)
