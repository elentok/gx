import { TAP_MESSAGE } from "./workerCore"

type TapRouter = { navigate: (to: string) => unknown }

let router: TapRouter | null = null

// The router is registered only while the app is mounted; the login and setup screens have none.
export function setTapRouter(next: TapRouter | null): () => void {
  router = next
  return () => {
    if (router === next) router = null
  }
}

// A route path starts with one "/" and holds no scheme or backslash.
function isRoutePath(path: unknown): path is string {
  return (
    typeof path === "string" &&
    path.startsWith("/") &&
    !path.startsWith("//") &&
    !path.includes("\\") &&
    !path.includes("://")
  )
}

function onMessage(event: MessageEvent) {
  const data = event.data as { type?: unknown; path?: unknown } | null
  if (data?.type !== TAP_MESSAGE || !isRoutePath(data.path)) return
  // The answer tells the worker a window is listening, so it never calls navigate().
  event.ports[0]?.postMessage(true)
  if (router) void router.navigate(data.path)
  else window.location.hash = `#${data.path}`
}

export function listenForNotificationTaps(): void {
  if (typeof navigator === "undefined" || !navigator.serviceWorker) return
  navigator.serviceWorker.addEventListener("message", onMessage)
}
