import { showToast } from "../ui/toaster"
import { CACHE_PREFIX } from "./workerCore"

// Clears only the worker's registration and shell caches (not cookies or session storage), then
// reloads. A failed clear still reloads: a stuck worker is the case this row exists to escape.
export async function forceRefresh(connected: boolean): Promise<void> {
  if (!connected) {
    showToast({ type: "info", title: "Can't reach gx" })
    return
  }
  showToast({ type: "info", title: "Clearing the cache and reloading…" })
  try {
    const registrations = (await navigator.serviceWorker?.getRegistrations()) ?? []
    await Promise.all(registrations.map((r) => r.unregister()))
    const names = (await globalThis.caches?.keys()) ?? []
    await Promise.all(names.filter((n) => n.startsWith(CACHE_PREFIX)).map((n) => caches.delete(n)))
  } catch {
    // reload anyway
  }
  location.reload()
}
