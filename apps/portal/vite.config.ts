import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { VitePWA } from 'vite-plugin-pwa'
import { mantineBase } from './plugins/mantine-base.ts'

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    mantineBase(),
    VitePWA({
      registerType: 'autoUpdate',
      manifest: {
        name: 'cronos',
        short_name: 'cronos',
        description: 'Reports and dashboards',
        theme_color: '#fcfcfb',
        background_color: '#f9f9f7',
        display: 'standalone',
        icons: [],
      },
      workbox: {
        // Definitions are safe to cache; results never are — they are
        // principal-scoped and must not survive a session.
        runtimeCaching: [
          {
            urlPattern: /\/v1\/(reports|dashboards|datasets)(\?|$)/,
            handler: 'StaleWhileRevalidate',
            options: { cacheName: 'cronos-definitions' },
          },
        ],
      },
    }),
  ],
  build: {
    // AGENTS.md budget: any lazy chunk <= 500 KB raw.
    chunkSizeWarningLimit: 500,
    rollupOptions: {
      output: {
        // Only the framework floor is pinned eager. Everything else is left to
        // the bundler so route-level splitting actually splits — a manualChunks
        // rule that names a UI library drags all of it into the first paint.
        manualChunks(id) {
          if (/node_modules\/(react|react-dom|scheduler)\//.test(id)) return 'react'
        },
      },
    },
  },
})
