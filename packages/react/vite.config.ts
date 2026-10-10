import { defineConfig } from 'vite'

/*
 * React is a peer, never bundled. Two Reacts in one page is the classic way a
 * widget breaks a host application's hooks, and it is exactly the failure an
 * ISV would blame on us.
 *
 * @cronos/embed is not bundled either. The element is loaded at run time from
 * the cronos server the report comes from (src/load.ts), so it always matches
 * that server's API; this package takes only its types. External here, and
 * scripts/size-check.mjs fails a build whose output imports it at all — a
 * runtime import added by mistake would otherwise ship the npm copy beside
 * the server's.
 */
export default defineConfig({
  build: {
    target: 'es2022',
    lib: { entry: 'src/index.ts', formats: ['es'], fileName: () => 'cronos-react.js' },
    rollupOptions: {
      external: ['react', 'react-dom', 'react/jsx-runtime', '@cronos/embed'],
      output: { inlineDynamicImports: true },
    },
  },
})
