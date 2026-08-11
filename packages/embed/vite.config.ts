import { defineConfig } from 'vite'

/*
 * One file, no chunks, no CSS asset.
 *
 * A host page adds this with a single script tag and must not discover that it
 * also needed to serve three more. The styles ship as a string in the bundle
 * because they are adopted into a shadow root, not linked from a document —
 * see styles.ts.
 */
export default defineConfig({
  build: {
    target: 'es2022',
    lib: {
      entry: 'src/index.ts',
      formats: ['es'],
      fileName: () => 'cronos-embed.js',
    },
    rollupOptions: {
      output: { inlineDynamicImports: true },
    },
    // The budget is the feature. Anything that pushes past it has to justify
    // itself against an ISV's page weight, not against our convenience.
    reportCompressedSize: true,
  },
})
