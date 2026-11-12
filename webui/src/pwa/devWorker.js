// Served by gx as /sw.js while Dev mode runs Vite. It removes what a built
// worker left behind and then removes itself, so dev pages load straight from
// Vite. The prefix is the page's CACHE_PREFIX (workerCore.ts); devWorker.test.ts
// keeps the two equal. Plain script: Go embeds this file and vitest runs it.
self.addEventListener("install", () => self.skipWaiting())

self.addEventListener("activate", (event) => {
  event.waitUntil(removeSelf())
})

async function removeSelf() {
  const names = await self.caches.keys()
  await Promise.all(
    names.filter((n) => n.startsWith("gx-shell-")).map((n) => self.caches.delete(n)),
  )
  await self.registration.unregister()
  const windows = await self.clients.matchAll({ type: "window" })
  await Promise.all(windows.map((w) => w.navigate(w.url)))
}
