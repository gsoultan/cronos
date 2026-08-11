import { defineConfig } from 'vite'

/*
 * React is a peer, never bundled. Two Reacts in one page is the classic way a
 * widget breaks a host application's hooks, and it is exactly the failure an
 * ISV would blame on us.
 *
 * @cronos/embed *is* bundled: it is 3 KB, it is ours, and a host adding one
 * dependency rather than two is worth more than the deduplication.
 */
export default defineConfig({
  build: {
    target: 'es2022',
    lib: { entry: 'src/index.ts', formats: ['es'], fileName: () => 'cronos-react.js' },
    rollupOptions: {
      external: ['react', 'react-dom', 'react/jsx-runtime'],
      output: { inlineDynamicImports: true },
    },
  },
})
