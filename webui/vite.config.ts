/// <reference types="vitest/config" />
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"
import cannedSnapshot from "./src/canned-snapshot.json"
import { serviceWorkerPlugin } from "./src/pwa/serviceWorkerPlugin"

// Serves the canned snapshot at /v1/snapshot so `make web-dev` needs no gx
// server.

const webuiDir = dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  // The Makefile passes the gx version; a plain build or the dev server reports "dev".
  define: { __WEB_VERSION__: JSON.stringify(process.env.GX_WEB_VERSION || "dev") },
  resolve: {
    // Pierre imports every Shiki grammar through this entry; swap in the short list.
    alias: [{ find: /^shiki\/langs$/, replacement: join(webuiDir, "src/diff/shikiLangs.ts") }],
  },
  plugins: [
    react(),
    serviceWorkerPlugin(),
    {
      // Gives webVitals.ts the build's chunk names, the allowlist for worst_chunk.
      name: "gx-chunk-names",
      generateBundle(_options, bundle) {
        const names = new Set<string>()
        for (const file of Object.values(bundle)) {
          if (file.type === "chunk") names.add(file.name)
        }
        const list = JSON.stringify([...names].sort().join(","))
        for (const file of Object.values(bundle)) {
          if (file.type === "chunk" && file.name === "webVitals") {
            file.code = file.code.replace(/"__GX_CHUNKS__"/, list)
          }
        }
      },
    },
    {
      name: "canned-snapshot",
      configureServer(server) {
        server.middlewares.use("/v1/snapshot", (_req, res) => {
          res.setHeader("Content-Type", "application/json")
          res.end(JSON.stringify(cannedSnapshot))
        })
        // An idle stream with heartbeats keeps the page from showing Offline.
        server.middlewares.use("/v1/events", (req, res) => {
          res.setHeader("Content-Type", "text/event-stream")
          res.write(": hello\n\n")
          const beat = setInterval(() => res.write(": hb\n\n"), 15000)
          req.on("close", () => clearInterval(beat))
        })
      },
    },
  ],
  server: {
    // gx runs its own Vite for Dev mode and reads this file, so it passes no
    // launcher arguments and finds the port itself. Vite's default "localhost"
    // can bind only ::1.
    host: "127.0.0.1",
    // gx's Vite is reachable through the web listener; keep /@fs/ inside webui/.
    fs: { strict: true, allow: [webuiDir] },
  },
  build: {
    rollupOptions: {
      // gallery.html is served by the dev server only; keep it out of the production input.
      input: "index.html",
      output: {
        manualChunks(id) {
          if (
            /node_modules\/(react|react-dom|scheduler|@chakra-ui|@ark-ui|@zag-js|@emotion)\//.test(
              id,
            )
          ) {
            return "vendor"
          }
        },
        entryFileNames: "assets/[name]-[hash].js",
        chunkFileNames: "assets/[name]-[hash].js",
        assetFileNames: "assets/[name]-[hash][extname]",
      },
    },
  },
  test: {
    environment: "jsdom",
    // Testing Library registers its between-test cleanup only when globals exist.
    globals: true,
    setupFiles: ["./src/test-setup.ts"],
    // Vitest's default timed out a quick test while other sessions loaded the machine.
    testTimeout: 20_000,
  },
})
