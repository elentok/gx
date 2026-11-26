import { useSyncExternalStore } from "react"

// TypeScript types `window.navigation` as always present; some browsers lack it.
const navigationApi = () => window.navigation as Navigation | undefined

function subscribe(onChange: () => void) {
  const nav = navigationApi()
  nav?.addEventListener("currententrychange", onChange)
  return () => nav?.removeEventListener("currententrychange", onChange)
}

const getSupported = () => Boolean(navigationApi())
const getCanGoBack = () => navigationApi()?.canGoBack ?? false
const getCanGoForward = () => navigationApi()?.canGoForward ?? false

/** Whether the browser can move through its session history. It only reports; callers use `navigate(±1)`. */
export function useHistoryState(): {
  supported: boolean
  canGoBack: boolean
  canGoForward: boolean
} {
  return {
    supported: useSyncExternalStore(subscribe, getSupported),
    canGoBack: useSyncExternalStore(subscribe, getCanGoBack),
    canGoForward: useSyncExternalStore(subscribe, getCanGoForward),
  }
}
