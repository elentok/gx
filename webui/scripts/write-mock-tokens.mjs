// Writes skills/gx-prototype/gx/{tokens,base}.css from the real Chakra system.
// Vite loads the TypeScript theme; webui has no other TS runner.
import { mkdirSync, writeFileSync } from "node:fs"
import { dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"
import { createServer } from "vite"

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const server = await createServer({
  root,
  configFile: false,
  appType: "custom",
  logLevel: "silent",
  server: { middlewareMode: true },
  optimizeDeps: { noDiscovery: true, include: [] },
})
try {
  const gen = await server.ssrLoadModule("/scripts/mock-tokens.ts")
  const out = resolve(root, gen.outDir)
  mkdirSync(out, { recursive: true })
  writeFileSync(resolve(out, "tokens.css"), gen.tokensCss())
  writeFileSync(resolve(out, "base.css"), gen.baseCss())
} finally {
  await server.close()
}
