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
        /*
           The application, and nothing the API answered.

           The shell is precached — it is the same bytes for everybody, so it
           loads instantly and works offline. No API response is cached at all,
           and the denylist is what keeps the navigation fallback from ever
           standing in for one.

           There was a runtimeCaching rule here that meant to cache definitions.
           It matched `/v1/reports`, `/v1/datasets` and `/v1/datasources`, none
           of which are routes — the endpoints are `/v1/catalog` and
           `/v1/definitions` — so it had never cached anything since the day it
           was written, and AGENTS.md claimed an offline feature that did not
           exist.

           It is gone rather than corrected, because correcting it is the
           dangerous half. Workbox keys a cache by URL; the bearer is a header
           and is not part of the key. Pointed at a route that exists, that rule
           would serve one principal's catalogue to the next person to sign in
           on the same browser — which on a shared machine is one tenant reading
           another's. Their own rule says it: a cache that ignores who asked is
           a data leak.

           Offline viewing is worth having and is a feature to design: a cache
           named per principal, emptied on sign-out, and a check that signs in as
           somebody else and looks. Not a regex.
        */
        navigateFallbackDenylist: [/^\/v1\//],
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
