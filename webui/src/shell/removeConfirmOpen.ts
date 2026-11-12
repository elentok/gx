import { useSyncExternalStore } from "react"
import type { SessionInfo } from "../generated/server"
import type { RemoveSession } from "../agents/sessionWrites"

export type RemoveConfirmRequest = {
  session: SessionInfo
  /** Each opener keeps its own navigation. */
  onRemoved: () => void
  /** Overrides the real writer; tests inject a fake. */
  remove?: RemoveSession
} | null

// App-level state for the remove confirm, kept outside React like newItemOpen so the row menu, the
// agent page menu and the `x` chord open one dialog and only the host re-renders.
let request: RemoveConfirmRequest = null
const listeners = new Set<() => void>()

function set(next: RemoveConfirmRequest) {
  request = next
  listeners.forEach((l) => l())
}

export const openRemoveConfirm = (open: NonNullable<RemoveConfirmRequest>) => set(open)
export const closeRemoveConfirm = () => set(null)

export function useRemoveConfirmRequest(): RemoveConfirmRequest {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => request,
  )
}
