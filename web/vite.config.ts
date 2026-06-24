import preact from '@preact/preset-vite'
import { defineConfig } from 'vite'

// gx landing site: Preact authored, prerendered to static HTML at build time and
// fully hydrated on the client. Hand-written CSS only (no CSS-in-JS). See ADR 0012.
// https://vite.dev/config/
export default defineConfig({
  plugins: [
    preact({
      prerender: {
        enabled: true,
        renderTarget: '#app',
      },
    }),
  ],
  build: {
    target: 'es2022',
  },
})
