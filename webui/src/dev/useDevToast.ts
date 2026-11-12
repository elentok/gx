import { useEffect, useRef } from "react"
import { useConnectionContext, useConnectionState } from "../connection"
import { postDevRestart as defaultPostDevRestart } from "../dev"
import type { DevState } from "../generated/server"
import { guarded } from "../ui/reportWriteFailure"
import { showToast } from "../ui/toaster"

/**
 * Toasts once each time a refetched snapshot moves Dev mode to down. The first snapshot only sets
 * the baseline, so a page that loads already down stays quiet.
 */
export function useDevToast({
  postDevRestart = defaultPostDevRestart,
}: {
  postDevRestart?: () => Promise<void>
} = {}) {
  const state: DevState | undefined = useConnectionState(useConnectionContext()).snapshot?.dev
    ?.state
  const prev = useRef<DevState | undefined>(undefined)
  useEffect(() => {
    if (!state) return
    if (prev.current && prev.current !== "down" && state === "down") {
      showToast({
        type: "error",
        title: "Vite is down. gx serves the built files.",
        actions: [{ label: "Restart Vite", onClick: () => guarded(postDevRestart()) }],
      })
    }
    prev.current = state
  }, [state, postDevRestart])
}
