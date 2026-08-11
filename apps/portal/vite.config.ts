import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { VitePWA } from 'vite-plugin-pwa'
import { mantineBase } from './plugins/mantine-base.ts'

export default defineConfig({
  server: {
    watch: {
      /* `make ui` builds into dist/ while this server is running, and thirty
         files landing under the watcher triggers a reload in every connected
         page — including the ten headless browsers running assertions against
         it. Two suites failed together that way and passed on their own,
         which is exactly the flake that gets a gate ignored. */
      ignored: ['**/dist/**', '**/dev-dist/**', '**/shots/**'],
    },
  },

  plugins: [
    react(),
    tailwindcss(),
    mantineBase(),
    VitePWA({
      /* Prompt, not autoUpdate. A silent reload while somebody is halfway
         through building a report throws their work away; the banner lets them
         finish the thought and then reload. */
      registerType: 'prompt',
      includeAssets: ['apple-touch-icon.png'],
      manifest: {
        name: 'cronos — reports',
        short_name: 'cronos',
        description: 'Build, schedule and share reports.',
        theme_color: '#fcfcfb',
        background_color: '#f9f9f7',
        display: 'standalone',
        start_url: '/',
        scope: '/',
        orientation: 'any',
        /* An empty icons array is not an incomplete manifest — it is an
           uninstallable one. Chrome needs 192 and 512, and a maskable variant
           so Android does not crop the mark into a circle. */
        icons: [
          { src: '/icon-192.png', sizes: '192x192', type: 'image/png' },
          { src: '/icon-512.png', sizes: '512x512', type: 'image/png' },
          { src: '/icon-maskable-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
        ],
      },
      workbox: {
        // Definitions are safe to cache; results never are — they are
        // principal-scoped and must not survive a session.
        navigateFallbackDenylist: [/^\/v1\//],
        runtimeCaching: [
          {
            /* Definitions are safe to cache: they are the same for everyone in
               a project and change rarely. Results are not, and never appear
               here — they are principal-scoped, and a cache that ignores who
               asked is a data leak. */
            urlPattern: /\/v1\/(reports|datasets|datasources)(\?|$)/,
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
