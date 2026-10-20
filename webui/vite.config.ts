/// <reference types="vitest/config" />
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";
import cannedSnapshot from "./src/canned-snapshot.json";

// Serves the canned snapshot at /v1/snapshot so `make web-dev` needs no gx
// server. Set GX_WEBUI_PROXY=1 to proxy /v1 to a real one instead.
const useRealServer = process.env.GX_WEBUI_PROXY === "1";

export default defineConfig({
  plugins: [
    react(),
    {
      name: "canned-snapshot",
      configureServer(server) {
        if (useRealServer) return;
        server.middlewares.use("/v1/snapshot", (_req, res) => {
          res.setHeader("Content-Type", "application/json");
          res.end(JSON.stringify(cannedSnapshot));
        });
        // An idle stream with heartbeats keeps the page from showing Offline.
        server.middlewares.use("/v1/events", (req, res) => {
          res.setHeader("Content-Type", "text/event-stream");
          res.write(": hello\n\n");
          const beat = setInterval(() => res.write(": hb\n\n"), 15000);
          req.on("close", () => clearInterval(beat));
        });
      },
    },
  ],
  server: {
    // gx server proxies to http://127.0.0.1:5173; Vite's default "localhost" can bind only ::1.
    host: "127.0.0.1",
    proxy: useRealServer ? { "/v1": "http://127.0.0.1:7421" } : undefined,
  },
  build: {
    rollupOptions: {
      // gallery.html is served by the dev server only; keep it out of the production input.
      input: "index.html",
      output: {
        manualChunks(id) {
          if (/node_modules\/(react|react-dom|scheduler|@chakra-ui|@ark-ui|@zag-js|@emotion)\//.test(id)) {
            return "vendor";
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
  },
});
