import { useEffect } from "react"
import { useConnectionContext, useConnectionState } from "../connection"
import { usePushDevice } from "./deviceState"

function apply(badge: number) {
  if (typeof navigator.setAppBadge !== "function") return
  const done = badge > 0 ? navigator.setAppBadge(badge) : navigator.clearAppBadge?.()
  done?.catch(() => {})
}

/**
 * Shows the server's badge count on the app icon while push is on. Only a live snapshot counts:
 * a cached one may predate the badge. The app never counts itself.
 */
export function useAppBadge() {
  const { snapshot, liveSnapshot } = useConnectionState(useConnectionContext())
  const { on } = usePushDevice()
  const badge = liveSnapshot ? snapshot?.badge : undefined

  useEffect(() => {
    if (!on || badge === undefined) return
    apply(badge)
    const onVisible = () => {
      if (document.visibilityState === "visible") apply(badge)
    }
    document.addEventListener("visibilitychange", onVisible)
    return () => document.removeEventListener("visibilitychange", onVisible)
  }, [on, badge])
}
