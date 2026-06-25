import preact from '@preact/preset-vite'
import { defineConfig, type Plugin } from 'vite'
import { resolve } from 'node:path'
import { readFileSync, mkdirSync, copyFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const __dirname = fileURLToPath(new URL('.', import.meta.url))

// Files served under /demos/ in dev and copied to dist/demos/ at build time.
// Source of truth lives in docs/ (shared with README). This plugin bridges
// the two without duplicating binaries in the repo.
const DEMO_FILES = [
  'demo-status.gif',
  'demo-log.gif',
  'demo-worktrees.gif',
  'demo-image-diff.gif',
  'banner-1280x640.png',
]

function demosPlugin(): Plugin {
  const docsDir = resolve(__dirname, '../docs')
  return {
    name: 'gx-demos',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        if (!req.url?.startsWith('/demos/')) { next(); return }
        const fileName = req.url.slice('/demos/'.length)
        if (!DEMO_FILES.includes(fileName)) { next(); return }
        try {
          const content = readFileSync(resolve(docsDir, fileName))
          const mime = fileName.endsWith('.gif') ? 'image/gif' : 'image/png'
          res.setHeader('Content-Type', mime)
          res.end(content)
        } catch { next() }
      })
    },
    closeBundle() {
      const outDir = resolve(__dirname, 'dist/demos')
      mkdirSync(outDir, { recursive: true })
      for (const file of DEMO_FILES) {
        try { copyFileSync(resolve(docsDir, file), resolve(outDir, file)) }
        catch { /* not yet recorded — skip */ }
      }
    },
  }
}

export default defineConfig({
  plugins: [
    preact({
      prerender: {
        enabled: true,
        renderTarget: '#app',
      },
    }),
    demosPlugin(),
  ],
  build: {
    target: 'es2022',
  },
})
