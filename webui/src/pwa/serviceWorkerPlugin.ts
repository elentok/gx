import { createHash } from "node:crypto";
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, relative } from "node:path";
import { type Plugin, transformWithEsbuild } from "vite";

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
    async closeBundle() {
      const hash = createHash("sha256");
      const assets = listFiles(outDir)
        .map((f) => ({ name: relative(outDir, f).split("\\").join("/"), file: f }))
        .filter((f) => f.name !== "sw.js")
        .sort((a, b) => a.name.localeCompare(b.name));
      for (const a of assets) hash.update(a.name).update(readFileSync(a.file));
      const buildId = hash.digest("hex").slice(0, 16);
      const urls = assets.map((a) => (a.name === "index.html" ? "/" : `/${a.name}`));
      const core = await transformWithEsbuild(
        readFileSync(join(import.meta.dirname, "workerCore.ts"), "utf8"),
        "workerCore.ts",
        { format: "iife", globalName: "gxWorkerCore", target: "es2020" },
      );
      writeFileSync(join(outDir, "sw.js"), workerSource(buildId, urls, core.code));
    },
  };
}

// The build id makes each build's worker byte-different, so it installs anew.
// The fetch rules live in workerCore.ts; this entry only wires events to them.
function workerSource(buildId: string, urls: string[], coreCode: string): string {
  return `// build ${buildId}
${coreCode}
const lifecycle = {
  caches: self.caches,
  fetch: (input, init) => fetch(input, init),
  origin: self.location.origin,
  buildId: ${JSON.stringify(buildId)},
  urls: ${JSON.stringify(urls)},
};
self.addEventListener("install", (event) => {
  self.skipWaiting();
  event.waitUntil(gxWorkerCore.precache(lifecycle));
});
self.addEventListener("activate", (event) =>
  event.waitUntil(gxWorkerCore.pruneOldCaches(lifecycle).then(() => self.clients.claim())),
);
self.addEventListener(
  "fetch",
  gxWorkerCore.createFetchHandler({
    caches: self.caches,
    fetch: (input, init) => fetch(input, init),
    cacheName: gxWorkerCore.cacheNameFor(lifecycle.buildId),
  }),
);
`;
}
