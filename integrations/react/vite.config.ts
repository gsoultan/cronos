import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

/*
 * An ordinary client-side React application.
 *
 * Served under /react/ by ../server.ts. In development, `bun run dev` runs
 * Vite's own server and sends /api to that backend, so the page and its token
 * route are one origin either way.
 */
export default defineConfig({
  base: '/react/',
  plugins: [react()],
  server: { proxy: { '/api': 'http://localhost:5301' } },
})
