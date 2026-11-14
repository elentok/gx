import type { SessionInfo } from "../generated/server"
import { createExternalStore } from "./externalStore"

export type RemoveConfirmRequest = {
  session: SessionInfo
  /** Each opener keeps its own navigation. */
  onRemoved: () => void
} | null

// App-level state for the remove confirm, kept outside React like newItemOpen so the row menu, the
// agent page menu and the `x` chord open one dialog and only the host re-renders.
const store = createExternalStore<RemoveConfirmRequest>(null)

export const openRemoveConfirm = (open: NonNullable<RemoveConfirmRequest>) => store.set(open)
export const closeRemoveConfirm = () => store.set(null)
export const useRemoveConfirmRequest = store.use
