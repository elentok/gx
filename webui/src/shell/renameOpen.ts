import { useSyncExternalStore } from "react"
import type { SessionInfo } from "../generated/server"
import type { RenameSession } from "../design/open"

export type RenameRequest = {
  session: SessionInfo
  /** Overrides the real writer; tests inject a fake. */
  rename?: RenameSession
} | null

// App-level state for the rename popup, kept outside React like removeConfirmOpen so the menu, the
// chord and the palette open one popup and only the host re-renders.
let request: RenameRequest = null
const listeners = new Set<() => void>()

function set(next: RenameRequest) {
  request = next
  listeners.forEach((l) => l())
}

export const openRename = (open: NonNullable<RenameRequest>) => set(open)
export const closeRename = () => set(null)

export function useRenameRequest(): RenameRequest {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => request,
  )
}
