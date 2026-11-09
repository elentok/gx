import { createHash } from "node:crypto";
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, relative } from "node:path";
import type { Plugin } from "vite";

function listFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) =>
    e.isDirectory() ? listFiles(join(dir, e.name)) : [join(dir, e.name)],
  );
}

// Writes dist/sw.js after the whole bundle (including public/) is on disk, so
// the build id and asset list cover every file the worker may precache.
// The name stays unhashed: the browser fetches /sw.js by that fixed URL.
export function serviceWorkerPlugin(): Plugin {
  let outDir = "dist";
  return {
    name: "service-worker",
    apply: "build",
    configResolved(config) {
      outDir = join(config.root, config.build.outDir);
    },
    closeBundle() {
      const hash = createHash("sha256");
      const assets = listFiles(outDir)
        .map((f) => ({ name: relative(outDir, f).split("\\").join("/"), file: f }))
        .filter((f) => f.name !== "sw.js")
        .sort((a, b) => a.name.localeCompare(b.name));
      for (const a of assets) hash.update(a.name).update(readFileSync(a.file));
      const buildId = hash.digest("hex").slice(0, 16);
      const urls = assets.map((a) => (a.name === "index.html" ? "/" : `/${a.name}`));
      writeFileSync(join(outDir, "sw.js"), workerSource(buildId, urls));
    },
  };
}

// A pass-through stub: no fetch handler, so every request goes to the network.
// The build id makes each build's worker byte-different, so it installs anew.
function workerSource(buildId: string, urls: string[]): string {
  return `// build ${buildId}
const BUILD_ID = ${JSON.stringify(buildId)};
const ASSETS = ${JSON.stringify(urls)};
void BUILD_ID;
void ASSETS;
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));
`;
}
